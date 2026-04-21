package mcp

import (
	"strings"
	"testing"
)

// testOpts is a small go-flags-compatible command tree used to exercise the
// help renderer without depending on the cmd package.
type testOpts struct {
	Verbose bool       `short:"v" long:"verbose" description:"enable verbose logging"`
	Nest    nestedCmd  `command:"nest" description:"a nested command with a subcommand"`
	Version versionCmd `command:"version" description:"print the version"`
}

type nestedCmd struct {
	Name string    `short:"n" long:"name" description:"thing to greet" default:"world"`
	Sub  greetSub  `command:"sub" description:"deeply nested greet"`
	Help stubHelp `command:"help" subcommands-optional:"true" description:"Print help"`
}

type greetSub struct {
	Times int `long:"times" description:"repetition count" default:"1"`
}

type versionCmd struct {
	Long bool `long:"long" description:"verbose form"`
}

type stubHelp struct{}

func (stubHelp) Execute([]string) error { return nil }

func newTestOpts() any { return &testOpts{} }

func TestRenderHelpFromFactoryRoot(t *testing.T) {
	r := RenderHelpFromFactory(newTestOpts)
	out, err := r(nil)
	if err != nil {
		t.Fatalf("RenderHelp: %v", err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected 'Usage:' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "nest") || !strings.Contains(out, "version") {
		t.Errorf("expected subcommand names in root help, got:\n%s", out)
	}
}

func TestRenderHelpFromFactorySubcommand(t *testing.T) {
	r := RenderHelpFromFactory(newTestOpts)
	out, err := r([]string{"nest"})
	if err != nil {
		t.Fatalf("RenderHelp: %v", err)
	}
	if !strings.Contains(out, "nest") || !strings.Contains(out, "--name") {
		t.Errorf("expected subcommand name and --name flag, got:\n%s", out)
	}
	// Must surface the subcommand's own flag (not just root flags).
	if !strings.Contains(out, "thing to greet") {
		t.Errorf("expected flag description, got:\n%s", out)
	}
}

func TestRenderHelpFromFactoryDeeplyNested(t *testing.T) {
	r := RenderHelpFromFactory(newTestOpts)
	out, err := r([]string{"nest", "sub"})
	if err != nil {
		t.Fatalf("RenderHelp: %v", err)
	}
	if !strings.Contains(out, "--times") {
		t.Errorf("expected --times flag for deeply nested command, got:\n%s", out)
	}
}

func TestRenderHelpFromFactoryUnknownFallsBack(t *testing.T) {
	r := RenderHelpFromFactory(newTestOpts)
	out, err := r([]string{"does-not-exist"})
	if err != nil {
		t.Fatalf("RenderHelp: %v", err)
	}
	// Should fall back to root help.
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected fallback root help, got:\n%s", out)
	}
}

func TestRenderHelpFromFactoryNilFactory(t *testing.T) {
	r := RenderHelpFromFactory(nil)
	_, err := r(nil)
	if err == nil {
		t.Fatal("expected error for nil factory")
	}
}

func TestRenderHelpFromFactoryNilResult(t *testing.T) {
	r := RenderHelpFromFactory(func() any { return nil })
	_, err := r(nil)
	if err == nil {
		t.Fatal("expected error for factory returning nil")
	}
}
