package ingest

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bestmethod/logger"
)

func (i *Ingest) PreProcess() error {
	logger.Debug("PreProcess running")
	i.progress.Lock()
	i.progress.PreProcessor.running = true
	i.progress.PreProcessor.wasRunning = true
	i.progress.PreProcessor.Finished = false
	i.progress.Unlock()
	defer func() {
		i.progress.Lock()
		i.progress.PreProcessor.running = false
		i.progress.Unlock()
	}()

	// cleanup any tmp_ in logs dir
	logger.Debug("PreProcess - cleaning tmp_")
	filepath.Walk(i.config.Directories.Logs, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(info.Name(), "tmp_") {
			os.Remove(path)
		}
		return nil
	})

	logger.Debug("Enumerating dirty dir")
	files, err := i.enum()
	if err != nil {
		return fmt.Errorf("failed to enumerate files: %s", err)
	}

	// dedup
	if i.config.Dedup.Enabled {
		logger.Debug("Deduplicating")
		i.Deduplicate(files)
		logger.Debug("Deduplication finished")
	} else {
		logger.Debug("Deduplication disabled")
	}

	// process
	logger.Debug("Pre-processing files")
	threads := make(chan bool, i.config.PreProcess.FileThreads)
	wg := new(sync.WaitGroup)
	for fn, file := range files {
		if !file.IsCollectInfo && !file.IsText {
			logger.Detail("pre-process %s is not text/collectinfo", fn)
			continue
		}
		if len(file.PreProcessDuplicateOf) > 0 {
			logger.Detail("pre-process %s is a duplicate of %s", fn, file.PreProcessDuplicateOf[0])
			continue
		}
		if file.IsCollectInfo {
			logger.Detail("pre-process %s is collectinfo, moving", fn)
			// deal with moving collectinfo
			i.progress.Lock()
			i.progress.PreProcessor.CollectInfoUniquePrefixes++
			prefix := "x" + strconv.Itoa(i.progress.PreProcessor.CollectInfoUniquePrefixes) + "_"
			_, fx := path.Split(fn)
			i.progress.PreProcessor.changed = true
			files[fn].PreProcessOutPaths = []string{path.Join(i.config.Directories.CollectInfo, prefix+fx)}
			i.progress.PreProcessor.Files[fn] = files[fn]
			i.progress.Unlock()
			os.Rename(fn, path.Join(i.config.Directories.CollectInfo, prefix+fx))
			continue
		}
		// deal with text files (could be aerospike log files)
		logger.Detail("pre-process %s is a text file, processing", fn)
		wg.Add(1)
		threads <- true
		go func(fn string) {
			err := i.PreProcessTextFile(fn, files)
			if err != nil {
				logger.Warn("Failed go pre-process text file %s: %s", fn, err)
				i.progress.Lock()
				files[fn].Errors = append(files[fn].Errors, err.Error())
				i.progress.PreProcessor.Files[fn] = files[fn]
				i.progress.PreProcessor.changed = true
				i.progress.Unlock()
			}
			<-threads
			wg.Done()
		}(fn)
	}
	wg.Wait()

	// move other files
	logger.Debug("Pre-process moving anything left over to the 'other' directory")
	dirtyRun := path.Join(i.config.Directories.OtherFiles, strconv.Itoa(int(time.Now().Unix())))
	err = os.MkdirAll(dirtyRun, 0755)
	if err != nil {
		return fmt.Errorf("could not create %s for other files: %s", dirtyRun, err)
	}
	others, err := os.ReadDir(i.config.Directories.DirtyTmp)
	if err != nil {
		return fmt.Errorf("could not list directory contents %s for other files: %s", i.config.Directories.DirtyTmp, err)
	}
	for _, other := range others {
		err = os.Rename(path.Join(i.config.Directories.DirtyTmp, other.Name()), path.Join(dirtyRun, other.Name()))
		if err != nil {
			logger.Error("could not move %s to %s: %s", path.Join(i.config.Directories.DirtyTmp, other.Name()), dirtyRun, err)
		}
	}

	// done
	i.progress.Lock()
	i.progress.PreProcessor.changed = true
	i.progress.PreProcessor.Files = files
	i.progress.PreProcessor.Finished = true
	i.progress.Unlock()
	logger.Debug("PreProcess finished")
	return nil
}

// TODO: test process on downloads that also have collectinfo and non-aerospike-logfiles and some jpg images
func (i *Ingest) PreProcessTextFile(fn string, files map[string]*enumFile) error {
	// TODO find correct format (json/tab,etc), and pre-process according to the format, storing the files in logs/clustername/prefix_nodeid_suffix
	// TODO: ensure we do not remove if the file was NOT aerospike, just return nil
	// TODO: process into tmp_prefix_nodeid_suffix first, and then rename all tmp_ files just before removal of original
	os.Remove(fn)
	i.progress.Lock()
	files[fn].PreProcessOutPaths = []string{} // TODO store the out paths
	i.progress.PreProcessor.Files[fn] = files[fn]
	i.progress.PreProcessor.changed = true
	i.progress.Unlock()
	return nil
}

func (i *Ingest) Deduplicate(files map[string]*enumFile) {
	filesBySize := make(map[int64][]string)
	logger.Detail("Dedplicate: sorting files by size")
	for fn, file := range files {
		if !file.IsText {
			// deduplicate only text type files
			continue
		}
		if _, ok := filesBySize[file.Size]; !ok {
			filesBySize[file.Size] = []string{fn}
		} else {
			filesBySize[file.Size] = append(filesBySize[file.Size], fn)
		}
	}
	logger.Detail("Dedplicate: sorting files where size is equal by shasum of the first %d bytes", i.config.Dedup.ReadBytes)
	filesBySha := make(map[[32]byte][]string)
	for _, bysize := range filesBySize {
		if len(bysize) < 2 {
			continue
		}
		for _, fn := range bysize {
			sha := i.genSha256(fn, files[fn].StartAt)
			if _, ok := filesBySha[sha]; !ok {
				filesBySha[sha] = []string{fn}
			} else {
				filesBySha[sha] = append(filesBySha[sha], fn)
			}
		}
	}
	logger.Detail("Deduplicate: marking and removing duplicates")
	for sha, duplicates := range filesBySha {
		if sha == [32]byte{} {
			// could not calculate sha, don't touch this
			continue
		}
		if len(duplicates) < 2 {
			// no duplicates
			continue
		}
		for i, dup := range duplicates {
			if i == 0 {
				continue
			}
			// mark everything except for first one as a duplicate
			files[dup].PreProcessDuplicateOf = append(files[dup].PreProcessDuplicateOf, duplicates[0])
			logger.Detail("DUPLICATE REMOVING %s duplicate of %s", dup, duplicates[0])
			os.Remove(dup) // delete duplicate files
		}
	}
}

func (i *Ingest) genSha256(fpath string, offset int64) [32]byte {
	f, err := os.Open(fpath)
	if err != nil {
		logger.Warn("Could not open file %s for sha256 generation: %s", fpath, err)
		return [32]byte{}
	}
	defer f.Close()
	f.Seek(offset, 0)
	b := make([]byte, i.config.Dedup.ReadBytes)
	_, err = f.Read(b)
	if err != nil {
		logger.Warn("Could not read file %s for sha256 generation: %s", fpath, err)
		return [32]byte{}
	}
	return sha256.Sum256(b)
}
