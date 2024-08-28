package ingest

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/rglonek/sbs"

	"github.com/nwaples/rardecode"
)

func unbz2(sourceFile string, destFile string) error {
	fd, err := os.Open(sourceFile)
	if err != nil {
		return err
	}
	defer fd.Close()
	decompressed := bzip2.NewReader(fd)
	fdDest, err := os.OpenFile(destFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer fdDest.Close()
	_, err = io.Copy(fdDest, decompressed)
	if err != nil {
		return err
	}
	return nil
}

func isTarGz(file string) bool {
	fd, err := os.Open(file)
	if err != nil {
		return false
	}
	defer fd.Close()
	fdgzip, err := gzip.NewReader(fd)
	if err != nil {
		return false
	}
	defer fdgzip.Close()
	buffer := make([]byte, 4096)
	rdCnt, err := fdgzip.Read(buffer)
	if err != nil {
		return false
	}
	contentType := mimetype.Detect(buffer[0:rdCnt])
	return contentType.Is("application/x-tar")
}

func isTarBz(file string) bool {
	fd, err := os.Open(file)
	if err != nil {
		return false
	}
	defer fd.Close()
	fdgzip := bzip2.NewReader(fd)
	buffer := make([]byte, 4096)
	rdCnt, err := fdgzip.Read(buffer)
	if err != nil {
		return false
	}
	contentType := mimetype.Detect(buffer[0:rdCnt])
	return contentType.Is("application/x-tar")
}

func ungz(sourceFile string, destFile string) error {
	fd, err := os.Open(sourceFile)
	if err != nil {
		return fmt.Errorf("open source file: %s", err)
	}
	defer fd.Close()
	decompressed, err := gzip.NewReader(fd)
	if err != nil {
		return fmt.Errorf("open gzip reader: %s", err)
	}
	fdDest, err := os.OpenFile(destFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open destination file: %s", err)
	}
	defer fdDest.Close()
	_, err = io.Copy(fdDest, decompressed)
	if err != nil {
		return fmt.Errorf("unpack: %s", err)
	}
	return nil
}

func unzip(src string, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}

func un7z(src string, dst string) error {
	if _, err := os.Stat(dst); err != nil {
		return err
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}
	out, err := exec.Command("7z", "x", "-aou", "-y", fmt.Sprintf("-o%s", dst), src).CombinedOutput()
	if err != nil {
		return fmt.Errorf("err:%s .. out:%s", err, sbs.ByteSliceToString(out))
	}
	return nil
}

func untar(dst string, r io.Reader) error {

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			prevDir, _ := filepath.Split(target)
			if _, err := os.Stat(prevDir); os.IsNotExist(err) {
				if err := os.MkdirAll(prevDir, 0755); err != nil {
					return err
				}
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}

func unrar(src string, dst string) error {

	tr, err := rardecode.OpenReader(src, "")
	if err != nil {
		return err
	}
	defer tr.Close()

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		if header.IsDir {

			// if its a dir and it doesn't exist create it
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

			// if it's a file create it
		} else {
			targetDir, _ := path.Split(target)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(0644))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}
