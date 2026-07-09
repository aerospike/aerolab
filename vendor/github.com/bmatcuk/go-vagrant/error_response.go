package vagrant

import (
	"errors"
	"strings"
)

// ErrorResponse adds the Error field to command output.
type ErrorResponse struct {
	// If set, there was an error while running the vagrant command.
	Error error
}

func newErrorResponse() ErrorResponse {
	return ErrorResponse{}
}

func (resp *ErrorResponse) handleOutput(target, key string, message []string) {
	// Only interested in:
	// * key: error-exit, message: X
	if key == "error-exit" {
		resp.Error = errors.New(strings.Join(message, ", "))
	}
}
