// Package filebrowser provides installation scripts for filebrowser from GitHub releases.
package filebrowser

import (
	"bytes"
	"embed"
	"errors"
	"strconv"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/github"
	"github.com/aerospike/aerolab/pkg/utils/installers"
)

//go:embed scripts
var scripts embed.FS

const (
	// Owner is the GitHub repository owner for filebrowser
	Owner = "filebrowser"
	// Repo is the GitHub repository name for filebrowser
	Repo = "filebrowser"
)

// GetDownloadURL retrieves the download URL for filebrowser tarball for the specified architecture.
//
// Parameters:
//   - arch: Architecture string ("amd64" or "arm64")
//
// Returns:
//   - string: The download URL for the filebrowser tarball
//   - error: nil on success, or an error describing what failed
func GetDownloadURL(arch string) (string, error) {
	release, err := github.GetLatestRelease(30*time.Second, Owner, Repo)
	if err != nil {
		return "", err
	}

	// filebrowser releases as linux-amd64-filebrowser.tar.gz or linux-arm64-filebrowser.tar.gz
	assetPrefix := "linux-" + arch + "-filebrowser"
	assets := release.Assets.WithNamePrefix(assetPrefix).WithNameSuffix(".tar.gz")
	if assets == nil || len(assets.List()) == 0 {
		return "", errors.New("no download URL found for architecture: " + arch)
	}
	asset := assets.First()
	if asset == nil {
		return "", errors.New("no download URL found for architecture: " + arch)
	}
	return asset.BrowserDownloadURL, nil
}

// GetLinuxInstallScript generates an installation script for filebrowser on Linux.
// The script downloads the appropriate tarball for the detected architecture,
// extracts it, installs the binary to the specified path, and optionally enables/starts the systemd service.
//
// Parameters:
//   - destPath: Destination path for the filebrowser binary (default: /usr/local/bin/filebrowser)
//   - enable: Enable the systemd service
//   - start: Start the systemd service
//
// Returns:
//   - []byte: The installation script
//   - error: nil on success, or an error describing what failed
func GetLinuxInstallScript(destPath string, enable bool, start bool) ([]byte, error) {
	if destPath == "" {
		destPath = "/usr/local/bin/filebrowser"
	}

	// Get download URLs for both architectures
	downloadURLAMD64, err := GetDownloadURL("amd64")
	if err != nil {
		return nil, err
	}
	downloadURLARM64, err := GetDownloadURL("arm64")
	if err != nil {
		return nil, err
	}

	s := installers.Software{
		Debug: true,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "tar", Package: "tar"},
			},
		},
	}

	installScript, err := processTemplate("scripts/install.sh.tpl", map[string]any{
		"DestPath":         destPath,
		"DownloadURLARM64": downloadURLARM64,
		"DownloadURLAMD64": downloadURLAMD64,
		"EnableFilebrowser": strconv.FormatBool(enable),
		"StartFilebrowser":  strconv.FormatBool(start),
	})
	if err != nil {
		return nil, err
	}

	return installers.GetInstallScript(s, installScript)
}

func processTemplate(scriptFile string, data map[string]any) ([]byte, error) {
	script, err := scripts.ReadFile(scriptFile)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("script").Parse(string(script))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

