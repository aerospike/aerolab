package cmd

import (
	"runtime"
	"strings"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/utils/installers/aerospike"
)

// aerospikeArch maps an instance architecture onto the labels used by
// Aerospike artifact names (x86_64 / aarch64).
func aerospikeArch(arch backends.Architecture) aerospike.ArchitectureType {
	switch arch {
	case backends.ArchitectureNative:
		switch runtime.GOARCH {
		case "amd64":
			return aerospike.ArchitectureTypeX86_64
		case "arm64":
			return aerospike.ArchitectureTypeAARCH64
		default:
			return aerospike.ArchitectureTypeUnknown
		}
	case backends.ArchitectureX8664:
		return aerospike.ArchitectureTypeX86_64
	case backends.ArchitectureARM64:
		return aerospike.ArchitectureTypeAARCH64
	default:
		return aerospike.ArchitectureTypeUnknown
	}
}

// isARMArch reports whether a CLI/user architecture string refers to ARM
// (arm64 and aarch64 are both accepted; distros disagree on uname -m).
func isARMArch(s string) bool {
	var a backends.Architecture
	if err := a.FromString(s); err != nil {
		return strings.EqualFold(s, "arm64") || strings.EqualFold(s, "aarch64")
	}
	return a.IsARM()
}
