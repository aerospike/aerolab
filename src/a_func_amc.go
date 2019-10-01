package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
)

func aeroFindUrlAmc(version string, user string, pass string) (url string, v string, err error) {

	var baseUrl string
	if version == "latest" || version == "latestc" {
		if version[len(version)-1] != 'c' {
			baseUrl = "https://www.aerospike.com/artifacts/aerospike-amc-enterprise/"
		} else {
			baseUrl = "https://www.aerospike.com/artifacts/aerospike-amc-community/"
		}
		client := &http.Client{}
		req, err := http.NewRequest("GET", baseUrl, nil)
		req.SetBasicAuth(user, pass)
		response, err := client.Do(req)
		if err != nil {
			return "", "", err
		}

		if response.StatusCode != 200 {
			err = errors.New(fmt.Sprintf(ERR_URL_NOT_FOUND, response.StatusCode))
			return "", "", err
		}

		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return "", "", err
		}
		for _, line := range strings.Split(string(responseData), "\n") {
			if strings.Contains(line, "folder.gif") {
				rp := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+[\.]*[0-9]*`)
				if version[len(version)-1] != 'c' {
					version = rp.FindString(line)
				} else {
					version = rp.FindString(line) + "c"
				}
				break
			}
		}
	}

	if version[len(version)-1] != 'c' {
		baseUrl = "https://www.aerospike.com/artifacts/aerospike-amc-enterprise/" + version + "/"
	} else {
		baseUrl = "https://www.aerospike.com/artifacts/aerospike-amc-community/" + version[:len(version)-1] + "/"
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET", baseUrl, nil)
	req.SetBasicAuth(user, pass)
	response, err := client.Do(req)
	if err != nil {
		return
	}
	if response.StatusCode != 200 {
		err = errors.New(fmt.Sprintf(ERR_URL_NOT_FOUND, response.StatusCode))
		return
	}
	url = baseUrl
	v = version
	return
}
