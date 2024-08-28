package ingest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bestmethod/logger"
	"github.com/gabriel-vasile/mimetype"
)

func (i *Ingest) enum() (map[string]*EnumFile, error) {
	files := make(map[string]*EnumFile)
	err := filepath.Walk(i.config.Directories.DirtyTmp, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Detail("enum: got error on walk: %s", err)
			return err
		}
		if info.IsDir() {
			logger.Detail("enum: isDir: %s", filePath)
			return nil
		}
		file := &EnumFile{
			Size: info.Size(),
		}
		if file.Size == 0 {
			files[filePath] = file
			logger.Detail("enum: empty: %s", filePath)
			return nil
		}
		if strings.Contains(filePath, "collect_info_") && strings.HasSuffix(filePath, ".tgz") && info.Size() < 10485760 {
			file.IsCollectInfo = true
			files[filePath] = file
			logger.Detail("enum: collectinfo-early: %s", filePath)
			return nil
		}
		fd, err := os.Open(filePath)
		if err != nil {
			logger.Warn("Could not open file, skipping: %s", filePath)
			return nil
		}
		defer fd.Close()
		buffer := make([]byte, 4096)
		rdCnt, err := fd.Read(buffer)
		if err != nil && err != io.EOF {
			logger.Warn("Could not read file, skipping: %s", filePath)
			return nil
		}
		contentType := mimetype.Detect(buffer[0:rdCnt])

		// workaround for log files starting with binary 000s, start detection at 4k mark after the 000s
		emptyBuffer := make([]byte, 4096)
		if contentType.Is("application/octet-stream") {
			for bytes.Equal(buffer, emptyBuffer) {
				rdCnt, err = fd.Read(buffer)
				if err != nil && err != io.EOF {
					logger.Warn("Could not read file, skipping: %s", filePath)
					return nil
				}
				if err != nil && err == io.EOF {
					break
				}
			}
			file.StartAt, _ = fd.Seek(0, 1)
			_, err = fd.Read(buffer)
			if err != nil && err != io.EOF {
				logger.Warn("Could not read file, skipping: %s", filePath)
				return nil
			}
			contentType = mimetype.Detect(buffer[0:rdCnt])
		}

		file.mimeType = contentType
		file.ContentType = contentType.String()

		// collectinfo further check
		if contentType.Is("application/gzip") && info.Size() < i.config.CollectInfoMaxSize {
			isCollect, err := i.enumCollectDeepCheck(fd)
			if err != nil {
				logger.Warn("Could not do deep-discovery to determine if file is collectinfo, will treat as normal: %s: %s", filePath, err)
			} else if isCollect {
				file.IsCollectInfo = true
				files[filePath] = file
				logger.Detail("enum: collectinfo: %s", filePath)
				return nil
			}
		}

		// not collectinfo assign archive/text vars
		if contentType.Is("application/gzip") || contentType.Is("application/zip") || contentType.Is("application/x-tar") || contentType.Is("application/x-bzip2") || contentType.Is("application/x-rar-compressed") || contentType.Is("application/x-7z-compressed") {
			file.IsArchive = true
		} else if contentType.Is("application/json") || contentType.Is("application/x-ndjson") || contentType.Is("text/plain") || contentType.Is("text/csv") || contentType.Is("text/tab-separated-values") {
			file.IsText = true
		}
		// is gzip->tar
		if contentType.Is("application/gzip") {
			file.IsTarGz = isTarGz(filePath)
		}
		// is bzip->tar
		if contentType.Is("application/x-bzip2") {
			file.IsTarBz = isTarBz(filePath)
		}
		files[filePath] = file
		logger.Detail("enum: file:%s isArchive:%t isText:%t isTagGz:%t isTagBz:%t contentType:%s", filePath, file.IsArchive, file.IsText, file.IsTarGz, file.IsTarBz, file.ContentType)
		return nil
	})
	return files, err
}

func (i *Ingest) enumCollectDeepCheck(fd *os.File) (bool, error) {
	// if gzip:
	_, err := fd.Seek(0, 0)
	if err != nil {
		return false, err
	}
	fdzip, err := gzip.NewReader(fd)
	if err != nil {
		return false, err
	}
	defer fdzip.Close()
	fdtar := tar.NewReader(fdzip)
	hasCollectInfo := false
	hasOtherFiles := false
	for {
		// get next file
		h, err := fdtar.Next()
		if err != nil {
			if err != io.EOF {
				return false, err
			} else {
				break
			}
		}
		// skip directories
		if h.FileInfo().IsDir() {
			continue
		}
		// skip hidden files
		if _, fName := path.Split(h.Name); strings.HasPrefix(fName, ".") {
			continue
		}
		// figure out if file is collectinfo or not
		if strings.HasPrefix(h.Name, "tmp/collect_info_") {
			hasCollectInfo = true
		} else {
			hasOtherFiles = true
		}
		// if has files other than collectinfo, break, we will not process this as collectinfo
		if hasOtherFiles {
			break
		}
	}
	if hasCollectInfo && !hasOtherFiles {
		return true, nil
	}
	return false, nil
}
