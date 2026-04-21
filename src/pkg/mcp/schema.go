package mcp

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Param describes a single CLI flag or positional argument in a form that is
// independent of the aerolab `cmd` package. The CLI side builds a []Param
// from its internal ParameterInfo tree (see cmdMcp.go adapters) so that this
// package has no import dependency on `cmd`.
//
// Type values (lowercase):
//
//	string, int, uint, float, bool, duration, object
//	[]string, []int, []file (slices are flagged via IsSlice in addition)
//
// The schema generator treats unknown types as "string".
type Param struct {
	Name         string
	Short        string
	Long         string
	Description  string
	Type         string
	Default      string
	Required     bool
	Choices      []string
	ChoiceLabels []string
	IsSlice      bool
	IsPositional bool
	IsFile       bool
	Optional     bool
	Group        string
	Namespace    string
}

// Command describes a single node in the aerolab CLI tree. The Path field is
// the slash-separated path below the root (e.g. "cluster/create"). Parameters
// include flags defined on the command itself and flags pulled up from its
// groups (AWS/GCP/Docker) and embedded structs.
type Command struct {
	Name        string
	DisplayName string
	Path        string
	Description string
	Hidden      bool
	Destructive bool // populated by gate.go
	Children    []*Command
	Parameters  []Param
}

// warnLog is invoked when BuildInputSchema encounters a problem worth
// surfacing (e.g. a generated schema key collides with a caller-supplied
// extraProps key, or two parameters would collide on the same JSON name).
// The default implementation writes to stderr; tests override it by
// assigning a no-op or a recording function.
var warnLog = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "aerolab mcp: "+format+"\n", args...)
}

// BuildInputSchema converts a slice of parameters to a JSON Schema object
// suitable for use as Tool.InputSchema with the MCP SDK, and returns a
// reverse mapping from the schema property name back to the CLI long-flag
// the caller should emit when running aerolab.
//
// The returned schema is a plain map so the SDK's remarshal path accepts
// it regardless of draft version. The root schema has type "object" and
// contains a properties map keyed by the parameter's JSON name (long flag,
// field name, or prefixed group-member name), plus a required list.
//
// extraProps is a map of additional JSON-Schema properties that are merged
// into the final schema after parameter properties. Typical use is the
// `confirm` / `timeout_sec` fields injected by auto-registered tools. If a
// key in extraProps collides with a parameter-derived property, the extra
// is dropped (the parameter wins) and a warning is emitted via warnLog.
// Pass nil when no extras are needed.
//
// Name collisions between parameters (two params resolving to the same
// JSON name, typically caused by different namespaces declaring the same
// long flag) are disambiguated by storing the later occurrences under
// "<namespace>__<long>" keys. The returned nameToFlag maps every schema
// property back to the bare CLI long-flag so the caller can build argv
// correctly.
//
// Parameters with Hidden=false but Optional/NoDefault are not forced
// required; only Required=true produces a required list entry.
func BuildInputSchema(params []Param, extraProps map[string]any) (map[string]any, map[string]string) {
	props := map[string]any{}
	nameToFlag := map[string]string{}
	var required []string

	for _, p := range params {
		name, prop := buildPropertySchema(p)
		if name == "" {
			continue
		}
		key := name
		if _, exists := props[key]; exists {
			// Two parameters resolve to the same JSON property name.
			// Disambiguate by namespace when available; fall back to
			// group, then a numeric suffix.
			key = collisionKey(name, p, props)
			if key == "" {
				warnLog("parameter %q collides with an earlier parameter and cannot be disambiguated; dropping duplicate", name)
				continue
			}
			warnLog("parameter %q collides with an earlier parameter; exposing second definition as %q", name, key)
		}
		props[key] = prop
		nameToFlag[key] = name
		if p.Required {
			required = append(required, key)
		}
	}

	for k, v := range extraProps {
		if _, exists := props[k]; exists {
			warnLog("extraProps key %q collides with a parameter of the same name; dropping extra", k)
			continue
		}
		props[k] = v
		// extraProps entries (confirm, timeout_sec, …) do not map to a
		// CLI flag; leave them out of nameToFlag so the caller knows to
		// consume them as control fields.
	}

	// Stable ordering of "required" so tests are deterministic.
	sort.Strings(required)

	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	s["additionalProperties"] = false
	return s, nameToFlag
}

// collisionKey returns a disambiguated schema key for parameter p when its
// natural name already exists in props. It prefers the parameter's
// Namespace, then Group, then appends a numeric suffix, and returns "" if
// no unique key can be generated (which should never happen in practice).
func collisionKey(name string, p Param, props map[string]any) string {
	try := func(candidate string) string {
		if candidate == "" {
			return ""
		}
		if _, exists := props[candidate]; !exists {
			return candidate
		}
		return ""
	}
	if k := try(sanitiseKey(p.Namespace) + "__" + name); k != "" {
		return k
	}
	if k := try(sanitiseKey(p.Group) + "__" + name); k != "" {
		return k
	}
	for i := 2; i < 100; i++ {
		if k := try(fmt.Sprintf("%s__%d", name, i)); k != "" {
			return k
		}
	}
	return ""
}

func sanitiseKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Lowercase + replace runs of non-alphanumerics with '_'.
	var b strings.Builder
	var lastUnderscore bool
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
			lastUnderscore = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

// BuildGenericSchema returns the JSON Schema for the generic execute_command
// tool. It accepts any command path and arbitrary arguments; individual
// commands are validated by the subprocess runner.
func BuildGenericSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Slash-separated command path below the aerolab root (e.g. 'cluster/create'). Use list_commands to discover available paths.",
			},
			"args": map[string]any{
				"type":                 "object",
				"description":          "Flag name/value map. Keys are long flag names (e.g. 'name', 'instance-type'); values are strings/numbers/booleans. Use describe_command to see available flags and their types.",
				"additionalProperties": true,
			},
			"positional": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Positional arguments appended after flags (rarely needed).",
			},
			"timeout_sec": map[string]any{
				"type":        "integer",
				"minimum":     0,
				"description": "Per-call timeout override in seconds. 0 uses the server default.",
			},
			"env": map[string]any{
				"type":                 "object",
				"description":          "Additional environment variables for the subprocess.",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"confirm": map[string]any{
				"type":        "boolean",
				"description": "Must be true to execute destructive commands when the server runs in the 'standard' profile.",
			},
		},
		"required": []string{"path"},
	}
}

// ParamName returns the JSON key used for a parameter in the generated schema.
// Positional parameters use their Name; flags use Long (preferred), Name, or
// the short flag as a last resort.
func ParamName(p Param) string {
	if p.IsPositional {
		return strings.TrimSpace(p.Name)
	}
	if p.Long != "" {
		return p.Long
	}
	if p.Name != "" {
		return p.Name
	}
	return p.Short
}

func buildPropertySchema(p Param) (string, map[string]any) {
	name := ParamName(p)
	if name == "" {
		return "", nil
	}

	prop := map[string]any{}

	desc := p.Description
	if p.Group != "" {
		desc = strings.TrimSpace(desc)
		if desc != "" {
			desc = "[" + p.Group + "] " + desc
		} else {
			desc = "[" + p.Group + "]"
		}
	}
	if p.Default != "" {
		if desc != "" {
			desc += " (default: " + p.Default + ")"
		} else {
			desc = "default: " + p.Default
		}
	}
	if len(p.Choices) > 0 && len(p.ChoiceLabels) > 0 && len(p.ChoiceLabels) == len(p.Choices) {
		// Append one-line list of value: label for dropdown-style choices.
		var b strings.Builder
		for i, v := range p.Choices {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(v)
			if p.ChoiceLabels[i] != "" && p.ChoiceLabels[i] != v {
				b.WriteString(" = ")
				b.WriteString(p.ChoiceLabels[i])
			}
		}
		if desc != "" {
			desc += " | choices: " + b.String()
		} else {
			desc = "choices: " + b.String()
		}
	}
	if desc != "" {
		prop["description"] = desc
	}

	// Type mapping. IsFile fields are surfaced as strings (absolute paths).
	switch {
	case p.IsFile:
		jsonType(prop, "string", p.IsSlice)
	default:
		switch strings.ToLower(strings.TrimPrefix(p.Type, "[]")) {
		case "int", "int8", "int16", "int32", "int64":
			jsonType(prop, "integer", p.IsSlice)
		case "uint", "uint8", "uint16", "uint32", "uint64":
			jsonType(prop, "integer", p.IsSlice)
			if !p.IsSlice {
				prop["minimum"] = 0
			}
		case "float", "float32", "float64", "number":
			jsonType(prop, "number", p.IsSlice)
		case "bool":
			jsonType(prop, "boolean", p.IsSlice)
		case "duration":
			// time.Duration: accept either a string ("30s", "1h") or an integer of nanoseconds.
			if p.IsSlice {
				prop["type"] = "array"
				prop["items"] = map[string]any{"type": "string"}
			} else {
				prop["type"] = []any{"string", "integer"}
			}
		case "object":
			jsonType(prop, "object", p.IsSlice)
		case "string", "":
			jsonType(prop, "string", p.IsSlice)
		default:
			// Unknown custom types (TypeClusterName, TypeYesNo, TypeExpiry,
			// guiInstanceType, …) serialize as strings at the CLI layer.
			jsonType(prop, "string", p.IsSlice)
		}
	}

	// Static choices → JSON Schema enum on the leaf type.
	if len(p.Choices) > 0 {
		enum := make([]any, 0, len(p.Choices))
		for _, c := range p.Choices {
			enum = append(enum, c)
		}
		if p.IsSlice {
			// Apply enum to items.
			if items, ok := prop["items"].(map[string]any); ok {
				items["enum"] = enum
			}
		} else {
			prop["enum"] = enum
		}
	}

	return name, prop
}

// jsonType populates prop for a given primitive JSON-schema type, wrapping it
// in an array schema when IsSlice is true.
func jsonType(prop map[string]any, t string, isSlice bool) {
	if isSlice {
		prop["type"] = "array"
		prop["items"] = map[string]any{"type": t}
		return
	}
	prop["type"] = t
}
