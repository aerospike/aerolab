package jfrog

import (
	"regexp"
	"strings"
)

// NameParts is the parsed form of a JFrog artifact filename. Only the
// "aerospike-server-{edition}" packages used by the cluster-create flow are
// parsed; everything else (asc signatures, checksums, container images,
// source tarballs, …) returns nil.
type NameParts struct {
	Edition   string // community | enterprise | federal
	Version   string // 8.1.3.0
	Release   string // 28 | 70-g282a6817d | "" (absent)
	OSName    string // amazon | centos | debian | ubuntu
	OSVersion string // 2023 | 9 | 12 | 24.04
	Arch      string // x86_64 | aarch64    (normalised to aerospike package conventions)
	Format    string // rpm | deb | tgz
}

// Parsing strategy: JFrog builds are produced by several different pipelines
// and the exact separator layout of an artifact name has changed more than
// once (numeric vs git-describe release fields, "-TEST" tags, doubled
// "aerospike-" prefixes, self-contained .tgz bundles alongside loose
// .deb/.rpm packages). Matching one rigid full-name regex per format made a
// single unexpected name silently invisible to the matcher, which surfaced to
// users as "no <edition> <format> package found" with no way to tell why.
//
// So the name is tokenised around the parts that are stable instead:
//
//	[<prefix>-]aerospike-server-<edition> <sep> <version>[<sep><release>] <osTag> <sep> <arch> . <ext>
//
// The OS tag ("ubuntu24.04", "el9", "amzn2023", "debian12") is the anchor:
// version/release sit before it, the architecture after it, and everything in
// between may use any of the separators the pipelines have used ("-", "_",
// ".", "~", "+") or carry an extra build tag. Names without an OS tag, an
// architecture or a numeric version are still rejected, which keeps container
// images and source archives out.

// serverPrefixRE matches "[<prefix>-]aerospike-server-<edition>" at the head
// of an artifact name; submatch 1 is the edition.
var serverPrefixRE = regexp.MustCompile(
	`^(?:.*?-)?aerospike-server-(community|enterprise|federal)(?:[^a-z]|$)`)

// toolsPrefixRE is the aerospike-tools equivalent of serverPrefixRE.
var toolsPrefixRE = regexp.MustCompile(`^(?:.*?-)?(aerospike-tools)(?:[^a-z]|$)`)

// osTagRE matches a JFrog OS tag. No delimiter is required in front of it:
// the deb layout runs the release straight into the tag
// ("…-28ubuntu24.04_amd64.deb", "…-70-g282a6817dubuntu24.04_amd64.deb"), so
// the last occurrence in the name is taken instead.
var osTagRE = regexp.MustCompile(`((?:amzn|debian|ubuntu|el)[0-9]+(?:\.[0-9]+)?)`)

// archRE matches an architecture token delimited by separators or ends of
// string. Both the rpm/tgz vocabulary (x86_64/aarch64) and the deb one
// (amd64/arm64) are accepted; debArch normalises them.
var archRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9])(x86_64|aarch64|amd64|arm64)(?:[^A-Za-z0-9]|$)`)

// versionRE matches the first separator-delimited numeric version run, so a
// leading build tag ("-TEST_8.1.3.0-36…") does not get mistaken for one.
var versionRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9])([0-9]+(?:\.[0-9]+)*)`)

// ToolsParts is the parsed form of an "aerospike-tools_*.tgz" artifact.
// The tools bundle (asinfo, asadm, aql, …) is edition-agnostic, so unlike
// NameParts it carries no edition/release/format.
type ToolsParts struct {
	Version   string // 11.2.2
	OSName    string // amazon | centos | debian | ubuntu
	OSVersion string // 2023 | 9 | 12 | 24.04
	Arch      string // x86_64 | aarch64
}

// ParseToolsFileName returns the parsed ToolsParts, or nil if the name is
// not an Aerospike tools .tgz.
//
//	aerospike-tools_11.2.2_ubuntu24.04_x86_64.tgz
//	aerospike-aerospike-tools_11.2.2-1_amzn2023_aarch64.tgz
func ParseToolsFileName(name string) *ToolsParts {
	base, format := splitPackageExt(name)
	if format != "tgz" {
		return nil
	}
	m := toolsPrefixRE.FindStringSubmatchIndex(base)
	if m == nil {
		return nil
	}
	p := parseTail(base[m[3]:])
	if p == nil {
		return nil
	}
	return &ToolsParts{
		Version:   p.version,
		OSName:    p.osName,
		OSVersion: p.osVersion,
		Arch:      p.arch,
	}
}

// ParseFileName returns the parsed NameParts, or nil if the name does not
// match an Aerospike server RPM, DEB or .tgz bundle.
//
//	aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm
//	aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb
//	aerospike-server-enterprise_8.1.3.0-70-g282a6817dubuntu24.04_amd64.deb
//	aerospike-server-enterprise_8.0.0.8_ubuntu24.04_x86_64.tgz
func ParseFileName(name string) *NameParts {
	base, format := splitPackageExt(name)
	if format == "" {
		return nil
	}
	m := serverPrefixRE.FindStringSubmatchIndex(base)
	if m == nil {
		return nil
	}
	edition := base[m[2]:m[3]]
	p := parseTail(base[m[3]:])
	if p == nil {
		return nil
	}
	return &NameParts{
		Edition:   edition,
		Version:   p.version,
		Release:   p.release,
		OSName:    p.osName,
		OSVersion: p.osVersion,
		Arch:      p.arch,
		Format:    format,
	}
}

// nameTail is the product-independent part of an artifact name: everything
// after the "aerospike-server-<edition>" / "aerospike-tools" head.
type nameTail struct {
	version   string
	release   string
	osName    string
	osVersion string
	arch      string
}

// parseTail extracts version/release, OS and architecture from the tail of an
// artifact name (extension already stripped). Returns nil when any of the
// required tokens is missing.
func parseTail(rest string) *nameTail {
	os := lastSubmatch(osTagRE, rest)
	if os == nil {
		return nil
	}
	osName, osVersion := splitOSTag(rest[os[0]:os[1]])
	if osName == "" {
		return nil
	}
	// the architecture always trails the OS tag
	after := rest[os[1]:]
	arch := lastSubmatch(archRE, after)
	if arch == nil {
		return nil
	}
	version, release := splitVersionRelease(rest[:os[0]])
	if version == "" {
		return nil
	}
	return &nameTail{
		version:   version,
		release:   release,
		osName:    osName,
		osVersion: osVersion,
		arch:      debArch(after[arch[0]:arch[1]]),
	}
}

// lastSubmatch returns the [start,end) offsets of the LAST occurrence of
// re's first capturing group in s, or nil when there is none. Taking the last
// match keeps a version or path fragment that happens to look like a token
// from shadowing the real one, which always sits at the end of the name.
func lastSubmatch(re *regexp.Regexp, s string) []int {
	all := re.FindAllStringSubmatchIndex(s, -1)
	if len(all) == 0 {
		return nil
	}
	m := all[len(all)-1]
	return []int{m[2], m[3]}
}

// splitVersionRelease splits the "<version>[<sep><release>]" segment that
// sits between the product name and the OS tag. The release is whatever
// follows the version, stripped of separators, and may be empty (.tgz
// bundles) or non-numeric ("70-g282a6817d" on git-describe builds).
func splitVersionRelease(mid string) (version, release string) {
	m := versionRE.FindStringSubmatchIndex(mid)
	if m == nil {
		return "", ""
	}
	return mid[m[2]:m[3]], strings.Trim(mid[m[3]:], "-_.~+")
}

// splitPackageExt strips a known package extension and returns the bare name
// plus the install format. Signatures and checksums (".deb.asc",
// ".rpm.sha256", …) do not end in a package extension and are rejected here.
func splitPackageExt(name string) (base, format string) {
	switch {
	case strings.HasSuffix(name, ".deb"):
		return strings.TrimSuffix(name, ".deb"), "deb"
	case strings.HasSuffix(name, ".rpm"):
		return strings.TrimSuffix(name, ".rpm"), "rpm"
	case strings.HasSuffix(name, ".tgz"):
		return strings.TrimSuffix(name, ".tgz"), "tgz"
	case strings.HasSuffix(name, ".tar.gz"):
		return strings.TrimSuffix(name, ".tar.gz"), "tgz"
	}
	return "", ""
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

// osTag is the inverse of splitOSTag: it renders an (osName, osVersion)
// pair back into the JFrog filename tag ("ubuntu24.04", "amzn2023",
// "el9", "debian12"). Returns "" for OS names JFrog does not publish.
func osTag(osName, osVersion string) string {
	switch osName {
	case "amazon":
		return "amzn" + osVersion
	case "centos", "rocky":
		return "el" + osVersion
	case "debian":
		return "debian" + osVersion
	case "ubuntu":
		return "ubuntu" + osVersion
	}
	return ""
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
