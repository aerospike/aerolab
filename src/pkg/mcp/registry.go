package mcp

import (
	"strings"
)

// Registry is the in-process view of the aerolab command tree consumed by
// the MCP server. It is constructed by the caller (typically cmdMcp.go)
// before the server is started, so the tree, help renderer, runner, and
// gate are all fixed for the lifetime of the server.
//
// The server only consults the registry; the registry does not know about
// the MCP SDK. This separation keeps the registry unit-testable without a
// running transport.
type Registry struct {
	// Root is the top level of the aerolab command tree. It is typically
	// a single "aerolab" root node with children.
	Root []*Command

	// Help renders per-command help text without touching os.Stdout or
	// calling os.Exit.
	Help HelpRenderer

	// Run executes aerolab subprocesses for callers; the server never
	// forks commands itself.
	Run *Runner

	// Gate decides whether a given leaf may be executed at the current
	// profile, and whether the caller passed --confirm for destructive
	// tools. A nil gate is treated as an always-allow admin gate.
	Gate *ProfileGate
}

// Leaves returns every executable leaf (command node without children)
// below the given root, in depth-first order. The root(s) themselves are
// also returned when they have no children. Paths use slash separators.
func (r *Registry) Leaves() []*Command {
	var out []*Command
	for _, c := range r.Root {
		collectLeaves(c, &out)
	}
	return out
}

// Find returns the command for a slash-separated path. An empty path
// (or "/") returns the first registered root node when exactly one root
// is configured, which is the typical single-"aerolab" layout; when
// multiple roots are configured or the registry is empty, it returns
// nil. Otherwise the path is resolved against the roots.
func (r *Registry) Find(path string) *Command {
	segs := splitPath(path)
	if len(segs) == 0 {
		if len(r.Root) == 1 {
			return r.Root[0]
		}
		return nil
	}
	// The root array typically contains a single "aerolab" node; the
	// caller may pass a path relative to that root or including it. Try
	// both interpretations.
	if cmd := findIn(r.Root, segs); cmd != nil {
		return cmd
	}
	// Try treating the first path segment as relative to the first root's
	// children, if any.
	for _, root := range r.Root {
		if cmd := findIn(root.Children, segs); cmd != nil {
			return cmd
		}
	}
	return nil
}

func collectLeaves(c *Command, out *[]*Command) {
	if c == nil {
		return
	}
	if len(c.Children) == 0 {
		*out = append(*out, c)
		return
	}
	for _, child := range c.Children {
		collectLeaves(child, out)
	}
}

func findIn(nodes []*Command, segs []string) *Command {
	if len(segs) == 0 || len(nodes) == 0 {
		return nil
	}
	want := segs[0]
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.Name != want {
			continue
		}
		if len(segs) == 1 {
			return n
		}
		if c := findIn(n.Children, segs[1:]); c != nil {
			return c
		}
	}
	return nil
}

// JoinPath normalizes a slash-separated path by trimming leading/trailing
// slashes and replacing multiple slashes with a single separator.
func JoinPath(segs []string) string {
	var out []string
	for _, s := range segs {
		s = strings.Trim(s, "/ \t")
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return strings.Join(out, "/")
}
