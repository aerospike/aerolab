package bvagrant

import (
	"strings"
	"testing"
)

func TestBoxNamingKnownCombos(t *testing.T) {
	cases := []struct {
		distro, version, arch, want string
	}{
		{"ubuntu", "24.04", "amd64", "bento/ubuntu-24.04"},
		{"ubuntu", "24.04", "arm64", "bento/ubuntu-24.04"},
		{"ubuntu", "22.04", "amd64", "bento/ubuntu-22.04"},
		{"ubuntu", "20.04", "amd64", "bento/ubuntu-20.04"},
		{"debian", "12", "amd64", "bento/debian-12"},
		{"debian", "11", "amd64", "bento/debian-11"},
		{"rocky", "9", "amd64", "bento/rockylinux-9"},
		{"rocky", "8", "amd64", "bento/rockylinux-8"},
		// case-insensitive distro
		{"Ubuntu", "24.04", "amd64", "bento/ubuntu-24.04"},
		{"UBUNTU", "24.04", "arm64", "bento/ubuntu-24.04"},
	}
	for _, c := range cases {
		got, err := boxNaming(c.distro, c.version, c.arch)
		if err != nil {
			t.Fatalf("boxNaming(%s,%s,%s): unexpected error: %v", c.distro, c.version, c.arch, err)
		}
		if got != c.want {
			t.Fatalf("boxNaming(%s,%s,%s) = %s, want %s", c.distro, c.version, c.arch, got, c.want)
		}
	}
}

func TestBoxNamingUnsupportedArchForVersion(t *testing.T) {
	// ubuntu 20.04 is amd64-only
	_, err := boxNaming("ubuntu", "20.04", "arm64")
	if err == nil {
		t.Fatalf("expected error for ubuntu 20.04 arm64")
	}
	if !strings.Contains(err.Error(), "arm64") {
		t.Fatalf("expected error to mention requested arch, got: %v", err)
	}

	// debian 11 is amd64-only
	_, err = boxNaming("debian", "11", "arm64")
	if err == nil {
		t.Fatalf("expected error for debian 11 arm64")
	}

	// rocky 8 is amd64-only
	_, err = boxNaming("rocky", "8", "arm64")
	if err == nil {
		t.Fatalf("expected error for rocky 8 arm64")
	}
}

func TestBoxNamingUnsupportedDistro(t *testing.T) {
	_, err := boxNaming("gentoo", "1.0", "amd64")
	if err == nil {
		t.Fatalf("expected error for unsupported distro")
	}
	if !strings.Contains(err.Error(), "gentoo") {
		t.Fatalf("expected error to mention requested distro, got: %v", err)
	}
}

func TestBoxNamingUnsupportedVersion(t *testing.T) {
	_, err := boxNaming("ubuntu", "99.99", "amd64")
	if err == nil {
		t.Fatalf("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "99.99") {
		t.Fatalf("expected error to mention requested version, got: %v", err)
	}
}

func TestBoxNamingUnsupportedArch(t *testing.T) {
	_, err := boxNaming("ubuntu", "24.04", "mips")
	if err == nil {
		t.Fatalf("expected error for unsupported arch")
	}
	if !strings.Contains(err.Error(), "mips") {
		t.Fatalf("expected error to mention requested arch, got: %v", err)
	}
}

func TestBoxCatalogSanity(t *testing.T) {
	seen := map[string]bool{}
	for _, entry := range boxCatalog {
		key := strings.ToLower(entry.Distro) + "|" + entry.Version
		if seen[key] {
			t.Fatalf("duplicate {distro,version} pair in catalog: %s", key)
		}
		seen[key] = true

		if entry.Box == "" {
			t.Fatalf("empty box name for %s %s", entry.Distro, entry.Version)
		}
		if len(entry.Archs) == 0 {
			t.Fatalf("no archs listed for %s %s", entry.Distro, entry.Version)
		}
		for _, a := range entry.Archs {
			if a != "amd64" && a != "arm64" {
				t.Fatalf("unexpected arch %q for %s %s", a, entry.Distro, entry.Version)
			}
		}
	}
}
