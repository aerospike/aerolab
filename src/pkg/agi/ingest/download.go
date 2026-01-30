package ingest

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/sftp"
	"log"
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
	i.progress.Downloader.Finished = false
	i.progress.Downloader.running = true
	i.progress.Downloader.wasRunning = true
	i.progress.Unlock()
	defer func() {
		i.progress.Lock()
		i.progress.Downloader.running = false
		i.progress.Unlock()
	}()
	errs := new(safeError)
	log.Printf("DEBUG: Downloader start")
	wg := new(sync.WaitGroup)
	if i.config.Downloader.S3Source != nil && i.config.Downloader.S3Source.Enabled {
		log.Printf("DEBUG: DOWNLOAD: pulling from S3 source")
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
		log.Printf("DEBUG: DOWNLOAD: pulling from sftp source")
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
	i.progress.Lock()
	i.progress.Downloader.Finished = true
	i.progress.Unlock()
	log.Printf("DEBUG: Downloader exit")
	return nil
}

func (i *Ingest) DownloadS3() error {
	log.Printf("DEBUG: Connecting to s3")
	var cfgParams []func(*config.LoadOptions) error
	if i.config.Downloader.S3Source.Region != "" {
		cfgParams = append(cfgParams, config.WithRegion(i.config.Downloader.S3Source.Region))
	}
	if i.config.Downloader.S3Source.KeyID != "" {
		cfgParams = append(cfgParams, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(i.config.Downloader.S3Source.KeyID, i.config.Downloader.S3Source.SecretKey, "")))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), cfgParams...)
	if err != nil {
		return fmt.Errorf("connecting to s3: %s", err)
	}
	var client *s3.Client
	if i.config.Downloader.S3Source.Endpoint != "" {
		client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.EndpointOptions.UseFIPSEndpoint = aws.FIPSEndpointStateEnabled
		})
	} else {
		client = s3.NewFromConfig(cfg)
	}
	log.Printf("DEBUG: S3 Connected, enumerating objects in bucket")
	var prefix *string
	if i.config.Downloader.S3Source.PathPrefix != "" {
		prefix = aws.String(i.config.Downloader.S3Source.PathPrefix)
	}
	fileList := make(map[string]*DownloaderFile)
	i.progress.RLock()
	for k, v := range i.progress.Downloader.S3Files {
		if !v.IsDownloaded {
			fileList[k] = &DownloaderFile{
				Size:         v.Size,
				LastModified: v.LastModified,
				IsDownloaded: false,
			}
		}
	}
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(i.config.Downloader.S3Source.BucketName),
		//Delimiter: aws.String("/"), // commented out to allow for partial matches in directory names (like having * at the end of the name, no need to trailing slash)
		Prefix: prefix,
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			i.progress.RUnlock()
			return fmt.Errorf("S3 list objects: %s", err)
		}
		for _, object := range page.Contents {
			if ofile, ok := i.progress.Downloader.S3Files[*object.Key]; !ok || ofile.Size != *object.Size || ofile.LastModified != *object.LastModified {
				if strings.HasSuffix(*object.Key, "/") {
					continue
				}
				if i.config.Downloader.S3Source.searchRegex != nil {
					regexOn := *object.Key
					if prefix != nil {
						regexOn = strings.TrimPrefix(*object.Key, *prefix)
					}
					if !i.config.Downloader.S3Source.searchRegex.MatchString(regexOn) {
						continue
					}
				}
				fileList[*object.Key] = &DownloaderFile{
					Size:         *object.Size,
					LastModified: *object.LastModified,
					IsDownloaded: false,
				}
			}
		}
	}
	i.progress.RUnlock()

	for okey, ofile := range fileList {
		log.Printf("DETAIL: S3 to-download: %s (size:%d lastModified:%v)", okey, ofile.Size, ofile.LastModified)
	}

	log.Printf("DEBUG: S3 Enumeration complete, saving results")
	i.progress.Lock()
	i.progress.Downloader.changed = true
	for k, v := range fileList {
		i.progress.Downloader.S3Files[k] = v
	}
	i.progress.Unlock()

	log.Printf("DEBUG: S3 Beginning download")
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.Downloader.S3Source.Threads)
	wg.Add(len(fileList))
	for f := range fileList {
		threads <- true
		go func(f string) {
			err := i.downloadS3File(client, f)
			if err != nil {
				i.downloadS3File(client, f)
			}
			<-threads
			wg.Done()
		}(f)
	}
	wg.Wait()
	log.Printf("DEBUG: S3Source download complete")
	return nil
}

func (i *Ingest) downloadS3File(client *s3.Client, f string) error {
	i.progress.Lock()
	i.progress.Downloader.changed = true
	i.progress.Downloader.S3Files[f].StartTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
	i.progress.Unlock()

	out, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(i.config.Downloader.S3Source.BucketName),
		Key:    aws.String(f),
	})
	if err != nil {
		log.Printf("WARN: S3 Failed to init download of file %s: %s", f, err)
		i.progress.Lock()
		i.progress.Downloader.changed = true
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		return err
	}
	fd, _ := path.Split(f)
	err = os.MkdirAll(path.Join(i.config.Directories.DirtyTmp, "s3source", fd), 0755)
	if err != nil {
		log.Printf("WARN: S3 Failed to create directory %s for file %s: %s", fd, f, err)
		i.progress.Lock()
		i.progress.Downloader.changed = true
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		out.Body.Close()
		return err
	}
	dst, err := os.Create(path.Join(i.config.Directories.DirtyTmp, "s3source", f))
	if err != nil {
		log.Printf("WARN: S3 Failed to create file %s: %s", f, err)
		i.progress.Lock()
		i.progress.Downloader.changed = true
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		out.Body.Close()
		return err
	}
	_, err = io.Copy(dst, out.Body)
	dst.Close()
	out.Body.Close()
	if err != nil {
		log.Printf("WARN: S3 Failed to download file %s: %s", f, err)
		i.progress.Lock()
		i.progress.Downloader.changed = true
		i.progress.Downloader.S3Files[f].Error = err.Error()
		i.progress.Unlock()
		return err
	}
	i.progress.Lock()
	i.progress.Downloader.changed = true
	i.progress.Downloader.S3Files[f].IsDownloaded = true
	i.progress.Downloader.S3Files[f].FinishTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
	i.progress.Unlock()
	return nil
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
	fileList := make(map[string]*DownloaderFile)
	var prefix *string
	if i.config.Downloader.SftpSource.PathPrefix != "" {
		prefix = &i.config.Downloader.SftpSource.PathPrefix
	}
	i.progress.RLock()
	for k, v := range i.progress.Downloader.SftpFiles {
		if !v.IsDownloaded {
			fileList[k] = &DownloaderFile{
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
			fileList[object] = &DownloaderFile{
				Size:         size,
				LastModified: lastModTime,
				IsDownloaded: false,
			}
		}
	}
	i.progress.RUnlock()

	for okey, ofile := range fileList {
		log.Printf("DETAIL: sftp to-download: %s (size:%d lastModified:%v)", okey, ofile.Size, ofile.LastModified)
	}

	log.Printf("DEBUG: sftp Enumeration complete, saving results")
	i.progress.Lock()
	i.progress.Downloader.changed = true
	for k, v := range fileList {
		i.progress.Downloader.SftpFiles[k] = v
	}
	i.progress.Unlock()

	log.Printf("DEBUG: sftp Beginning download")
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.Downloader.SftpSource.Threads)
	wg.Add(len(fileList))
	for f := range fileList {
		log.Printf("DETAIL: sftp Downloading %s", f)
		threads <- true
		go func(f string) {
			log.Printf("DETAIL: sftp Thread secured, proceeding to download %s", f)
			i.progress.Lock()
			i.progress.Downloader.changed = true
			i.progress.Downloader.SftpFiles[f].StartTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
			i.progress.Unlock()
			err := sftpDownload(sclient, f, path.Join(i.config.Directories.DirtyTmp, "sftpsource"))
			if err != nil {
				err = sftpDownload(sclient, f, path.Join(i.config.Directories.DirtyTmp, "sftpsource"))
			}
			if err != nil {
				log.Printf("WARN: %s (%s)", err, f)
				i.progress.Lock()
				i.progress.Downloader.changed = true
				i.progress.Downloader.SftpFiles[f].Error = err.Error()
				i.progress.Unlock()
			} else {
				i.progress.Lock()
				i.progress.Downloader.changed = true
				i.progress.Downloader.SftpFiles[f].IsDownloaded = true
				i.progress.Downloader.SftpFiles[f].FinishTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
				i.progress.Unlock()
			}
			<-threads
			wg.Done()
		}(f)
	}
	wg.Wait()

	log.Printf("DEBUG: SftpSource Download Complete")
	return nil
}

func sftpDownload(sclient *sftp.Client, f string, dstDir string) error {
	log.Printf("DETAIL: sftp open %s", f)
	src, err := sclient.Open(f)
	if err != nil {
		return fmt.Errorf("sftp could not open remote file: %s", err)
	}
	defer src.Close()
	fd, _ := path.Split(f)
	log.Printf("DETAIL: sftp mkdir for %s", f)
	err = os.MkdirAll(path.Join(dstDir, fd), 0755)
	if err != nil {
		return fmt.Errorf("sftp failed to create directory: %s", err)
	}
	log.Printf("DETAIL: sftp create local for %s", f)
	dst, err := os.Create(path.Join(dstDir, f))
	if err != nil {
		return fmt.Errorf("sftp Failed to create file: %s", err)
	}
	defer dst.Close()
	log.Printf("DETAIL: sftp start copy for %s", f)
	_, err = src.WriteTo(dst)
	if err != nil {
		return fmt.Errorf("sftp failed to download file: %s", err)
	}
	log.Printf("DETAIL: sftp end copy for %s", f)
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

func SftpCheckLogin(config *Config, getFileList bool) (map[string]*DownloaderFile, error) {
	if config.Downloader.SftpSource.SearchRegex != "" {
		regex, err := regexp.Compile(config.Downloader.SftpSource.SearchRegex)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %s: %s", config.Downloader.SftpSource.SearchRegex, err)
		}
		config.Downloader.SftpSource.searchRegex = regex
	}
	i := &Ingest{
		config: config,
	}
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
		return nil, fmt.Errorf("sftp failed to connect: %s", err)
	}
	defer client.Close()
	sclient, err := sftp.NewClient(client.client)
	if err != nil {
		return nil, fmt.Errorf("sftp failed to establish protocol: %s", err)
	}
	defer sclient.Close()
	fileList := make(map[string]*DownloaderFile)
	var prefix *string
	if i.config.Downloader.SftpSource.PathPrefix != "" {
		prefix = &i.config.Downloader.SftpSource.PathPrefix
	}
	if getFileList {
		walker := sclient.Walk(i.config.Downloader.SftpSource.PathPrefix)
		for walker.Step() {
			if err = walker.Err(); err != nil {
				return nil, fmt.Errorf("sftp failed to walk directories: %s", err)
			}
			wstat := walker.Stat()
			if wstat.IsDir() {
				continue
			}
			lastModTime := wstat.ModTime()
			size := wstat.Size()
			object := walker.Path()
			if i.config.Downloader.SftpSource.searchRegex != nil {
				regexOn := object
				if prefix != nil {
					regexOn = strings.TrimPrefix(strings.TrimPrefix(object, *prefix), "/")
				}
				if !i.config.Downloader.SftpSource.searchRegex.MatchString(regexOn) {
					continue
				}
			}
			fileList[object] = &DownloaderFile{
				Size:         size,
				LastModified: lastModTime,
				IsDownloaded: false,
			}
		}
		return fileList, nil
	}
	fi, err := sclient.Stat(i.config.Downloader.SftpSource.PathPrefix)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		fl, err := sclient.ReadDir(i.config.Downloader.SftpSource.PathPrefix)
		if err != nil {
			return nil, err
		}
		for _, f := range fl {
			fileList[f.Name()] = &DownloaderFile{
				Size: f.Size(),
			}
		}
	} else {
		fileList[fi.Name()] = &DownloaderFile{
			Size: fi.Size(),
		}
	}
	return fileList, nil
}
