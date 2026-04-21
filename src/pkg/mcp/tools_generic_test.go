package mcp

import (
	"context"
	"runtime"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestRegistry returns a deterministic two-leaf registry with a working
// help renderer and an echoing runner for integration tests.
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh-based script test skipped on windows")
	}
	script := writeScript(t, "echo invoked $*")

	tree := []*Command{{
		Name:        "aerolab",
		Path:        "aerolab",
		Description: "root",
		Children: []*Command{
			{
				Name:        "inventory",
				Path:        "inventory",
				Description: "inventory",
				Children: []*Command{
					{
						Name:        "list",
						Path:        "inventory/list",
						Description: "list resources",
						Parameters: []Param{
							{Name: "Verbose", Long: "verbose", Type: "bool"},
						},
					},
				},
			},
			{
				Name:        "cluster",
				Path:        "cluster",
				Description: "cluster mgmt",
				Children: []*Command{
					{
						Name:        "destroy",
						Path:        "cluster/destroy",
						Destructive: true,
						Description: "destroy a cluster",
						Parameters: []Param{
							{Name: "Name", Long: "name", Type: "string", Required: true},
						},
					},
				},
			},
		},
	}}

	return &Registry{
		Root: tree,
		Help: RenderHelpFromFactory(newTestOpts),
		Run:  &Runner{Binary: script, DefaultTimeout: 5 * 1_000_000_000},
		Gate: NewGate(ProfileStandard),
	}
}

// connectInMemory hooks the server to an in-memory client session for
// calling tools directly from tests.
func connectInMemory(t *testing.T, server *sdkmcp.Server) *sdkmcp.ClientSession {
	t.Helper()
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "0"}, nil)
	sess, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func TestGenericListCommands(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "aerolab-mcp", Version: "test"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	ctx := context.Background()
	res, err := sess.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      ToolListCommands,
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %+v", res)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "inventory/list") {
		t.Errorf("expected inventory/list in output, got:\n%s", text)
	}
	if !strings.Contains(text, "[DESTRUCTIVE] cluster/destroy") {
		t.Errorf("expected destructive marker, got:\n%s", text)
	}
}

func TestGenericListCommandsSkipsHelp(t *testing.T) {
	// Help leaves on the tree must be filtered out of list_commands so
	// agents don't see dozens of noisy "<cmd>/help" entries.
	reg := &Registry{
		Root: []*Command{{
			Name: "aerolab",
			Path: "aerolab",
			Children: []*Command{
				{Name: "cluster", Path: "cluster", Children: []*Command{
					{Name: "help", Path: "cluster/help", Description: "help"},
					{Name: "list", Path: "cluster/list", Description: "list clusters"},
				}},
			},
		}},
		Run:  &Runner{},
		Gate: NewGate(ProfileStandard),
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      ToolListCommands,
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if strings.Contains(text, "cluster/help") {
		t.Errorf("expected help leaves to be filtered, got:\n%s", text)
	}
	if !strings.Contains(text, "cluster/list") {
		t.Errorf("expected non-help leaf to remain, got:\n%s", text)
	}
}

func TestGenericListCommandsPrefix(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "aerolab-mcp", Version: "test"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      ToolListCommands,
		Arguments: map[string]any{"prefix": "cluster/", "includeDestructive": false},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if strings.Contains(text, "cluster/destroy") {
		t.Errorf("expected destructive to be filtered out, got:\n%s", text)
	}
	if strings.Contains(text, "inventory/list") {
		t.Errorf("expected prefix to filter inventory, got:\n%s", text)
	}
}

func TestGenericDescribeCommand(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "aerolab-mcp", Version: "test"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      ToolDescribeCommand,
		Arguments: map[string]any{"path": "cluster/destroy"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError")
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "DESTRUCTIVE") {
		t.Errorf("expected destructive callout: %s", text)
	}
}

func TestGenericDescribeCommandUnknown(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "aerolab-mcp", Version: "test"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      ToolDescribeCommand,
		Arguments: map[string]any{"path": "nope"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError for unknown path")
	}
}

func TestGenericExecuteCommandGated(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "aerolab-mcp", Version: "test"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	// Destructive command without confirm should be blocked.
	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name: ToolExecuteCommand,
		Arguments: map[string]any{
			"path": "cluster/destroy",
			"args": map[string]any{"name": "mydc"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError for gated destructive command")
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("expected confirm=true hint, got: %s", text)
	}
}

func TestGenericExecuteCommandHappyPath(t *testing.T) {
	reg := newTestRegistry(t)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "aerolab-mcp", Version: "test"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name: ToolExecuteCommand,
		Arguments: map[string]any{
			"path": "inventory/list",
			"args": map[string]any{"verbose": true},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %v", res.Content[0].(*sdkmcp.TextContent).Text)
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "invoked inventory list --verbose") {
		t.Errorf("expected invoked output, got: %s", text)
	}
}
