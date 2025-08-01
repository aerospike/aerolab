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

type Versions []Version

type Version struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

func GetVersions(timeout time.Duration, product Product) (Versions, error) {
	req, err := http.NewRequest("GET", product.Link, nil)
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
	versions := make(Versions, 0)
	for _, item := range data {
		if item.name == "Parent Directory" {
			continue
		}
		if len(item.name) == 0 {
			continue
		}
		if item.name[0] < '0' || item.name[0] > '9' {
			continue
		}
		if item.link == "" {
			continue
		}
		link, err := url.JoinPath(product.Link, item.link)
		if err != nil {
			return nil, err
		}
		versions = append(versions, Version{
			Name: strings.TrimSuffix(item.name, "/"),
			Link: link,
		})
	}
	sort.Slice(versions, func(i, j int) bool {
		return vcompare(versions[j].Name, versions[i].Name)
	})
	return versions, nil
}

func vcompare(a, b string) bool {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	ax := strings.Split(a, "-")
	bx := strings.Split(b, "-")
	av := strings.Split(ax[0], ".")
	bv := strings.Split(bx[0], ".")
	atail := ""
	btail := ""
	if len(ax) > 1 {
		atail = ax[1]
	}
	if len(bx) > 1 {
		btail = bx[1]
	}
	for i := 0; i < len(av) && i < len(bv); i++ {
		ai, _ := strconv.Atoi(av[i])
		bi, _ := strconv.Atoi(bv[i])
		if ai != bi {
			return ai < bi
		}
	}
	if len(av) != len(bv) {
		return len(av) < len(bv)
	}
	return atail < btail
}

// default sort order means [0] is latest, use this to have [0] as oldest
func (v Versions) SortOldestFirst() Versions {
	versions := make(Versions, len(v))
	copy(versions, v)
	sort.Slice(versions, func(i, j int) bool {
		return vcompare(versions[i].Name, versions[j].Name)
	})
	return versions
}

func (v Versions) WithName(name string) Versions {
	versions := make(Versions, 0)
	for _, version := range v {
		if version.Name == name {
			versions = append(versions, version)
		}
	}
	return versions
}

func (v Versions) WithNamePrefix(prefix string) Versions {
	versions := make(Versions, 0)
	for _, version := range v {
		if strings.HasPrefix(version.Name, prefix) {
			versions = append(versions, version)
		}
	}
	return versions
}

func (v Versions) WithNameSuffix(suffix string) Versions {
	versions := make(Versions, 0)
	for _, version := range v {
		if strings.HasSuffix(version.Name, suffix) {
			versions = append(versions, version)
		}
	}
	return versions
}

func (v Versions) WithNameContains(contains string) Versions {
	versions := make(Versions, 0)
	for _, version := range v {
		if strings.Contains(version.Name, contains) {
			versions = append(versions, version)
		}
	}
	return versions
}

func (v Versions) WithNamePattern(pattern string) Versions {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	versions := make(Versions, 0)
	for _, version := range v {
		if re.MatchString(version.Name) {
			versions = append(versions, version)
		}
	}
	return versions
}

func (v Versions) Latest() *Version {
	if len(v) == 0 {
		return nil
	}
	return &v[0]
}
