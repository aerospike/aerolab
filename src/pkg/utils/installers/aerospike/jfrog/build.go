package jfrog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Build identifies a single JFrog build run.
type Build struct {
	cfg    *Config
	Name   string
	Number string // canonical, always with the "-artifacts" suffix
}

// ResolveBuild normalises the user-supplied -v string into a Build that
// can be queried for artifacts. JFrog stores build runs both with and
// without the "-artifacts" suffix (the no-suffix entry is just an alias),
// so we always append the suffix if missing to get the canonical form.
//
// No `latest` resolution is performed: JFrog dev builds are not assumed
// to expose a stable "latest" alias.
func (c *Config) ResolveBuild(version string) (*Build, error) {
	if c == nil {
		return nil, fmt.Errorf("jfrog: nil config")
	}
	v := strings.TrimSpace(version)
	if v == "" {
		return nil, fmt.Errorf("jfrog: empty version")
	}
	if v == "latest" {
		return nil, fmt.Errorf("jfrog: 'latest' is not supported for JFrog dev builds")
	}
	v = strings.TrimSuffix(v, "-artifacts") + "-artifacts"
	return &Build{cfg: c, Name: c.BuildName, Number: v}, nil
}

// Files queries JFrog for every artifact attached to the build via AQL
// and returns the parsed result list.
func (b *Build) Files(ctx context.Context) (Files, error) {
	if b == nil || b.cfg == nil {
		return nil, fmt.Errorf("jfrog: nil build")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.cfg.timeout())
		defer cancel()
	}

	// items.find is the non-admin-friendly form; @build.name / @build.number
	// is the fast index lookup (vs the slower artifact.module.build.* path).
	query := fmt.Sprintf(
		`items.find({"@build.name":"%s","@build.number":"%s"}).include("repo","path","name","size","actual_sha1","created")`,
		jsonEscape(b.Name), jsonEscape(b.Number),
	)
	raw, err := b.cfg.AQL(ctx, query)
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
		return nil, fmt.Errorf("jfrog: parse AQL response: %w", err)
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("jfrog: no artifacts found for build %q number %q", b.Name, b.Number)
	}

	files := make(Files, 0, len(resp.Results))
	for _, r := range resp.Results {
		files = append(files, File{
			Repo:        r.Repo,
			Path:        r.Path,
			Name:        r.Name,
			Size:        r.Size,
			SHA1:        r.SHA1,
			Created:     r.Created,
			DownloadURL: b.cfg.ArtifactoryURL("/" + r.Repo + "/" + r.Path + "/" + r.Name),
			Parts:       ParseFileName(r.Name),
		})
	}
	return files, nil
}

func jsonEscape(s string) string {
	// minimal escape for use inside a "..." JSON string literal
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// BuildSummary is a single entry from "list builds for this name".
type BuildSummary struct {
	Name   string // build name, e.g. "aerospike-server"
	Number string // build number, e.g. "8.1.3.0-28-g302194ebc-artifacts"
	Repo   string // build-info repo, e.g. "database-build-info"
}

// BuildSummaries is an ordered list of build entries.
type BuildSummaries []BuildSummary

// artifactsSuffix marks the canonical build entry that has artifacts
// attached. Every run also publishes a non-suffixed alias pointing at the
// same artifacts.
const artifactsSuffix = "-artifacts"

// OnlyArtifacts keeps only the canonical entries whose build number ends
// with "-artifacts" (the ones that actually have downloadable packages),
// preserving order.
func (b BuildSummaries) OnlyArtifacts() BuildSummaries {
	out := make(BuildSummaries, 0, len(b))
	for _, s := range b {
		if strings.HasSuffix(s.Number, artifactsSuffix) {
			out = append(out, s)
		}
	}
	return out
}

// listBuildsLimit caps the number of build numbers returned by a single
// AQL query. JFrog's default limit is unbounded which can be expensive.
const listBuildsLimit = 5000

// ListBuilds returns build numbers registered under cfg.BuildName via the
// AQL builds domain:
//
//	builds.find({"name":"<name>"[,"number":{"$match":"<glob>*"}]})
//	      .include("name","number","repo")
//	      .sort({"$desc":["number"]})
//	      .limit(100)
//
// numberMatch is an optional build-number prefix; when non-empty a
// trailing "*" is appended so JFrog treats it as a glob. Results are
// returned in JFrog's descending-by-number order.
func (c *Config) ListBuilds(ctx context.Context, numberMatch string) (BuildSummaries, error) {
	if c == nil {
		return nil, fmt.Errorf("jfrog: nil config")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout())
		defer cancel()
	}

	find := fmt.Sprintf(`"name":"%s"`, jsonEscape(c.BuildName))
	if m := strings.TrimSpace(numberMatch); m != "" {
		glob := strings.TrimSuffix(m, "*") + "*"
		find += fmt.Sprintf(`,"number":{"$match":"%s"}`, jsonEscape(glob))
	}
	query := fmt.Sprintf(
		`builds.find({%s}).include("name","number","repo").sort({"$desc":["number"]}).limit(%d)`,
		find, listBuildsLimit,
	)

	raw, err := c.AQL(ctx, query)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []struct {
			Name   string `json:"build.name"`
			Number string `json:"build.number"`
			Repo   string `json:"build.repo"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("jfrog: parse build list: %w", err)
	}

	out := make(BuildSummaries, 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.Number == "" {
			continue
		}
		out = append(out, BuildSummary{Name: r.Name, Number: r.Number, Repo: r.Repo})
	}
	return out, nil
}
