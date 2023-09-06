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
	i.progress.Downloader = new(progressDownloader)
	i.progress.CollectinfoProcessor = new(progressCollectProcessor)
	i.progress.LogProcessor = new(progressLogProcessor)
	i.progress.PreProcessor = new(progressPreProcessor)
	i.progress.Unpacker = new(progressUnpacker)
	i.progress.Downloader.S3Files = make(map[string]*downloaderFile)
	i.progress.Downloader.SftpFiles = make(map[string]*downloaderFile)
	os.MkdirAll(i.config.ProgressFile.OutputFilePath, 0755)
	fileList := []string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json"}
	logger.Debug("INIT: Loading progress")
	for _, file := range fileList {
		filex := file
		if i.config.ProgressFile.Compress {
			filex = file + ".gz"
		}
		fpath := path.Join(i.config.ProgressFile.OutputFilePath, filex)
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			logger.Debug("INIT: Not loading %s progress - file not found", fpath)
			continue
		}
		logger.Debug("INIT: Loading %s", fpath)
		err := i.loadProgressFile(file)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *Ingest) loadProgressFile(fname string) error {
	var item interface{}
	switch fname {
	case "downloader.json":
		item = i.progress.Downloader
	case "unpacker.json":
		item = i.progress.Unpacker
	case "pre-processor.json":
		item = i.progress.PreProcessor
	case "log-processor.json":
		item = i.progress.LogProcessor
	case "cf-processor.json":
		item = i.progress.CollectinfoProcessor
	}
	if i.config.ProgressFile.Compress {
		fname = fname + ".gz"
	}
	fname = path.Join(i.config.ProgressFile.OutputFilePath, fname)
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	if !i.config.ProgressFile.Compress {
		return json.NewDecoder(f).Decode(item)
	}
	fgz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer fgz.Close()
	return json.NewDecoder(fgz).Decode(item)
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
	if !i.progress.Downloader.changed && !i.progress.CollectinfoProcessor.changed && !i.progress.LogProcessor.changed && !i.progress.PreProcessor.changed && !i.progress.Unpacker.changed {
		logger.Detail("SAVE-PROGRESS Not changed, not saving")
		return nil
	}
	logger.Detail("SAVE-PROGRESS Saving")
	fileList := []string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json"}
	for _, file := range fileList {
		switch file {
		case "downloader.json":
			if !i.progress.Downloader.changed {
				continue
			}
		case "unpacker.json":
			if !i.progress.Unpacker.changed {
				continue
			}
		case "pre-processor.json":
			if !i.progress.PreProcessor.changed {
				continue
			}
		case "log-processor.json":
			if !i.progress.LogProcessor.changed {
				continue
			}
		case "cf-processor.json":
			if !i.progress.CollectinfoProcessor.changed {
				continue
			}
		}
		err := i.saveProgressDo(file)
		if err != nil {
			return err
		}
		if i.config.ProgressFile.Compress {
			file = file + ".gz"
		}
		err = os.Rename(path.Join(i.config.ProgressFile.OutputFilePath, file+".tmp"), path.Join(i.config.ProgressFile.OutputFilePath, file))
		if err != nil {
			return err
		}
	}
	logger.Detail("SAVE-PROGRESS Saved, rename and return")
	return nil
}
func (i *Ingest) saveProgressDo(file string) error {
	var item interface{}
	switch file {
	case "downloader.json":
		item = i.progress.Downloader
	case "unpacker.json":
		item = i.progress.Unpacker
	case "pre-processor.json":
		item = i.progress.PreProcessor
	case "log-processor.json":
		item = i.progress.LogProcessor
	case "cf-processor.json":
		item = i.progress.CollectinfoProcessor
	}
	if i.config.ProgressFile.Compress {
		file = file + ".gz"
	}
	f, err := os.OpenFile(path.Join(i.config.ProgressFile.OutputFilePath, file+".tmp"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
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
	err = enc.Encode(item)
	if err != nil {
		return err
	}
	switch file {
	case "downloader.json":
		i.progress.Downloader.changed = false
	case "unpacker.json":
		i.progress.Unpacker.changed = false
	case "pre-processor.json":
		i.progress.PreProcessor.changed = false
	case "log-processor.json":
		i.progress.LogProcessor.changed = false
	case "cf-processor.json":
		i.progress.CollectinfoProcessor.changed = false
	}
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
		if !i.progress.Downloader.wasRunning {
			logger.Info("downloader finished")
		}
	}
	if i.progress.Unpacker.wasRunning {
		if i.progress.Unpacker.running {
			logger.Info("unpacker running")
		} else {
			logger.Info("unpacker finished")
			i.progress.Unpacker.wasRunning = i.progress.Unpacker.running
			if i.config.ProgressPrint.PrintDetailProgress {
				for fn, file := range i.progress.Unpacker.Files {
					logger.Info("unpacker detail file:%s (size:%s) (isArchive:%t isCollectInfo:%t isTarBz:%t isTarGz:%t isText:%t) (unpackFailed:%t) (contentType:%s)", fn, convSize(file.Size), file.IsArchive, file.IsCollectInfo, file.IsTarBz, file.IsTarGz, file.IsText, file.UnpackFailed, file.ContentType)
				}
			}
		}
	}
	// TODO: progress of pre-processor
	// TODO: progress of processor
	// TODO: progress of collectInfo rename
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
