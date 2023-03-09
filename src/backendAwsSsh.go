package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
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
	buffer, err := os.ReadFile(file)
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
		return fmt.Errorf("mode not supported: %d", mode)
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

func remoteAttachAndRun(user string, addr string, privateKey string, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer, node int) error {
	client := &SSH{
		Ip:   addr,
		User: user,
		Cert: privateKey,
	}
	err := client.Connect(CERT_PUBLIC_KEY_FILE)
	if err != nil {
		return err
	}
	err = client.session.Setenv("NODE", strconv.Itoa(node))
	if err != nil {
		log.Printf("WARN: Setenv(NODE) failed: %s", err)
	}
	err = client.RunAttachCmd(cmd, stdin, stdout, stderr)
	client.Close()
	return err
}

func (ssh_client *SSH) RunAttachCmd(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	ssh_client.session.Stdin = stdin
	ssh_client.session.Stdout = stdout
	ssh_client.session.Stderr = stderr
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

func remoteRun(user string, addr string, privateKey string, cmd string, node int) ([]byte, error) {
	client := &SSH{
		Ip:   addr,
		User: user,
		Cert: privateKey,
	}
	err := client.Connect(CERT_PUBLIC_KEY_FILE)
	if err != nil {
		return nil, err
	}
	err = client.session.Setenv("NODE", strconv.Itoa(node))
	if err != nil {
		log.Printf("WARN: Setenv(NODE) failed: %s", err)
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
		err := scpFile(user, addr, privateKey, file)
		if err != nil {
			log.Printf("error: %s", err)
			return err
		}
	}
	return nil
}

func scpFile(user string, addr string, privateKey string, file fileList) error {
	file.fileContents.Seek(0, 0)
	//fmt.Printf("%s %s %s %s %d\n", user, addr, privateKey, file.filePath, file.fileSize)
	sess, err := remoteSession(user, addr, privateKey)
	if err != nil {
		return err
	}
	session := sess.session

	go func() {
		w, err := session.StdinPipe()
		if err != nil {
			log.Printf("error: %s", err)
			return
		}
		defer func() { _ = w.Close() }()
		if file.fileSize == 0 {
			contents, err := io.ReadAll(file.fileContents)
			if err != nil {
				log.Printf("error: %s", err)
				return
			}
			file.fileSize = len(contents)
			file.fileContents = bytes.NewReader(contents)
		}
		_, err = fmt.Fprintln(w, "C"+"0755", file.fileSize, path.Base(file.filePath))
		if err != nil {
			log.Printf("error: %s", err)
			return
		}
		_, err = io.Copy(w, file.fileContents)
		if err != nil {
			log.Printf("error: %s", err)
			return
		}
		_, err = fmt.Fprintln(w, "\x00")
		if err != nil {
			log.Printf("error: %s", err)
			return
		}
	}()

	err = session.Run("/usr/bin/scp -qt " + path.Dir(file.filePath))

	if err != nil && err.Error() != "Process exited with status 1" {
		return err
	}
	_ = session.Close()
	sess.Close()
	return nil
}

func scpExecDownload(user string, ip string, port string, privateKey string, sourcePath string, destPath string, out io.Writer, timeout time.Duration, verbose bool) error {
	params := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	if timeout != 0 {
		params = append(params, []string{"-o", "ConnectTimeout=" + strconv.Itoa(int(timeout.Seconds()))}...)
	}
	if !verbose {
		params = append(params, "-q")
	}
	params = append(params, []string{"-r", "-i", privateKey, "-P" + port, user + "@" + ip + ":" + sourcePath, destPath}...)
	return scpExec(params, out)
}

func scpExecUpload(user string, ip string, port string, privateKey string, sourcePath string, destPath string, out io.Writer, timeout time.Duration, verbose bool) error {
	params := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	if timeout != 0 {
		params = append(params, []string{"-o", "ConnectTimeout=" + strconv.Itoa(int(timeout.Seconds()))}...)
	}
	if !verbose {
		params = append(params, "-q")
	}
	params = append(params, []string{"-r", "-i", privateKey, "-P" + port, sourcePath, user + "@" + ip + ":" + destPath}...)
	return scpExec(params, out)
}

func scpExec(params []string, out io.Writer) error {
	cmd := exec.Command("scp", params...)
	if out == nil {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s\n%s", string(out), err)
		}
		return nil
	}
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
