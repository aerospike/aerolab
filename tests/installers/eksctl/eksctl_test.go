//go:build integration_docker

package eksctl_test

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/utils/installers/eksctl"
	"github.com/aerospike-community/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

func TestEksctlLatestUbuntu24(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := eksctl.GetInstallScript()
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestEksctlLatestCentos8(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := eksctl.GetInstallScript()
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "quay.io/centos/{arch}:stream8", script)
}
