package ingest

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/bestmethod/logger"
)

func (i *Ingest) loadProgress() error {
	i.progress.Downloader.S3Files = make(map[string]*downloaderFile)
	i.progress.Downloader.SftpFiles = make(map[string]*downloaderFile)
	dir, _ := path.Split(i.config.ProgressFile.OutputFilePath)
	os.MkdirAll(dir, 0755)
	if _, err := os.Stat(i.config.ProgressFile.OutputFilePath); os.IsNotExist(err) {
		logger.Debug("INIT: Not loading progress - file not found")
		return nil
	}
	logger.Debug("INIT: Loading progress")
	f, err := os.Open(i.config.ProgressFile.OutputFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if !i.config.ProgressFile.Compress {
		return json.NewDecoder(f).Decode(i.progress)
	}
	fgz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer fgz.Close()
	return json.NewDecoder(fgz).Decode(i.progress)
}

func (i *Ingest) saveProgressInterval() {
	if i.config.ProgressFile.DisableWrite {
		logger.Debug("INIT: saving progress is disabled")
		return
	}
	logger.Debug("INIT: saving progress will run every %v", i.config.ProgressFile.WriteInterval)
	for {
		time.Sleep(i.config.ProgressFile.WriteInterval)
		err := i.saveProgress()
		if err != nil {
			logger.Warn("Progress could not be saved: %s", err)
		}
	}
}

func (i *Ingest) saveProgress() error {
	i.progress.Lock()
	defer i.progress.Unlock()
	if !i.progress.changed {
		logger.Detail("SAVE-PROGRESS Not changed, not saving")
		return nil
	}
	logger.Detail("SAVE-PROGRESS Saving")
	err := i.saveProgressDo()
	if err != nil {
		return err
	}
	logger.Detail("SAVE-PROGRESS Saved, rename and return")
	return os.Rename(i.config.ProgressFile.OutputFilePath+".tmp", i.config.ProgressFile.OutputFilePath)
}
func (i *Ingest) saveProgressDo() error {
	f, err := os.OpenFile(i.config.ProgressFile.OutputFilePath+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	var enc *json.Encoder
	if i.config.ProgressFile.Compress {
		fgz := gzip.NewWriter(f)
		defer fgz.Close()
		enc = json.NewEncoder(fgz)
	} else {
		enc = json.NewEncoder(f)
	}
	err = enc.Encode(i.progress)
	if err != nil {
		return err
	}
	i.progress.changed = false
	return nil
}

func (i *Ingest) printProgressInterval() {
	if !i.config.ProgressPrint.Enable {
		logger.Debug("PRINT-PROGRESS Not enabled, will not print")
		return
	}
	logger.Debug("PRINT-PROGRESS Will print every %v", i.config.ProgressPrint.UpdateInterval)
	for {
		time.Sleep(i.config.ProgressPrint.UpdateInterval)
		err := i.printProgress()
		if err != nil {
			logger.Warn("Progress could not be printed: %s", err)
		}
	}
}

func (i *Ingest) printProgress() error {
	i.progress.RLock()
	if i.progress.Downloader.wasRunning {
		s3total := 0
		s3done := 0
		sftptotal := 0
		sftpdone := 0
		s3sizeTotal := int64(0)
		s3sizeDown := int64(0)
		sftpSizeTotal := int64(0)
		sftpSizeDown := int64(0)
		for k, o := range i.progress.Downloader.S3Files {
			s3total++
			s3sizeTotal += o.Size
			downloadedSize := o.Size
			if o.IsDownloaded {
				s3done++
				s3sizeDown += o.Size
			} else {
				fs, err := os.Stat(path.Join(i.config.Directories.DirtyTmp, "s3source", k))
				if err == nil {
					s3sizeDown += fs.Size()
					downloadedSize = fs.Size()
				} else {
					downloadedSize = 0
				}
			}
			if i.config.ProgressPrint.PrintDetailProgress {
				logger.Info("downloader detail source:s3 file:%s size:%s downloadedSize=%s modified:%v isDownloaded:%t error:'%s'", k, convSize(o.Size), convSize(downloadedSize), o.LastModified, o.IsDownloaded, o.Error)
			}
		}
		for k, o := range i.progress.Downloader.SftpFiles {
			sftptotal++
			sftpSizeTotal += o.Size
			downloadedSize := o.Size
			if o.IsDownloaded {
				sftpdone++
				sftpSizeDown += o.Size
			} else {
				fs, err := os.Stat(path.Join(i.config.Directories.DirtyTmp, "sftpsource", k))
				if err == nil {
					sftpSizeDown += fs.Size()
					downloadedSize = fs.Size()
				} else {
					downloadedSize = 0
				}
			}
			if i.config.ProgressPrint.PrintDetailProgress {
				logger.Info("downloader detail source:sftp file:%s size:%s downloadedSize:%s modified:%v isDownloaded:%t error:'%s'", k, convSize(o.Size), convSize(downloadedSize), o.LastModified, o.IsDownloaded, o.Error)
			}
		}
		if i.config.ProgressPrint.PrintOverallProgress {
			logger.Info("downloader progress source:s3   totalFiles:%d downloadedFiles:%d totalSize:%s downloadedSize:%s", s3total, s3done, convSize(s3sizeTotal), convSize(s3sizeDown))
			logger.Info("downloader progress source:sftp totalFiles:%d downloadedFiles:%d totalSize:%s downloadedSize:%s", sftptotal, sftpdone, convSize(sftpSizeTotal), convSize(sftpSizeDown))
		}
		i.progress.Downloader.wasRunning = i.progress.Downloader.running
	}
	// TODO: progress of other steps
	i.progress.RUnlock()
	return nil
}

func convSize(size int64) string {
	var sizeString string
	if size > 1023 && size < 1024*1024 {
		sizeString = fmt.Sprintf("%.2f KiB", float64(size)/1024)
	} else if size < 1024 {
		sizeString = fmt.Sprintf("%v B", size)
	} else if size >= 1024*1024 && size < 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f MiB", float64(size)/1024/1024)
	} else if size >= 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f GiB", float64(size)/1024/1024/1024)
	}
	return sizeString
}
