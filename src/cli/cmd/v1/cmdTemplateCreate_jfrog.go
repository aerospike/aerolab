package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/aerospike/aerolab/pkg/utils/installers"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike/jfrog"
	"github.com/rglonek/logger"
)

// jfrogPlan is the JFrog-mode equivalent of the public-flow result: a
// resolved install script, plus the local package file that has to be
// SFTP-uploaded to each instance before the script runs.
type jfrogPlan struct {
	script        []byte
	pkgLocalPath  string // path on operator's laptop
	pkgRemotePath string // path on target instance
	version       string // canonical build number (with -artifacts suffix)
	edition       string // community | enterprise | federal
	osVersion     string // never "latest"; resolved before return
}

// resolveJFrogPlan does the JFrog-specific equivalent of
// resolveAerospikeServerVersion + aerospike.GetFiles +
// files.GetInstallScript, and additionally pre-downloads the package
// file to the operator's local cache so it can be SFTP-uploaded by the
// caller. It returns nil-nil when JFrog mode is not active.
func resolveJFrogPlan(system *System, log *logger.Logger, aerospikeVersion, distro, distroVersion, arch string, debug bool) (*jfrogPlan, error) {
	cfg := jfrog.FromEnv()
	if cfg == nil {
		return nil, nil
	}
	if distroVersion == "latest" {
		// JFrog mode has discrete artifacts per OS version; users must
		// pick one rather than relying on the public flow's fallback.
		return nil, fmt.Errorf("JFrog mode (%s set) requires --distro-version to be explicit, not 'latest'",
			jfrog.EnvArtifactsURL)
	}

	edition, cleanVer := jfrog.EditionFromInput(aerospikeVersion, "enterprise")
	build, err := cfg.ResolveBuild(cleanVer)
	if err != nil {
		return nil, err
	}
	log.Info("Querying JFrog build %q number %q", build.Name, build.Number)
	files, err := build.Files(context.Background())
	if err != nil {
		return nil, err
	}
	log.Info("Found %d artifacts on build", len(files))

	jfArch := arch
	if arch == "amd64" {
		jfArch = "x86_64"
	} else if arch == "arm64" {
		jfArch = "aarch64"
	}
	osName := distro
	if osName == "rocky" {
		osName = "centos"
	}
	match, err := files.Match(jfrog.MatchCriteria{
		Edition:   edition,
		OSName:    osName,
		OSVersion: distroVersion,
		Arch:      jfArch,
	})
	if err != nil {
		return nil, err
	}
	log.Info("Selected artifact: %s/%s/%s (%d bytes)", match.Repo, match.Path, match.Name, match.Size)

	cacheDir, err := jfrogCacheDir()
	if err != nil {
		return nil, err
	}
	log.Info("Downloading to local cache %s", cacheDir)
	local, err := cfg.Download(context.Background(), match, cacheDir)
	if err != nil {
		return nil, err
	}

	pkgScript, err := jfrog.InstallScript(match, debug, false)
	if err != nil {
		return nil, err
	}
	// Wrap with the same "basic tools" optional dependency set the
	// public flow uses so the resulting templates are interchangeable.
	wrapped, err := installers.GetInstallScript(templateOptionalDeps(debug), pkgScript)
	if err != nil {
		return nil, fmt.Errorf("could not add basic tools to JFrog install script: %w", err)
	}

	return &jfrogPlan{
		script:        wrapped,
		pkgLocalPath:  local,
		pkgRemotePath: jfrog.RemotePackagePath(match),
		version:       build.Number,
		edition:       edition,
		osVersion:     distroVersion,
	}, nil
}

// jfrogResolveLight returns the canonical build number and edition
// without doing any network I/O. It is used by callers (cluster create)
// that need the resolved (version, flavor) pair early, before they
// delegate to TemplateCreate where the full plan is built.
//
// Returns (false, ...) when JFrog mode is not active so the caller can
// fall through to the public-download resolver.
func jfrogResolveLight(aerospikeVersion string) (active bool, canonicalVersion, edition string, err error) {
	cfg := jfrog.FromEnv()
	if cfg == nil {
		return false, "", "", nil
	}
	ed, cleanVer := jfrog.EditionFromInput(aerospikeVersion, "enterprise")
	build, err := cfg.ResolveBuild(cleanVer)
	if err != nil {
		return true, "", "", err
	}
	return true, build.Number, ed, nil
}

// jfrogCacheDir returns the operator-local cache directory for JFrog
// downloads. It piggybacks on AerolabRootDir so the same cleanup tools
// users already have for aerolab apply to the JFrog cache too.
func jfrogCacheDir() (string, error) {
	root, err := AerolabRootDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve aerolab root dir: %w", err)
	}
	return filepath.Join(root, "cache", "jfrog"), nil
}

// templateOptionalDeps returns the "basic tools" Software set that the
// public flow appends after the aerospike install script. Extracted so
// the JFrog flow can apply the same wrapping.
func templateOptionalDeps(debug bool) installers.Software {
	return installers.Software{
		Debug: debug,
		Optional: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "jq", Package: "jq"},
				{Command: "unzip", Package: "unzip"},
				{Command: "zip", Package: "zip"},
				{Command: "wget", Package: "wget"},
				{Command: "git", Package: "git"},
				{Command: "vim", Package: "vim"},
				{Command: "nano", Package: "nano"},
				{Command: "less", Package: "less"},
				{Command: "lnav", Package: "lnav"},
				{Command: "iptables", Package: "iptables"},
				{Command: "tcpdump", Package: "tcpdump"},
				{Command: "telnet", Package: "telnet"},
				{Command: "mpstat", Package: "sysstat"},
				{Command: "dig", Package: "dnsutils"},
				{Command: "dig", Package: "bind-utils"},
				{Command: "strings", Package: "binutils"},
				{Command: "which", Package: "which"},
				{Command: "ip", Package: "iproute2"},
				{Command: "ip", Package: "iproute"},
				{Command: "ip", Package: "iproute-tc"},
				{Command: "python3", Package: "python3"},
				{Command: "python3", Package: "python"},
				{Command: "nc", Package: "netcat"},
				{Command: "nc", Package: "nc"},
				{Command: "ping", Package: "iputils-ping"},
				{Command: "ping", Package: "iputils"},
				{Command: "ldapsearch", Package: "ldap-utils"},
				{Command: "netstat", Package: "net-tools"},
				{Command: "lsb_release", Package: "lsb-release"},
				{Command: "lsb_release", Package: "redhat-lsb-core"},
				{Command: "lsb_release", Package: "redhat-lsb"},
				{Command: "ps", Package: "procps"},
				{Command: "ps", Package: "procps-ng"},
			},
			Packages: []string{"python3-setuptools", "python3-distutils", "libcurl4", "libcurl4-openssl-dev", "libldap-common", "libcurl-openssl-devel", "initscripts"},
		},
	}
}
