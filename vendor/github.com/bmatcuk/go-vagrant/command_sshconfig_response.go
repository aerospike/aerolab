package vagrant

import (
	"strconv"
	"strings"
)

// The SSHConfig struct has all of the settings you'll need to connect to the
// vagrant machine via ssh. The fields and values match the fields and values
// that an ssh config file is expecting. For example, you could build a ssh
// config file like:
//   Host ...
//     HostName ...
//     Port ...
//     User ...
//     IdentityFile ...
type SSHConfig struct {
	// Any additional fields returned from the ssh-config command.
	AdditionalFields map[string]string

	// Whether or not to enable ForwardAgent - "yes" or "no"
	ForwardAgent string

	// The Host matches the vagrant machine name (ex: default)
	Host string

	// The HostName to connect to (ex: 127.0.0.1)
	HostName string

	// Whether or not to enable IdentitiesOnly - "yes" or "no"
	IdentitiesOnly string

	// Path to a private key file to use for the connection (ex: ~/.ssh/id_rsa)
	IdentityFile string

	// Level of logging to enable (ex: FATAL)
	LogLevel string

	// Whether or not to enable password authentication - "yes" or "no"
	PasswordAuthentication string

	// Port to connect to (ex: 22)
	Port int

	// Whether or not to enable StrictHostKeyChecking - "yes" or "no"
	StrictHostKeyChecking string

	// User to connect as (ex: root)
	User string

	// Path to a known hosts file (ex: /dev/null)
	UserKnownHostsFile string
}

// SSHConfigResponse has the output from vagrant ssh-config
type SSHConfigResponse struct {
	ErrorResponse

	// SSH Configs per VM. Map keys match vagrant VM names (ex: default) and
	// the values are configs.
	Configs map[string]SSHConfig
}

func newSSHConfigResponse() SSHConfigResponse {
	return SSHConfigResponse{Configs: make(map[string]SSHConfig)}
}

func (resp *SSHConfigResponse) handleOutput(target, key string, message []string) {
	// Only interested in:
	// * target: X, key: ssh-config, message: Y
	// * key: error-exit, message: X
	if target != "" && key == "ssh-config" {
		config := SSHConfig{AdditionalFields: make(map[string]string)}
		for _, line := range strings.Split(strings.Join(message, ","), "\n") {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) == 2 {
				switch fields[0] {
				case "ForwardAgent":
					config.ForwardAgent = fields[1]
				case "Host":
					config.Host = fields[1]
				case "HostName":
					config.HostName = fields[1]
				case "IdentitiesOnly":
					config.IdentitiesOnly = fields[1]
				case "IdentityFile":
					config.IdentityFile = fields[1]
				case "LogLevel":
					config.LogLevel = fields[1]
				case "PasswordAuthentication":
					config.PasswordAuthentication = fields[1]
				case "Port":
					config.Port, _ = strconv.Atoi(fields[1])
				case "StrictHostKeyChecking":
					config.StrictHostKeyChecking = fields[1]
				case "User":
					config.User = fields[1]
				case "UserKnownHostsFile":
					config.UserKnownHostsFile = fields[1]
				default:
					config.AdditionalFields[fields[0]] = fields[1]
				}
			}
		}
		resp.Configs[target] = config
	} else {
		resp.ErrorResponse.handleOutput(target, key, message)
	}
}
