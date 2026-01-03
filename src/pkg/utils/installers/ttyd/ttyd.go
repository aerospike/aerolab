// Package ttyd provides installation scripts for ttyd (web terminal) from GitHub releases.
package ttyd

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
	// Owner is the GitHub repository owner for ttyd
	Owner = "tsl0922"
	// Repo is the GitHub repository name for ttyd
	Repo = "ttyd"
)

// GetDownloadURL retrieves the download URL for ttyd binary for the specified architecture.
//
// Parameters:
//   - arch: Architecture string ("x86_64" or "aarch64")
//
// Returns:
//   - string: The download URL for the ttyd binary
//   - error: nil on success, or an error describing what failed
func GetDownloadURL(arch string) (string, error) {
	release, err := github.GetLatestRelease(30*time.Second, Owner, Repo)
	if err != nil {
		return "", err
	}

	// ttyd releases binaries as ttyd.x86_64 or ttyd.aarch64
	assetName := "ttyd." + arch
	asset := release.Assets.WithName(assetName)
	if asset == nil {
		return "", errors.New("no download URL found for architecture: " + arch)
	}
	return asset.BrowserDownloadURL, nil
}

// GetLinuxInstallScript generates an installation script for ttyd on Linux.
// The script downloads the appropriate binary for the detected architecture,
// installs it to the specified path, and optionally enables/starts the systemd service.
//
// Parameters:
//   - destPath: Destination path for the ttyd binary (default: /usr/local/bin/ttyd)
//   - enable: Enable the systemd service
//   - start: Start the systemd service
//
// Returns:
//   - []byte: The installation script
//   - error: nil on success, or an error describing what failed
func GetLinuxInstallScript(destPath string, enable bool, start bool) ([]byte, error) {
	if destPath == "" {
		destPath = "/usr/local/bin/ttyd"
	}

	// Get download URLs for both architectures
	downloadURLAMD64, err := GetDownloadURL("x86_64")
	if err != nil {
		return nil, err
	}
	downloadURLARM64, err := GetDownloadURL("aarch64")
	if err != nil {
		return nil, err
	}

	s := installers.Software{
		Debug: true,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
			},
		},
	}

	installScript, err := processTemplate("scripts/install.sh.tpl", map[string]any{
		"DestPath":         destPath,
		"DownloadURLARM64": downloadURLARM64,
		"DownloadURLAMD64": downloadURLAMD64,
		"EnableTtyd":       strconv.FormatBool(enable),
		"StartTtyd":        strconv.FormatBool(start),
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

