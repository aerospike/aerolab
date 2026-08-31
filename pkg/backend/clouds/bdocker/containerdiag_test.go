package bdocker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// fakeDiagnoser answers inspect and logs calls from canned per-container data.
type fakeDiagnoser struct {
	inspect map[string]container.InspectResponse
	logs    map[string]string
	logErr  error
}

func (f *fakeDiagnoser) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	c, ok := f.inspect[id]
	if !ok {
		return client.ContainerInspectResult{}, fmt.Errorf("no such container: %s: %w", id, cerrdefs.ErrNotFound)
	}
	return client.ContainerInspectResult{Container: c}, nil
}

func (f *fakeDiagnoser) ContainerLogs(_ context.Context, id string, _ client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	if f.logErr != nil {
		return nil, f.logErr
	}
	return io.NopCloser(strings.NewReader(f.logs[id])), nil
}

// ttyContainer builds an inspect response for a container started the way
// aerolab starts them: with a TTY, so its log stream is unframed.
func ttyContainer(state *container.State, bindings network.PortMap) container.InspectResponse {
	return container.InspectResponse{
		State:           state,
		Config:          &container.Config{Tty: true},
		NetworkSettings: &container.NetworkSettings{Ports: bindings},
	}
}

func sshBinding(hostPort string) network.PortMap {
	return network.PortMap{
		network.MustParsePort("22/tcp"): []network.PortBinding{
			{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
		},
	}
}

func TestStoppedContainers(t *testing.T) {
	f := &fakeDiagnoser{inspect: map[string]container.InspectResponse{
		"running":   ttyContainer(&container.State{Status: "running", Running: true}, sshBinding("2200")),
		"exited":    ttyContainer(&container.State{Status: "exited", ExitCode: 1}, nil),
		"stateless": {},
	}}
	got := stoppedContainers(f, []string{"running", "exited", "stateless", "gone"})
	want := []string{"exited", "stateless", "gone"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("stoppedContainers = %v, want %v", got, want)
	}
}

// An inspect failure that is not "no such container" tells us nothing about
// the container, so it must not abort a wait that could still succeed.
func TestStoppedContainersIgnoresAPIErrors(t *testing.T) {
	f := &errInspector{err: errors.New("connection reset by peer")}
	if got := stoppedContainers(f, []string{"c1"}); len(got) != 0 {
		t.Errorf("stoppedContainers = %v, want none", got)
	}
}

type errInspector struct{ err error }

func (e *errInspector) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, e.err
}

func TestDescribeContainerExited(t *testing.T) {
	id := "670039577a063af69f7c39b16c7c89bc05ab261a704fd8513813281d493bced8"
	f := &fakeDiagnoser{
		inspect: map[string]container.InspectResponse{
			id: ttyContainer(&container.State{Status: "exited", ExitCode: 127, Error: "exec format error"}, nil),
		},
		logs: map[string]string{id: "starting aerolab init\r\nexec /usr/sbin/init: exec format error\r\n"},
	}
	got := describeContainer(f, id)
	for _, want := range []string{"670039577a06", "state=exited", "exitCode=127", "exec format error", "hostPorts=none"} {
		if !strings.Contains(got, want) {
			t.Errorf("description should mention %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, id) {
		t.Errorf("description should use the short container ID, got:\n%s", got)
	}
}

// The case that matters most for a podman host: the container is running and
// the mapping exists, so the diagnosis has to point at the host side.
func TestDescribeContainerRunning(t *testing.T) {
	f := &fakeDiagnoser{
		inspect: map[string]container.InspectResponse{
			"c1": ttyContainer(&container.State{Status: "running", Running: true}, sshBinding("2201")),
		},
		logs: map[string]string{"c1": "sshd listening on 0.0.0.0:22\n"},
	}
	got := describeContainer(f, "c1")
	for _, want := range []string{"state=running", "hostPorts=22/tcp->0.0.0.0:2201", "sshd listening"} {
		if !strings.Contains(got, want) {
			t.Errorf("description should mention %q, got:\n%s", want, got)
		}
	}
}

func TestDescribeContainerGone(t *testing.T) {
	got := describeContainer(&fakeDiagnoser{}, "c1")
	if !strings.Contains(got, "auto-removed") {
		t.Errorf("a missing container should be reported as auto-removed, got: %s", got)
	}
}

// Diagnostics run on an error path, so a daemon that will not hand over logs
// must not prevent the state from being reported.
func TestDescribeContainerLogsUnavailable(t *testing.T) {
	f := &fakeDiagnoser{
		inspect: map[string]container.InspectResponse{"c1": ttyContainer(&container.State{Status: "exited"}, nil)},
		logErr:  errors.New("logs unavailable"),
	}
	got := describeContainer(f, "c1")
	if !strings.Contains(got, "state=exited") {
		t.Errorf("state should still be reported, got: %s", got)
	}
	if strings.Contains(got, "last output") {
		t.Errorf("no log section expected, got: %s", got)
	}
}

func TestDescribeContainersCapsOutput(t *testing.T) {
	ids := []string{}
	inspect := map[string]container.InspectResponse{}
	for i := range 5 {
		id := fmt.Sprintf("c%d", i)
		ids = append(ids, id)
		inspect[id] = ttyContainer(&container.State{Status: "exited"}, nil)
	}
	got := describeContainers(&fakeDiagnoser{inspect: inspect}, ids)
	if strings.Count(got, "state=exited") != containersDescribedOnFailure {
		t.Errorf("expected %d containers described, got:\n%s", containersDescribedOnFailure, got)
	}
	if !strings.Contains(got, "and 2 more container(s)") {
		t.Errorf("expected a note about the containers left out, got:\n%s", got)
	}
}

func TestDescribeHostBindings(t *testing.T) {
	if got := describeHostBindings(nil); got != "none" {
		t.Errorf("describeHostBindings(nil) = %q, want %q", got, "none")
	}
	if got := describeHostBindings(&container.NetworkSettings{}); got != "none" {
		t.Errorf("describeHostBindings(empty) = %q, want %q", got, "none")
	}
	// An exposed-but-unpublished port is exactly the state that resolves to
	// host port 0, so it has to be visible in the diagnosis.
	ports := network.PortMap{network.MustParsePort("3000/tcp"): nil}
	if got := describeHostBindings(&container.NetworkSettings{Ports: ports}); got != "3000/tcp->unpublished" {
		t.Errorf("describeHostBindings = %q", got)
	}
}

func TestIndentLogTail(t *testing.T) {
	if got := indentLogTail("\r\n  \n"); got != "" {
		t.Errorf("blank output should render as empty, got %q", got)
	}
	lines := []string{}
	for i := range containerLogTailLines + 5 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	got := indentLogTail(strings.Join(lines, "\r\n"))
	gotLines := strings.Split(got, "\n")
	if len(gotLines) != containerLogTailLines {
		t.Fatalf("expected %d lines, got %d", containerLogTailLines, len(gotLines))
	}
	if !strings.HasPrefix(gotLines[0], "    line 5") {
		t.Errorf("expected the last lines, indented; got %q", gotLines[0])
	}
	if strings.Contains(got, "\r") {
		t.Errorf("carriage returns should be stripped, got %q", got)
	}
}

func TestDiagnoseStopped(t *testing.T) {
	s := &b{}
	f := &fakeDiagnoser{inspect: map[string]container.InspectResponse{
		"running": ttyContainer(&container.State{Status: "running", Running: true}, sshBinding("2200")),
		"exited":  ttyContainer(&container.State{Status: "exited", ExitCode: 1}, nil),
	}}
	if got := s.diagnoseStopped(f, []string{"running"}); got != "" {
		t.Errorf("all running should describe nothing, got %q", got)
	}
	got := s.diagnoseStopped(f, []string{"running", "exited"})
	if !strings.HasPrefix(got, "\n") {
		t.Errorf("diagnostics should start on their own line, got %q", got)
	}
	if !strings.Contains(got, "exitCode=1") || strings.Contains(got, "state=running") {
		t.Errorf("only the stopped container should be described, got %q", got)
	}
}
