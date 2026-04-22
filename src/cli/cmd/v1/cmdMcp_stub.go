//go:build noaerolabmcp

package cmd

import "errors"

// McpCmd is a build-tag stub used when aerolab is compiled with
// `-tags noaerolabmcp`. The real implementation, and its dependency on
// pkg/mcp plus the reflection helpers in cmdWebUIReflect.go, lives in
// cmdMcp.go and is excluded under that tag. Embedders (e.g. aerospike
// voyager) use noaerolabmcp to strip the MCP server and its reflection
// machinery out of their build.
type McpCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute returns a clear error explaining that MCP support was stripped
// from this build via the noaerolabmcp tag, instead of silently falling
// through to a no-op. Matches the real Execute signature so go-flags can
// still dispatch `aerolab mcp` into this stub.
func (c *McpCmd) Execute(args []string) error {
	return errors.New("MCP subcommand is not available: this build was compiled with -tags noaerolabmcp")
}
