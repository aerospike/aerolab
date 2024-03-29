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
	i.progress.Downloader = new(ProgressDownloader)
	i.progress.CollectinfoProcessor = new(ProgressCollectProcessor)
	i.progress.LogProcessor = new(ProgressLogProcessor)
	i.progress.LogProcessor.LineErrors = new(lineErrors)
	i.progress.PreProcessor = new(ProgressPreProcessor)
	i.progress.Unpacker = new(ProgressUnpacker)
	i.progress.Downloader.S3Files = make(map[string]*DownloaderFile)
	i.progress.Downloader.SftpFiles = make(map[string]*DownloaderFile)
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
	i.progress.LogProcessor.LineErrors = new(lineErrors)
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
		i.endLock.Lock()
		if i.end {
			i.endLock.Unlock()
			return
		}
		i.endLock.Unlock()
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
	if i.progress.LogProcessor.LineErrors.isChanged() {
		i.progress.LogProcessor.changed = true
	}
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
	if i.config.LogLevel > 4 {
		enc.SetIndent("", "  ")
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
		i.endLock.Lock()
		if i.end {
			i.endLock.Unlock()
			return
		}
		i.endLock.Unlock()
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
					logger.Detail("TrackProcess: stat %s: %s", path.Join(i.config.Directories.DirtyTmp, "sftpsource", k), err)
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
					logger.Detail("TrackProcess: stat %s: %s", path.Join(i.config.Directories.DirtyTmp, "sftpsource", k), err)
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
					logger.Info("unpacker detail file:%s (size:%s) (isArchive:%t isCollectInfo:%t isTarBz:%t isTarGz:%t isText:%t) (unpackFailed:%t) (contentType:%s) (errors:%v)", fn, convSize(file.Size), file.IsArchive, file.IsCollectInfo, file.IsTarBz, file.IsTarGz, file.IsText, file.UnpackFailed, file.ContentType, file.Errors)
				}
			}
		}
	}
	if i.progress.PreProcessor.wasRunning {
		if i.progress.PreProcessor.running {
			logger.Info("pre-processor running")
		} else {
			logger.Info("pre-processor finished")
			i.progress.PreProcessor.wasRunning = i.progress.PreProcessor.running
		}
		if i.config.ProgressPrint.PrintDetailProgress {
			for fn, file := range i.progress.PreProcessor.Files {
				if len(file.PreProcessDuplicateOf) > 0 {
					dup := ""
					for _, a := range file.PreProcessDuplicateOf {
						dup = dup + "\n\t" + a
					}
					logger.Info("pre-processor detail file:%s (size:%s) (isArchive:%t isCollectInfo:%t isTarBz:%t isTarGz:%t isText:%t) (contentType:%s) (errors:%v) duplicateOf:%v", fn, convSize(file.Size), file.IsArchive, file.IsCollectInfo, file.IsTarBz, file.IsTarGz, file.IsText, file.ContentType, file.Errors, dup)
				} else {
					out := ""
					for _, a := range file.PreProcessOutPaths {
						out = out + "\n\t" + a
					}
					logger.Info("pre-processor detail file:%s (size:%s) (isArchive:%t isCollectInfo:%t isTarBz:%t isTarGz:%t isText:%t) (contentType:%s) (errors:%v) outputFiles:%v", fn, convSize(file.Size), file.IsArchive, file.IsCollectInfo, file.IsTarBz, file.IsTarGz, file.IsText, file.ContentType, file.Errors, out)
				}
			}
		}
	}
	if i.progress.CollectinfoProcessor.wasRunning {
		if i.progress.CollectinfoProcessor.running {
			logger.Info("CollectinfoProcessor running")
		} else {
			logger.Info("CollectinfoProcessor finished")
			i.progress.CollectinfoProcessor.wasRunning = i.progress.CollectinfoProcessor.running
		}
		if i.config.ProgressPrint.PrintDetailProgress {
			for fn, file := range i.progress.CollectinfoProcessor.Files {
				dup := ""
				for _, a := range file.Errors {
					dup = dup + "\n\t" + a
				}
				if dup == "" {
					dup = "nil"
				}
				logger.Info("CollectinfoProcessor detail file:%s (size:%s) (nodeID:%s) (renameAttempt:%t renamed:%t processAttempt:%t processed:%t) (originalName:%s) errors:%s", fn, convSize(file.Size), file.NodeID, file.RenameAttempted, file.Renamed, file.ProcessingAttempted, file.Processed, file.OriginalName, dup)
			}
		}
	}
	if i.progress.LogProcessor.wasRunning {
		if i.progress.LogProcessor.running {
			logger.Info("LogProcessor running")
		} else {
			logger.Info("LogProcessor finished")
			i.progress.LogProcessor.wasRunning = i.progress.LogProcessor.running
		}
		if i.config.ProgressPrint.PrintDetailProgress {
			for fn, file := range i.progress.LogProcessor.Files {
				logger.Info("LogProcessor detail file:%s (size:%s) (processed:%s) (clusterName:%s) (finished:%t) (fullNodeIdent:%s)", fn, convSize(file.Size), convSize(file.Processed), file.ClusterName, file.Finished, file.NodePrefix+"_"+file.NodeID+"_"+file.NodeSuffix)
			}
		}
		if i.config.ProgressPrint.PrintOverallProgress {
			timePassedx := time.Since(i.progress.LogProcessor.StartTime)
			timePassed := int64(timePassedx.Seconds())
			if timePassed < 1 {
				timePassed = 1
			}
			totalSize := int64(0)
			processedSize := int64(0)
			for _, file := range i.progress.LogProcessor.Files {
				totalSize += file.Size
				processedSize += file.Processed
			}
			percentComplete := int64(0)
			if totalSize > 0 {
				percentComplete = processedSize * 100 / totalSize
			}
			perSecond := processedSize / timePassed
			remainingSize := totalSize - processedSize
			remainingTime := time.Hour * 24
			if perSecond >= 1 {
				remainingTime = time.Second * time.Duration(remainingSize/perSecond)
			}
			logger.Info("LogProcessor summary: (processed:%s) (total:%s) (remaining:%s) (speed:%s/second) (pct-complete:%d) (runTime:%s) (remainingTime:%s)", convSize(processedSize), convSize(totalSize), convSize(remainingSize), convSize(perSecond), percentComplete, timePassedx.String(), remainingTime.String())
		}
	}
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
