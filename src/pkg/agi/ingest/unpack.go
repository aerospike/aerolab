package ingest

import (
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"log"
)

type safeBool struct {
	sync.Mutex
	v bool
}

func (b *safeBool) Set(v bool) {
	b.Lock()
	b.v = v
	b.Unlock()
}

func (b *safeBool) Get() bool {
	b.Lock()
	defer b.Unlock()
	return b.v
}

type safeStringSlice struct {
	sync.Mutex
	v []string
}

func (b *safeStringSlice) Set(v []string) {
	b.Lock()
	b.v = v
	b.Unlock()
}

func (b *safeStringSlice) Append(v string) {
	b.Lock()
	b.v = append(b.v, v)
	b.Unlock()
}

func (b *safeStringSlice) Get() []string {
	b.Lock()
	defer b.Unlock()
	return b.v
}

type safeStringMap struct {
	sync.Mutex
	v map[string]string
}

func (b *safeStringMap) Set(k string, v string) {
	b.Lock()
	if b.v == nil {
		b.v = make(map[string]string)
	}
	b.v[k] = v
	b.Unlock()
}

func (b *safeStringMap) Get() map[string]string {
	return b.v
}

func (i *Ingest) Unpack() error {
	log.Printf("DEBUG: Unpack running")
	i.progress.Lock()
	i.progress.Unpacker.running = true
	i.progress.Unpacker.wasRunning = true
	i.progress.Unpacker.Finished = false
	i.progress.Unlock()
	defer func() {
		i.progress.Lock()
		i.progress.Unpacker.running = false
		i.progress.Unlock()
	}()

	readOnlyInput := i.config.Directories.ReadOnlyInput
	stagingDir := ""
	if readOnlyInput {
		// In read-only mode, we need a staging area for archives since we can't extract in place
		stagingDir = filepath.Join(filepath.Dir(i.config.Directories.DirtyTmp), "staging")
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			return fmt.Errorf("failed to create staging directory: %w", err)
		}
		log.Printf("DEBUG: Unpack: using staging directory %s for read-only input mode", stagingDir)
	}

	var files map[string]*EnumFile
	var err error
	ignoreFailedUnpacks := new(safeStringSlice)
	ignoreFailedErrors := new(safeStringMap)
	for {
		log.Printf("DETAIL: unpack: enumerate")
		files, err = i.enum()
		if err != nil {
			return fmt.Errorf("failed to enumerate files: %s", err)
		}

		// In read-only mode, also enumerate the staging directory
		if readOnlyInput && stagingDir != "" {
			stagingFiles, err := i.enumDir(stagingDir)
			if err != nil {
				log.Printf("WARN: Failed to enumerate staging directory: %s", err)
			} else {
				for fn, file := range stagingFiles {
					files[fn] = file
				}
			}
		}

		foundArchives := new(safeBool)
		wg := new(sync.WaitGroup)
		threads := make(chan bool, i.config.PreProcess.UnpackerFileThreads)
		log.Printf("DETAIL: unpack: unpacking")
		for fn, file := range files {
			if !file.IsArchive || slices.Contains(ignoreFailedUnpacks.Get(), fn) {
				failedUnpack := false
				if file.IsArchive {
					failedUnpack = true
				}
				log.Printf("DETAIL: unpack %s no-unpack isArchive:%t unpackFailed:%t", fn, file.IsArchive, failedUnpack)
				continue
			}
			wg.Add(1)
			threads <- true
			log.Printf("DETAIL: unpack %s starting, threads:%d", fn, len(threads))
			go func(fn string, file *EnumFile) {
				err = i.unpackFile(fn, file)
				if err != nil {
					log.Printf("WARN: Unpack of %s failed with %s", fn, err)
					ignoreFailedUnpacks.Append(fn)
					ignoreFailedErrors.Set(fn, err.Error())
				} else {
					foundArchives.Set(true)
				}
				<-threads
				wg.Done()
				log.Printf("DETAIL: unpack %s end", fn)
			}(fn, file)
		}
		wg.Wait()
		if !foundArchives.Get() {
			log.Printf("DETAIL: unpack: finished unpacking")
			break
		}
		log.Printf("DETAIL: unpack: had archives, looping to start")
	}
	log.Printf("DETAIL: unpack: last enumerate, merge and store progress")
	for fn, errs := range ignoreFailedErrors.Get() {
		if _, ok := files[fn]; ok {
			files[fn].UnpackFailed = true
			files[fn].Errors = []string{errs}
		}
	}
	i.progress.Lock()
	i.progress.Unpacker.changed = true
	i.progress.Unpacker.Files = files
	i.progress.Unpacker.Finished = true
	i.progress.Unlock()
	log.Printf("DEBUG: Unpack finished")
	return nil
}

func (i *Ingest) unpackFile(fileName string, fileInfo *EnumFile) error {
	readOnlyInput := i.config.Directories.ReadOnlyInput
	workFileName := fileName

	// In read-only mode, copy archive to staging area first
	if readOnlyInput && strings.HasPrefix(fileName, i.config.Directories.DirtyTmp) {
		stagingDir := filepath.Join(filepath.Dir(i.config.Directories.DirtyTmp), "staging")
		relPath, err := filepath.Rel(i.config.Directories.DirtyTmp, fileName)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		workFileName = filepath.Join(stagingDir, relPath)

		// Create parent directory in staging
		if err := os.MkdirAll(filepath.Dir(workFileName), 0755); err != nil {
			return fmt.Errorf("failed to create staging subdirectory: %w", err)
		}

		// Copy file to staging
		srcFile, err := os.Open(fileName)
		if err != nil {
			return fmt.Errorf("failed to open source archive: %w", err)
		}
		defer srcFile.Close()

		dstFile, err := os.Create(workFileName)
		if err != nil {
			return fmt.Errorf("failed to create staging archive: %w", err)
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("failed to copy archive to staging: %w", err)
		}
		dstFile.Close()
		srcFile.Close()

		log.Printf("DETAIL: unpack: copied %s to staging %s for extraction", fileName, workFileName)
	}

	contentType := fileInfo.mimeType
	var err error
	if fileInfo.IsTarGz {
		// handle optimisation to auto-unpack tar-gz in one swoop
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		var fd *os.File
		var fdgzip *gzip.Reader
		fd, err = os.Open(workFileName)
		if err != nil {
			return err
		}
		defer fd.Close()
		fdgzip, err = gzip.NewReader(fd)
		if err != nil {
			return err
		}
		defer fdgzip.Close()
		err = untar(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)), fdgzip)
		if err != nil {
			return err
		}
		// Only remove the archive if not in read-only mode, or if it's in staging
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if fileInfo.IsTarBz {
		// handle optimisation to auto-unpack tar-bz in one swoop
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		var fd *os.File
		fd, err = os.Open(workFileName)
		if err != nil {
			return err
		}
		defer fd.Close()
		fdbzip := bzip2.NewReader(fd)
		err = untar(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)), fdbzip)
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if contentType.Is("application/gzip") {
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		nNewFile := nfile
		nNewFile = strings.TrimSuffix(nNewFile, ".gz")
		if strings.HasSuffix(nNewFile, ".tgz") {
			nNewFile = fmt.Sprintf("%s.tar", nNewFile[:len(nNewFile)-4])
		}
		nNewFile = filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile), nNewFile)
		err = ungz(workFileName, nNewFile)
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if contentType.Is("application/x-xz") {
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		nNewFile := nfile
		nNewFile = strings.TrimSuffix(nNewFile, ".xz")
		nNewFile = filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile), nNewFile)
		err = unxz(workFileName, nNewFile)
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if contentType.Is("application/zip") {
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		_, err = unzip(workFileName, filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if contentType.Is("application/x-tar") {
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		var fd *os.File
		fd, err = os.Open(workFileName)
		if err != nil {
			return err
		}
		defer fd.Close()
		err = untar(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)), fd)
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if contentType.Is("application/x-bzip2") {
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		nNewFile := nfile
		if strings.HasSuffix(nNewFile, ".bz2") {
			nNewFile = nNewFile[:len(nNewFile)-3]
		}
		if strings.HasSuffix(nNewFile, ".bzip") {
			nNewFile = nNewFile[:len(nNewFile)-4]
		}
		if strings.HasSuffix(nNewFile, ".bz") {
			nNewFile = nNewFile[:len(nNewFile)-2]
		}
		if strings.HasSuffix(nNewFile, ".bzip2") {
			nNewFile = nNewFile[:len(nNewFile)-5]
		}
		destFile := filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile), nNewFile)
		err = unbz2(workFileName, destFile)
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if contentType.Is("application/x-rar-compressed") {
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		err = unrar(workFileName, filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else if contentType.Is("application/x-7z-compressed") {
		ndir, nfile := filepath.Split(workFileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		err = un7z(workFileName, filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		if !readOnlyInput || workFileName != fileName {
			err = os.Remove(workFileName)
		}
	} else {
		err = errors.New("we never should have reached this part of code, wtf?")
	}
	return err
}

func (i *Ingest) createDir(name string) error {
	_, err := os.Stat(name)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	err = os.MkdirAll(name, 0755)
	return err
}
