package aerospike

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Files []File

type File struct {
	Name         string    `json:"name"`
	DownloadLink string    `json:"download_link"`
	DateUpdated  time.Time `json:"date_updated"`
	SizeBytes    int64     `json:"size_bytes"`
}

func GetFiles(timeout time.Duration, version Version) (Files, error) {
	req, err := http.NewRequest("GET", version.Link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	client := &http.Client{
		Timeout: timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Join(err, errors.New(string(body)))
	}
	data := getHtmlData(resp.Body)
	files := make(Files, 0)
	for _, item := range data {
		if item.name == "Parent Directory" {
			continue
		}
		if item.link == "" {
			continue
		}
		link, err := url.JoinPath(version.Link, item.link)
		if err != nil {
			return nil, err
		}
		date, err := time.Parse("2006-01-02 15:04", item.date)
		if err != nil {
			return nil, err
		}
		multiplier := 1
		if strings.HasSuffix(item.size, "M") {
			multiplier = 1024 * 1024
		} else if strings.HasSuffix(item.size, "K") {
			multiplier = 1024
		} else if strings.HasSuffix(item.size, "G") {
			multiplier = 1024 * 1024 * 1024
		}
		size := strings.TrimRight(item.size, "MKG")
		sizeBytes, err := strconv.ParseFloat(size, 64)
		if err != nil {
			return nil, err
		}
		files = append(files, File{
			Name:         item.name,
			DownloadLink: link,
			DateUpdated:  date,
			SizeBytes:    int64(sizeBytes * float64(multiplier)),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].DateUpdated.After(files[j].DateUpdated)
	})
	return files, nil
}

func (f Files) WithName(name string) Files {
	files := make(Files, 0)
	for _, file := range f {
		if file.Name == name {
			files = append(files, file)
		}
	}
	return files
}

func (f Files) WithNamePrefix(prefix string) Files {
	files := make(Files, 0)
	for _, file := range f {
		if strings.HasPrefix(file.Name, prefix) {
			files = append(files, file)
		}
	}
	return files
}

func (f Files) WithNameSuffix(suffix string) Files {
	files := make(Files, 0)
	for _, file := range f {
		if strings.HasSuffix(file.Name, suffix) {
			files = append(files, file)
		}
	}
	return files
}

func (f Files) WithNameContains(contains string) Files {
	files := make(Files, 0)
	for _, file := range f {
		if strings.Contains(file.Name, contains) {
			files = append(files, file)
		}
	}
	return files
}

func (f Files) WithNamePattern(pattern string) Files {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	files := make(Files, 0)
	for _, file := range f {
		if re.MatchString(file.Name) {
			files = append(files, file)
		}
	}
	return files
}

func (f File) Download(timeout time.Duration) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", f.DownloadLink, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

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
		return nil, errors.Join(errors.New(resp.Status), errors.New(string(body)))
	}
	return resp.Body, nil
}
