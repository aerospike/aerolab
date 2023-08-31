package ingest

import (
	"encoding/json"
	"log"
	"os"
	"path"
	"time"
)

func (i *Ingest) loadProgress() error {
	dir, _ := path.Split(i.config.ProgressFile.OutputFilePath)
	os.MkdirAll(dir, 0755)
	if _, err := os.Stat(i.config.ProgressFile.OutputFilePath); os.IsNotExist(err) {
		return nil
	}
	f, err := os.Open(i.config.ProgressFile.OutputFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(i.progress)
}

func (i *Ingest) saveProgressInterval() {
	if i.config.ProgressFile.DisableWrite {
		return
	}
	for {
		time.Sleep(i.config.ProgressFile.WriteInterval)
		err := i.saveProgress()
		if err != nil {
			log.Printf("WARN: progress could not be saved: %s", err)
		}
	}
}

func (i *Ingest) saveProgress() error {
	i.progress.Lock()
	defer i.progress.Unlock()
	if !i.progress.changed {
		return nil
	}
	f, err := os.OpenFile(i.config.ProgressFile.OutputFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(i.progress)
}

func (i *Ingest) printProgressInterval() {
	if !i.config.ProgressPrint.Enable {
		return
	}
	for {
		time.Sleep(i.config.ProgressPrint.UpdateInterval)
		err := i.printProgress()
		if err != nil {
			log.Printf("WARN: progress could not be printed: %s", err)
		}
	}
}

func (i *Ingest) printProgress() error {
	i.progress.Lock()
	defer i.progress.Unlock()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(i.progress)
	// TODO: make it pretty
}
