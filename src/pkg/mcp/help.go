package mcp

import (
	"bytes"
	"errors"

	flags "github.com/rglonek/go-flags"
)

// HelpRenderer renders the CLI help text for a command path (slash- or
// space-separated path segments). Implementations must not write to the
// real stdout/stderr or call os.Exit.
type HelpRenderer func(path []string) (string, error)

// OptionsFactory returns a freshly allocated root options struct, typically
// &cmd.Commands{}. Using a factory lets pkg/mcp render help without
// importing the cmd package (which would create a circular dependency).
type OptionsFactory func() any

// RenderHelpFromFactory returns a HelpRenderer that instantiates a fresh
// options struct per call, parses the command path with go-flags, and
// captures the parser's help output to a buffer.
//
// Unlike cmd.PrintHelp, this path never calls os.Exit and never prints to the
// real stdout/stderr. It is safe to call concurrently.
//
// Behavior:
//   - An empty path produces root-level help.
//   - Paths pointing to a subcommand produce that subcommand's help.
//   - Unknown paths fall back to root help with no error (useful for tool
//     descriptions).
func RenderHelpFromFactory(factory OptionsFactory) HelpRenderer {
	return func(path []string) (string, error) {
		if factory == nil {
			return "", errors.New("mcp: help renderer missing options factory")
		}
		opts := factory()
		if opts == nil {
			return "", errors.New("mcp: help renderer factory returned nil")
		}
		// HelpFlag produces ErrHelp when -h is encountered (we rely on that
		// to set the active subcommand). PassDoubleDash keeps positional
		// args intact. We intentionally omit PrintErrors so go-flags does
		// not touch os.Stderr.
		parser := flags.NewParser(opts, flags.HelpFlag|flags.PassDoubleDash)

		args := append(append([]string{}, path...), "-h")
		if _, err := parser.ParseArgs(args); err != nil {
			// Expected error path: flags.ErrHelp. Anything else (unknown
			// command, invalid flag) is swallowed so we can still render
			// the root help for the caller.
			if ferr, ok := err.(*flags.Error); !ok || ferr.Type != flags.ErrHelp {
				// Retry at root level so the caller gets something useful.
				rootParser := flags.NewParser(factory(), flags.HelpFlag|flags.PassDoubleDash)
				_, _ = rootParser.ParseArgs([]string{"-h"})
				parser = rootParser
			}
		}

		var buf bytes.Buffer
		parser.WriteHelp(&buf)
		return buf.String(), nil
	}
}
