package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAutoTools registers one specific MCP tool per leaf command in
// the registry, in addition to the three generic tools. The auto tools
// expose a strict JSON Schema derived from the CLI's ParameterInfo, so
// agents that know which command they want get full typed introspection
// without having to call describe_command first.
//
// Tool names are derived from the command path: "cluster/create" becomes
// "aerolab_cluster_create". Invalid characters are sanitised to '_'.
// Commands named "help" are always skipped.
//
// The returned count is the number of tools added. Names that collide
// with an already-registered name (e.g. when the generic tools are
// registered first) or with a previously added auto-tool are skipped.
func RegisterAutoTools(server *sdkmcp.Server, reg *Registry) int {
	seen := map[string]struct{}{
		ToolListCommands:    {},
		ToolDescribeCommand: {},
		ToolExecuteCommand:  {},
	}
	return RegisterAutoToolsWith(server, reg, seen)
}

// RegisterAutoToolsWith is identical to RegisterAutoTools but routes
// through a shared "seen" set. When NewServer calls both registrars it
// seeds a single empty set and passes it into the generic and auto
// tool registrations so any collision across the two is detected.
func RegisterAutoToolsWith(server *sdkmcp.Server, reg *Registry, seen map[string]struct{}) int {
	if server == nil || reg == nil {
		return 0
	}
	if seen == nil {
		seen = map[string]struct{}{}
	}

	var count int
	for _, cmd := range reg.Leaves() {
		if cmd == nil || cmd.Hidden {
			continue
		}
		if isHelpLeaf(cmd) {
			continue
		}
		name := AutoToolName(cmd.Path)
		if name == "" {
			continue
		}
		tool, nameToFlag := autoTool(cmd, name)
		if addUnique(server, seen, nil, tool, autoHandler(reg, cmd, nameToFlag)) {
			count++
		}
	}
	return count
}

// AutoToolName converts a slash-separated command path into a valid MCP
// tool name. Tool names are sanitised to match
// "^[A-Za-z_][A-Za-z0-9_-]*$" — any character outside that set is
// replaced with '_' and the whole path is prefixed with "aerolab_".
func AutoToolName(path string) string {
	segs := splitPath(path)
	if len(segs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(segs)+1)
	parts = append(parts, "aerolab")
	for _, s := range segs {
		s = sanitiseToolSegment(s)
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts, "_")
}

var toolNameBadChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func sanitiseToolSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return toolNameBadChars.ReplaceAllString(s, "_")
}

func isHelpLeaf(cmd *Command) bool {
	if cmd == nil {
		return false
	}
	if strings.EqualFold(cmd.Name, "help") {
		return true
	}
	segs := splitPath(cmd.Path)
	if len(segs) > 0 && strings.EqualFold(segs[len(segs)-1], "help") {
		return true
	}
	return false
}

// autoTool builds the sdk tool definition for an auto-registered command
// and returns both the tool and the reverse map from schema property name
// → CLI long-flag. The caller threads nameToFlag into autoHandler so arg
// keys that were disambiguated by BuildInputSchema (e.g. collisions like
// "aws__count") are emitted with the correct bare flag name.
func autoTool(cmd *Command, name string) (*sdkmcp.Tool, map[string]string) {
	extras := map[string]any{
		"confirm": map[string]any{
			"type":        "boolean",
			"description": "Must be true to execute destructive commands when the server runs in the 'standard' profile. Ignored otherwise.",
		},
		"timeout_sec": map[string]any{
			"type":        "integer",
			"minimum":     0,
			"description": "Per-call timeout override in seconds. 0 uses the server default.",
		},
	}
	schema, nameToFlag := BuildInputSchema(cmd.Parameters, extras)
	schemaBytes, _ := json.Marshal(schema)

	desc := strings.TrimSpace(cmd.Description)
	if desc == "" {
		desc = "aerolab " + strings.ReplaceAll(cmd.Path, "/", " ")
	}
	destr := IsDestructive(cmd)
	if destr {
		desc = "DESTRUCTIVE: " + desc + " — requires confirm=true under the 'standard' profile; forbidden under 'read-only'."
	}

	return &sdkmcp.Tool{
		Name:        name,
		Description: desc + " | Equivalent CLI: aerolab " + strings.ReplaceAll(cmd.Path, "/", " "),
		InputSchema: json.RawMessage(schemaBytes),
		Annotations: &sdkmcp.ToolAnnotations{
			Title:           strings.ReplaceAll(cmd.Path, "/", " "),
			DestructiveHint: ptr(destr),
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    !destr && isReadOnlyName(cmd),
			IdempotentHint:  !destr && isReadOnlyName(cmd),
		},
	}, nameToFlag
}

// isReadOnlyName returns true for common read-only verb leaves (list,
// status, inventory, show, get, ...). This is advisory: clients use the
// hint for rendering, it does not affect gating.
func isReadOnlyName(cmd *Command) bool {
	if cmd == nil {
		return false
	}
	segs := splitPath(cmd.Path)
	if len(segs) == 0 {
		return false
	}
	switch strings.ToLower(segs[len(segs)-1]) {
	case "list", "status", "inventory", "show", "get", "describe",
		"ls", "info", "version":
		return true
	}
	return false
}

func autoHandler(reg *Registry, cmd *Command, nameToFlag map[string]string) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if reg.Run == nil {
			return errorResult(errors.New("runner not configured")), nil
		}

		raw := map[string]any{}
		if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &raw); err != nil {
				return errorResult(fmt.Errorf("invalid arguments: %w", err)), nil
			}
		}

		// Pull out server-meta fields before we pass the rest as CLI args.
		confirm, _ := raw["confirm"].(bool)
		delete(raw, "confirm")
		var timeoutSec int
		if v, ok := raw["timeout_sec"]; ok {
			delete(raw, "timeout_sec")
			switch t := v.(type) {
			case float64:
				timeoutSec = int(t)
			case int:
				timeoutSec = t
			}
		}

		if err := reg.Gate.Check(cmd, confirm); err != nil {
			return errorResult(err), nil
		}

		// Translate schema property names back to CLI long-flags so any
		// collision-mangled names (e.g. "aws__count") land as "--count"
		// on the command line.
		args := translateArgs(raw, nameToFlag)

		// Simple-mode check happens after translation so we compare
		// CLI long-flag names against the rule set (which stores
		// parameter paths as "<cmd>.<long>").
		if reg.SimpleModeGate != nil {
			if err := reg.SimpleModeGate.CheckCommand(cmd.Path); err != nil {
				return errorResult(err), nil
			}
			if err := reg.SimpleModeGate.CheckArgs(cmd.Path, args); err != nil {
				return errorResult(err), nil
			}
		}

		// Nudge read-style commands toward JSON so the LLM doesn't have
		// to parse ANSI-decorated tables. Honours explicit caller values
		// and the server-wide DisableForceJSONOutput switch.
		args = maybeInjectJSONOutput(reg, cmd, args)

		out := reg.Run.Execute(ctx, RunInput{
			Path:            cmd.Path,
			Args:            args,
			TimeoutOverride: time.Duration(timeoutSec) * time.Second,
		})
		return runOutputToResult(out), nil
	}
}

// translateArgs maps each key in raw through nameToFlag to recover the
// bare CLI long-flag. Keys absent from nameToFlag (or an empty map) are
// passed through unchanged; the auto-registered confirm/timeout_sec
// controls never reach this function. When two schema keys resolve to
// the same flag the second occurrence wins, mirroring BuildArgv's
// behaviour of emitting one --flag per map entry.
func translateArgs(raw map[string]any, nameToFlag map[string]string) map[string]any {
	if len(raw) == 0 {
		return raw
	}
	if len(nameToFlag) == 0 {
		return raw
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		if flag, ok := nameToFlag[k]; ok && flag != "" {
			out[flag] = v
			continue
		}
		out[k] = v
	}
	return out
}
