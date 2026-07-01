package jfrog

import "testing"

func TestParseFileName_RPM(t *testing.T) {
	cases := []struct {
		name string
		want NameParts
	}{
		{
			"aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm",
			NameParts{Edition: "community", Version: "8.1.3.0", Release: "28", OSName: "amazon", OSVersion: "2023", Arch: "aarch64", Format: "rpm"},
		},
		{
			"aerospike-server-community-8.1.3.0-28.amzn2023.x86_64.rpm",
			NameParts{Edition: "community", Version: "8.1.3.0", Release: "28", OSName: "amazon", OSVersion: "2023", Arch: "x86_64", Format: "rpm"},
		},
		{
			"aerospike-server-enterprise-8.1.3.0-28.el9.x86_64.rpm",
			NameParts{Edition: "enterprise", Version: "8.1.3.0", Release: "28", OSName: "centos", OSVersion: "9", Arch: "x86_64", Format: "rpm"},
		},
		{
			"aerospike-server-federal-8.1.3.0-28.amzn2.aarch64.rpm",
			NameParts{Edition: "federal", Version: "8.1.3.0", Release: "28", OSName: "amazon", OSVersion: "2", Arch: "aarch64", Format: "rpm"},
		},
		// resilient: double "aerospike-" prefix + "-TEST" tag
		{
			"aerospike-aerospike-server-enterprise-TEST-8.1.3.0-36.el9.x86_64.rpm",
			NameParts{Edition: "enterprise", Version: "8.1.3.0", Release: "36", OSName: "centos", OSVersion: "9", Arch: "x86_64", Format: "rpm"},
		},
	}
	for _, tc := range cases {
		got := ParseFileName(tc.name)
		if got == nil {
			t.Errorf("%s: parse returned nil", tc.name)
			continue
		}
		if *got != tc.want {
			t.Errorf("%s:\n  got  %+v\n  want %+v", tc.name, *got, tc.want)
		}
	}
}

func TestParseFileName_DEB(t *testing.T) {
	cases := []struct {
		name string
		want NameParts
	}{
		{
			"aerospike-server-community_8.1.3.0-28debian12_amd64.deb",
			NameParts{Edition: "community", Version: "8.1.3.0", Release: "28", OSName: "debian", OSVersion: "12", Arch: "x86_64", Format: "deb"},
		},
		{
			"aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb",
			NameParts{Edition: "enterprise", Version: "8.1.3.0", Release: "28", OSName: "ubuntu", OSVersion: "24.04", Arch: "aarch64", Format: "deb"},
		},
		{
			"aerospike-server-enterprise_8.1.3.0-28ubuntu26.04_amd64.deb",
			NameParts{Edition: "enterprise", Version: "8.1.3.0", Release: "28", OSName: "ubuntu", OSVersion: "26.04", Arch: "x86_64", Format: "deb"},
		},
		// resilient: double "aerospike-" prefix + "-TEST" tag (the real-world case)
		{
			"aerospike-aerospike-server-enterprise-TEST_8.1.3.0-36ubuntu24.04_arm64.deb",
			NameParts{Edition: "enterprise", Version: "8.1.3.0", Release: "36", OSName: "ubuntu", OSVersion: "24.04", Arch: "aarch64", Format: "deb"},
		},
	}
	for _, tc := range cases {
		got := ParseFileName(tc.name)
		if got == nil {
			t.Errorf("%s: parse returned nil", tc.name)
			continue
		}
		if *got != tc.want {
			t.Errorf("%s:\n  got  %+v\n  want %+v", tc.name, *got, tc.want)
		}
	}
}

func TestParseFileName_Ignored(t *testing.T) {
	skip := []string{
		"aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm.asc",
		"aerospike-server-community_8.1.3.0-28debian12_amd64.deb.asc",
		"aerospike-server-enterprise_8.0.0.8_ubuntu24.04_x86_64.tgz", // public-download style
		"aerospike-tools_11.2.2_ubuntu24.04_aarch64.tgz",
		"random.txt",
		"",
	}
	for _, n := range skip {
		if got := ParseFileName(n); got != nil {
			t.Errorf("%s: expected nil, got %+v", n, *got)
		}
	}
}

func TestEditionFromInput(t *testing.T) {
	cases := []struct {
		in           string
		def          string
		wantEdition  string
		wantVersion  string
	}{
		{"8.1.3.0-28-g302194ebc", "enterprise", "enterprise", "8.1.3.0-28-g302194ebc"},
		// git SHA ending in 'c' must NOT be treated as community shorthand
		{"8.1.3.0-28-g302194ebc", "", "enterprise", "8.1.3.0-28-g302194ebc"},
		// explicit separators
		{"8.1.3.0-28-g302194ebc:c", "enterprise", "community", "8.1.3.0-28-g302194ebc"},
		{"8.1.3.0-28-g302194ebc:community", "enterprise", "community", "8.1.3.0-28-g302194ebc"},
		{"8.1.3.0-28-g302194ebc:f", "enterprise", "federal", "8.1.3.0-28-g302194ebc"},
		{"8.1.3.0-28-g302194ebc:e", "community", "enterprise", "8.1.3.0-28-g302194ebc"},
		// default override
		{"8.1.3.0-28-g302194ebc", "community", "community", "8.1.3.0-28-g302194ebc"},
		// unknown suffix is left in place
		{"foo:bar", "enterprise", "enterprise", "foo:bar"},
	}
	for _, tc := range cases {
		gotEd, gotV := EditionFromInput(tc.in, tc.def)
		if gotEd != tc.wantEdition || gotV != tc.wantVersion {
			t.Errorf("EditionFromInput(%q, %q) = (%q, %q); want (%q, %q)",
				tc.in, tc.def, gotEd, gotV, tc.wantEdition, tc.wantVersion)
		}
	}
}
