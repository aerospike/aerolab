//go:build integration_docker

package easytc_test

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/utils/installers/easytc"
	"github.com/aerospike-community/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

func TestEasytcLatestUbuntu24(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := easytc.GetLinuxInstallScript(nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestEasytcLatestCentos8(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := easytc.GetLinuxInstallScript(nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "quay.io/centos/{arch}:stream8", script)
}

func TestEasytcLatestStable(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := easytc.GetLinuxInstallScript(nil, new(false), false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}
