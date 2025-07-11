package compilers_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/aerospike/aerolab/pkg/utils/installers/compilers"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

func TestCompilersLatestUbuntu24(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

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

	img := "amd64/ubuntu:24.04"
	uuid := shortuuid.New()
	err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
	require.NoError(t, err)
	out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, img, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
	_ = out
	//fmt.Println(string(out))
	require.NoError(t, err)
}

func TestCompilersLatestCentos8(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

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

	img := "quay.io/centos/amd64:stream8"
	uuid := shortuuid.New()
	err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
	require.NoError(t, err)
	out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, img, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
	_ = out
	//fmt.Println(string(out))
	require.NoError(t, err)
}
