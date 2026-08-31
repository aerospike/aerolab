package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/aerospike-community/aerolab/pkg/utils/installers"
	"github.com/aerospike-community/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike-community/aerolab/pkg/utils/installers/aerospike/jfrog"
	"github.com/rglonek/logger"
)

// jfrogPlan is the JFrog-mode equivalent of the public-flow result: a
// resolved install script, plus the local package file that has to be
// SFTP-uploaded to each instance before the script runs.
type jfrogPlan struct {
	script             []byte
	pkgLocalPath       string // path on operator's laptop
	pkgRemotePath      string // path on target instance
	toolsPkgLocalPath  string // aerospike-tools .tgz on operator's laptop ("" if not found)
	toolsPkgRemotePath string // aerospike-tools .tgz path on target instance ("" if not found)
	version            string // canonical build number (with -artifacts suffix)
	edition            string // community | enterprise | federal
	osVersion          string // never "latest"; resolved before return
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

	jfArch := "x86_64"
	if isARMArch(arch) {
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

	// The JFrog server .deb/.rpm installs only the server (asd); it does not
	// bundle the tools the public .tgz flow gets via asinstall. Without
	// asinfo/asadm/aql, aerolab commands such as `roster apply`, `roster show`
	// and `aerospike-is-stable` fail on the resulting cluster. Resolve a
	// tools install (preferring this build, then latest on JFrog, then the
	// public channel) so JFrog templates stay interchangeable with public ones.
	toolsScript, toolsLocal, toolsRemote, err := resolveJFrogTools(cfg, files, cacheDir, osName, distroVersion, jfArch, debug, log)
	if err != nil {
		log.Warn("could not provision aerospike-tools from any source (this build, JFrog, or the public channel): %s; the cluster will lack asinfo/asadm/aql and commands such as `roster apply`, `roster show` and `aerospike-is-stable` will fail", err)
	} else if len(toolsScript) > 0 {
		pkgScript = append(pkgScript, toolsScript...)
	}

	// Wrap with the same "basic tools" optional dependency set the
	// public flow uses so the resulting templates are interchangeable.
	wrapped, err := installers.GetInstallScript(templateOptionalDeps(debug), pkgScript)
	if err != nil {
		return nil, fmt.Errorf("could not add basic tools to JFrog install script: %w", err)
	}

	return &jfrogPlan{
		script:             wrapped,
		pkgLocalPath:       local,
		pkgRemotePath:      jfrog.RemotePackagePath(match),
		toolsPkgLocalPath:  toolsLocal,
		toolsPkgRemotePath: toolsRemote,
		version:            build.Number,
		edition:            edition,
		osVersion:          distroVersion,
	}, nil
}

// resolveJFrogTools builds the aerospike-tools install portion of a JFrog
// template, trying the sources below in order of preference so a cluster
// always ends up with asinfo/asadm/aql:
//
//  1. the aerospike-tools .tgz attached to THIS build (same version);
//  2. the latest aerospike-tools .tgz anywhere on the JFrog instance;
//  3. the latest aerospike-tools from the public Aerospike channel.
//
// Sources 1 and 2 are JFrog artifacts the node may not be able to reach, so
// they are pre-downloaded to the operator's cache and returned via
// (localPath, remotePath) for the caller to SFTP-upload. Source 3 is fetched
// by the node itself at install time, so it returns empty paths. It returns
// an error only when all three sources fail.
func resolveJFrogTools(cfg *jfrog.Config, buildFiles jfrog.Files, cacheDir, osName, distroVersion, jfArch string, debug bool, log *logger.Logger) (script []byte, localPath, remotePath string, err error) {
	// 1: the tools shipped with this exact build.
	if m := buildFiles.MatchTools(jfrog.MatchCriteria{OSName: osName, OSVersion: distroVersion, Arch: jfArch}); m != nil {
		log.Info("Using aerospike-tools from this build: %s (%d bytes)", m.Name, m.Size)
		return downloadJFrogTools(cfg, m, cacheDir)
	}

	// 2: the latest tools anywhere on JFrog.
	log.Warn("this build has no matching aerospike-tools package for %s/%s/%s; searching JFrog for the latest tools", osName, distroVersion, jfArch)
	if m, e := cfg.LatestToolsFile(context.Background(), osName, distroVersion, jfArch); e != nil {
		log.Warn("could not query JFrog for the latest aerospike-tools: %s", e)
	} else if m != nil {
		log.Info("Using latest aerospike-tools from JFrog: %s (%d bytes)", m.Name, m.Size)
		return downloadJFrogTools(cfg, m, cacheDir)
	}

	// 3: the latest tools from the public download channel (node fetches it).
	log.Warn("no aerospike-tools found on JFrog for %s/%s/%s; falling back to the public Aerospike download channel", osName, distroVersion, jfArch)
	s, e := publicToolsInstallScript(osName, distroVersion, jfArch, debug)
	if e != nil {
		return nil, "", "", e
	}
	log.Info("Using latest aerospike-tools from the public Aerospike channel")
	return s, "", "", nil
}

// downloadJFrogTools caches a JFrog tools artifact locally and returns its
// install-script snippet plus the local/remote package paths for upload.
func downloadJFrogTools(cfg *jfrog.Config, m *jfrog.File, cacheDir string) (script []byte, localPath, remotePath string, err error) {
	localPath, err = cfg.Download(context.Background(), m, cacheDir)
	if err != nil {
		return nil, "", "", fmt.Errorf("could not download aerospike-tools package: %w", err)
	}
	script, err = jfrog.ToolsInstallScript(m, false)
	if err != nil {
		return nil, "", "", fmt.Errorf("could not build aerospike-tools install script: %w", err)
	}
	return script, localPath, jfrog.RemotePackagePath(m), nil
}

// publicToolsInstallScript resolves the latest aerospike-tools on the public
// download channel and returns an install script that downloads and installs
// it on the target node itself (no operator-side pre-download).
func publicToolsInstallScript(osName, distroVersion, jfArch string, debug bool) ([]byte, error) {
	products, err := aerospike.GetProducts(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not list public products: %w", err)
	}
	prod := products.WithName("aerospike-tools")
	if len(prod) == 0 {
		return nil, fmt.Errorf("no aerospike-tools product found on the public channel")
	}
	versions, err := aerospike.GetVersions(30*time.Second, prod[0])
	if err != nil {
		return nil, fmt.Errorf("could not list public aerospike-tools versions: %w", err)
	}
	version := versions.Latest()
	if version == nil {
		return nil, fmt.Errorf("no public aerospike-tools versions found")
	}
	files, err := aerospike.GetFiles(30*time.Second, *version)
	if err != nil {
		return nil, fmt.Errorf("could not list public aerospike-tools files: %w", err)
	}
	arch := aerospike.ArchitectureTypeX86_64
	if jfArch == "aarch64" {
		arch = aerospike.ArchitectureTypeAARCH64
	}
	script, err := files.GetInstallScript(arch, aerospike.OSName(osName), distroVersion, debug, true, true, true)
	if err != nil {
		return nil, fmt.Errorf("no public aerospike-tools package for %s/%s/%s: %w", osName, distroVersion, jfArch, err)
	}
	return script, nil
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
				// The rhel counterpart also pulls in openldap itself, which
				// provides the liblber.so.2 / libldap.so.2 that the aerospike
				// server rpm links against. RHEL 10 base images no longer ship
				// them, so without this the server rpm fails to install.
				{Command: "ldapsearch", Package: "openldap-clients"},
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
