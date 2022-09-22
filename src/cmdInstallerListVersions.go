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

type installerListVersionsCmd struct {
	Prefix    string  `short:"v" long:"version" description:"Version Prefix to search for" default:""`
	Community bool    `short:"c" long:"community" description:"Set this switch to list community editions"`
	Reverse   bool    `short:"r" long:"reverse" description:"Reverse-sort the results"`
	Url       bool    `short:"l" long:"show-url" description:"Show direct access url instead of version number"`
	Help      helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *installerListVersionsCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	var baseUrl string
	version := strings.TrimSuffix(c.Prefix, "*")
	community := false
	if c.Community {
		community = true
	}
	reverseSort := false
	if c.Reverse {
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
		return fmt.Errorf("could not request %s: %s", baseUrl, err)
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not get %s: %s", baseUrl, err)
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("got status code %d for %s: %s", response.StatusCode, baseUrl, response.Status)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("could not read body of %s: %s", baseUrl, err)
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
		return errors.New("given versions not found")
	}

	sort.Slice(versions, func(i, j int) bool {
		if reverseSort {
			return VersionCheck(versions[i], versions[j]) == 1
		}
		return VersionCheck(versions[i], versions[j]) == -1
	})

	for _, ver := range versions {
		if c.Url {
			fmt.Println(baseUrl + ver + "/")
		} else {
			fmt.Println(ver)
		}
	}
	return nil
}
