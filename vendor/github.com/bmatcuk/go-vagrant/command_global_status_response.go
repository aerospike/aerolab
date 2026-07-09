package vagrant

import (
	"strings"
)

// GlobalStatus has status information about a single Vagrant VM
type GlobalStatus struct {
	// Id of the vagrant vm
	Id string

	// Name of the vagrant vm
	Name string

	// Provider of the vagrant vm
	Provider string

	// The State of the vagrant vm
	State string

	// Directory where the vagrant vm was created
	Directory string

	// AdditionalInfo has data which may be provided by vagrant in the future as
	// a map of string keys and values.
	AdditionalInfo map[string]string
}

func (gs *GlobalStatus) moveInfoToProperties() {
	if id, ok := gs.AdditionalInfo["id"]; ok {
		delete(gs.AdditionalInfo, "id")
		gs.Id = id
	}
	if name, ok := gs.AdditionalInfo["name"]; ok {
		delete(gs.AdditionalInfo, "name")
		gs.Name = name
	}
	if provider, ok := gs.AdditionalInfo["provider"]; ok {
		delete(gs.AdditionalInfo, "provider")
		gs.Provider = provider
	}
	if state, ok := gs.AdditionalInfo["state"]; ok {
		delete(gs.AdditionalInfo, "state")
		gs.State = state
	}
	if directory, ok := gs.AdditionalInfo["directory"]; ok {
		delete(gs.AdditionalInfo, "directory")
		gs.Directory = directory
	}
}

// GlobalStatusResponse has the output from vagrant global-status
type GlobalStatusResponse struct {
	ErrorResponse

	// Status per Vagrant VM. Keys are vagrant vm ID's (ex: 1a2b3c4d) and values
	// are GlobalStatus objects.
	Status map[string]*GlobalStatus

	hasKeys       bool
	keys          []string
	currentKey    int
	currentStatus *GlobalStatus
}

func newGlobalStatusResponse() GlobalStatusResponse {
	return GlobalStatusResponse{
		Status: make(map[string]*GlobalStatus),
	}
}

func (resp *GlobalStatusResponse) handleOutput(target, key string, message []string) {
	// Only interested in the following output:
	// * target: _, key: ui, message: [info, X]
	if key == "ui" && len(message) > 1 && message[0] == "info" {
		if len(message[1]) > 0 && (strings.Contains(message[1], "\n") || strings.Trim(message[1], "-") == "") {
			// There's a line of all -'s that separates the keys from the statuses,
			// and the last line contains a paragraph of text with newlines.
			// Ignore both.
			return
		}

		if resp.hasKeys {
			if message[1] == "" {
				// we're done loading the status for a machine
				resp.currentStatus.moveInfoToProperties()
				resp.Status[resp.currentStatus.Id] = resp.currentStatus
				resp.currentStatus = nil
				resp.currentKey = 0
				return
			}
			if resp.currentKey >= len(resp.keys) {
				// more data than there are keys?
				return
			}

			if resp.currentStatus == nil {
				resp.currentStatus = &GlobalStatus{AdditionalInfo: make(map[string]string)}
			}
			resp.currentStatus.AdditionalInfo[resp.keys[resp.currentKey]] = strings.TrimSpace(message[1])
			resp.currentKey++
		} else {
			if message[1] == "" {
				// we're done loading all the keys
				resp.hasKeys = true
				return
			}
			resp.keys = append(resp.keys, strings.TrimSpace(message[1]))
		}
	} else {
		resp.ErrorResponse.handleOutput(target, key, message)
	}
}
