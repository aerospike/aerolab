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

// Website contains the embedded website archive (www.tgz) that includes all static
// web assets for the Aerolab web interface. This is embedded at compile time.
//
//go:embed www.tgz
var Website []byte

// InstallWebsite extracts and installs the embedded website files to the specified destination directory.
// The function extracts a gzipped tar archive containing all web assets and creates the necessary
// directory structure. It handles both directories and regular files, preserving file permissions.
//
// Parameters:
//   - dst: The destination directory where the website files will be extracted
//   - website: The gzipped tar archive bytes containing the website files
//
// Returns:
//   - error: nil on success, or an error if extraction fails
//
// Usage:
//
//	err := webui.InstallWebsite("/var/www/aerolab", webui.Website)
//	if err != nil {
//	    log.Fatal("Failed to install website:", err)
//	}
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
