package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestParamName(t *testing.T) {
	tests := []struct {
		name string
		p    Param
		want string
	}{
		{"long wins", Param{Name: "n", Long: "name", Short: "n"}, "name"},
		{"fall back to name", Param{Name: "flag", Short: "f"}, "flag"},
		{"short last resort", Param{Short: "s"}, "s"},
		{"positional uses name", Param{IsPositional: true, Name: "TargetFile"}, "TargetFile"},
		{"empty param yields empty", Param{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParamName(tc.p); got != tc.want {
				t.Fatalf("ParamName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildInputSchemaPrimitiveTypes(t *testing.T) {
	params := []Param{
		{Long: "name", Type: "string", Description: "Cluster name", Required: true},
		{Long: "count", Type: "int", Default: "1"},
		{Long: "ratio", Type: "float"},
		{Long: "enabled", Type: "bool"},
		{Long: "timeout", Type: "duration"},
		{Long: "nodes", Type: "[]string", IsSlice: true},
		{Long: "size", Type: "uint"},
	}
	schema, _ := BuildInputSchema(params, nil)

	if schema["type"] != "object" {
		t.Fatalf("root type = %v, want object", schema["type"])
	}
	props := schema["properties"].(map[string]any)

	want := map[string]string{
		"name":    "string",
		"count":   "integer",
		"ratio":   "number",
		"enabled": "boolean",
		"size":    "integer",
	}
	for k, v := range want {
		got, ok := props[k].(map[string]any)
		if !ok {
			t.Fatalf("property %s missing", k)
		}
		if got["type"] != v {
			t.Errorf("%s type = %v, want %v", k, got["type"], v)
		}
	}

	// Duration accepts string OR integer.
	dur := props["timeout"].(map[string]any)
	if _, ok := dur["type"].([]any); !ok {
		t.Errorf("duration type should be a []any union, got %T", dur["type"])
	}

	// Slice produces array+items schema.
	nodes := props["nodes"].(map[string]any)
	if nodes["type"] != "array" {
		t.Errorf("slice type = %v, want array", nodes["type"])
	}

	// Required list contains only name.
	req, _ := schema["required"].([]string)
	if !reflect.DeepEqual(req, []string{"name"}) {
		t.Errorf("required = %v, want [name]", req)
	}
}

func TestBuildInputSchemaChoicesAndDescription(t *testing.T) {
	p := Param{
		Long:        "mode",
		Type:        "string",
		Description: "Heartbeat mode",
		Default:     "mesh",
		Choices:     []string{"mesh", "mcast", "default"},
	}
	schema, _ := BuildInputSchema([]Param{p}, nil)
	props := schema["properties"].(map[string]any)
	mode := props["mode"].(map[string]any)

	if mode["type"] != "string" {
		t.Fatalf("type = %v, want string", mode["type"])
	}
	enum, ok := mode["enum"].([]any)
	if !ok || len(enum) != 3 {
		t.Fatalf("enum = %v, want 3 values", mode["enum"])
	}
	if desc, _ := mode["description"].(string); desc == "" ||
		!containsAll(desc, "Heartbeat mode", "default: mesh") {
		t.Errorf("description = %q, want to include description and default", desc)
	}
}

func TestBuildInputSchemaCustomTypesFallBackToString(t *testing.T) {
	// TypeClusterName, TypeYesNo, TypeExpiry, guiInstanceType should all
	// serialize as string at the schema layer.
	for _, typ := range []string{
		"cmd.TypeClusterName",
		"cmd.TypeYesNo",
		"cmd.TypeExpiry",
		"cmd.guiInstanceType",
		"flags.Filename",
		"",
	} {
		s, _ := BuildInputSchema([]Param{{Long: "x", Type: typ}}, nil)
		p := s["properties"].(map[string]any)["x"].(map[string]any)
		if p["type"] != "string" {
			t.Errorf("type=%q mapped to %v, want string", typ, p["type"])
		}
	}
}

func TestBuildInputSchemaFileFields(t *testing.T) {
	s, _ := BuildInputSchema([]Param{
		{Long: "cert", Type: "string", IsFile: true},
		{Long: "paths", Type: "[]string", IsSlice: true, IsFile: true},
	}, nil)
	props := s["properties"].(map[string]any)
	cert := props["cert"].(map[string]any)
	if cert["type"] != "string" {
		t.Errorf("file scalar type = %v, want string", cert["type"])
	}
	paths := props["paths"].(map[string]any)
	if paths["type"] != "array" {
		t.Errorf("file slice type = %v, want array", paths["type"])
	}
}

func TestBuildInputSchemaDuplicateNamesDisambiguated(t *testing.T) {
	// Silence the collision warning for the duration of the test.
	prev := warnLog
	warnLog = func(string, ...any) {}
	defer func() { warnLog = prev }()

	s, nameToFlag := BuildInputSchema([]Param{
		{Long: "name", Type: "string", Namespace: "aws"},
		{Long: "name", Type: "int", Namespace: "gcp"},
	}, nil)
	props := s["properties"].(map[string]any)
	if len(props) != 2 {
		t.Fatalf("expected 2 properties after namespace disambiguation, got %d: %v", len(props), props)
	}
	first := props["name"].(map[string]any)
	if first["type"] != "string" {
		t.Errorf("first definition should keep the bare name; got %v", first["type"])
	}
	second, ok := props["gcp__name"].(map[string]any)
	if !ok {
		t.Fatalf("expected collision key gcp__name, got %v", props)
	}
	if second["type"] != "integer" {
		t.Errorf("second definition should map to integer, got %v", second["type"])
	}
	if nameToFlag["name"] != "name" {
		t.Errorf("nameToFlag[name] = %q, want 'name'", nameToFlag["name"])
	}
	if nameToFlag["gcp__name"] != "name" {
		t.Errorf("nameToFlag[gcp__name] = %q, want 'name'", nameToFlag["gcp__name"])
	}
}

func TestBuildInputSchemaExtraPropsMerged(t *testing.T) {
	extras := map[string]any{
		"confirm":     map[string]any{"type": "boolean"},
		"timeout_sec": map[string]any{"type": "integer"},
	}
	s, nameToFlag := BuildInputSchema([]Param{
		{Long: "foo", Type: "string"},
	}, extras)
	props := s["properties"].(map[string]any)
	for _, k := range []string{"foo", "confirm", "timeout_sec"} {
		if _, ok := props[k]; !ok {
			t.Errorf("expected property %q in merged schema", k)
		}
	}
	// nameToFlag should omit the extras so callers can tell them apart
	// from CLI-backed keys.
	if _, ok := nameToFlag["confirm"]; ok {
		t.Errorf("nameToFlag should not contain extraProps key 'confirm'")
	}
	if _, ok := nameToFlag["timeout_sec"]; ok {
		t.Errorf("nameToFlag should not contain extraProps key 'timeout_sec'")
	}
	if nameToFlag["foo"] != "foo" {
		t.Errorf("nameToFlag[foo] = %q, want 'foo'", nameToFlag["foo"])
	}
}

func TestBuildInputSchemaExtraPropsCollision(t *testing.T) {
	// When a parameter and an extraProps key share the same name, the
	// parameter must win and warnLog must fire.
	var warnings []string
	prev := warnLog
	warnLog = func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}
	defer func() { warnLog = prev }()

	s, _ := BuildInputSchema(
		[]Param{{Long: "confirm", Type: "string", Description: "user-defined confirm flag"}},
		map[string]any{"confirm": map[string]any{"type": "boolean"}},
	)
	props := s["properties"].(map[string]any)
	confirm := props["confirm"].(map[string]any)
	if confirm["type"] != "string" {
		t.Errorf("parameter should win over extraProps; got type %v", confirm["type"])
	}
	if len(warnings) == 0 {
		t.Errorf("expected warnLog to fire on extraProps collision")
	}
}

func TestBuildInputSchemaGroupPrefixInDescription(t *testing.T) {
	s, _ := BuildInputSchema([]Param{
		{Long: "instance-type", Type: "string", Group: "AWS", Description: "Instance type"},
	}, nil)
	props := s["properties"].(map[string]any)
	it := props["instance-type"].(map[string]any)
	if desc, _ := it["description"].(string); !containsAll(desc, "[AWS]", "Instance type") {
		t.Errorf("description = %q, want group prefix", desc)
	}
}

func TestBuildInputSchemaMarshalsToJSON(t *testing.T) {
	// InputSchema ultimately becomes JSON on the wire. Ensure it round-trips.
	s, _ := BuildInputSchema([]Param{
		{Long: "name", Type: "string", Required: true},
		{Long: "count", Type: "int"},
	}, nil)
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round["type"] != "object" {
		t.Errorf("roundtrip root type = %v", round["type"])
	}
}

func TestBuildGenericSchemaShape(t *testing.T) {
	s := BuildGenericSchema()
	if s["type"] != "object" {
		t.Fatalf("root type = %v", s["type"])
	}
	req, _ := s["required"].([]string)
	if !reflect.DeepEqual(req, []string{"path"}) {
		t.Errorf("required = %v, want [path]", req)
	}
	props := s["properties"].(map[string]any)
	for _, k := range []string{"path", "args", "positional", "timeout_sec", "env", "confirm"} {
		if _, ok := props[k]; !ok {
			t.Errorf("missing property %q", k)
		}
	}
}

func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		if !containsIgnoreCase(s, n) {
			return false
		}
	}
	return true
}

func containsIgnoreCase(haystack, needle string) bool {
	// shallow: the tests only need exact substrings.
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
