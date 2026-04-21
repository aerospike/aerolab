package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Generic tool names exposed by every aerolab MCP server. Keeping them as
// constants makes it easier to refer to them in docs and tests.
const (
	ToolListCommands    = "aerolab_list_commands"
	ToolDescribeCommand = "aerolab_describe_command"
	ToolExecuteCommand  = "aerolab_execute_command"
)

// RegisterGenericTools wires the three hybrid explorer tools onto the
// server:
//
//   - aerolab_list_commands    -> walk the command tree
//   - aerolab_describe_command -> schema + help for a path
//   - aerolab_execute_command  -> catch-all subprocess runner
//
// These tools are always registered, regardless of the profile. The
// profile gate is enforced inside execute_command.
func RegisterGenericTools(server *sdkmcp.Server, reg *Registry) {
	RegisterGenericToolsWith(server, reg, nil)
}

// RegisterGenericToolsWith is identical to RegisterGenericTools but
// routes through a shared "seen" set so duplicate tool names across
// callers are detected instead of silently overwritten.
func RegisterGenericToolsWith(server *sdkmcp.Server, reg *Registry, seen map[string]struct{}) {
	if server == nil || reg == nil {
		return
	}
	addUnique(server, seen, nil, listCommandsTool(), listCommandsHandler(reg))
	addUnique(server, seen, nil, describeCommandTool(), describeCommandHandler(reg))
	addUnique(server, seen, nil, executeCommandTool(), executeCommandHandler(reg))
}

// ----- aerolab_list_commands -----

func listCommandsTool() *sdkmcp.Tool {
	return &sdkmcp.Tool{
		Name: ToolListCommands,
		Description: "List every aerolab CLI command available to this MCP server. " +
			"Returns a flat list of leaf command paths (e.g. 'cluster/create', " +
			"'inventory/list'), optionally filtered by a prefix. Use this tool first " +
			"to discover what aerolab can do before calling describe_command or " +
			"execute_command. Read-only and idempotent.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"prefix":{"type":"string","description":"Optional path prefix filter (e.g. 'cluster/' to list only cluster subcommands)."},
				"includeDestructive":{"type":"boolean","description":"When false, destructive commands are omitted. Default true."}
			}
		}`),
		Annotations: &sdkmcp.ToolAnnotations{
			Title:          "List aerolab commands",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(false),
		},
	}
}

type listCommandsArgs struct {
	Prefix             string `json:"prefix,omitempty"`
	IncludeDestructive *bool  `json:"includeDestructive,omitempty"`
}

func listCommandsHandler(reg *Registry) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args listCommandsArgs
		if err := unmarshalArgs(req, &args); err != nil {
			return errorResult(err), nil
		}

		prefix := strings.Trim(args.Prefix, "/")
		includeDestructive := true
		if args.IncludeDestructive != nil {
			includeDestructive = *args.IncludeDestructive
		}

		leaves := reg.Leaves()
		type row struct {
			Path        string `json:"path"`
			Description string `json:"description"`
			Destructive bool   `json:"destructive"`
		}
		rows := make([]row, 0, len(leaves))
		for _, cmd := range leaves {
			if cmd == nil || cmd.Hidden {
				continue
			}
			if isHelpLeaf(cmd) {
				continue
			}
			if prefix != "" && !strings.HasPrefix(cmd.Path, prefix) {
				continue
			}
			destr := IsDestructive(cmd)
			if destr && !includeDestructive {
				continue
			}
			rows = append(rows, row{
				Path:        cmd.Path,
				Description: cmd.Description,
				Destructive: destr,
			})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })

		var b strings.Builder
		fmt.Fprintf(&b, "aerolab commands (%d):\n", len(rows))
		for _, r := range rows {
			if r.Destructive {
				fmt.Fprintf(&b, "  [DESTRUCTIVE] %s — %s\n", r.Path, r.Description)
			} else {
				fmt.Fprintf(&b, "  %s — %s\n", r.Path, r.Description)
			}
		}

		return &sdkmcp.CallToolResult{
			Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: b.String()}},
			StructuredContent: map[string]any{"commands": rows},
		}, nil
	}
}

// ----- aerolab_describe_command -----

func describeCommandTool() *sdkmcp.Tool {
	return &sdkmcp.Tool{
		Name: ToolDescribeCommand,
		Description: "Describe a single aerolab command: its purpose, destructive flag, " +
			"JSON input schema (flag name/type/default/choices), and the verbatim CLI " +
			"help text. Call this before execute_command to understand what arguments " +
			"are required and what they mean. An empty path describes the root of the " +
			"command tree. Read-only and idempotent.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"path":{"type":"string","description":"Slash-separated command path (e.g. 'cluster/create'). Use aerolab_list_commands to discover paths. Empty or omitted to describe the root."}
			}
		}`),
		Annotations: &sdkmcp.ToolAnnotations{
			Title:          "Describe aerolab command",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(false),
		},
	}
}

type describeCommandArgs struct {
	Path string `json:"path"`
}

func describeCommandHandler(reg *Registry) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args describeCommandArgs
		if err := unmarshalArgs(req, &args); err != nil {
			return errorResult(err), nil
		}

		// Empty path is allowed when exactly one root is registered; it
		// describes the aerolab root (its children, top-level flags).
		cmd := reg.Find(args.Path)
		if cmd == nil {
			if args.Path == "" {
				return errorResult(errors.New("path is required (registry has no single root to describe)")), nil
			}
			return errorResult(fmt.Errorf("unknown command path %q (try aerolab_list_commands)", args.Path)), nil
		}

		schema, _ := BuildInputSchema(cmd.Parameters, nil)
		help := ""
		if reg.Help != nil {
			h, err := reg.Help(splitPath(args.Path))
			if err == nil {
				help = h
			}
		}

		structured := map[string]any{
			"path":        cmd.Path,
			"description": cmd.Description,
			"destructive": IsDestructive(cmd),
			"hidden":      cmd.Hidden,
			"inputSchema": schema,
			"help":        help,
		}

		var b strings.Builder
		fmt.Fprintf(&b, "# %s\n\n", cmd.Path)
		if cmd.Description != "" {
			fmt.Fprintf(&b, "%s\n\n", cmd.Description)
		}
		if IsDestructive(cmd) {
			b.WriteString("⚠ DESTRUCTIVE: this command mutates or removes resources. " +
				"In the 'standard' profile you must pass confirm=true to execute_command.\n\n")
		}
		if help != "" {
			b.WriteString("## CLI help\n\n")
			b.WriteString("```\n")
			b.WriteString(help)
			if !strings.HasSuffix(help, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("```\n")
		}

		return &sdkmcp.CallToolResult{
			Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: b.String()}},
			StructuredContent: structured,
		}, nil
	}
}

// ----- aerolab_execute_command -----

func executeCommandTool() *sdkmcp.Tool {
	schemaBytes, _ := json.Marshal(BuildGenericSchema())
	return &sdkmcp.Tool{
		Name: ToolExecuteCommand,
		Description: "Execute any aerolab CLI command by path, with arbitrary flag " +
			"arguments. The server forks the aerolab binary as a subprocess and " +
			"returns merged stdout+stderr. Use aerolab_describe_command first to " +
			"discover flag names, types, defaults, and choices. " +
			"Destructive commands require confirm=true in the 'standard' profile and " +
			"are rejected outright in 'read-only'. " +
			"Prefer the per-command auto-generated tools (e.g. 'aerolab_cluster_create') when " +
			"you know the exact command — they expose a strict schema. This tool is " +
			"the generic escape hatch.",
		InputSchema: json.RawMessage(schemaBytes),
		Annotations: &sdkmcp.ToolAnnotations{
			Title:          "Execute aerolab command",
			OpenWorldHint:  ptr(true),
			IdempotentHint: false,
		},
	}
}

type executeCommandArgs struct {
	Path       string            `json:"path"`
	Args       map[string]any    `json:"args,omitempty"`
	Positional []string          `json:"positional,omitempty"`
	TimeoutSec int               `json:"timeout_sec,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Confirm    bool              `json:"confirm,omitempty"`
}

func executeCommandHandler(reg *Registry) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args executeCommandArgs
		if err := unmarshalArgs(req, &args); err != nil {
			return errorResult(err), nil
		}
		if args.Path == "" {
			return errorResult(errors.New("path is required")), nil
		}

		cmd := reg.Find(args.Path)
		if cmd == nil {
			return errorResult(fmt.Errorf("unknown command path %q", args.Path)), nil
		}
		if err := reg.Gate.Check(cmd, args.Confirm); err != nil {
			return errorResult(err), nil
		}

		if reg.Run == nil {
			return errorResult(errors.New("runner not configured")), nil
		}

		input := RunInput{
			Path:            args.Path,
			Args:            args.Args,
			Positional:      args.Positional,
			TimeoutOverride: time.Duration(args.TimeoutSec) * time.Second,
			EnvOverride:     args.Env,
		}
		out := reg.Run.Execute(ctx, input)
		return runOutputToResult(out), nil
	}
}

// ----- helpers -----

func unmarshalArgs(req *sdkmcp.CallToolRequest, into any) error {
	if req == nil || req.Params == nil {
		return nil
	}
	if len(req.Params.Arguments) == 0 {
		return nil
	}
	return json.Unmarshal(req.Params.Arguments, into)
}

// runOutputToResult converts a subprocess result into an MCP tool result.
// Non-nil errors set IsError=true; the merged output is always placed in a
// TextContent block so the LLM can reason about partial output.
func runOutputToResult(out *RunOutput) *sdkmcp.CallToolResult {
	if out == nil {
		return errorResult(errors.New("runner returned nil output"))
	}
	var b strings.Builder
	if len(out.Argv) > 0 {
		fmt.Fprintf(&b, "$ aerolab %s\n", strings.Join(out.Argv, " "))
	}
	if out.Output != "" {
		b.WriteString(out.Output)
		if !strings.HasSuffix(out.Output, "\n") {
			b.WriteString("\n")
		}
	}
	if out.TimedOut {
		b.WriteString("(timed out)\n")
	}
	if out.Err != nil {
		fmt.Fprintf(&b, "error: %s\n", out.Err)
	}
	if out.ExitCode != 0 {
		fmt.Fprintf(&b, "(exit code %d)\n", out.ExitCode)
	}
	structured := map[string]any{
		"argv":      out.Argv,
		"output":    out.Output,
		"truncated": out.Truncated,
		"exitCode":  out.ExitCode,
		"timedOut":  out.TimedOut,
	}
	return &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: b.String()}},
		StructuredContent: structured,
		IsError:           out.Err != nil,
	}
}

// errorResult produces a CallToolResult with IsError=true and the given
// error message as the only text content. This mirrors the pattern used
// across the SDK examples.
func errorResult(err error) *sdkmcp.CallToolResult {
	msg := "error"
	if err != nil {
		msg = err.Error()
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// ptr returns a pointer to any non-nil value; used for SDK annotation
// fields that take *bool.
func ptr[T any](v T) *T { return &v }
