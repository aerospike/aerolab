package ingest

import (
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bestmethod/logger"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type safeError struct {
	sync.Mutex
	err error
}

func (e *safeError) Set(err error) {
	e.Lock()
	e.err = err
	e.Unlock()
}

func (e *safeError) Get() error {
	e.Lock()
	defer e.Unlock()
	return e.err
}

func (i *Ingest) Download() error {
	i.progress.Lock()
	i.progress.Downloader.running = true
	i.progress.Downloader.wasRunning = true
	i.progress.Unlock()
	defer func() {
		i.progress.Lock()
		i.progress.Downloader.running = false
		i.progress.Unlock()
	}()
	errs := new(safeError)
	logger.Debug("Downloader start")
	wg := new(sync.WaitGroup)
	if i.config.Downloader.S3Source != nil && i.config.Downloader.S3Source.Enabled {
		logger.Debug("DOWNLOAD: pulling from S3 source")
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := i.DownloadS3()
			if err != nil {
				errs.Set(err)
			}
		}()
	}
	if !i.config.Downloader.ConcurrentSources {
		wg.Wait()
		if err := errs.Get(); err != nil {
			return err
		}
	}
	if i.config.Downloader.SftpSource != nil && i.config.Downloader.SftpSource.Enabled {
		logger.Debug("DOWNLOAD: pulling from sftp source")
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := i.DownloadAsftp()
			if err != nil {
				errs.Set(err)
			}
		}()
	}
	wg.Wait()
	if err := errs.Get(); err != nil {
		return err
	}
	logger.Debug("Downloader exit")
	return nil
}

func (i *Ingest) DownloadS3() error {
	logger.Debug("Connecting to s3")
	cfg := aws.NewConfig()
	if i.config.Downloader.S3Source.Region != "" {
		cfg.Region = aws.String(i.config.Downloader.S3Source.Region)
	}
	if i.config.Downloader.S3Source.KeyID != "" {
		cfg.Credentials = credentials.NewStaticCredentials(i.config.Downloader.S3Source.KeyID, i.config.Downloader.S3Source.SecretKey, "")
	}
	sess, err := session.NewSession(cfg)
	if err != nil {
		return fmt.Errorf("connecting to s3: %s", err)
	}
	client := s3.New(sess)

	logger.Debug("S3 Connected, enumerating objects in bucket")
	var prefix *string
	if i.config.Downloader.S3Source.PathPrefix != "" {
		prefix = aws.String(i.config.Downloader.S3Source.PathPrefix)
	}
	fileList := make(map[string]*downloaderFile)
	i.progress.RLock()
	for k, v := range i.progress.Downloader.S3Files {
		if !v.IsDownloaded {
			fileList[k] = &downloaderFile{
				Size:         v.Size,
				LastModified: v.LastModified,
				IsDownloaded: false,
			}
		}
	}
	err = client.ListObjectsV2Pages(&s3.ListObjectsV2Input{
		Bucket:    aws.String(i.config.Downloader.S3Source.BucketName),
		Delimiter: aws.String("/"),
		Prefix:    prefix,
	}, func(page *s3.ListObjectsV2Output, lastPage bool) (continueIter bool) {
		for _, object := range page.Contents {
			if ofile, ok := i.progress.Downloader.S3Files[*object.Key]; !ok || ofile.Size != *object.Size || ofile.LastModified != *object.LastModified {
				if i.config.Downloader.S3Source.searchRegex != nil {
					regexOn := *object.Key
					if prefix != nil {
						regexOn = strings.TrimPrefix(*object.Key, *prefix)
					}
					if !i.config.Downloader.S3Source.searchRegex.MatchString(regexOn) {
						continue
					}
				}
				fileList[*object.Key] = &downloaderFile{
					Size:         *object.Size,
					LastModified: *object.LastModified,
					IsDownloaded: false,
				}
			}
		}
		return true
	})
	i.progress.RUnlock()
	if err != nil {
		return fmt.Errorf("S3 list objects: %s", err)
	}

	for okey, ofile := range fileList {
		logger.Detail("S3 to-download: %s (size:%d lastModified:%v)", okey, ofile.Size, ofile.LastModified)
	}

	logger.Debug("S3 Enumeration complete, saving results")
	i.progress.LockChange(true)
	for k, v := range fileList {
		i.progress.Downloader.S3Files[k] = v
	}
	i.progress.Unlock()

	logger.Debug("S3 Beginning download")
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.Downloader.S3Source.Threads)
	wg.Add(len(fileList))
	for f := range fileList {
		threads <- true
		go func(f string) {
			i.downloadS3File(client, f)
			<-threads
			wg.Done()
		}(f)
	}
	wg.Wait()
	logger.Debug("S3Source download complete")
	return nil
}

func (i *Ingest) downloadS3File(client *s3.S3, f string) {
	out, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(i.config.Downloader.S3Source.BucketName),
		Key:    aws.String(f),
	})
	if err != nil {
		logger.Warn("S3 Failed to init download of file %s: %s", f, err)
		i.progress.LockChange(true)
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		return
	}
	fd, _ := path.Split(f)
	err = os.MkdirAll(path.Join(i.config.Directories.DirtyTmp, "s3source", fd), 0755)
	if err != nil {
		logger.Warn("S3 Failed to create directory %s for file %s: %s", fd, f, err)
		i.progress.LockChange(true)
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		out.Body.Close()
		return
	}
	dst, err := os.Create(path.Join(i.config.Directories.DirtyTmp, "s3source", f))
	if err != nil {
		logger.Warn("S3 Failed to create file %s: %s", f, err)
		i.progress.LockChange(true)
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		out.Body.Close()
		return
	}
	_, err = io.Copy(dst, out.Body)
	dst.Close()
	out.Body.Close()
	if err != nil {
		logger.Warn("S3 Failed to download file %s: %s", f, err)
		i.progress.LockChange(true)
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		return
	}
	i.progress.LockChange(true)
	i.progress.Downloader.S3Files[f].IsDownloaded = true
	i.progress.Unlock()
}

func (i *Ingest) DownloadAsftp() error {
	client := &SSH{
		Ip:   fmt.Sprintf("%s:%d", i.config.Downloader.SftpSource.Host, i.config.Downloader.SftpSource.Port),
		User: i.config.Downloader.SftpSource.Username,
		Cert: i.config.Downloader.SftpSource.KeyFile,
		Pass: i.config.Downloader.SftpSource.Password,
	}
	mode := 2
	if i.config.Downloader.SftpSource.Password != "" {
		mode = 1
	}
	err := client.Connect(mode)
	if err != nil {
		return fmt.Errorf("sftp failed to connect: %s", err)
	}
	defer client.Close()
	sclient, err := sftp.NewClient(client.client)
	if err != nil {
		return fmt.Errorf("sftp failed to establish protocol: %s", err)
	}
	defer sclient.Close()
	fileList := make(map[string]*downloaderFile)
	var prefix *string
	if i.config.Downloader.SftpSource.PathPrefix != "" {
		prefix = &i.config.Downloader.SftpSource.PathPrefix
	}
	i.progress.RLock()
	for k, v := range i.progress.Downloader.SftpFiles {
		if !v.IsDownloaded {
			fileList[k] = &downloaderFile{
				Size:         v.Size,
				LastModified: v.LastModified,
				IsDownloaded: false,
			}
		}
	}
	walker := sclient.Walk(i.config.Downloader.SftpSource.PathPrefix)
	for walker.Step() {
		if err = walker.Err(); err != nil {
			i.progress.RUnlock()
			return fmt.Errorf("sftp failed to walk directories: %s", err)
		}
		wstat := walker.Stat()
		if wstat.IsDir() {
			continue
		}
		lastModTime := wstat.ModTime()
		size := wstat.Size()
		object := walker.Path()
		if ofile, ok := i.progress.Downloader.SftpFiles[object]; !ok || ofile.Size != size || ofile.LastModified != lastModTime {
			if i.config.Downloader.SftpSource.searchRegex != nil {
				regexOn := object
				if prefix != nil {
					regexOn = strings.TrimPrefix(strings.TrimPrefix(object, *prefix), "/")
				}
				if !i.config.Downloader.SftpSource.searchRegex.MatchString(regexOn) {
					continue
				}
			}
			fileList[object] = &downloaderFile{
				Size:         size,
				LastModified: lastModTime,
				IsDownloaded: false,
			}
		}
	}
	i.progress.RUnlock()

	for okey, ofile := range fileList {
		logger.Detail("sftp to-download: %s (size:%d lastModified:%v)", okey, ofile.Size, ofile.LastModified)
	}

	logger.Debug("sftp Enumeration complete, saving results")
	i.progress.LockChange(true)
	for k, v := range fileList {
		i.progress.Downloader.SftpFiles[k] = v
	}
	i.progress.Unlock()

	logger.Debug("sftp Beginning download")
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.Downloader.SftpSource.Threads)
	wg.Add(len(fileList))
	for f := range fileList {
		threads <- true
		go func(f string) {
			if err := sftpDownload(sclient, f, path.Join(i.config.Directories.DirtyTmp, "sftpsource")); err != nil {
				logger.Warn("%s (%s)", err, f)
				i.progress.LockChange(true)
				i.progress.Downloader.SftpFiles[f].Error = err.Error()
				i.progress.Unlock()
			} else {
				i.progress.LockChange(true)
				i.progress.Downloader.SftpFiles[f].IsDownloaded = true
				i.progress.Unlock()
			}
			<-threads
			wg.Done()
		}(f)
	}
	wg.Wait()

	logger.Debug("SftpSource Download Complete")
	return nil
}

func sftpDownload(sclient *sftp.Client, f string, dstDir string) error {
	src, err := sclient.Open(f)
	if err != nil {
		return fmt.Errorf("sftp could not open remote file: %s", err)
	}
	defer src.Close()
	fd, _ := path.Split(f)
	err = os.MkdirAll(path.Join(dstDir, fd), 0755)
	if err != nil {
		return fmt.Errorf("sftp failed to create directory: %s", err)
	}
	dst, err := os.Create(path.Join(dstDir, f))
	if err != nil {
		return fmt.Errorf("sftp Failed to create file: %s", err)
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("sftp failed to download file: %s", err)
	}
	return nil
}

type SSH struct {
	Ip     string
	User   string
	Cert   string // key file path
	Pass   string // password
	client *ssh.Client
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
	if mode == 1 {
		auth = []ssh.AuthMethod{ssh.Password(ssh_client.Pass)}
	} else if mode == 2 {
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

	ssh_client.client = client
	return nil
}

func (ssh_client *SSH) Close() {
	ssh_client.client.Close()
}
