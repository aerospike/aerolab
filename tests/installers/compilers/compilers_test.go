//go:build integration_docker

package compilers_test

import (
	"testing"

	"github.com/citrusleaf/aerolab/pkg/utils/installers/compilers"
	"github.com/citrusleaf/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

func compilersScript(t *testing.T) []byte {
	t.Helper()
	script, err := compilers.GetInstallScript(
		[]compilers.Compiler{compilers.CompilerBuildEssentials, compilers.CompilerDotnet, compilers.CompilerGo, compilers.CompilerPython3},
		"21",
		"9.0",
		"",
		[]string{"numpy"},        // required pip stuff
		[]string{"pandas"},       // optional pip stuff
		[]string{"curl", "wget"}, // extra apt stuff
		[]string{"curl", "wget"}, // extra yum stuff
	)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)
	return script
}

func TestCompilersLatestUbuntu24(t *testing.T) {
	installertest.RequireDocker(t)
	installertest.RunScriptInImage(t, "{arch}/ubuntu:24.04", compilersScript(t))
}

func TestCompilersLatestCentos8(t *testing.T) {
	installertest.RequireDocker(t)
	installertest.RunScriptInImage(t, "quay.io/centos/{arch}:stream8", compilersScript(t))
}
