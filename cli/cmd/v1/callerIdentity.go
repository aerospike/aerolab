package cmd

import (
	"os"
	"strings"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/utils/callerip"
)

// EnvFirewallAutolock disables the automatic creation, re-locking and
// attachment of the caller's firewall when set to a false-y value.
const EnvFirewallAutolock = "AEROLAB_FIREWALL_AUTOLOCK"

// callerIdentity describes the current user to the backend, so AeroLab-managed
// firewalls can be named per user and locked to that user's own source
// address. Returning nil leaves the backend with no identity, which disables
// caller-IP firewall handling altogether.
func (s *System) callerIdentity() *backends.Identity {
	owner := GetCurrentOwnerUser()
	if owner == "" {
		s.Logger.Warn("Could not determine the current username; per-user firewalls are disabled")
		return nil
	}
	if cidr := s.Opts.Config.Backend.FirewallCidr; cidr != "" {
		if err := callerip.SetOverride(cidr); err != nil {
			s.Logger.Warn("Ignoring the configured firewall CIDR: %s", err)
		}
	}
	return &backends.Identity{
		Owner:       owner,
		CallerCidrs: callerip.Resolve,
		Autolock:    s.firewallAutolockEnabled(),
	}
}

// firewallAutolockEnabled reports whether firewalls may be reconciled and
// attached as a side effect of commands which touch instances.
func (s *System) firewallAutolockEnabled() bool {
	if s.Opts.Config.Backend.NoFirewallAutolock {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvFirewallAutolock))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}
