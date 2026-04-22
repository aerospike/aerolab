//go:build !noaerolabmcp

package cmd

import (
	"testing"

	aerolabmcp "github.com/aerospike/aerolab/pkg/mcp"
)

// TestConvertParamsForMCPPreservesInjectionSignal is a regression guard
// for the bug where convertParamsForMCP used to mutate the output
// parameter's Default from "table" to "json". Once the Default was
// flipped, aerolabmcp.ShouldForceJSONOutputParam would return false at
// runtime (because it excludes params already defaulting to a JSON-
// family value), which in turn disabled the auto-injection the whole
// feature was supposed to provide.
//
// The converter must therefore leave Default alone and let the runtime
// decide whether to inject.
func TestConvertParamsForMCPPreservesInjectionSignal(t *testing.T) {
	ci := &CommandInfo{
		Name: "list",
		Path: "cluster/list",
		Parameters: []ParameterInfo{
			{
				Name:        "Output",
				Long:        "output",
				Type:        "string",
				Default:     "table",
				Description: "Output format (text, table, json, json-indent, jq)",
			},
		},
	}

	got := convertParamsForMCP(ci, nil, true, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 param, got %d", len(got))
	}
	p := got[0]
	if p.Default != "table" {
		t.Fatalf("expected Default to be preserved as 'table', got %q", p.Default)
	}
	if !aerolabmcp.ShouldForceJSONOutputParam(p) {
		t.Fatalf("expected ShouldForceJSONOutputParam to still return true after conversion; got false (Default=%q)", p.Default)
	}
	if p.Description == ci.Parameters[0].Description {
		t.Errorf("expected description to be annotated with MCP hint; got unchanged: %q", p.Description)
	}
}

// TestConvertParamsForMCPDisabled verifies no description mutation when
// the force-json feature is disabled.
func TestConvertParamsForMCPDisabled(t *testing.T) {
	ci := &CommandInfo{
		Name: "list",
		Path: "cluster/list",
		Parameters: []ParameterInfo{
			{
				Name:        "Output",
				Long:        "output",
				Type:        "string",
				Default:     "table",
				Description: "Output format (text, table, json, json-indent, jq)",
			},
		},
	}
	got := convertParamsForMCP(ci, nil, false, false)
	if got[0].Description != ci.Parameters[0].Description {
		t.Errorf("expected description untouched when forceJSONOutput=false; got %q", got[0].Description)
	}
}
