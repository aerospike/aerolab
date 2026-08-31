//go:build integration_docker

package aerospike_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/aerospike-community/aerolab/pkg/utils/installers/aerospike"
	"github.com/stretchr/testify/require"
)

func apeScript(t *testing.T) aerospike.Files {
	products, err := aerospike.GetProducts(time.Second * 10)
	require.NoError(t, err)
	require.NotNil(t, products)
	require.NotEmpty(t, products)
	product := products.WithName("aerospike-prometheus-exporter")
	require.NotEmpty(t, product)

	versions, err := aerospike.GetVersions(time.Second*10, product[0])
	require.NoError(t, err)
	require.NotNil(t, versions)
	require.NotEmpty(t, versions)
	version := versions.Latest()
	require.NotNil(t, version)

	files, err := aerospike.GetFiles(time.Second*10, *version)
	require.NoError(t, err)
	require.NotNil(t, files)
	require.NotEmpty(t, files)

	return files
}

func Test01_ApeScriptDocker(t *testing.T) {
	files := apeScript(t)
	d, e := files.GetInstallScript(hostArch(), aerospike.OSName("centos"), "9", true, true, true, true)
	require.NoError(t, e)
	require.NotNil(t, d)
	require.NotEmpty(t, d)
	fmt.Println(string(d))
}

func Test01_ApeScript(t *testing.T) {
	files := apeScript(t)
	runMatrix(t, targets(files,
		[]string{"quay.io/centos/{arch}:stream8", "quay.io/centos/{arch}:stream9", "{arch}/rockylinux:8", "{arch}/rockylinux:9", "{arch}/ubuntu:20.04", "{arch}/ubuntu:22.04", "{arch}/ubuntu:24.04", "{arch}/debian:11", "{arch}/debian:12"},
		[]string{"centos", "centos", "centos", "centos", "ubuntu", "ubuntu", "ubuntu", "debian", "debian"},
		[]string{"8", "9", "8", "9", "20.04", "22.04", "24.04", "11", "12"},
	))
}
