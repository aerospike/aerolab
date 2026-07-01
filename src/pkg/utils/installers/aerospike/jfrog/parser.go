package jfrog

import (
	"regexp"
	"strings"
)

// NameParts is the parsed form of a JFrog artifact filename. Only the
// "aerospike-server-{edition}" RPM/DEB packages used by the cluster-create
// flow are parsed; everything else (asc signatures, source tarballs, etc.)
// returns nil.
type NameParts struct {
	Edition   string // community | enterprise | federal
	Version   string // 8.1.3.0
	Release   string // 28
	OSName    string // amazon | centos | debian | ubuntu
	OSVersion string // 2023 | 9 | 12 | 24.04
	Arch      string // x86_64 | aarch64    (normalised to aerospike package conventions)
	Format    string // rpm | deb
}

// Resilient matching notes:
//   - An optional prefix ending in "-" is allowed before "aerospike-server-"
//     so double-prefixed CI names like "aerospike-aerospike-server-..." match.
//   - An optional tag (e.g. "-TEST") may sit between the edition and the
//     version separator, so names like
//     "aerospike-aerospike-server-enterprise-TEST_8.1.3.0-36ubuntu24.04_arm64.deb"
//     match. For rpm the tag segments must start with a letter so they can't
//     eat the numeric version that follows the "-" separator.

// rpm: [prefix-]aerospike-server-{edition}[-{tag}]-{version}-{release}.{osTag}.{arch}.rpm
//
//	aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm
//	aerospike-aerospike-server-enterprise-TEST-8.1.3.0-36.ubuntu24.04.aarch64.rpm
var rpmRE = regexp.MustCompile(
	`^(?:.*?-)?aerospike-server-(community|enterprise|federal)` +
		`(?:-[A-Za-z][A-Za-z0-9]*)*-` +
		`([0-9][0-9.]*)-([0-9]+)\.` +
		`((?:amzn|el|debian|ubuntu)[0-9]+(?:\.[0-9]+)?)\.` +
		`(x86_64|aarch64)\.rpm$`,
)

// deb: [prefix-]aerospike-server-{edition}[-{tag}]_{version}-{release}{osTag}_{arch}.deb
//
//	aerospike-server-community_8.1.3.0-28debian12_amd64.deb
//	aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb
//	aerospike-aerospike-server-enterprise-TEST_8.1.3.0-36ubuntu24.04_arm64.deb
//
// Note: there is no underscore between {release} and {osTag}.
var debRE = regexp.MustCompile(
	`^(?:.*?-)?aerospike-server-(community|enterprise|federal)` +
		`(?:-[A-Za-z0-9]+)*_` +
		`([0-9][0-9.]*)-([0-9]+)` +
		`((?:amzn|el|debian|ubuntu)[0-9]+(?:\.[0-9]+)?)` +
		`_(amd64|arm64)\.deb$`,
)

// ParseFileName returns the parsed NameParts, or nil if the name does not
// match an Aerospike server RPM or DEB.
func ParseFileName(name string) *NameParts {
	if m := rpmRE.FindStringSubmatch(name); m != nil {
		os, ver := splitOSTag(m[4])
		return &NameParts{
			Edition:   m[1],
			Version:   m[2],
			Release:   m[3],
			OSName:    os,
			OSVersion: ver,
			Arch:      m[5],
			Format:    "rpm",
		}
	}
	if m := debRE.FindStringSubmatch(name); m != nil {
		os, ver := splitOSTag(m[4])
		return &NameParts{
			Edition:   m[1],
			Version:   m[2],
			Release:   m[3],
			OSName:    os,
			OSVersion: ver,
			Arch:      debArch(m[5]),
			Format:    "deb",
		}
	}
	return nil
}

// splitOSTag turns a JFrog osTag like "amzn2023" / "debian12" / "ubuntu24.04"
// into the (OSName, OSVersion) pair the rest of aerolab expects.
func splitOSTag(tag string) (osName, osVersion string) {
	switch {
	case strings.HasPrefix(tag, "amzn"):
		return "amazon", strings.TrimPrefix(tag, "amzn")
	case strings.HasPrefix(tag, "el"):
		return "centos", strings.TrimPrefix(tag, "el")
	case strings.HasPrefix(tag, "debian"):
		return "debian", strings.TrimPrefix(tag, "debian")
	case strings.HasPrefix(tag, "ubuntu"):
		return "ubuntu", strings.TrimPrefix(tag, "ubuntu")
	}
	return "", tag
}

// debArch maps Debian's package arch labels to the rpm/aerolab labels so
// the matcher only ever has to think in one vocabulary.
func debArch(in string) string {
	switch in {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return in
}

// EditionFromInput extracts the desired edition from a -v string.
//
// The public-download path uses a single trailing 'c' / 'f' to switch
// edition. JFrog build numbers can end with a git SHA whose last hex char
// could legitimately be 'c' or 'f', so we require an explicit ":c", ":f"
// or ":e" separator in JFrog mode and never strip a plain trailing char.
// If neither separator nor env var is present, the caller's `defaultEdition`
// is returned.
func EditionFromInput(version, defaultEdition string) (edition, cleanVersion string) {
	if i := strings.LastIndex(version, ":"); i >= 0 {
		switch version[i+1:] {
		case "c", "community":
			return "community", version[:i]
		case "f", "federal":
			return "federal", version[:i]
		case "e", "enterprise":
			return "enterprise", version[:i]
		}
	}
	if defaultEdition == "" {
		defaultEdition = "enterprise"
	}
	return defaultEdition, version
}
