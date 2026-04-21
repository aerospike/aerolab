package mcp

import (
	"errors"
	"testing"
)

func TestParseProfile(t *testing.T) {
	cases := []struct {
		in   string
		want Profile
		err  bool
	}{
		{"", ProfileStandard, false},
		{"standard", ProfileStandard, false},
		{"STANDARD", ProfileStandard, false},
		{"read-only", ProfileReadOnly, false},
		{"readonly", ProfileReadOnly, false},
		{"admin", ProfileAdmin, false},
		{"root", "", true},
	}
	for _, c := range cases {
		got, err := ParseProfile(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParseProfile(%q) err=%v, want err=%v", c.in, err, c.err)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("ParseProfile(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsDestructiveHeuristics(t *testing.T) {
	cases := []struct {
		name string
		cmd  *Command
		want bool
	}{
		{"nil", nil, false},
		{"list", &Command{Name: "list", Path: "inventory/list"}, false},
		{"explicit flag", &Command{Name: "list", Path: "inventory/list", Destructive: true}, true},
		{"destroy leaf", &Command{Name: "destroy", Path: "cluster/destroy"}, true},
		{"delete leaf", &Command{Name: "delete", Path: "agi/delete"}, true},
		{"terminate leaf", &Command{Name: "terminate", Path: "eks/terminate"}, true},
		{"reboot leaf", &Command{Name: "reboot", Path: "cluster/reboot"}, true},
		{"status leaf", &Command{Name: "status", Path: "cluster/status"}, false},
		{"contains destroy", &Command{Name: "destroy-plan", Path: "agi/destroy-plan"}, true},
	}
	for _, c := range cases {
		if got := IsDestructive(c.cmd); got != c.want {
			t.Errorf("IsDestructive(%q): got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGateReadOnly(t *testing.T) {
	g := NewGate(ProfileReadOnly)
	if err := g.Check(&Command{Name: "list", Path: "inventory/list"}, false); err != nil {
		t.Errorf("read-only list: unexpected err %v", err)
	}
	err := g.Check(&Command{Name: "destroy", Path: "cluster/destroy"}, true)
	if !errors.Is(err, ErrProfileReadOnly) {
		t.Errorf("read-only destroy: want ErrProfileReadOnly, got %v", err)
	}
}

func TestGateStandard(t *testing.T) {
	g := NewGate(ProfileStandard)
	if err := g.Check(&Command{Name: "list", Path: "inventory/list"}, false); err != nil {
		t.Errorf("standard list: unexpected err %v", err)
	}
	err := g.Check(&Command{Name: "destroy", Path: "cluster/destroy"}, false)
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Errorf("standard destroy without confirm: want ErrConfirmationRequired, got %v", err)
	}
	if err := g.Check(&Command{Name: "destroy", Path: "cluster/destroy"}, true); err != nil {
		t.Errorf("standard destroy with confirm: unexpected err %v", err)
	}
}

func TestGateAdmin(t *testing.T) {
	g := NewGate(ProfileAdmin)
	if err := g.Check(&Command{Name: "destroy", Path: "cluster/destroy"}, false); err != nil {
		t.Errorf("admin destroy: unexpected err %v", err)
	}
}

func TestGateNil(t *testing.T) {
	var g *ProfileGate
	if err := g.Check(&Command{Name: "destroy", Path: "x/destroy"}, false); err != nil {
		t.Errorf("nil gate should allow everything, got %v", err)
	}
}
