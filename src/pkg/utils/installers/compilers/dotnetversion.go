package compilers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type DotnetReleaseIndex struct {
	ReleasesIndex []struct {
		ChannelVersion string `json:"channel-version"`
		SupportPhase   string `json:"support-phase"`
	} `json:"releases-index"`
}

// ex 9.0
func GetLatestDotnetChannelVersion() (string, error) {
	resp, err := http.Get("https://dotnetcli.blob.core.windows.net/dotnet/release-metadata/releases-index.json")
	if err != nil {
		return "", fmt.Errorf("failed to fetch JSON: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var index DotnetReleaseIndex
	if err := json.Unmarshal(body, &index); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	var activeChannels []string
	for _, r := range index.ReleasesIndex {
		if r.SupportPhase == "active" {
			activeChannels = append(activeChannels, r.ChannelVersion)
		}
	}

	if len(activeChannels) == 0 {
		return "", fmt.Errorf("no active channels found")
	}

	return activeChannels[0], nil
}
