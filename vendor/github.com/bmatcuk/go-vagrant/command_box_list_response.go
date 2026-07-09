package vagrant

import (
	"errors"
	"strings"
)

// Box defines a box from the box list command
type Box struct {
	// The box name
	Name string

	// The box provider
	Provider string

	// The box version
	Version string
}

// BoxListResponse is the output from the vagrant box list command
type BoxListResponse struct {
	ErrorResponse
	boxesIndex int

	// Boxes is a list of vagrant boxes.
	Boxes []*Box
}

func newBoxListResponse() BoxListResponse {
	return BoxListResponse{
		Boxes:      make([]*Box, 0),
		boxesIndex: -1,
	}
}

func (resp *BoxListResponse) handleOutput(target, key string, message []string) {
	// Only interested in:
	// * key: box-name, message: X
	// * key: box-provider, message: X
	// * key: error-exit, message: X
	if key == "box-name" {
		// since this is always the first key in a box listing, we use it to
		// distinguish boxes
		resp.Boxes = append(resp.Boxes, &Box{Name: strings.Join(message, "")})
		resp.boxesIndex += 1
	} else if key == "box-version" {
		if resp.boxesIndex < 0 {
			resp.Error = errors.New("assertion broken: no box-name key for box")
			return
		}
		resp.Boxes[resp.boxesIndex].Version = strings.Join(message, "")
	} else if key == "box-provider" {
		if resp.boxesIndex < 0 {
			resp.Error = errors.New("assertion broken: no box-name key for box")
			return
		}
		resp.Boxes[resp.boxesIndex].Provider = strings.Join(message, "")
	} else {
		resp.ErrorResponse.handleOutput(target, key, message)
	}
}
