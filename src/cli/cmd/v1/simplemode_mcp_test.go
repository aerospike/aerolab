//go:build !noaerolabmcp

package cmd

import (
	"strings"
	"testing"
)

// TestSimpleModeAlwaysAllowedIncludesMCP pins the allow-list contract so a
// `-all` rule cannot accidentally lock users out of the MCP server itself
// (same reasoning as webui/config/help).
func TestSimpleModeAlwaysAllowedIncludesMCP(t *testing.T) {
	for _, name := range []string{"config", "webui", "mcp", "help", "version", "completion", "upgrade"} {
		if !simpleModeAlwaysAllowed[name] {
			t.Errorf("expected %q in simpleModeAlwaysAllowed", name)
		}
	}
}

// TestConvertTreeForMCPDropsBlockedCommands verifies the MCP-facing tree
// conversion omits commands whose effective SimpleMode is false once the
// caller opts into forceSimpleMode.
func TestConvertTreeForMCPDropsBlockedCommands(t *testing.T) {
	root := &CommandInfo{
		Name: "aerolab", SimpleMode: true,
		Children: []*CommandInfo{
			{Name: "cluster", Path: "cluster", SimpleMode: true, Children: []*CommandInfo{
				{Name: "create", Path: "cluster/create", SimpleMode: true},
				{Name: "destroy", Path: "cluster/destroy", SimpleMode: false},
			}},
			{Name: "agi", Path: "agi", SimpleMode: false, Children: []*CommandInfo{
				{Name: "create", Path: "agi/create", SimpleMode: true},
			}},
			// Always-allowed top-level must survive even with SimpleMode=false.
			{Name: "mcp", Path: "mcp", SimpleMode: false},
		},
	}

	got := convertTreeForMCP(root, nil, "", false, true)
	if got == nil {
		t.Fatal("expected non-nil tree")
	}

	names := map[string]bool{}
	for _, c := range got.Children {
		names[c.Name] = true
	}
	if !names["cluster"] {
		t.Error("expected cluster to remain (SimpleMode=true)")
	}
	if names["agi"] {
		t.Error("expected agi subtree to be dropped (SimpleMode=false)")
	}
	if !names["mcp"] {
		t.Error("expected mcp to survive via simpleModeAlwaysAllowed")
	}

	// Verify cluster.destroy was dropped while cluster.create was kept.
	for _, c := range got.Children {
		if c.Name != "cluster" {
			continue
		}
		for _, sub := range c.Children {
			if sub.Name == "destroy" {
				t.Error("expected cluster/destroy to be dropped in forced simple mode")
			}
		}
	}
}

// TestConvertTreeForMCPNoForceKeepsEverything confirms that when
// forceSimpleMode=false the converter does not filter anything even if
// nodes carry SimpleMode=false (the soft mode preserves existing behavior).
func TestConvertTreeForMCPNoForceKeepsEverything(t *testing.T) {
	root := &CommandInfo{
		Name: "aerolab", SimpleMode: true,
		Children: []*CommandInfo{
			{Name: "agi", Path: "agi", SimpleMode: false, Children: []*CommandInfo{
				{Name: "create", Path: "agi/create", SimpleMode: false},
			}},
		},
	}
	got := convertTreeForMCP(root, nil, "", false, false)
	if got == nil || len(got.Children) != 1 || got.Children[0].Name != "agi" {
		t.Fatalf("expected agi subtree preserved when not forcing simple mode, got %+v", got)
	}
}

// TestConvertParamsForMCPFiltersBlocked verifies that parameters marked
// SimpleMode=false are stripped from the MCP schema when forceSimpleMode
// is true and preserved otherwise.
func TestConvertParamsForMCPFiltersBlocked(t *testing.T) {
	ci := &CommandInfo{
		Name: "create", Path: "cluster/create",
		Parameters: []ParameterInfo{
			{Name: "Name", Long: "name", Type: "string", SimpleMode: true},
			{Name: "Count", Long: "count", Type: "int", SimpleMode: false},
		},
	}

	kept := convertParamsForMCP(ci, nil, false, true)
	if len(kept) != 1 || kept[0].Long != "name" {
		t.Fatalf("expected only 'name' to survive forced simple mode, got %+v", kept)
	}

	all := convertParamsForMCP(ci, nil, false, false)
	if len(all) != 2 {
		t.Fatalf("expected both parameters when not forcing simple mode, got %+v", all)
	}
}

// TestSimpleModeGateCheckCommand verifies CheckCommand delegates to
// SimpleModeConfig.CheckCommandAllowed (using dotted paths).
func TestSimpleModeGateCheckCommand(t *testing.T) {
	cfg := &SimpleModeConfig{
		ForceEnabled: true,
		Rules:        []SimpleModeRule{{Action: SimpleModeHide, Pattern: "cluster.destroy"}},
	}
	root := &CommandInfo{
		Name: "aerolab", SimpleMode: true,
		Children: []*CommandInfo{
			{Name: "cluster", Path: "cluster", SimpleMode: true, Children: []*CommandInfo{
				{Name: "destroy", Path: "cluster/destroy", SimpleMode: true},
				{Name: "create", Path: "cluster/create", SimpleMode: true},
			}},
		},
	}
	cfg.ApplyToCommandTree(root)

	gate := newSimpleModeGate(cfg, root)
	if err := gate.CheckCommand("cluster/destroy"); err == nil {
		t.Error("expected cluster/destroy to be blocked")
	}
	if err := gate.CheckCommand("cluster/create"); err != nil {
		t.Errorf("expected cluster/create to be allowed, got %v", err)
	}
}

// TestSimpleModeGateCheckArgs verifies CheckArgs rejects blocked flags
// that were explicitly supplied and ignores unknown keys + bool=false.
func TestSimpleModeGateCheckArgs(t *testing.T) {
	cfg := &SimpleModeConfig{
		ForceEnabled: true,
		Rules:        []SimpleModeRule{{Action: SimpleModeHide, Pattern: "cluster.create.count"}},
	}
	root := &CommandInfo{
		Name: "aerolab", SimpleMode: true,
		Children: []*CommandInfo{
			{Name: "cluster", Path: "cluster", SimpleMode: true, Children: []*CommandInfo{
				{Name: "create", Path: "cluster/create", SimpleMode: true,
					Parameters: []ParameterInfo{
						{Name: "Count", Long: "count", Type: "int", SimpleMode: true},
						{Name: "Name", Long: "name", Type: "string", SimpleMode: true},
					},
				},
			}},
		},
	}
	cfg.ApplyToCommandTree(root)

	gate := newSimpleModeGate(cfg, root)

	// Blocked flag explicitly passed -> error.
	err := gate.CheckArgs("cluster/create", map[string]any{"count": 3})
	if err == nil || !strings.Contains(err.Error(), "--count") {
		t.Errorf("expected --count to be blocked, got %v", err)
	}

	// Allowed flag -> no error.
	if err := gate.CheckArgs("cluster/create", map[string]any{"name": "dc1"}); err != nil {
		t.Errorf("expected name to be allowed, got %v", err)
	}

	// Unknown key -> ignored (runner will catch it).
	if err := gate.CheckArgs("cluster/create", map[string]any{"no-such-flag": 42}); err != nil {
		t.Errorf("expected unknown key to be ignored, got %v", err)
	}

	// bool=false is treated as "not set" (matches CLI semantics).
	if err := gate.CheckArgs("cluster/create", map[string]any{"count": false}); err != nil {
		t.Errorf("expected bool=false to be treated as unset, got %v", err)
	}
}

// TestSimpleModeGateDisabled confirms the gate is a no-op when ForceEnabled
// is false — soft simple mode still advertises everything in MCP.
func TestSimpleModeGateDisabled(t *testing.T) {
	cfg := &SimpleModeConfig{ForceEnabled: false}
	gate := newSimpleModeGate(cfg, nil)
	if err := gate.CheckCommand("anything"); err != nil {
		t.Errorf("expected no-op gate, got %v", err)
	}
	if err := gate.CheckArgs("anything", map[string]any{"x": 1}); err != nil {
		t.Errorf("expected no-op gate, got %v", err)
	}
}
