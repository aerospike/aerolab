package mcp

import (
	"errors"
	"fmt"
	"strings"
)

// Profile describes which classes of aerolab commands an MCP client is
// allowed to invoke. Profiles are coarse-grained; fine-grained permission
// control should be handled by the caller (for example by restricting the
// auth token's scope).
type Profile string

const (
	// ProfileReadOnly allows only safe, read-only inventory and inspection
	// commands (e.g. list, inventory, status). Any command marked as
	// destructive is rejected.
	ProfileReadOnly Profile = "read-only"

	// ProfileStandard allows all commands but requires the caller to pass
	// confirm=true for destructive commands.
	ProfileStandard Profile = "standard"

	// ProfileAdmin allows every command without confirmation. Use only
	// for trusted local agents or tests.
	ProfileAdmin Profile = "admin"
)

// ErrConfirmationRequired is returned when a destructive command is
// invoked in the standard profile without an explicit confirm=true.
var ErrConfirmationRequired = errors.New("mcp: destructive command requires confirm=true in 'standard' profile")

// ErrProfileReadOnly is returned when a destructive or mutating command is
// invoked under the read-only profile.
var ErrProfileReadOnly = errors.New("mcp: profile is read-only; destructive commands are not allowed")

// ErrInvalidProfile is returned when an unknown profile is supplied.
var ErrInvalidProfile = errors.New("mcp: invalid profile (expected read-only|standard|admin)")

// ProfileGate decides whether a given aerolab command may be executed at
// the configured profile, and whether confirmation was supplied by the
// caller when required.
//
// Destructiveness is determined by a combination of explicit per-command
// flags (Command.Destructive, typically populated from the CLI's
// "invwebforce" tag) and a fallback heuristic based on command name and
// the path's leaf segment.
type ProfileGate struct {
	Profile Profile
}

// ParseProfile converts a string to a Profile, returning ErrInvalidProfile
// when the value is not recognized. Empty input defaults to ProfileStandard.
func ParseProfile(s string) (Profile, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "standard":
		return ProfileStandard, nil
	case "read-only", "readonly":
		return ProfileReadOnly, nil
	case "admin":
		return ProfileAdmin, nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidProfile, s)
}

// NewGate constructs a ProfileGate. Empty profile maps to standard.
func NewGate(p Profile) *ProfileGate {
	if p == "" {
		p = ProfileStandard
	}
	return &ProfileGate{Profile: p}
}

// Check enforces the gate for a single call. When cmd is nil, the call is
// treated as read-only (describe/list tools). When cmd is destructive, the
// caller must either run under admin or pass confirm=true under standard.
func (g *ProfileGate) Check(cmd *Command, confirm bool) error {
	if g == nil {
		return nil
	}
	destructive := IsDestructive(cmd)

	switch g.Profile {
	case ProfileReadOnly:
		if destructive {
			return fmt.Errorf("%w: %s", ErrProfileReadOnly, commandLabel(cmd))
		}
		return nil
	case ProfileAdmin:
		return nil
	case ProfileStandard, "":
		if destructive && !confirm {
			return fmt.Errorf("%w: %s", ErrConfirmationRequired, commandLabel(cmd))
		}
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidProfile, g.Profile)
}

// IsDestructive reports whether a command modifies or destroys remote
// state. It returns false for nil input so callers can reuse Check for
// metadata tools.
func IsDestructive(cmd *Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Destructive {
		return true
	}
	// Heuristic fallback so commands not tagged in the CLI still behave
	// sensibly. The fallback matches common destructive verb names on the
	// leaf path segment.
	segs := splitPath(cmd.Path)
	var leaf string
	if len(segs) > 0 {
		leaf = strings.ToLower(segs[len(segs)-1])
	} else {
		leaf = strings.ToLower(cmd.Name)
	}
	switch leaf {
	case "destroy", "delete", "remove", "terminate", "wipe",
		"reboot", "reset", "restart", "kill",
		"stop", "halt", "shutdown",
		"format", "repartition", "nuke":
		return true
	}
	// Any path segment containing 'destroy' is destructive (e.g.
	// cluster/add/destroyed, agi/destroy-plan, ...).
	for _, s := range segs {
		if strings.Contains(strings.ToLower(s), "destroy") {
			return true
		}
	}
	return false
}

func commandLabel(cmd *Command) string {
	if cmd == nil {
		return "<unknown>"
	}
	if cmd.Path != "" {
		return cmd.Path
	}
	return cmd.Name
}
