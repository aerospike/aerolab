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

// GetLinuxInstallScript returns a shell script that installs aerolab on Linux.
//
// Parameters:
//   - currentVersion: the version string from GetAerolabVersion() (e.g., "v8.0.0-abc1234" or "v8.0.0-abc1234-unofficial").
//     If provided, this exact version will be installed. The "-unofficial" suffix is stripped when matching.
//     If nil or empty, the latest release matching the version/prerelease filters will be used.
//   - version: if currentVersion is nil, specify a version to install. If it ends with '*', it will match
//     with prefix of version, and if multiple found, it will use the latest that matches that prefix.
//   - prerelease: if currentVersion is nil, filter by prerelease status. nil means all releases.
func GetLinuxInstallScript(currentVersion string, version *string, prerelease *bool) ([]byte, error) {
	releases, err := github.GetReleases(30*time.Second, "aerospike", "aerolab")
	if err != nil {
		return nil, err
	}

	var release *github.Release

	// If currentVersion is specified, find the exact matching release
	if currentVersion != "" {
		// Strip -unofficial suffix if present
		targetVersion := strings.TrimSuffix(currentVersion, "-unofficial")

		// Try to find exact match first
		release = releases.WithTag(targetVersion)
		if release == nil {
			// Try matching by tag prefix (handles v8.0.0-abc1234 format)
			matchedReleases := releases.WithTagPrefix(targetVersion)
			if len(matchedReleases) > 0 {
				release = matchedReleases.Latest()
			}
		}
		if release == nil {
			return nil, errors.New("no release found matching current version: " + targetVersion)
		}
	} else {
		// Original behavior: filter by prerelease and version
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
		release = releases.Latest()
		if release == nil {
			return nil, errors.New("no release found (3)")
		}
	}

	downloadURLARM64 := release.Assets.WithNamePrefix("aerolab-linux-arm64-").WithNameSuffix(".zip").First()
	downloadURLAMD64 := release.Assets.WithNamePrefix("aerolab-linux-amd64-").WithNameSuffix(".zip").First()
	if downloadURLARM64 == nil || downloadURLAMD64 == nil {
		return nil, errors.New("no download URL found for release: " + release.TagName)
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
