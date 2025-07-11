package compilers

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type GoVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// ex go1.23.4
func LatestGoVersion() (string, error) {
	resp, err := http.Get("https://go.dev/dl/?mode=json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var versions []GoVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", err
	}

	for _, v := range versions {
		if v.Stable {
			return v.Version, nil
		}
	}
	return "", fmt.Errorf("no stable go version found")
}
