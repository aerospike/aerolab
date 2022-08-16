package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

func (c *config) F_listVersions() (ret int64, err error) {
	var baseUrl string
	version := strings.TrimSuffix(c.ListVersions.Prefix, "*")
	community := false
	if c.ListVersions.Community > 0 {
		community = true
	}
	reverseSort := false
	if c.ListVersions.Reverse > 0 {
		reverseSort = true
	}
	if !community {
		baseUrl = "https://artifacts.aerospike.com/aerospike-server-enterprise/"
	} else {
		baseUrl = "https://artifacts.aerospike.com/aerospike-server-community/"
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET", baseUrl, nil)
	if err != nil {
		return 5, fmt.Errorf("Could not request %s: %s", baseUrl, err)
	}
	response, err := client.Do(req)
	if err != nil {
		return 5, fmt.Errorf("Could not get %s: %s", baseUrl, err)
	}

	if response.StatusCode != 200 {
		return 5, fmt.Errorf("Got status code %d for %s: %s", response.StatusCode, baseUrl, response.Status)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return 5, fmt.Errorf("Could not read body of %s: %s", baseUrl, err)
	}
	versions := []string{}
	for _, line := range strings.Split(string(responseData), "\n") {
		if strings.Contains(line, "folder.gif") {
			rp := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+[\.]*[0-9]*[^/]*`)
			nver := rp.FindString(line)
			if (version == "" || strings.HasPrefix(nver, version)) && nver != "" {
				versions = append(versions, nver)
			}
		}
	}

	if len(versions) == 0 {
		return 5, errors.New("Given versions not found")
	}

	sort.Slice(versions, func(i, j int) bool {
		if reverseSort {
			return VersionCheck(versions[i], versions[j]) == 1
		}
		return VersionCheck(versions[i], versions[j]) == -1
	})

	for _, ver := range versions {
		if c.ListVersions.Url > 0 {
			fmt.Println(baseUrl + ver + "/")
		} else {
			fmt.Println(ver)
		}
	}
	return 0, nil
}
