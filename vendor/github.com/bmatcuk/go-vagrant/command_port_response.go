package vagrant

import (
	"strconv"
)

// ForwardedPort defines the host port that maps to a guest port.
type ForwardedPort struct {
	// Port on the guest OS
	Guest int

	// Port on the host which forwards to the guest
	Host int
}

// PortResponse is the output from the vagrant port command.
type PortResponse struct {
	ErrorResponse

	// ForwardedPorts is a list of ports forwarded from the host OS to the guest
	// OS for the requested vagrant machine.
	ForwardedPorts []ForwardedPort
}

func newPortResponse() PortResponse {
	return PortResponse{ForwardedPorts: []ForwardedPort{}}
}

func (resp *PortResponse) handleOutput(target, key string, message []string) {
	// Only interested in:
	// * target: X, key: forwarded_port, message: [Y, Z]
	if target != "" && key == "forwarded_port" && len(message) == 2 {
		var guest, host int
		var err error
		if guest, err = strconv.Atoi(message[0]); err != nil {
			return
		}
		if host, err = strconv.Atoi(message[1]); err != nil {
			return
		}

		resp.ForwardedPorts = append(resp.ForwardedPorts, ForwardedPort{Guest: guest, Host: host})
	} else {
		resp.ErrorResponse.handleOutput(target, key, message)
	}
}
