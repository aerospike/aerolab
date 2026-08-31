package bdocker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
)

func TestImageNaming(t *testing.T) {
	cases := []struct {
		distro, version, arch, want string
	}{
		{"ubuntu", "22.04", "amd64", "amd64/ubuntu:22.04"},
		{"ubuntu", "22.04", "arm64", "arm64v8/ubuntu:22.04"},
		{"ubuntu", "26.04", "amd64", "amd64/ubuntu:26.04"},
		{"ubuntu", "26.04", "arm64", "arm64v8/ubuntu:26.04"},
		{"debian", "12", "amd64", "amd64/debian:12"},
		{"ubuntu", "22.04", "", "ubuntu:22.04"}, // fallthrough to default
		{"rocky", "9", "amd64", "amd64/rockylinux:9"},
		{"rocky", "9", "arm64", "arm64v8/rockylinux:9"},
		{"rocky", "9", "", "rockylinux:9"},
		// The arch-prefixed rockylinux repos stop at 9; 10+ is multi-arch only.
		{"rocky", "10", "amd64", "rockylinux/rockylinux:10"},
		{"rocky", "10", "arm64", "rockylinux/rockylinux:10"},
		{"rocky", "10", "", "rockylinux/rockylinux:10"},
		{"amazon", "2023", "amd64", "amd64/amazonlinux:2023"},
		{"amazon", "2023", "arm64", "arm64v8/amazonlinux:2023"},
		{"centos", "6", "amd64", "quay.io/centos/centos:6"},
		{"centos", "7", "arm64", "quay.io/centos/centos:7"},
		{"centos", "9", "amd64", "quay.io/centos/amd64:stream9"},
		{"centos", "9", "arm64", "quay.io/centos/arm64v8:stream9"},
		{"centos", "9", "", "quay.io/centos/centos:stream9"},
		{"centos", "10", "amd64", "quay.io/centos/amd64:stream10"},
		{"centos", "10", "arm64", "quay.io/centos/arm64v8:stream10"},
		{"alpine", "3.19", "amd64", "alpine:3.19"}, // unknown distro -> default
	}
	for _, c := range cases {
		if got := ImageNaming(c.distro, c.version, c.arch); got != c.want {
			t.Errorf("ImageNaming(%q,%q,%q) = %q, want %q", c.distro, c.version, c.arch, got, c.want)
		}
	}
}

func TestComputeAccessURL(t *testing.T) {
	ports := []container.PortSummary{
		{PrivatePort: 8080, PublicPort: 49153},
		{PrivatePort: 3000, PublicPort: 0}, // not published
	}
	cases := []struct {
		clientType string
		ports      []container.PortSummary
		want       string
	}{
		{"", ports, ""},
		{"unknown", ports, ""},
		{"vscode", ports, "http://localhost:49153"},
		{"ams", ports, ""},   // mapped port has PublicPort 0
		{"graph", ports, ""}, // no matching private port
		{"vscode", nil, ""},
	}
	for _, c := range cases {
		if got := computeAccessURL(c.clientType, c.ports); got != c.want {
			t.Errorf("computeAccessURL(%q, %v) = %q, want %q", c.clientType, c.ports, got, c.want)
		}
	}
}

func TestLifeCycleState(t *testing.T) {
	cases := []struct {
		state container.ContainerState
		want  backends.LifeCycleState
	}{
		{"running", backends.LifeCycleStateRunning},
		{"created", backends.LifeCycleStateCreated},
		{"exited", backends.LifeCycleStateStopped},
		{"dead", backends.LifeCycleStateFail},
		{"paused", backends.LifeCycleStateUnknown},
		{"restarting", backends.LifeCycleStateStarting},
		// Unknown states report as running, matching the historic mapping.
		{"removing", backends.LifeCycleStateRunning},
		{"", backends.LifeCycleStateRunning},
	}
	for _, c := range cases {
		if got := lifeCycleState(c.state); got != c.want {
			t.Errorf("lifeCycleState(%q) = %q, want %q", c.state, got, c.want)
		}
	}
}

func instanceWithPorts(id string, ports []container.PortSummary) *backends.Instance {
	return &backends.Instance{
		InstanceID:      id,
		BackendSpecific: &InstanceDetail{Docker: container.Summary{ID: id, Ports: ports}},
	}
}

func TestSSHHostPort(t *testing.T) {
	got, err := sshHostPort(instanceWithPorts("c1", []container.PortSummary{
		{PrivatePort: 3000, PublicPort: 49152, Type: "tcp"},
		{PrivatePort: 22, PublicPort: 2200, Type: "tcp"},
	}))
	if err != nil {
		t.Fatalf("sshHostPort with a published port 22: unexpected error: %v", err)
	}
	if got != 2200 {
		t.Errorf("sshHostPort = %d, want 2200", got)
	}
}

// A summary read before the container was running carries either no ports at
// all or port 22 with no host port. Both used to silently resolve to host port
// 0 and were then retried until the caller's budget expired, so both must now
// surface as errors.
func TestSSHHostPortUnresolved(t *testing.T) {
	cases := []struct {
		name  string
		ports []container.PortSummary
	}{
		{"no ports at all", nil},
		{"port 22 not published", []container.PortSummary{{PrivatePort: 22, PublicPort: 0, Type: "tcp"}}},
		{"other ports only", []container.PortSummary{{PrivatePort: 3000, PublicPort: 49152, Type: "tcp"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			port, err := sshHostPort(instanceWithPorts("c1", c.ports))
			if err == nil {
				t.Fatalf("expected an error, got port %d", port)
			}
			if port != 0 {
				t.Errorf("expected port 0 alongside the error, got %d", port)
			}
			if !strings.Contains(err.Error(), "c1") {
				t.Errorf("error should name the container, got %q", err)
			}
		})
	}
}

func TestUnresolvedSSHTargets(t *testing.T) {
	running := func(i *backends.Instance) *backends.Instance {
		i.InstanceState = backends.LifeCycleStateRunning
		return i
	}
	sshPublished := []container.PortSummary{{PrivatePort: 22, PublicPort: 2200, Type: "tcp"}}

	cases := []struct {
		name      string
		instances backends.InstanceList
		want      bool
	}{
		{"all resolved", backends.InstanceList{running(instanceWithPorts("c1", sshPublished))}, false},
		{"no ports yet", backends.InstanceList{running(instanceWithPorts("c1", nil))}, true},
		{"not running yet", backends.InstanceList{instanceWithPorts("c1", sshPublished)}, true},
		{"one of two unresolved", backends.InstanceList{
			running(instanceWithPorts("c1", sshPublished)),
			running(instanceWithPorts("c2", nil)),
		}, true},
		{"empty list", backends.InstanceList{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unresolvedSSHTargets(c.instances); got != c.want {
				t.Errorf("unresolvedSSHTargets = %v, want %v", got, c.want)
			}
		})
	}
}

// fakeInspector returns a canned state per call, so the wait loop can be driven
// through its transitions without a daemon.
type fakeInspector struct {
	states []*container.State
	calls  int
	err    error
}

func (f *fakeInspector) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if f.err != nil {
		return client.ContainerInspectResult{}, f.err
	}
	state := f.states[min(f.calls, len(f.states)-1)]
	f.calls++
	return client.ContainerInspectResult{
		Container: container.InspectResponse{State: state},
	}, nil
}

type discardLog struct{}

func (discardLog) Detail(string, ...any) {}

func TestWaitForContainersRunning(t *testing.T) {
	s := &b{}
	created := &container.State{Status: "created"}
	run := &container.State{Status: "running", Running: true}

	t.Run("already running", func(t *testing.T) {
		f := &fakeInspector{states: []*container.State{run}}
		if err := s.waitForContainersRunning(f, []string{"c1"}, time.Minute, discardLog{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("running after a few polls", func(t *testing.T) {
		f := &fakeInspector{states: []*container.State{created, created, run}}
		if err := s.waitForContainersRunning(f, []string{"c1"}, time.Minute, discardLog{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.calls != 3 {
			t.Errorf("expected 3 inspect calls, got %d", f.calls)
		}
	})

	// An exited container never recovers on its own, so the wait must report the
	// exit code straight away instead of consuming the whole budget.
	t.Run("exited fails fast", func(t *testing.T) {
		f := &fakeInspector{states: []*container.State{{Status: "exited", ExitCode: 137, Error: "oom"}}}
		start := time.Now()
		err := s.waitForContainersRunning(f, []string{"c1"}, time.Minute, discardLog{})
		if err == nil {
			t.Fatal("expected an error for an exited container")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("should not have waited out the budget, took %s", elapsed)
		}
		for _, want := range []string{"exited", "137", "oom"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should mention %q", err, want)
			}
		}
	})

	t.Run("budget exhausted", func(t *testing.T) {
		f := &fakeInspector{states: []*container.State{created}}
		err := s.waitForContainersRunning(f, []string{"c1"}, 10*time.Millisecond, discardLog{})
		if err == nil {
			t.Fatal("expected a timeout error")
		}
		if !strings.Contains(err.Error(), "did not reach running state") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("nil state", func(t *testing.T) {
		f := &fakeInspector{states: []*container.State{nil}}
		err := s.waitForContainersRunning(f, []string{"c1"}, time.Minute, discardLog{})
		if err == nil || !strings.Contains(err.Error(), "no state") {
			t.Fatalf("expected a nil-state error, got %v", err)
		}
	})

	t.Run("inspect error", func(t *testing.T) {
		f := &fakeInspector{err: errors.New("boom")}
		err := s.waitForContainersRunning(f, []string{"c1"}, time.Minute, discardLog{})
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected the inspect error to surface, got %v", err)
		}
	})
}

func TestDescribePorts(t *testing.T) {
	if got := describePorts(nil); got != "none" {
		t.Errorf("describePorts(nil) = %q, want %q", got, "none")
	}
	got := describePorts([]container.PortSummary{
		{PrivatePort: 22, PublicPort: 2200, Type: "tcp"},
		{PrivatePort: 3000, PublicPort: 0, Type: "tcp"},
	})
	want := "2200->22/tcp,0->3000/tcp"
	if got != want {
		t.Errorf("describePorts = %q, want %q", got, want)
	}
}

func TestEncodeAuthToBase64(t *testing.T) {
	auth := registry.AuthConfig{Username: "user", Password: "pass", ServerAddress: "ghcr.io"}
	got, err := encodeAuthToBase64(auth)
	if err != nil {
		t.Fatalf("encodeAuthToBase64: %v", err)
	}
	decoded, err := base64.URLEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("result is not valid base64url: %v", err)
	}
	var back registry.AuthConfig
	if err := json.Unmarshal(decoded, &back); err != nil {
		t.Fatalf("decoded payload is not valid JSON auth: %v", err)
	}
	if back.Username != auth.Username || back.Password != auth.Password || back.ServerAddress != auth.ServerAddress {
		t.Errorf("round-trip mismatch: got %+v, want %+v", back, auth)
	}
}
