package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Runner executes aerolab commands as subprocesses and returns the captured
// stdout+stderr (merged). It is safe to construct once and reuse across
// concurrent tool calls.
type Runner struct {
	// Binary is the path to the aerolab executable invoked for every call.
	// If empty, runner will return an error.
	Binary string

	// DefaultTimeout is applied to every call when the per-call override is
	// zero. A zero DefaultTimeout disables the timeout entirely.
	DefaultTimeout time.Duration

	// MaxOutputBytes caps the merged stdout+stderr capture per call. When
	// the subprocess exceeds this, the captured output is truncated with a
	// trailing "... output truncated" marker. A zero or negative value
	// disables the cap.
	MaxOutputBytes int

	// Env is an extra environment appended to the subprocess. Entries are
	// expected in "KEY=VALUE" form. The subprocess also inherits the
	// server's environment.
	Env []string
}

// RunInput describes a single invocation.
type RunInput struct {
	// Path is the slash- or space-separated command path below the aerolab
	// root (e.g. "cluster/create"). An empty path is treated as the root.
	Path string

	// Args maps long-flag names to values. Booleans set the flag when
	// true; slice values repeat the flag.
	Args map[string]any

	// Positional arguments appended after the flags (rarely used).
	Positional []string

	// TimeoutOverride overrides Runner.DefaultTimeout for this call.
	// Zero or negative values fall through to Runner.DefaultTimeout. To
	// explicitly disable the per-call timeout, set TimeoutOverride to 0
	// and ensure Runner.DefaultTimeout is also 0.
	TimeoutOverride time.Duration

	// EnvOverride is merged with (and overrides) Runner.Env for this call.
	EnvOverride map[string]string
}

// RunOutput is the captured result of a subprocess invocation. Output is
// always non-nil (possibly empty). Err is non-nil when the subprocess could
// not start, was killed by the timeout, or exited with a non-zero status.
type RunOutput struct {
	Argv      []string
	Output    string
	Truncated bool
	ExitCode  int
	TimedOut  bool
	Err       error
}

// Execute runs the configured binary with argv derived from input and
// captures merged stdout+stderr (optionally truncated). It respects ctx
// cancellation and the effective timeout.
func (r *Runner) Execute(ctx context.Context, input RunInput) *RunOutput {
	out := &RunOutput{}
	if r == nil || r.Binary == "" {
		out.Err = errors.New("mcp: runner not configured (missing binary path)")
		return out
	}

	argv, err := BuildArgv(input)
	if err != nil {
		out.Err = err
		return out
	}
	out.Argv = argv

	effective := r.effectiveTimeout(input.TimeoutOverride)
	runCtx := ctx
	var cancel context.CancelFunc
	if effective > 0 {
		runCtx, cancel = context.WithTimeout(ctx, effective)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, r.Binary, argv...)

	// Merge env. Setting cmd.Env to anything non-nil fully replaces the
	// parent environment for the subprocess, so start from os.Environ().
	// EnvOverride is walked in sorted key order so repeated calls with
	// the same override produce a deterministic cmd.Env (important for
	// subprocess reproducibility and snapshot tests).
	if len(r.Env) > 0 || len(input.EnvOverride) > 0 {
		merged := append([]string{}, os.Environ()...)
		merged = append(merged, r.Env...)
		overrideKeys := make([]string, 0, len(input.EnvOverride))
		for k := range input.EnvOverride {
			overrideKeys = append(overrideKeys, k)
		}
		sort.Strings(overrideKeys)
		for _, k := range overrideKeys {
			merged = append(merged, k+"="+input.EnvOverride[k])
		}
		cmd.Env = merged
	}

	var w io.Writer
	buf := &limitedBuffer{max: r.MaxOutputBytes}
	w = buf
	cmd.Stdout = w
	cmd.Stderr = w

	runErr := cmd.Run()

	out.Output = buf.String()
	out.Truncated = buf.truncated

	if runCtx.Err() == context.DeadlineExceeded {
		out.TimedOut = true
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			out.ExitCode = exitErr.ExitCode()
			out.Err = fmt.Errorf("aerolab %s exited with code %d", strings.Join(argv, " "), out.ExitCode)
		} else if out.TimedOut {
			out.Err = fmt.Errorf("aerolab %s timed out after %s", strings.Join(argv, " "), effective)
		} else {
			out.Err = fmt.Errorf("aerolab %s: %w", strings.Join(argv, " "), runErr)
		}
	}

	return out
}

// effectiveTimeout resolves the per-call timeout. A positive override
// wins; zero or negative overrides fall through to Runner.DefaultTimeout.
// A zero DefaultTimeout disables the timeout entirely.
func (r *Runner) effectiveTimeout(override time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	if r.DefaultTimeout > 0 {
		return r.DefaultTimeout
	}
	return 0
}

// BuildArgv converts a RunInput into a subprocess argument vector. It
// validates basic input shape and returns an error for malformed paths or
// unsupported argument values.
//
// Argument formatting rules:
//   - Bool values: true -> "--<flag>", false -> omitted.
//   - Slice values: the flag is repeated once per element.
//   - Nil values are skipped.
//   - All other scalars are formatted via fmt.Sprintf("%v", v).
//
// Argument keys are emitted in stable (alphabetical) order so snapshots are
// deterministic and tests don't flake.
func BuildArgv(input RunInput) ([]string, error) {
	segs := splitPath(input.Path)

	for _, s := range segs {
		if s == "" {
			return nil, fmt.Errorf("mcp: invalid command path %q (empty segment)", input.Path)
		}
		if strings.HasPrefix(s, "-") {
			return nil, fmt.Errorf("mcp: invalid command path %q (segment starts with '-')", input.Path)
		}
	}

	argv := append([]string{}, segs...)

	// Stable ordering for determinism.
	keys := make([]string, 0, len(input.Args))
	for k := range input.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := input.Args[k]
		if v == nil {
			continue
		}
		flag := "--" + k

		switch val := v.(type) {
		case bool:
			if val {
				argv = append(argv, flag)
			}
		case []any:
			for _, item := range val {
				s, err := formatScalar(item)
				if err != nil {
					return nil, fmt.Errorf("mcp: flag %q: %w", k, err)
				}
				argv = append(argv, flag+"="+s)
			}
		case []string:
			for _, item := range val {
				argv = append(argv, flag+"="+item)
			}
		default:
			s, err := formatScalar(v)
			if err != nil {
				return nil, fmt.Errorf("mcp: flag %q: %w", k, err)
			}
			argv = append(argv, flag+"="+s)
		}
	}

	argv = append(argv, input.Positional...)
	return argv, nil
}

func formatScalar(v any) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case bool:
		return strconv.FormatBool(val), nil
	case int:
		return strconv.Itoa(val), nil
	case int32:
		return strconv.FormatInt(int64(val), 10), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case uint:
		return strconv.FormatUint(uint64(val), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(val), 10), nil
	case uint64:
		return strconv.FormatUint(val, 10), nil
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32), nil
	case float64:
		// Avoid printing "1e+06" for integer-valued floats (which JSON
		// always unmarshals to float64).
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10), nil
		}
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case nil:
		return "", errors.New("nil value")
	case map[string]any, map[string]string:
		return "", fmt.Errorf("unsupported argument type %T (maps are not valid CLI values)", val)
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// splitPath accepts slash-separated, space-separated, or pre-split paths and
// normalizes them into a slice of non-empty segments.
func splitPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	fields := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == ' ' || r == '\t'
	})
	return fields
}

// limitedBuffer is an io.Writer that caps how many bytes it retains. Writes
// beyond the cap are dropped but their count is tracked in totalWritten so
// callers can report truncation.
type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return b.buf.Write(p)
	}
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) <= remaining {
		return b.buf.Write(p)
	}
	_, _ = b.buf.Write(p[:remaining])
	b.truncated = true
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	s := b.buf.String()
	if b.truncated {
		s += "\n... output truncated"
	}
	return s
}
