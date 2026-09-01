//go:build integration_docker

package aerospike_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/citrusleaf/aerolab/pkg/utils/installers/aerospike"
	"github.com/stretchr/testify/require"
)

func serverScript(t *testing.T, np string) aerospike.Files {
	products, err := aerospike.GetProducts(time.Second * 10)
	require.NoError(t, err)
	require.NotNil(t, products)
	require.NotEmpty(t, products)
	product := products.WithName("aerospike-server-enterprise")
	require.NotEmpty(t, product)

	versions, err := aerospike.GetVersions(time.Second*10, product[0])
	require.NoError(t, err)
	require.NotNil(t, versions)
	require.NotEmpty(t, versions)
	version := versions.WithNamePrefix(np)
	require.NotNil(t, version)

	files, err := aerospike.GetFiles(time.Second*10, version[0])
	require.NoError(t, err)
	require.NotNil(t, files)
	require.NotEmpty(t, files)

	return files
}

func Test000_LatestVersion(t *testing.T) {
	products, err := aerospike.GetProducts(time.Second * 10)
	require.NoError(t, err)
	require.NotNil(t, products)
	require.NotEmpty(t, products)
	product := products.WithName("aerospike-server-enterprise")
	require.NotEmpty(t, product)

	versions, err := aerospike.GetVersions(time.Second*10, product[0])
	require.NoError(t, err)
	require.NotNil(t, versions)
	require.NotEmpty(t, versions)
	version := versions.Latest()
	require.NotNil(t, version)
	fmt.Println(version.Name)
}

func Test00_ServerScript(t *testing.T) {
	files := serverScript(t, "6.")
	filesNew := serverScript(t, "8.")

	// Older distros are covered with the 6.x line, newer ones with 8.x, since
	// each release line only ships packages for the distros current at the time.
	old := targets(files,
		[]string{"quay.io/centos/{arch}:stream8", "quay.io/centos/{arch}:stream9", "{arch}/rockylinux:8", "{arch}/rockylinux:9", "{arch}/ubuntu:20.04", "{arch}/ubuntu:22.04"},
		[]string{"centos", "centos", "centos", "centos", "ubuntu", "ubuntu"},
		[]string{"8", "9", "8", "9", "20.04", "22.04"},
	)
	recent := targets(filesNew,
		[]string{"{arch}/ubuntu:24.04", "{arch}/debian:11", "{arch}/debian:12"},
		[]string{"ubuntu", "debian", "debian"},
		[]string{"24.04", "11", "12"},
	)
	runMatrix(t, append(old, recent...))
}

func Test00_ServerScriptOldAsd(t *testing.T) {
	files := serverScript(t, "5.1.")
	runMatrix(t, targets(files,
		[]string{"quay.io/centos/{arch}:stream8", "{arch}/rockylinux:8", "{arch}/ubuntu:20.04", "{arch}/debian:10"},
		[]string{"centos", "centos", "ubuntu", "debian"},
		[]string{"8", "8", "20.04", "10"},
	))
}

func Test00_ServerScriptVOldAsd(t *testing.T) {
	files := serverScript(t, "4.8.")
	runMatrix(t, targets(files,
		[]string{"quay.io/centos/{arch}:stream8"},
		[]string{"centos"},
		[]string{"8"},
	))
}
