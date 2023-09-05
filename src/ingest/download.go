package ingest

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bestmethod/logger"
)

func (i *Ingest) Download() error {
	logger.Debug("Downloader start")
	if i.config.Downloader.S3Source != nil {
		logger.Debug("DOWNLOAD: pulling from S3 source")
		err := i.DownloadS3()
		if err != nil {
			return err
		}
	}
	if i.config.Downloader.SftpSource != nil {
		logger.Debug("DOWNLOAD: pulling from sftp source")
		err := i.DownloadAsftp()
		if err != nil {
			return err
		}
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
	fileList := make(map[string]*downloaderS3File)
	i.progress.RLock()
	for k, v := range i.progress.Downloader.S3Files {
		if !v.IsDownloaded {
			fileList[k] = &downloaderS3File{
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
				fileList[*object.Key] = &downloaderS3File{
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
	for f := range fileList {
		out, err := client.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(i.config.Downloader.S3Source.BucketName),
			Key:    aws.String(f),
		})
		if err != nil {
			logger.Warn("S3 Failed to init download of file %s: %s", f, err)
			i.progress.LockChange(true)
			i.progress.Downloader.S3Files[f].Error = err.Error()
			i.progress.Unlock()
			continue
		}
		fd, _ := path.Split(f)
		err = os.MkdirAll(path.Join(i.config.Directories.DirtyTmp, "s3source", fd), 0755)
		if err != nil {
			logger.Warn("S3 Failed to create directory %s for file %s: %s", fd, f, err)
			i.progress.LockChange(true)
			i.progress.Downloader.S3Files[f].Error = err.Error()
			i.progress.Unlock()
			out.Body.Close()
			continue
		}
		dst, err := os.Create(path.Join(i.config.Directories.DirtyTmp, "s3source", f))
		if err != nil {
			logger.Warn("S3 Failed to create file %s: %s", f, err)
			i.progress.LockChange(true)
			i.progress.Downloader.S3Files[f].Error = err.Error()
			i.progress.Unlock()
			out.Body.Close()
			continue
		}
		_, err = io.Copy(dst, out.Body)
		dst.Close()
		out.Body.Close()
		if err != nil {
			logger.Warn("S3 Failed to download file %s: %s", f, err)
			i.progress.LockChange(true)
			i.progress.Downloader.S3Files[f].Error = err.Error()
			i.progress.Unlock()
			continue
		}
		i.progress.LockChange(true)
		i.progress.Downloader.S3Files[f].IsDownloaded = true
		i.progress.Unlock()
	}

	logger.Debug("S3Source download complete")
	return nil
}

func (i *Ingest) DownloadAsftp() error {
	return nil // TODO
}
