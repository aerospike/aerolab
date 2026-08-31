//go:build integration_docker

package grafana_test

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/utils/installers/grafana"
	"github.com/aerospike-community/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

func TestGrafanaLatestUbuntu24(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := grafana.GetInstallScript("", false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestGrafanaLatestCentos8(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := grafana.GetInstallScript("", false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "quay.io/centos/{arch}:stream8", script)
}

func TestGrafanaVersioned(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := grafana.GetInstallScript("10.4.19", false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)
	require.Contains(t, string(script), "10.4.19")

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}
