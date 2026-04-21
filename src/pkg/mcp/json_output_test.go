package mcp

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestShouldForceJSONOutputParam(t *testing.T) {
	cases := []struct {
		name string
		p    Param
		want bool
	}{
		{
			name: "standard aerolab output flag",
			p:    Param{Long: "output", Default: "table", Description: "Output format (text, table, json, json-indent, jq)"},
			want: true,
		},
		{
			name: "uppercased JSON in description still matches",
			p:    Param{Long: "output", Default: "table", Description: "Output format (JSON only)"},
			want: true,
		},
		{
			name: "explicit choices advertising json",
			p:    Param{Long: "output", Default: "table", Choices: []string{"table", "json"}},
			want: true,
		},
		{
			name: "already defaults to json",
			p:    Param{Long: "output", Default: "json", Description: "Output format (text, json)"},
			want: false,
		},
		{
			name: "already defaults to json-indent",
			p:    Param{Long: "output", Default: "json-indent", Description: "json, json-indent"},
			want: false,
		},
		{
			name: "not the output flag",
			p:    Param{Long: "format", Default: "table", Description: "Output format (json)"},
			want: false,
		},
		{
			name: "output flag with no json support",
			p:    Param{Long: "output", Default: "text", Description: "Output format (text, csv)"},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ShouldForceJSONOutputParam(c.p); got != c.want {
				t.Fatalf("ShouldForceJSONOutputParam(%+v) = %v, want %v", c.p, got, c.want)
			}
		})
	}
}

func TestShouldForceJSONOutputCommand(t *testing.T) {
	cmd := &Command{
		Name: "list",
		Path: "cluster/list",
		Parameters: []Param{
			{Long: "owner"},
			{Long: "output", Default: "table", Description: "Output format (text, table, json)"},
		},
	}
	flag, ok := ShouldForceJSONOutput(cmd)
	if !ok || flag != "output" {
		t.Fatalf("expected (output,true), got (%q,%v)", flag, ok)
	}

	// A command without a matching parameter reports false.
	if _, ok := ShouldForceJSONOutput(&Command{Parameters: []Param{{Long: "owner"}}}); ok {
		t.Fatal("expected false for command without output param")
	}
	if _, ok := ShouldForceJSONOutput(nil); ok {
		t.Fatal("expected false for nil command")
	}
}

func TestMaybeInjectJSONOutput(t *testing.T) {
	cmd := &Command{
		Parameters: []Param{{Long: "output", Default: "table", Description: "Output format (text, json)"}},
	}

	t.Run("injects when caller omits output", func(t *testing.T) {
		args := maybeInjectJSONOutput(&Registry{}, cmd, map[string]any{"owner": "alice"})
		if args["output"] != ForcedJSONOutputValue {
			t.Fatalf("expected output=json, got %v", args["output"])
		}
	})

	t.Run("allocates map when input is nil", func(t *testing.T) {
		args := maybeInjectJSONOutput(&Registry{}, cmd, nil)
		if args == nil || args["output"] != ForcedJSONOutputValue {
			t.Fatalf("expected allocated map with output=json, got %v", args)
		}
	})

	t.Run("preserves explicit caller value", func(t *testing.T) {
		args := maybeInjectJSONOutput(&Registry{}, cmd, map[string]any{"output": "csv"})
		if args["output"] != "csv" {
			t.Fatalf("expected output=csv preserved, got %v", args["output"])
		}
	})

	t.Run("respects DisableForceJSONOutput", func(t *testing.T) {
		args := maybeInjectJSONOutput(&Registry{DisableForceJSONOutput: true}, cmd, map[string]any{})
		if _, set := args["output"]; set {
			t.Fatalf("expected no injection when disabled, got %v", args)
		}
	})

	t.Run("nil registry defaults to injection enabled", func(t *testing.T) {
		args := maybeInjectJSONOutput(nil, cmd, map[string]any{})
		if args["output"] != ForcedJSONOutputValue {
			t.Fatalf("expected injection with nil registry, got %v", args)
		}
	})

	t.Run("no-op when command has no output param", func(t *testing.T) {
		bare := &Command{Parameters: []Param{{Long: "name"}}}
		args := maybeInjectJSONOutput(&Registry{}, bare, map[string]any{"name": "mydc"})
		if _, set := args["output"]; set {
			t.Fatalf("expected no injection, got %v", args)
		}
	})
}

// newJSONOutputRegistry builds a minimal registry with a single leaf
// whose only parameter is the read-style --output flag. The echoing
// script lets us assert the final argv the runner assembled.
func newJSONOutputRegistry(t *testing.T) *Registry {
	t.Helper()
	script := writeScript(t, "echo invoked $*")
	return &Registry{
		Root: []*Command{{
			Name:        "aerolab",
			Path:        "aerolab",
			Description: "root",
			Children: []*Command{{
				Name:        "cluster",
				Path:        "cluster",
				Description: "cluster",
				Children: []*Command{{
					Name:        "list",
					Path:        "cluster/list",
					Description: "list clusters",
					Parameters: []Param{
						{Long: "output", Type: "string", Default: "table",
							Description: "Output format (text, table, json, json-indent, jq)"},
					},
				}},
			}},
		}},
		Help: RenderHelpFromFactory(newTestOpts),
		Run:  &Runner{Binary: script, DefaultTimeout: 5_000_000_000},
		Gate: NewGate(ProfileStandard),
	}
}

func TestAutoToolInjectsJSONOutput(t *testing.T) {
	reg := newJSONOutputRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_cluster_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %v", res.Content[0].(*sdkmcp.TextContent).Text)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "cluster list --output=json") {
		t.Errorf("expected '--output=json' injection in argv, got:\n%s", text)
	}
}

func TestAutoToolPreservesExplicitOutput(t *testing.T) {
	reg := newJSONOutputRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_cluster_list",
		Arguments: map[string]any{"output": "csv"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "cluster list --output=csv") {
		t.Errorf("expected '--output=csv' preserved, got:\n%s", text)
	}
	if strings.Contains(text, "--output=json") {
		t.Errorf("did not expect '--output=json' after explicit caller value, got:\n%s", text)
	}
}

func TestAutoToolHonoursDisableForceJSONOutput(t *testing.T) {
	reg := newJSONOutputRegistry(t)
	reg.DisableForceJSONOutput = true
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_cluster_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if strings.Contains(text, "--output=json") {
		t.Errorf("expected no injection when DisableForceJSONOutput=true, got:\n%s", text)
	}
}

func TestExecuteCommandInjectsJSONOutput(t *testing.T) {
	reg := newJSONOutputRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name: ToolExecuteCommand,
		Arguments: map[string]any{
			"path": "cluster/list",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %v", res.Content[0].(*sdkmcp.TextContent).Text)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "cluster list --output=json") {
		t.Errorf("expected --output=json injection via execute_command, got:\n%s", text)
	}
}
