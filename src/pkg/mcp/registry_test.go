package mcp

import "testing"

func TestRegistryFindEmptyReturnsRoot(t *testing.T) {
	root := &Command{Name: "aerolab", Path: "aerolab"}
	reg := &Registry{Root: []*Command{root}}

	if got := reg.Find(""); got != root {
		t.Errorf("Find(\"\") = %v, want root", got)
	}
	if got := reg.Find("/"); got != root {
		t.Errorf("Find(\"/\") = %v, want root", got)
	}
}

func TestRegistryFindEmptyNilOnMultipleRoots(t *testing.T) {
	reg := &Registry{Root: []*Command{
		{Name: "a", Path: "a"},
		{Name: "b", Path: "b"},
	}}
	if got := reg.Find(""); got != nil {
		t.Errorf("Find(\"\") with two roots = %v, want nil", got)
	}
}

func TestRegistryFindEmptyNilOnNoRoots(t *testing.T) {
	reg := &Registry{}
	if got := reg.Find(""); got != nil {
		t.Errorf("Find(\"\") on empty registry = %v, want nil", got)
	}
}

func TestRegistryFindChild(t *testing.T) {
	leaf := &Command{Name: "list", Path: "cluster/list"}
	reg := &Registry{Root: []*Command{{
		Name: "aerolab", Path: "aerolab",
		Children: []*Command{{Name: "cluster", Path: "cluster", Children: []*Command{leaf}}},
	}}}
	if got := reg.Find("cluster/list"); got != leaf {
		t.Errorf("Find child = %v, want leaf", got)
	}
}
