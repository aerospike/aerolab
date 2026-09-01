//go:build integration_docker

package aerolab_test

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/utils/installers/aerolab"
	"github.com/aerospike-community/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

func TestAerolabLatestUbuntu24(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := aerolab.GetLinuxInstallScript("", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestAerolabLatestCentos8(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := aerolab.GetLinuxInstallScript("", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "quay.io/centos/{arch}:stream8", script)
}

func TestAerolabLatestStable(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := aerolab.GetLinuxInstallScript("", nil, new(false))
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestAerolabLatestPrelease(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := aerolab.GetLinuxInstallScript("", nil, new(true))
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestAerolabVersioned(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := aerolab.GetLinuxInstallScript("", new("7.7.0"), new(false))
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)
	require.Contains(t, string(script), "7.7.0")

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestAerolabVersionPrefixed(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := aerolab.GetLinuxInstallScript("", new("7.7.*"), new(false))
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)
	// The selector resolves to the newest 7.7.x, so only the prefix is stable.
	require.Contains(t, string(script), "7.7.")

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}
