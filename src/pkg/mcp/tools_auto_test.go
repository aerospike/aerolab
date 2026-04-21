package mcp

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAutoToolName(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"cluster/create", "aerolab_cluster_create"},
		{"inventory/list", "aerolab_inventory_list"},
		{"agi/create-plan", "aerolab_agi_create-plan"},
		{"foo/bar baz", "aerolab_foo_bar_baz"},
		{"", ""},
	}
	for _, c := range cases {
		if got := AutoToolName(c.path); got != c.want {
			t.Errorf("AutoToolName(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestRegisterAutoToolsSkipsHelpAndHidden(t *testing.T) {
	reg := &Registry{
		Root: []*Command{{
			Name: "aerolab",
			Children: []*Command{
				{Name: "list", Path: "list", Description: "list"},
				{Name: "help", Path: "help", Description: "help"},
				{Name: "secret", Path: "secret", Hidden: true},
			},
		}},
		Run:  &Runner{},
		Gate: NewGate(ProfileStandard),
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	n := RegisterAutoTools(server, reg)
	if n != 1 {
		t.Fatalf("expected 1 auto tool registered, got %d", n)
	}
}

func TestAutoToolExecutes(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterGenericTools(server, reg)
	n := RegisterAutoTools(server, reg)
	if n != 2 {
		t.Fatalf("expected 2 auto tools, got %d", n)
	}
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_inventory_list",
		Arguments: map[string]any{"verbose": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %v", res.Content[0].(*sdkmcp.TextContent).Text)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "invoked inventory list --verbose") {
		t.Errorf("expected invocation, got: %s", text)
	}
}

func TestAutoToolDestructiveGate(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_cluster_destroy",
		Arguments: map[string]any{"name": "mydc"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected destructive gate to block the call")
	}

	res, err = sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_cluster_destroy",
		Arguments: map[string]any{"name": "mydc", "confirm": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Errorf("expected success with confirm=true, got: %s", res.Content[0].(*sdkmcp.TextContent).Text)
	}
}

func TestAutoToolCollisionUnmangled(t *testing.T) {
	// Two parameters on the same command that collide on the JSON schema
	// property name must both be callable, and the auto-handler must
	// translate the mangled schema key back to the bare CLI long-flag.
	prev := warnLog
	warnLog = func(string, ...any) {}
	defer func() { warnLog = prev }()

	cmd := &Command{
		Name:        "probe",
		Path:        "probe",
		Description: "probe",
		Parameters: []Param{
			{Long: "region", Type: "string", Namespace: "aws"},
			{Long: "region", Type: "string", Namespace: "gcp"},
		},
	}

	_, nameToFlag := autoTool(cmd, "aerolab_probe")
	if nameToFlag["region"] != "region" || nameToFlag["gcp__region"] != "region" {
		t.Fatalf("unexpected nameToFlag: %v", nameToFlag)
	}
	args := translateArgs(map[string]any{"region": "us-east-1", "gcp__region": "us-central1"}, nameToFlag)
	// Collision mangled key maps to the bare flag; iteration order is
	// non-deterministic so we only assert that the bare key is present
	// exactly once and holds one of the two supplied values.
	if v, ok := args["region"]; !ok {
		t.Fatalf("expected translated args to contain bare flag 'region', got %v", args)
	} else if s, _ := v.(string); s != "us-east-1" && s != "us-central1" {
		t.Fatalf("unexpected translated value for 'region': %v", v)
	}
	if _, ok := args["gcp__region"]; ok {
		t.Errorf("translated args must not keep the mangled key, got %v", args)
	}
}

func TestListToolsReportsAutoTools(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterGenericTools(server, reg)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	seen := map[string]bool{}
	for tool, err := range sess.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools iter: %v", err)
		}
		seen[tool.Name] = true
	}
	for _, want := range []string{
		ToolListCommands, ToolDescribeCommand, ToolExecuteCommand,
		"aerolab_inventory_list", "aerolab_cluster_destroy",
	} {
		if !seen[want] {
			t.Errorf("expected tool %q to be listed; have: %v", want, seen)
		}
	}
}
