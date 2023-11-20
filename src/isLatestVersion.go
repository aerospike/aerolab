package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type GitHubRelease struct {
	LatestVersion  string            `json:"tag_name"`
	CurrentVersion string            `json:"current_version"`
	Assets         []*ghReleaseAsset `json:"assets"`
	Prerelease     bool              `json:"prerelease"`
	CommitHash     string            `json:"target_commitish"`
}

type ghReleaseAsset struct {
	DownloadUrl string `json:"browser_download_url"`
	FileName    string `json:"name"`         // aerolab-{windows|macos|linux}-{arm64|amd64}-{version}.zip
	ContentType string `json:"content_type"` // "application/zip"
}

func (a *aerolab) isLatestVersion() {
	rootDir, err := a.aerolabRootDir()
	if err != nil {
		return
	}
	versionFile := path.Join(rootDir, "version-check.json")
	v := &GitHubRelease{}
	out, err := os.ReadFile(versionFile)
	if err != nil {
		err = a.isLatestVersionQuery(v, versionFile)
		if err != nil {
			return
		}
	}
	err = json.Unmarshal(out, v)
	if err != nil || v.LatestVersion == "" || v.CurrentVersion == "" {
		err = a.isLatestVersionQuery(v, versionFile)
		if err != nil {
			return
		}
	}
	if v.CurrentVersion != vBranch {
		err = a.isLatestVersionQuery(v, versionFile)
		if err != nil {
			return
		}
	}
	if VersionCheck(v.CurrentVersion, v.LatestVersion) > 0 {
		log.Println("AEROLAB VERSION: A new version of AeroLab is available, download link: https://github.com/aerospike/aerolab/releases")
	}
	if VersionCheck(v.CurrentVersion, v.LatestVersion) == 0 && strings.Contains(vEdition, "prerelease") {
		log.Println("AEROLAB VERSION: Current version is a dev build, a stable release is available, download link: https://github.com/aerospike/aerolab/releases")
	}
}

func (a *aerolab) isLatestVersionQuery(v *GitHubRelease, versionFile string) error {
	client := &http.Client{}
	client.Timeout = 5 * time.Second
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", "https://api.github.com/repos/aerospike/aerolab/releases/latest", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		err = fmt.Errorf("GET 'https://api.github.com/repos/aerospike/aerolab/releases/latest': exit code (%d), message: %s", response.StatusCode, string(body))
		return err
	}
	err = json.NewDecoder(response.Body).Decode(v)
	if err != nil {
		return err
	}
	v.CurrentVersion = vBranch

	if versionFile != "" {
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		os.WriteFile(versionFile, data, 0600)
	}
	return nil
}

func (a *aerolab) isLatestVersionQueryPrerelease() (*GitHubRelease, error) {
	client := &http.Client{}
	client.Timeout = 5 * time.Second
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", "https://api.github.com/repos/aerospike/aerolab/releases", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		err = fmt.Errorf("GET 'https://api.github.com/repos/aerospike/aerolab/releases': exit code (%d), message: %s", response.StatusCode, string(body))
		return nil, err
	}
	vPrerelease := []*GitHubRelease{}
	err = json.NewDecoder(response.Body).Decode(&vPrerelease)
	if err != nil {
		return nil, err
	}
	for _, pre := range vPrerelease {
		if !pre.Prerelease {
			continue
		}
		pre.CurrentVersion = vBranch
		return pre, nil
	}
	return nil, errors.New("NOT FOUND")
}
