//go:build integration_docker

package vscode_test

import (
	"testing"

	"github.com/citrusleaf/aerolab/pkg/utils/installers/vscode"
	"github.com/citrusleaf/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

func TestVscodeLatestUbuntu24(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := vscode.GetLinuxInstallScript(false, false, nil, nil, nil, nil, false, nil, "/root", "root")
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", script)
}

func TestVscodeLatestCentos8(t *testing.T) {
	installertest.RequireDocker(t)

	script, err := vscode.GetLinuxInstallScript(false, false, new("testpw"), new("0.0.0.0:8080"), []string{"golang.go"}, []string{"some-does-not-exist"}, true, new("/opt"), "/root", "root")
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	installertest.RunScriptInImage(t, "quay.io/centos/{arch}:stream8", script)
}
