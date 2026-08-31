package cmd

import (
	"runtime"
	"testing"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/utils/installers/aerospike"
	"github.com/stretchr/testify/require"
)

func TestAerospikeArch(t *testing.T) {
	require.Equal(t, aerospike.ArchitectureTypeX86_64, aerospikeArch(backends.ArchitectureX8664))
	require.Equal(t, aerospike.ArchitectureTypeAARCH64, aerospikeArch(backends.ArchitectureARM64))

	native := aerospikeArch(backends.ArchitectureNative)
	switch runtime.GOARCH {
	case "amd64":
		require.Equal(t, aerospike.ArchitectureTypeX86_64, native)
	case "arm64":
		require.Equal(t, aerospike.ArchitectureTypeAARCH64, native)
	default:
		require.Equal(t, aerospike.ArchitectureTypeUnknown, native)
	}
}

func TestIsARMArchAliases(t *testing.T) {
	require.True(t, isARMArch("arm64"))
	require.True(t, isARMArch("aarch64"))
	require.True(t, isARMArch("ARM64"))
	require.True(t, isARMArch("AARCH64"))
	require.False(t, isARMArch("amd64"))
	require.False(t, isARMArch("x86_64"))
	require.False(t, isARMArch(""))
}
