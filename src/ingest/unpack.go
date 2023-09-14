package ingest

import (
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
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
	logger.Debug("Unpack running")
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
	var files map[string]*enumFile
	var err error
	ignoreFailedUnpacks := new(safeStringSlice)
	ignoreFailedErrors := new(safeStringMap)
	for {
		logger.Detail("unpack: enumerate")
		files, err = i.enum()
		if err != nil {
			return fmt.Errorf("failed to enumerate files: %s", err)
		}
		foundArchives := new(safeBool)
		wg := new(sync.WaitGroup)
		threads := make(chan bool, i.config.PreProcess.UnpackerFileThreads)
		logger.Detail("unpack: unpacking")
		for fn, file := range files {
			if !file.IsArchive || inslice.HasString(ignoreFailedUnpacks.Get(), fn) {
				failedUnpack := false
				if file.IsArchive {
					failedUnpack = true
				}
				logger.Detail("unpack %s no-unpack isArchive:%t unpackFailed:%t", fn, file.IsArchive, failedUnpack)
				continue
			}
			wg.Add(1)
			threads <- true
			logger.Detail("unpack %s starting, threads:%d", fn, len(threads))
			go func(fn string, file *enumFile) {
				err = i.unpackFile(fn, file)
				if err != nil {
					logger.Warn("Unpack of %s failed with %s", fn, err)
					ignoreFailedUnpacks.Append(fn)
					ignoreFailedErrors.Set(fn, err.Error())
				} else {
					foundArchives.Set(true)
				}
				<-threads
				wg.Done()
				logger.Detail("unpack %s end", fn)
			}(fn, file)
		}
		wg.Wait()
		if !foundArchives.Get() {
			logger.Detail("unpack: finished unpacking")
			break
		}
		logger.Detail("unpack: had archives, looping to start")
	}
	logger.Detail("unpack: last enumerate, merge and store progress")
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
	logger.Debug("Unpack finished")
	return nil
}

func (i *Ingest) unpackFile(fileName string, fileInfo *enumFile) error {
	contentType := fileInfo.mimeType
	var err error
	if fileInfo.IsTarGz {
		// handle optimisation to auto-unpack tar-gz in one swoop
		ndir, nfile := filepath.Split(fileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		var fd *os.File
		var fdgzip *gzip.Reader
		fd, err = os.Open(fileName)
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
		err = os.Remove(fileName)
	} else if fileInfo.IsTarBz {
		// handle optimisation to auto-unpack tar-bz in one swoop
		ndir, nfile := filepath.Split(fileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		var fd *os.File
		fd, err = os.Open(fileName)
		if err != nil {
			return err
		}
		defer fd.Close()
		fdbzip := bzip2.NewReader(fd)
		err = untar(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)), fdbzip)
		if err != nil {
			return err
		}
		err = os.Remove(fileName)
	} else if contentType.Is("application/gzip") {
		ndir, nfile := filepath.Split(fileName)
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
		err = ungz(fileName, nNewFile)
		if err != nil {
			return err
		}
		err = os.Remove(fileName)
	} else if contentType.Is("application/zip") {
		ndir, nfile := filepath.Split(fileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		_, err = unzip(fileName, filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		err = os.Remove(fileName)
	} else if contentType.Is("application/x-tar") {
		ndir, nfile := filepath.Split(fileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		var fd *os.File
		fd, err = os.Open(fileName)
		if err != nil {
			return err
		}
		defer fd.Close()
		err = untar(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)), fd)
		if err != nil {
			return err
		}
		err = os.Remove(fileName)
	} else if contentType.Is("application/x-bzip2") {
		ndir, nfile := filepath.Split(fileName)
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
		err = unbz2(fileName, destFile)
		if err != nil {
			return err
		}
		err = os.Remove(fileName)
	} else if contentType.Is("application/x-rar-compressed") {
		ndir, nfile := filepath.Split(fileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		err = unrar(fileName, filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		err = os.Remove(fileName)
	} else if contentType.Is("application/x-7z-compressed") {
		ndir, nfile := filepath.Split(fileName)
		err = i.createDir(filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		err = un7z(fileName, filepath.Join(ndir, fmt.Sprintf("%s.dir", nfile)))
		if err != nil {
			return err
		}
		err = os.Remove(fileName)
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
