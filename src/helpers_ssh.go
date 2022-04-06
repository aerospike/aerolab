package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const (
	CERT_PASSWORD        = 1
	CERT_PUBLIC_KEY_FILE = 2
	DEFAULT_TIMEOUT      = 3 // second
)

type SSH struct {
	Ip      string
	User    string
	Cert    string //password or key file path
	session *ssh.Session
	client  *ssh.Client
}

func (ssh_client *SSH) readPublicKeyFile(file string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(key), nil
}

func (ssh_client *SSH) Connect(mode int) error {

	var ssh_config *ssh.ClientConfig
	var auth []ssh.AuthMethod
	if mode == CERT_PASSWORD {
		auth = []ssh.AuthMethod{ssh.Password(ssh_client.Cert)}
	} else if mode == CERT_PUBLIC_KEY_FILE {
		key, err := ssh_client.readPublicKeyFile(ssh_client.Cert)
		if err != nil {
			return err
		}
		auth = []ssh.AuthMethod{key}
	} else {
		return fmt.Errorf("Mode not supported: %d", mode)
	}

	ssh_config = &ssh.ClientConfig{
		User: ssh_client.User,
		Auth: auth,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		Timeout: time.Second * 5,
	}

	client, err := ssh.Dial("tcp", ssh_client.Ip, ssh_config)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return err
	}

	ssh_client.session = session
	ssh_client.client = client
	return nil
}

func remoteAttachAndRun(user string, addr string, privateKey string, cmd string) error {
	client := &SSH{
		Ip:   addr,
		User: user,
		Cert: privateKey,
	}
	err := client.Connect(CERT_PUBLIC_KEY_FILE)
	if err != nil {
		return err
	}
	err = client.RunAttachCmd(cmd)
	client.Close()
	return err
}

func (ssh_client *SSH) RunAttachCmd(cmd string) error {
	ssh_client.session.Stdin = os.Stdin
	ssh_client.session.Stdout = os.Stdout
	ssh_client.session.Stderr = os.Stderr
	fileDescriptor := int(os.Stdin.Fd())
	var err error
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if term.IsTerminal(fileDescriptor) {
		originalState, err := term.MakeRaw(fileDescriptor)
		if err != nil {
			return err
		}
		defer func() { _ = term.Restore(fileDescriptor, originalState) }()

		termWidth, termHeight, err := term.GetSize(fileDescriptor)
		if err != nil {
			return err
		}

		err = ssh_client.session.RequestPty("xterm-256color", termHeight, termWidth, modes)
		if err != nil {
			return err
		}
	} else {
		err = ssh_client.session.RequestPty("vt100", 24, 80, modes)
		if err != nil {
			return err
		}
	}
	err = ssh_client.session.Run(cmd)
	return err
}

func (ssh_client *SSH) RunCmd(cmd string) ([]byte, error) {
	out, err := ssh_client.session.CombinedOutput(cmd)
	if err != nil {
		return out, err
	}
	return out, nil
}

func (ssh_client *SSH) Close() {
	_ = ssh_client.session.Close()
	_ = ssh_client.client.Close()
}

func remoteRun(user string, addr string, privateKey string, cmd string) ([]byte, error) {
	client := &SSH{
		Ip:   addr,
		User: user,
		Cert: privateKey,
	}
	err := client.Connect(CERT_PUBLIC_KEY_FILE)
	if err != nil {
		return nil, err
	}
	ret, err := client.RunCmd(cmd)
	client.Close()
	return ret, err
}

func remoteSession(user string, addr string, privateKey string) (*SSH, error) {
	client := &SSH{
		Ip:   addr,
		User: user,
		Cert: privateKey,
	}
	err := client.Connect(CERT_PUBLIC_KEY_FILE)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func scp(user string, addr string, privateKey string, files []fileList) error {

	for _, file := range files {
		sess, err := remoteSession(user, addr, privateKey)
		if err != nil {
			return err
		}
		session := sess.session

		go func() {
			w, err := session.StdinPipe()
			if err != nil {
				return
			}
			defer func() { _ = w.Close() }()
			_, _ = fmt.Fprintln(w, "C"+"0755", len(file.fileContents), path.Base(file.filePath))
			_, _ = io.Copy(w, bytes.NewReader(file.fileContents))
			_, _ = fmt.Fprintln(w, "\x00")
		}()

		err = session.Run("/usr/bin/scp -qt " + path.Dir(file.filePath))

		if err != nil && err.Error() != "Process exited with status 1" {
			return err
		}
		_ = session.Close()
		sess.Close()
	}

	return nil
}
