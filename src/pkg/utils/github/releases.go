package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Releases []Release

type Assets []Asset

type Release struct {
	URL             string    `json:"url"`
	AssetsURL       string    `json:"assets_url"`
	ID              int       `json:"id"`
	TagName         string    `json:"tag_name"`
	TargetCommitish string    `json:"target_commitish"`
	Name            string    `json:"name"`
	Prerelease      bool      `json:"prerelease"`
	CreatedAt       time.Time `json:"created_at"`
	PublishedAt     time.Time `json:"published_at"`
	Assets          Assets    `json:"assets"`
	TarballURL      string    `json:"tarball_url"`
	ZipballURL      string    `json:"zipball_url"`
	Body            string    `json:"body"`
}

type Asset struct {
	URL                string    `json:"url"`
	ID                 int       `json:"id"`
	Name               string    `json:"name"`
	Label              string    `json:"label"`
	ContentType        string    `json:"content_type"`
	Size               int       `json:"size"`
	State              string    `json:"state"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	BrowserDownloadURL string    `json:"browser_download_url"`
}

func GetLatestRelease(timeout time.Duration, owner, repo string) (Release, error) {
	url := "https://api.github.com/repos/" + owner + "/" + repo + "/releases/latest"
	var release Release
	if err := get(url, timeout, &release); err != nil {
		return Release{}, err
	}
	return release, nil
}

func GetReleases(timeout time.Duration, owner, repo string) (Releases, error) {
	url := "https://api.github.com/repos/" + owner + "/" + repo + "/releases"
	var releases Releases
	if err := get(url, timeout, &releases); err != nil {
		return nil, err
	}
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].PublishedAt.After(releases[j].PublishedAt)
	})
	return releases, nil
}

func get(url string, timeout time.Duration, out interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: timeout,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Join(fmt.Errorf("failed to get releases: %s", resp.Status), errors.New(string(body)))
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(out); err != nil {
		return err
	}

	return nil
}

func (r *Releases) WithTag(tag string) *Release {
	for _, release := range *r {
		if release.TagName == tag {
			return &release
		}
	}
	return nil
}

func (r *Releases) WithTagPrefix(prefix string) Releases {
	releases := make(Releases, 0)
	for _, release := range *r {
		if strings.HasPrefix(release.TagName, prefix) {
			releases = append(releases, release)
		}
	}
	return releases
}

// either give only prereleases or only non-prereleases
func (r *Releases) WithPrerelease(prerelease bool) Releases {
	releases := make(Releases, 0)
	for _, release := range *r {
		if release.Prerelease == prerelease {
			releases = append(releases, release)
		}
	}
	return releases
}

func (r *Releases) Latest() *Release {
	var answer *Release
	for _, release := range *r {
		if answer == nil || release.PublishedAt.After(answer.PublishedAt) {
			answer = &release
		}
	}
	return answer
}

func (a *Assets) WithName(name string) *Asset {
	for _, asset := range *a {
		if asset.Name == name {
			return &asset
		}
	}
	return nil
}

func (a *Assets) WithNamePrefix(prefix string) Assets {
	assets := make(Assets, 0)
	for _, asset := range *a {
		if strings.HasPrefix(asset.Name, prefix) {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (a *Assets) WithNameSuffix(suffix string) Assets {
	assets := make(Assets, 0)
	for _, asset := range *a {
		if strings.HasSuffix(asset.Name, suffix) {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (a *Assets) WithNameContains(contains string) Assets {
	assets := make(Assets, 0)
	for _, asset := range *a {
		if strings.Contains(asset.Name, contains) {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (a *Assets) WithNamePattern(regex string) Assets {
	re := regexp.MustCompile(regex)
	assets := make(Assets, 0)
	for _, asset := range *a {
		if re.MatchString(asset.Name) {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (a *Assets) WithContentType(ctype string) Assets {
	assets := make(Assets, 0)
	for _, asset := range *a {
		if asset.ContentType == ctype {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (a *Asset) Download(timeout time.Duration) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", a.BrowserDownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", a.ContentType)
	client := &http.Client{
		Timeout: timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, errors.Join(fmt.Errorf("failed to download asset: %s", resp.Status), errors.New(string(body)))
	}
	return resp.Body, nil
}
