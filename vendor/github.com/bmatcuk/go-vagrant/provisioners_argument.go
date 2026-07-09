package vagrant

import (
	"strings"
)

// ProvisionersArgument adds the Provisioners argument to a command.
type ProvisionersArgument struct {
	// Enabled provisioners by type or name (default: blank which means they're
	// all enable or disabled depending on the Provisioning flag)
	Provisioners []string
}

func (parg *ProvisionersArgument) buildArguments() []string {
	if parg.Provisioners != nil && len(parg.Provisioners) > 0 {
		return []string{"--provision-with", strings.Join(parg.Provisioners, ",")}
	}
	return []string{}
}
