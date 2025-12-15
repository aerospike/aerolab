package nodeexporter

import (
	"bytes"
	"embed"
	"errors"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/github"
	"github.com/aerospike/aerolab/pkg/utils/installers"
)

//go:embed scripts
var scripts embed.FS

// GetLinuxInstallScript returns the node_exporter installation script.
//
// Parameters:
//   - version: Specific version to install (e.g., "1.5.0"), or nil for latest
//   - prerelease: true to include prereleases, false for stable only, nil for all
//   - enable: Enable systemd service auto-start
//   - start: Start the service immediately after installation
//
// Returns:
//   - []byte: Installation script
//   - error: nil on success, or an error if script generation fails
//
// Usage:
//
//	script, err := nodeexporter.GetLinuxInstallScript(nil, nil, true, true)
//	if err != nil {
//	    log.Fatal(err)
//	}
func GetLinuxInstallScript(version *string, prerelease *bool, enable bool, start bool) ([]byte, error) {
	if version != nil {
		newv := "v" + strings.TrimPrefix(*version, "v")
		version = &newv
	}
	releases, err := github.GetReleases(30*time.Second, "prometheus", "node_exporter")
	if err != nil {
		return nil, err
	}
	if prerelease != nil {
		releases = releases.WithPrerelease(*prerelease)
	}
	if len(releases) == 0 {
		return nil, errors.New("no release found (1)")
	}
	if version != nil {
		if strings.HasSuffix(*version, "*") {
			releases = releases.WithTagPrefix(strings.TrimSuffix(*version, "*"))
		} else {
			releases = github.Releases{*releases.WithTag(*version)}
		}
	}
	if len(releases) == 0 {
		return nil, errors.New("no release found (2)")
	}
	release := releases.Latest()
	if release == nil {
		return nil, errors.New("no release found (3)")
	}
	downloadURLARM64 := release.Assets.WithNamePrefix("node_exporter-").WithNameSuffix(".linux-arm64.tar.gz").First()
	downloadURLAMD64 := release.Assets.WithNamePrefix("node_exporter-").WithNameSuffix(".linux-amd64.tar.gz").First()
	if downloadURLARM64 == nil || downloadURLAMD64 == nil {
		return nil, errors.New("no download URL found")
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
		"Version":            strings.TrimPrefix(release.TagName, "v"),
		"DownloadURLARM64":   downloadURLARM64.BrowserDownloadURL,
		"DownloadURLAMD64":   downloadURLAMD64.BrowserDownloadURL,
		"EnableNodeExporter": strconv.FormatBool(enable),
		"StartNodeExporter":  strconv.FormatBool(start),
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
