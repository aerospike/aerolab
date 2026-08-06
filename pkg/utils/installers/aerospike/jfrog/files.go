package jfrog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// File is a single artifact resolved from a JFrog build.
type File struct {
	Repo        string
	Path        string
	Name        string
	Size        int64
	SHA1        string
	Created     time.Time
	DownloadURL string
	Parts       *NameParts // nil for signatures, source archives, etc.
}

// Files is a list of File with picker helpers.
type Files []File

// MatchCriteria describes the install target we want a package for.
//
// OSName  : "amazon" | "centos" | "debian" | "ubuntu" (post-translation)
// OSVersion: e.g. "2023", "9", "12", "24.04"
// Arch    : "x86_64" | "aarch64" (after debArch normalisation)
// Edition : "community" | "enterprise" | "federal"
type MatchCriteria struct {
	OSName    string
	OSVersion string
	Arch      string
	Edition   string
}

// Match returns the single File matching the criteria. The native package
// format (rpm vs deb) is implied by OSName: amazon/centos use rpm,
// debian/ubuntu use deb. Builds that ship the self-contained
// "aerospike-server-<edition>_<version>_<osTag>_<arch>.tgz" bundle instead of
// (or as well as) loose packages are handled too — the native package is
// preferred and the bundle is the fallback, since both install the same
// server via the same OS package underneath.
func (fs Files) Match(c MatchCriteria) (*File, error) {
	wantFormat := formatForOS(c.OSName)
	if wantFormat == "" {
		return nil, fmt.Errorf("jfrog: unsupported OS %q (only amazon/centos/debian/ubuntu have JFrog packages)", c.OSName)
	}

	for _, format := range []string{wantFormat, "tgz"} {
		for i := range fs {
			f := &fs[i]
			if f.Parts == nil {
				continue
			}
			if f.Parts.Format != format {
				continue
			}
			if f.Parts.Edition != c.Edition {
				continue
			}
			if f.Parts.OSName != c.OSName {
				continue
			}
			if f.Parts.OSVersion != c.OSVersion {
				continue
			}
			if f.Parts.Arch != c.Arch {
				continue
			}
			return f, nil
		}
	}
	return nil, fs.noMatchError(c, wantFormat)
}

// diagSampleSize caps how many artifact names a failed match reports. A
// build carries hundreds of artifacts; a sample is enough to tell whether the
// package is missing, named unexpectedly, or hidden by repository
// permissions, without burying the error.
const diagSampleSize = 15

// noMatchError explains a failed Match. When packages for the requested
// edition exist it lists them; when none do it samples what the build
// actually carried, because that is the only place the operator can see it —
// the artifact list is fetched, matched and discarded in one call.
func (fs Files) noMatchError(c MatchCriteria, wantFormat string) error {
	var candidates, others []string
	for i := range fs {
		f := &fs[i]
		if f.Parts != nil && f.Parts.Edition == c.Edition &&
			(f.Parts.Format == wantFormat || f.Parts.Format == "tgz") {
			candidates = append(candidates, fmt.Sprintf("%s/%s/%s",
				f.Parts.OSName+f.Parts.OSVersion, f.Parts.Arch, f.Name))
			continue
		}
		others = append(others, f.pathName())
	}
	if len(candidates) > 0 {
		return fmt.Errorf(
			"jfrog: no %s %s package matches %s %s %s; available %s candidates: %v",
			c.Edition, wantFormat, c.OSName, c.OSVersion, c.Arch, c.Edition, candidates)
	}
	tag := osTag(c.OSName, c.OSVersion)
	debArchName := "amd64"
	if c.Arch == "aarch64" {
		debArchName = "arm64"
	}
	example := fmt.Sprintf("aerospike-server-%s-<version>-<release>.%s.%s.rpm", c.Edition, tag, c.Arch)
	if wantFormat == "deb" {
		example = fmt.Sprintf("aerospike-server-%s_<version>-<release>%s_%s.deb", c.Edition, tag, debArchName)
	}
	return fmt.Errorf(
		"jfrog: no %s %s (or .tgz bundle) package found for %s/%s/%s; "+
			"none of the %d artifacts on this build parsed as an %s server package. "+
			"Expected a name such as %s or aerospike-server-%s_<version>_%s_%s.tgz. "+
			"If the build genuinely has none, the package may live in a repository your "+
			"JFrog credentials cannot read (AQL silently omits those). Artifacts on the build: %s",
		c.Edition, wantFormat, c.OSName, c.OSVersion, c.Arch,
		len(fs), c.Edition,
		example, c.Edition, tag, c.Arch,
		sample(others, diagSampleSize))
}

// pathName renders a file as "<path>/<name>" so the sample also shows the
// repository layout (some pipelines encode the distro in the path rather
// than the filename).
func (f *File) pathName() string {
	if f.Path == "" {
		return f.Name
	}
	return strings.TrimSuffix(f.Path, "/") + "/" + f.Name
}

// sample renders at most max entries of in, noting how many were elided.
func sample(in []string, max int) string {
	if len(in) == 0 {
		return "(none)"
	}
	if len(in) <= max {
		return strings.Join(in, ", ")
	}
	return fmt.Sprintf("%s, ... (+%d more)", strings.Join(in[:max], ", "), len(in)-max)
}

// MatchTools returns the "aerospike-tools_*.tgz" artifact matching the OS
// and architecture in c. Edition and package format are ignored — the tools
// bundle is edition-agnostic and always shipped as a .tgz. Returns nil when
// the build has no matching tools package, so callers can fall back to a
// server-only install (and warn the operator).
func (fs Files) MatchTools(c MatchCriteria) *File {
	for i := range fs {
		f := &fs[i]
		tp := ParseToolsFileName(f.Name)
		if tp == nil {
			continue
		}
		if tp.OSName != c.OSName || tp.OSVersion != c.OSVersion || tp.Arch != c.Arch {
			continue
		}
		return f
	}
	return nil
}

// LatestToolsFile searches the whole JFrog instance (not just the current
// build) for the most recently created "aerospike-tools_*.tgz" matching the
// given OS and architecture. It is the second-preference fallback used when
// the resolved build has no matching tools artifact of its own. Returns
// nil (no error) when nothing matches.
func (c *Config) LatestToolsFile(ctx context.Context, osName, osVersion, arch string) (*File, error) {
	if c == nil {
		return nil, fmt.Errorf("jfrog: nil config")
	}
	tag := osTag(osName, osVersion)
	if tag == "" {
		return nil, fmt.Errorf("jfrog: unsupported OS %q for tools lookup", osName)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout())
		defer cancel()
	}

	glob := fmt.Sprintf("aerospike-tools_*_%s_%s.tgz", tag, arch)
	query := fmt.Sprintf(
		`items.find({"name":{"$match":"%s"}}).include("repo","path","name","size","actual_sha1","created").sort({"$desc":["created"]}).limit(50)`,
		jsonEscape(glob),
	)
	raw, err := c.AQL(ctx, query)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Results []struct {
			Repo    string    `json:"repo"`
			Path    string    `json:"path"`
			Name    string    `json:"name"`
			Size    int64     `json:"size"`
			SHA1    string    `json:"actual_sha1"`
			Created time.Time `json:"created"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("jfrog: parse tools AQL response: %w", err)
	}
	// Results are created-desc; return the first that actually parses and
	// matches (the glob is a coarse filter, ParseToolsFileName is exact).
	for _, r := range resp.Results {
		tp := ParseToolsFileName(r.Name)
		if tp == nil || tp.OSName != osName || tp.OSVersion != osVersion || tp.Arch != arch {
			continue
		}
		return &File{
			Repo:        r.Repo,
			Path:        r.Path,
			Name:        r.Name,
			Size:        r.Size,
			SHA1:        r.SHA1,
			Created:     r.Created,
			DownloadURL: c.ArtifactoryURL("/" + r.Repo + "/" + r.Path + "/" + r.Name),
			Parts:       ParseFileName(r.Name),
		}, nil
	}
	return nil, nil
}

// formatForOS returns the package format JFrog publishes for a given OS.
func formatForOS(osName string) string {
	switch osName {
	case "amazon", "centos", "rocky":
		return "rpm"
	case "debian", "ubuntu":
		return "deb"
	}
	return ""
}
