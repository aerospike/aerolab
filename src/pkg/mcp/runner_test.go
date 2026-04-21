package mcp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildArgvBasic(t *testing.T) {
	argv, err := BuildArgv(RunInput{
		Path: "cluster/create",
		Args: map[string]any{
			"name":          "mydc",
			"count":         float64(3),
			"verbose":       true,
			"skip-download": false,
		},
	})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}

	got := strings.Join(argv, " ")
	// Deterministic order: path segments first, then flags alphabetical.
	want := "cluster create --count=3 --name=mydc --verbose"
	if got != want {
		t.Errorf("argv mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestBuildArgvSpaceSeparatedPath(t *testing.T) {
	argv, err := BuildArgv(RunInput{Path: "cluster create"})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	want := []string{"cluster", "create"}
	if len(argv) != 2 || argv[0] != want[0] || argv[1] != want[1] {
		t.Errorf("argv mismatch: got %v, want %v", argv, want)
	}
}

func TestBuildArgvSlices(t *testing.T) {
	argv, err := BuildArgv(RunInput{
		Path: "files/upload",
		Args: map[string]any{
			"path": []string{"a", "b"},
			"tag":  []any{"red", float64(3)},
		},
	})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "--path=a") || !strings.Contains(joined, "--path=b") {
		t.Errorf("expected repeated --path, got: %s", joined)
	}
	if !strings.Contains(joined, "--tag=red") || !strings.Contains(joined, "--tag=3") {
		t.Errorf("expected mixed slice elements, got: %s", joined)
	}
}

func TestBuildArgvPositional(t *testing.T) {
	argv, err := BuildArgv(RunInput{
		Path:       "attach/shell",
		Positional: []string{"--", "ls", "-la"},
	})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	want := "attach shell -- ls -la"
	if got := strings.Join(argv, " "); got != want {
		t.Errorf("argv: got %q, want %q", got, want)
	}
}

func TestBuildArgvRejectsDashSegment(t *testing.T) {
	_, err := BuildArgv(RunInput{Path: "cluster/--malicious"})
	if err == nil {
		t.Fatal("expected error for dash-prefixed path segment")
	}
}

func TestBuildArgvNilValueSkipped(t *testing.T) {
	argv, err := BuildArgv(RunInput{
		Path: "x",
		Args: map[string]any{"a": nil, "b": "v"},
	})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	if strings.Contains(strings.Join(argv, " "), "--a") {
		t.Errorf("expected nil-value flag to be skipped, got: %v", argv)
	}
}

func TestBuildArgvMapRejected(t *testing.T) {
	_, err := BuildArgv(RunInput{
		Path: "x",
		Args: map[string]any{"a": map[string]any{"nested": true}},
	})
	if err == nil {
		t.Fatal("expected error for map value")
	}
}

func TestLimitedBufferTruncation(t *testing.T) {
	b := &limitedBuffer{max: 5}
	_, _ = b.Write([]byte("hello"))
	if b.truncated {
		t.Fatal("expected no truncation after exactly max bytes")
	}
	_, _ = b.Write([]byte("world"))
	if !b.truncated {
		t.Fatal("expected truncation after exceeding max")
	}
	s := b.String()
	if !strings.HasPrefix(s, "hello") {
		t.Errorf("unexpected captured output: %q", s)
	}
	if !strings.Contains(s, "truncated") {
		t.Errorf("expected truncation marker, got: %q", s)
	}
}

// writeScript creates a shell script file for use as a test "binary" and
// returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestRunnerExecuteCapturesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-based test unsupported on windows")
	}
	script := writeScript(t, "echo hello\necho world >&2")
	r := &Runner{
		Binary:         script,
		DefaultTimeout: 5 * time.Second,
		MaxOutputBytes: 1024,
	}
	out := r.Execute(context.Background(), RunInput{})
	if out.Err != nil {
		t.Fatalf("Execute: %v (output: %s)", out.Err, out.Output)
	}
	if !strings.Contains(out.Output, "hello") || !strings.Contains(out.Output, "world") {
		t.Errorf("expected merged stdout+stderr, got: %q", out.Output)
	}
}

func TestRunnerExecuteTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep-based test unsupported on windows")
	}
	script := writeScript(t, "sleep 5")
	r := &Runner{
		Binary:         script,
		DefaultTimeout: 100 * time.Millisecond,
	}
	out := r.Execute(context.Background(), RunInput{})
	if !out.TimedOut {
		t.Fatalf("expected TimedOut=true, got err=%v", out.Err)
	}
	if out.Err == nil {
		t.Fatal("expected non-nil err on timeout")
	}
}

func TestRunnerExecuteNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	script := writeScript(t, "exit 7")
	r := &Runner{
		Binary:         script,
		DefaultTimeout: 5 * time.Second,
	}
	out := r.Execute(context.Background(), RunInput{})
	if out.Err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if out.ExitCode != 7 {
		t.Errorf("expected exit code 7, got %d", out.ExitCode)
	}
}

func TestRunnerExecuteMissingBinary(t *testing.T) {
	r := &Runner{}
	out := r.Execute(context.Background(), RunInput{Path: "x"})
	if out.Err == nil {
		t.Fatal("expected error when Binary empty")
	}
}

func TestRunnerEnvOverrideDeterministic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	// Print every env var starting with AEROLAB_MCP_TEST_ in the order
	// the subprocess receives them. Because the runner must merge the
	// override map in sorted key order, successive invocations with the
	// same map should produce the same output.
	script := writeScript(t, "env | grep '^AEROLAB_MCP_TEST_' | sort")
	r := &Runner{
		Binary:         script,
		DefaultTimeout: 5 * time.Second,
	}
	override := map[string]string{
		"AEROLAB_MCP_TEST_Z": "z",
		"AEROLAB_MCP_TEST_A": "a",
		"AEROLAB_MCP_TEST_M": "m",
	}
	first := r.Execute(context.Background(), RunInput{EnvOverride: override})
	if first.Err != nil {
		t.Fatalf("first Execute: %v", first.Err)
	}
	for i := 0; i < 5; i++ {
		next := r.Execute(context.Background(), RunInput{EnvOverride: override})
		if next.Err != nil {
			t.Fatalf("iter %d Execute: %v", i, next.Err)
		}
		if next.Output != first.Output {
			t.Fatalf("non-deterministic env order\nfirst=%q\niter %d=%q", first.Output, i, next.Output)
		}
	}
}

func TestRunnerExecuteEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	script := writeScript(t, "echo \"X=$AEROLAB_MCP_TEST\"")

	r := &Runner{
		Binary:         script,
		DefaultTimeout: 5 * time.Second,
		Env:            []string{"AEROLAB_MCP_TEST=from-runner"},
	}
	out := r.Execute(context.Background(), RunInput{})
	if out.Err != nil {
		t.Fatalf("Execute: %v (output: %s)", out.Err, out.Output)
	}
	if !strings.Contains(out.Output, "X=from-runner") {
		t.Errorf("expected env override, got: %q", out.Output)
	}

	out = r.Execute(context.Background(), RunInput{
		EnvOverride: map[string]string{"AEROLAB_MCP_TEST": "from-override"},
	})
	if out.Err != nil {
		t.Fatalf("Execute: %v", out.Err)
	}
	if !strings.Contains(out.Output, "X=from-override") {
		t.Errorf("expected per-call override to win, got: %q", out.Output)
	}
}
