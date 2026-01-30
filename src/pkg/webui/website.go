package webui

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"os"
	"path/filepath"
)

//go:generate bash -c "cd ../../../web && ./build.sh"

//go:embed www.tgz
var Website []byte

// Install will install the website in dst/www
func InstallWebsite(dst string, website []byte) error {
	br := bytes.NewReader(website)
	r, err := gzip.NewReader(br)
	if err != nil {
		return err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case header == nil:
			continue
		}
		target := filepath.Join(dst, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}
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
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
}
