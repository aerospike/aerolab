package aerolab

import (
	"bytes"
	"embed"
	"errors"
	"strings"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/github"
	"github.com/aerospike/aerolab/pkg/utils/installers"
)

//go:embed scripts
var scripts embed.FS

func GetRelease(version string) (*github.Release, error) {
	releases, err := github.GetReleases(30*time.Second, "aerospike", "aerolab")
	if err != nil {
		return nil, err
	}
	releases = releases.WithTagPrefix(version)
	if len(releases) == 0 {
		return nil, errors.New("no release found")
	}
	return releases.Latest(), nil
}

func GetLatestVersion(stable bool) (*github.Release, error) {
	releases, err := github.GetReleases(30*time.Second, "aerospike", "aerolab")
	if err != nil {
		return nil, err
	}
	if stable {
		releases = releases.WithPrerelease(false)
	}
	if len(releases) == 0 {
		return nil, errors.New("no release found (1)")
	}
	release := releases.Latest()
	if release == nil {
		return nil, errors.New("no release found (2)")
	}
	return release, nil
}

// specify a specific version or get latest
// if version ends with '*', it will match with prefix of version, and if multiple found, it will use the latest that matches that prefix
// prerelease will only look through prereleases, otherwise only stable releases. nil means all releases
func GetLinuxInstallScript(version *string, prerelease *bool) ([]byte, error) {
	releases, err := github.GetReleases(30*time.Second, "aerospike", "aerolab")
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
	downloadURLARM64 := release.Assets.WithNamePrefix("aerolab-linux-arm64-").WithNameSuffix(".zip").First()
	downloadURLAMD64 := release.Assets.WithNamePrefix("aerolab-linux-amd64-").WithNameSuffix(".zip").First()
	if downloadURLARM64 == nil || downloadURLAMD64 == nil {
		return nil, errors.New("no download URL found")
	}
	s := installers.Software{
		Debug: true,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "unzip", Package: "unzip"},
			},
		},
	}
	installScript, err := processTemplate("scripts/install.sh.tpl", map[string]any{
		"DownloadURLARM64": downloadURLARM64.BrowserDownloadURL,
		"DownloadURLAMD64": downloadURLAMD64.BrowserDownloadURL,
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
