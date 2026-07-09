package vagrant

import (
	"strings"
)

// VersionResponse is the output from vagrant version
type VersionResponse struct {
	ErrorResponse

	// The current version
	InstalledVersion string

	// The latest version
	LatestVersion string
}

func newVersionResponse() VersionResponse {
	return VersionResponse{}
}

func (resp *VersionResponse) handleOutput(target, key string, message []string) {
	// Only interested in:
	// * key: version-installed, message: X
	// * key: version-latest, message: X
	// * key: error-exit, message: X
	if key == "version-installed" {
		resp.InstalledVersion = strings.Join(message, ",")
	} else if key == "version-latest" {
		resp.LatestVersion = strings.Join(message, ",")
	} else {
		resp.ErrorResponse.handleOutput(target, key, message)
	}
}
