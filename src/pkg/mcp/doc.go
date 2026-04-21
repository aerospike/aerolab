// Package mcp implements the aerolab MCP (Model Context Protocol) server.
//
// The server exposes every aerolab CLI command as an MCP tool via self
// introspection of the existing command tree (BuildCommandTree /
// ParameterInfo). It supports two transports:
//
//   - stdio: for local agents (Claude Desktop, Cursor, etc.) that spawn the
//     aerolab binary as a subprocess and speak MCP over stdin/stdout.
//   - http:  streamable HTTP on a host:port, for remote agents. Optional
//     bearer-token auth.
//
// The hybrid tool strategy registers three generic explorer tools
// (list_commands, describe_command, execute_command) plus one auto-registered
// tool per CLI leaf subcommand. All execution paths fork the aerolab binary
// as a subprocess and capture merged stdout+stderr.
//
// Packages:
//
//   - schema.go       - ParameterInfo to JSON Schema conversion.
//   - help.go         - Programmatic help text rendering.
//   - runner.go       - Subprocess argv builder and executor.
//   - tools_generic.go - Generic list/describe/execute tools.
//   - tools_auto.go   - Per-leaf auto-registration.
//   - gate.go         - Destructive-operation detection and profile gating.
//   - server.go       - MCP server wiring and transport selection.
//   - auth.go         - Bearer middleware for HTTP transport.
package mcp
