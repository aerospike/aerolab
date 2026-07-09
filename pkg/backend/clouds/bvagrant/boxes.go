package bvagrant

import (
	"fmt"
	"slices"
	"strings"
)

// boxDef describes a single supported OS/version combination and the Vagrant box that
// provides it, along with the CPU architectures that box is available for.
type boxDef struct {
	Distro  string
	Version string
	Box     string
	Archs   []string
}

// boxCatalog is the list of OS distro/version combinations supported by the vagrant
// backend, and the bento Vagrant box + architectures each one is available for.
var boxCatalog = []boxDef{
	{Distro: "ubuntu", Version: "20.04", Box: "bento/ubuntu-20.04", Archs: []string{"amd64"}},
	{Distro: "ubuntu", Version: "22.04", Box: "bento/ubuntu-22.04", Archs: []string{"amd64", "arm64"}},
	{Distro: "ubuntu", Version: "24.04", Box: "bento/ubuntu-24.04", Archs: []string{"amd64", "arm64"}},
	{Distro: "debian", Version: "11", Box: "bento/debian-11", Archs: []string{"amd64"}},
	{Distro: "debian", Version: "12", Box: "bento/debian-12", Archs: []string{"amd64", "arm64"}},
	{Distro: "rocky", Version: "8", Box: "bento/rockylinux-8", Archs: []string{"amd64"}},
	{Distro: "rocky", Version: "9", Box: "bento/rockylinux-9", Archs: []string{"amd64", "arm64"}},
}

// boxNaming resolves the Vagrant box name for a given distro, version, and architecture.
// distro matching is case-insensitive; version matching is exact; arch is validated
// against the archs supported by the matched catalog entry.
func boxNaming(distro, version, arch string) (string, error) {
	distroLower := strings.ToLower(distro)

	var matchedDistro bool
	for _, entry := range boxCatalog {
		if strings.ToLower(entry.Distro) != distroLower {
			continue
		}
		matchedDistro = true
		if entry.Version != version {
			continue
		}
		if !slices.Contains(entry.Archs, arch) {
			return "", fmt.Errorf("unsupported architecture %q for %s %s (supported: %v)", arch, distro, version, entry.Archs)
		}
		return entry.Box, nil
	}

	if matchedDistro {
		return "", fmt.Errorf("unsupported version %q for distro %q", version, distro)
	}
	return "", fmt.Errorf("unsupported distro %q", distro)
}
