//go:build integration_docker

package prometheus_test

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/utils/installers/prometheus"
	"github.com/aerospike-community/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

func TestPrometheusLatestUbuntu24(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := prometheus.GetLinuxInstallScript(nil, new(false), false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestPrometheusLatestCentos8(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := prometheus.GetLinuxInstallScript(nil, new(false), false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "quay.io/centos/{arch}:stream8", script)
}
