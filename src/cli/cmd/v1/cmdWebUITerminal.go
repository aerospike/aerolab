//go:build !nowebui

package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// getWSUpgrader returns a WebSocket upgrader with proper origin checking.
func (c *WebUICmd) getWSUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  8192,
		WriteBufferSize: 8192,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // no origin header = same-origin or non-browser
			}

			// Parse the origin to extract host
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			originHost := u.Host

			// Accept if origin matches the request host
			if originHost == r.Host {
				return true
			}

			// Accept if origin matches the configured proxy origin
			if c.WsProxyOrigin != "" && originHost == c.WsProxyOrigin {
				return true
			}

			return false
		},
	}
}

// wsControlMessage is a JSON control message from the client (e.g. resize)
type wsControlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// handleTerminalWS handles WebSocket connections for interactive SSH terminals.
// Query parameters:
//   - type: cluster | client | agi
//   - name: cluster/client/AGI name
//   - node: node number (optional, defaults to first found)
//
// Protocol:
//   - Binary WebSocket messages: raw terminal I/O (both directions)
//   - Text WebSocket messages from client: JSON control messages (e.g. {"type":"resize","cols":80,"rows":24})
//   - Text WebSocket messages from server: informational messages (errors, connection status)
func (c *WebUICmd) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	connectType := r.URL.Query().Get("type")
	name := r.URL.Query().Get("name")
	nodeStr := r.URL.Query().Get("node")

	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if connectType == "" {
		connectType = "cluster"
	}

	inventory := c.getInventory()
	if inventory == nil {
		http.Error(w, "Backend not initialized", http.StatusInternalServerError)
		return
	}

	// Find the target instance
	var instances backends.InstanceList
	switch connectType {
	case "cluster":
		instances = inventory.Instances.
			WithTags(map[string]string{"aerolab.type": "aerospike"}).
			WithClusterName(name).
			WithState(backends.LifeCycleStateRunning).Describe()
	case "client":
		instances = inventory.Instances.
			WithTags(map[string]string{"aerolab.old.type": "client"}).
			WithClusterName(name).
			WithState(backends.LifeCycleStateRunning).Describe()
	case "agi":
		instances = inventory.Instances.
			WithTags(map[string]string{"aerolab.type": "agi"}).
			WithClusterName(name).
			WithState(backends.LifeCycleStateRunning).Describe()
	default:
		http.Error(w, fmt.Sprintf("invalid type: %s", connectType), http.StatusBadRequest)
		return
	}

	if nodeStr != "" {
		nodeNo, err := strconv.Atoi(nodeStr)
		if err != nil {
			http.Error(w, "invalid node number", http.StatusBadRequest)
			return
		}
		instances = instances.WithNodeNo(nodeNo).Describe()
	}

	if instances.Count() == 0 {
		http.Error(w, "instance not found or not running", http.StatusNotFound)
		return
	}

	inst := instances.Describe()[0]

	// Get SSH configuration for the instance
	sshConf, err := inst.GetSftpConfig("root")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get SSH config: %s", err), http.StatusInternalServerError)
		return
	}

	// Upgrade HTTP connection to WebSocket
	upgrader := c.getWSUpgrader()
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %s", err)
		return
	}
	defer wsConn.Close()

	// Mutex for concurrent WebSocket writes (gorilla/websocket supports one writer at a time)
	var wsMu sync.Mutex
	wsWrite := func(msgType int, data []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return wsConn.WriteMessage(msgType, data)
	}

	// Establish SSH connection
	sshConfig, err := makeSshClientConfig(sshConf)
	if err != nil {
		wsWrite(websocket.TextMessage, []byte("\r\n*** SSH config error: "+err.Error()+" ***\r\n"))
		return
	}

	sshConn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", sshConf.Host, sshConf.Port), sshConfig)
	if err != nil {
		wsWrite(websocket.TextMessage, []byte("\r\n*** SSH connection failed: "+err.Error()+" ***\r\n"))
		return
	}
	defer sshConn.Close()

	session, err := sshConn.NewSession()
	if err != nil {
		wsWrite(websocket.TextMessage, []byte("\r\n*** SSH session failed: "+err.Error()+" ***\r\n"))
		return
	}
	defer session.Close()

	// Request PTY with xterm-256color
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		wsWrite(websocket.TextMessage, []byte("\r\n*** PTY request failed: "+err.Error()+" ***\r\n"))
		return
	}

	// Get stdin/stdout pipes
	stdinPipe, err := session.StdinPipe()
	if err != nil {
		wsWrite(websocket.TextMessage, []byte("\r\n*** Stdin pipe failed: "+err.Error()+" ***\r\n"))
		return
	}
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		wsWrite(websocket.TextMessage, []byte("\r\n*** Stdout pipe failed: "+err.Error()+" ***\r\n"))
		return
	}

	// Start interactive shell
	if err := session.Shell(); err != nil {
		wsWrite(websocket.TextMessage, []byte("\r\n*** Shell start failed: "+err.Error()+" ***\r\n"))
		return
	}

	done := make(chan struct{})

	// Goroutine: SSH stdout → WebSocket (binary messages)
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				if writeErr := wsWrite(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Goroutine: WebSocket → SSH stdin (with control message handling)
	go func() {
		defer stdinPipe.Close()
		defer session.Close() // close session when browser disconnects
		for {
			msgType, msg, err := wsConn.ReadMessage()
			if err != nil {
				return
			}

			switch msgType {
			case websocket.TextMessage:
				// Text messages may be JSON control messages (e.g. resize)
				var ctrl wsControlMessage
				if json.Unmarshal(msg, &ctrl) == nil && ctrl.Type == "resize" && ctrl.Cols > 0 && ctrl.Rows > 0 {
					//nolint:errcheck
					session.WindowChange(ctrl.Rows, ctrl.Cols)
					continue
				}
				// Otherwise treat as terminal input
				//nolint:errcheck
				stdinPipe.Write(msg)
			case websocket.BinaryMessage:
				// Binary messages are raw terminal input
				//nolint:errcheck
				stdinPipe.Write(msg)
			}
		}
	}()

	// Goroutine: periodic ping to keep WebSocket alive
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := wsWrite(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Wait for SSH session to finish
	//nolint:errcheck
	session.Wait()
	close(done)

	// Send session-ended notification and close the WebSocket
	wsWrite(websocket.TextMessage, []byte("\r\n*** Session ended ***\r\n"))
	wsWrite(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session ended"))
}

// makeSshClientConfig creates an ssh.ClientConfig from an sshexec.ClientConf
func makeSshClientConfig(conf *sshexec.ClientConf) (*ssh.ClientConfig, error) {
	config := &ssh.ClientConfig{
		User:            conf.Username,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	if len(conf.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(conf.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("unable to parse private key: %v", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}
	if conf.Password != "" {
		config.Auth = append(config.Auth, ssh.Password(conf.Password))
	}
	if conf.ConnectTimeout != 0 {
		config.Timeout = conf.ConnectTimeout
	}
	return config, nil
}
