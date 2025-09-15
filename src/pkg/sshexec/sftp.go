package sshexec

import (
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Sftp struct {
	ClientConf
	conn   *ssh.Client
	client *sftp.Client
}

func NewSftp(i *ClientConf) (*Sftp, error) {
	o := &Sftp{
		ClientConf: *i,
	}
	// get client config
	config, err := makeClientConfig(i)
	if err != nil {
		return o, err
	}

	// ssh dial
	currentTimeout := i.ConnectTimeout
	start := time.Now()
	var conn *ssh.Client
	for {
		conn, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", i.Host, i.Port), config)
		if err == nil {
			break
		}
		currentTimeout -= time.Since(start)
		if currentTimeout <= 0 && i.ConnectTimeout > 0 {
			return o, fmt.Errorf("failed to dial: %s", err)
		}
		time.Sleep(time.Second)
	}
	o.conn = conn

	// get sftp client
	client, err := sftp.NewClient(conn)
	if err != nil {
		o.conn.Close()
		return o, err
	}
	o.client = client

	// done
	return o, nil
}

// cleanup
func (i *Sftp) Close() {
	i.client.Close()
	i.conn.Close()
}

// get remote client can be used to perform operations on the client directly by the caller
func (i *Sftp) GetRemoteClient() *sftp.Client {
	return i.client
}

// this is what a file definition looks like for WriteFile
type FileWriter struct {
	DestPath    string      // destination path on the remote
	Source      io.Reader   // source reader to read from in order to store the file
	Permissions os.FileMode // optional; if unset, default ssh/sftp mask is applied
}

// this is what a file definition looks like for ReadFile
type FileReader struct {
	SourcePath  string    // source path on the remote
	Destination io.Writer // destination writer to which the file will be written
}

func (i *Sftp) RawClient() *sftp.Client {
	return i.client
}

func (i *Sftp) IsExists(path string) bool {
	_, err := i.client.Stat(path)
	return err == nil
}

func (i *Sftp) Mkdir(path string, perm os.FileMode) error {
	if i.IsExists(path) {
		return nil
	}
	err := i.client.Mkdir(path)
	if err != nil {
		return err
	}
	err = i.client.Chmod(path, perm)
	if err != nil {
		return err
	}
	return nil
}

// write a file to remote
// if mkdir is set, will check if directory exists; if it doesn't, one will be created
func (i *Sftp) WriteFile(mkdir bool, f *FileWriter) error {
	if mkdir {
		dir, _ := path.Split(f.DestPath)
		if _, err := i.client.Stat(dir); err != nil && os.IsNotExist(err) {
			err = i.client.MkdirAll(dir)
			if err != nil {
				return err
			}
		}
	}
	fh, err := i.client.OpenFile(f.DestPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	_, err = fh.ReadFrom(f.Source)
	fh.Close()
	if err != nil {
		return err
	}
	if f.Permissions != 0 {
		err = i.client.Chmod(f.DestPath, f.Permissions)
		if err != nil {
			return err
		}
	}
	return nil
}

// read a file from remote
func (i *Sftp) ReadFile(f *FileReader) error {
	fh, err := i.client.Open(f.SourcePath)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.WriteTo(f.Destination)
	if err != nil {
		return err
	}
	return nil
}

// upload files recursively to remote
func (i *Sftp) Upload(sourcePath string, destPath string) error {
	return i.upload(sourcePath, destPath, path.Base(sourcePath), 0)
}

func (i *Sftp) upload(sourcePath string, destPath string, sourceRoot string, depth int) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to get source info: %v", err)
	}

	// if destPath exists and is a directory, append the source file name to the destPath
	if depth == 0 {
		if st, err := i.client.Stat(destPath); err == nil && st.IsDir() {
			destPath = path.Join(destPath, sourceRoot)
		}
	}

	// Check if source is a directory
	if info.IsDir() {
		// Create remote directory
		err = i.client.MkdirAll(destPath)
		if err != nil {
			return fmt.Errorf("failed to create remote directory: %v", err)
		}

		// Iterate through the directory contents
		entries, err := os.ReadDir(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read source directory: %v", err)
		}

		for _, entry := range entries {
			src := path.Join(sourcePath, entry.Name())
			dst := path.Join(destPath, entry.Name())

			// Recursively call Upload
			err = i.upload(src, dst, sourceRoot, depth+1)
			if err != nil {
				return fmt.Errorf("failed to upload %s: %v", src, err)
			}
		}
	} else {
		// Upload a single file
		err = i.uploadFile(sourcePath, destPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (i *Sftp) uploadFile(sourcePath string, destPath string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer file.Close()

	writer := &FileWriter{
		DestPath: destPath,
		Source:   file,
	}
	err = i.WriteFile(true, writer)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}
	return nil
}

// download files recursively from remote
func (i *Sftp) Download(sourcePath string, destPath string) error {
	return i.download(sourcePath, destPath, 0)
}

func (i *Sftp) download(sourcePath string, destPath string, depth int) error {
	info, err := i.client.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to get remote source info: %v", err)
	}

	// Check if source is a directory
	if info.IsDir() {
		// Create local directory
		err = os.MkdirAll(destPath, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create local directory: %v", err)
		}

		// Iterate through the directory contents
		entries, err := i.client.ReadDir(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read remote directory: %v", err)
		}

		for _, entry := range entries {
			src := path.Join(sourcePath, entry.Name())
			dst := path.Join(destPath, entry.Name())

			// Recursively call Download
			err = i.download(src, dst, depth+1)
			if err != nil {
				return fmt.Errorf("failed to download %s: %v", src, err)
			}
		}
	} else {
		// Download a single file
		err = i.downloadFile(sourcePath, destPath, depth)
		if err != nil {
			return err
		}
	}

	return nil
}

func (i *Sftp) downloadFile(sourcePath string, destPath string, depth int) error {
	if depth == 0 {
		// Check if destPath is a directory
		destInfo, err := os.Stat(destPath)
		if err == nil && destInfo.IsDir() {
			// Join destPath with filename from sourcePath
			destPath = path.Join(destPath, path.Base(sourcePath))
		}
	}
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %v", err)
	}
	defer file.Close()

	reader := &FileReader{
		SourcePath:  sourcePath,
		Destination: file,
	}
	err = i.ReadFile(reader)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}
	return nil
}
