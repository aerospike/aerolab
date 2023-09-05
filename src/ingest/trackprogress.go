package ingest

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path"
	"time"

	"github.com/bestmethod/logger"
)

func (i *Ingest) loadProgress() error {
	i.progress.Downloader.S3Files = make(map[string]*downloaderS3File)
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
	defer i.progress.RUnlock()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(i.progress)
	// TODO: make it pretty
}
