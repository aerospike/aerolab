//go:build integration_docker

package aerospike_test

import (
	"testing"

	"github.com/citrusleaf/aerolab/pkg/utils/installers/aerospike"
	"github.com/citrusleaf/aerolab/tests/installers/installertest"
	"github.com/stretchr/testify/require"
)

// hostArch returns the Aerospike package architecture matching the machine
// running the tests. The installer suites only ever exercise native packages, so
// an arm64 host installs aarch64 builds and an amd64 host installs x86_64 ones.
func hostArch() aerospike.ArchitectureType {
	if installertest.Arch() == "arm64" {
		return aerospike.ArchitectureTypeAARCH64
	}
	return aerospike.ArchitectureTypeX86_64
}

// target is one entry of an installer matrix: which file set to install from,
// the container image to install into (the "{arch}" placeholder is resolved to
// the host architecture), and the os name/version that selects the package.
type target struct {
	files     aerospike.Files
	image     string
	osName    string
	osVersion string
}

// targets builds a matrix from parallel slices, which is how the upstream OS
// lists are most readable to maintain.
func targets(files aerospike.Files, images, osNames, osVersions []string) []target {
	out := make([]target, 0, len(images))
	for i := range images {
		out = append(out, target{files: files, image: images[i], osName: osNames[i], osVersion: osVersions[i]})
	}
	return out
}

// shipsArch reports whether a file set contains any build for arch. Release
// lines that predate arm64 support ship none at all.
func shipsArch(files aerospike.Files, arch aerospike.ArchitectureType) bool {
	for _, f := range files {
		if d := f.ParseNameParts(); d != nil && d.Architecture == arch {
			return true
		}
	}
	return false
}

// runMatrix generates the install script for every target and runs it in the
// matching container on the host architecture.
//
// A target with no upstream build for the host architecture is logged and
// skipped: not every distro has a package in every release line. Failures are
// recorded per target rather than aborting, so one broken distro does not hide
// the state of the remaining ones.
//
// If nothing at all ran, the outcome depends on why. A release line that ships
// no builds for this architecture (anything predating arm64 support) skips,
// because there is genuinely nothing for this host to exercise. A release line
// that does ship the architecture but matched no target means the matrix is
// wrong, and that fails.
func runMatrix(t *testing.T, all []target) {
	t.Helper()
	installertest.RequireDocker(t)

	arch := hostArch()
	ran := 0
	for _, tg := range all {
		script, err := tg.files.GetInstallScript(arch, aerospike.OSName(tg.osName), tg.osVersion, true, true, true, true)
		if err != nil {
			t.Logf("skipping %s: no %s build for %s %s (%v)",
				installertest.Image(tg.image), arch, tg.osName, tg.osVersion, err)
			continue
		}
		if len(script) == 0 {
			t.Errorf("empty install script for %s %s (%s)", tg.osName, tg.osVersion, arch)
			continue
		}
		installertest.RunScriptInImage(t, tg.image, script)
		ran++
	}
	if ran > 0 {
		return
	}
	for _, tg := range all {
		require.Falsef(t, shipsArch(tg.files, arch),
			"matrix matched no target even though %s builds exist; the distro list is out of date", arch)
	}
	t.Skipf("no %s build exists for any target in this release line; nothing to test on this host", arch)
}
