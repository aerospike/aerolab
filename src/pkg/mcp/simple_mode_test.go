package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// stubSimpleModeGate is a trivial SimpleModeGate that mirrors the minimal
// public contract: paths in blockedCommands fail CheckCommand, keys in
// blockedArgs[path] fail CheckArgs. Used to verify tools_auto and
// tools_generic consult the gate without pulling in the cmd package.
type stubSimpleModeGate struct {
	blockedCommands map[string]struct{}
	blockedArgs     map[string]map[string]struct{}
}

func (g *stubSimpleModeGate) CheckCommand(path string) error {
	if _, bad := g.blockedCommands[path]; bad {
		return errors.New("command '" + path + "' is not available in simple mode")
	}
	return nil
}

func (g *stubSimpleModeGate) CheckArgs(path string, args map[string]any) error {
	blocked, ok := g.blockedArgs[path]
	if !ok {
		return nil
	}
	for k, v := range args {
		if b, isBool := v.(bool); isBool && !b {
			continue
		}
		if _, bad := blocked[k]; bad {
			return errors.New("parameter '--" + k + "' is not available in simple mode")
		}
	}
	return nil
}

func TestAutoToolRejectsBlockedCommand(t *testing.T) {
	reg := newTestRegistry(t)
	reg.SimpleModeGate = &stubSimpleModeGate{
		blockedCommands: map[string]struct{}{"inventory/list": {}},
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_inventory_list",
		Arguments: map[string]any{"verbose": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected simple-mode gate to block the call")
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "not available in simple mode") {
		t.Errorf("expected simple mode error, got: %s", text)
	}
}

func TestAutoToolRejectsBlockedParameter(t *testing.T) {
	reg := newTestRegistry(t)
	reg.SimpleModeGate = &stubSimpleModeGate{
		blockedArgs: map[string]map[string]struct{}{
			"inventory/list": {"verbose": {}},
		},
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_inventory_list",
		Arguments: map[string]any{"verbose": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected simple-mode gate to block the parameter")
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "--verbose") {
		t.Errorf("expected --verbose in error, got: %s", text)
	}
}

func TestGenericExecuteRejectsBlockedCommand(t *testing.T) {
	reg := newTestRegistry(t)
	reg.SimpleModeGate = &stubSimpleModeGate{
		blockedCommands: map[string]struct{}{"cluster/destroy": {}},
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterGenericTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name: ToolExecuteCommand,
		Arguments: map[string]any{
			"path":    "cluster/destroy",
			"args":    map[string]any{"name": "mydc"},
			"confirm": true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected simple-mode gate to block the call")
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "not available in simple mode") {
		t.Errorf("expected simple mode error, got: %s", text)
	}
}

func TestGenericExecuteRejectsBlockedParameter(t *testing.T) {
	reg := newTestRegistry(t)
	reg.SimpleModeGate = &stubSimpleModeGate{
		blockedArgs: map[string]map[string]struct{}{
			"inventory/list": {"verbose": {}},
		},
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
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
	if !res.IsError {
		t.Fatal("expected simple-mode gate to block the parameter")
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "--verbose") {
		t.Errorf("expected --verbose in error, got: %s", text)
	}
}

func TestSimpleModeGateNilIsPermissive(t *testing.T) {
	reg := newTestRegistry(t)
	reg.SimpleModeGate = nil
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAutoTools(server, reg)
	sess := connectInMemory(t, server)

	res, err := sess.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "aerolab_inventory_list",
		Arguments: map[string]any{"verbose": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success with nil gate, got: %s", res.Content[0].(*sdkmcp.TextContent).Text)
	}
}
