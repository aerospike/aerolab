package vagrant

import (
	"strings"
)

// StatusResponse has the output from vagrant status
type StatusResponse struct {
	ErrorResponse

	// Status per Vagrant VM. Keys are Vagrant VM names (ex: default) and values
	// are the status of the VM.
	Status map[string]string
}

func newStatusResponse() StatusResponse {
	return StatusResponse{Status: make(map[string]string)}
}

func (resp *StatusResponse) handleOutput(target, key string, message []string) {
	// Only interested in:
	// * target: X, key: state, message: Y
	// * key: error-exit, message: X
	if target != "" && key == "state" {
		resp.Status[target] = strings.Join(message, ",")
	} else {
		resp.ErrorResponse.handleOutput(target, key, message)
	}
}
