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
	got := describeContainer(f, nil, id)
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
	got := describeContainer(f, nil, "c1")
	for _, want := range []string{"state=running", "hostPorts=22/tcp->0.0.0.0:2201", "sshd listening"} {
		if !strings.Contains(got, want) {
			t.Errorf("description should mention %q, got:\n%s", want, got)
		}
	}
}

func TestDescribeContainerGone(t *testing.T) {
	got := describeContainer(&fakeDiagnoser{}, nil, "c1")
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
	got := describeContainer(f, nil, "c1")
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
	got := describeContainers(&fakeDiagnoser{inspect: inspect}, nil, ids)
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

func TestTailLines(t *testing.T) {
	if got := tailLines("\r\n  \n", containerLogTailLines); got != "" {
		t.Errorf("blank output should render as empty, got %q", got)
	}
	lines := []string{}
	for i := range containerLogTailLines + 5 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	got := tailLines(strings.Join(lines, "\r\n"), containerLogTailLines)
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

func TestGrepLines(t *testing.T) {
	logs := strings.Join([]string{
		"systemd: starting dbus.service",
		"supervisor[ssh.service]: ExecStartPre=/usr/sbin/sshd -t failed with code=1",
		"systemd: ssh.service: Job for ssh.service failed",
		"systemd: Startup finished",
	}, "\n")
	got := grepLines(logs, "ssh", containerLogSSHLines)
	if strings.Count(got, "\n") != 1 {
		t.Errorf("expected the two ssh lines, got:\n%s", got)
	}
	if strings.Contains(got, "dbus") {
		t.Errorf("non-matching lines should be dropped, got:\n%s", got)
	}
	if got := grepLines(logs, "nothingmatches", containerLogSSHLines); got != "" {
		t.Errorf("no matches should render as empty, got %q", got)
	}
}

// The probe is what separates "sshd never started" from "the host cannot reach
// a healthy container", so both readings have to come out of a real proc dump.
func TestListeningOnPort(t *testing.T) {
	listening := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 21421 1 0000 100 0\n"
	established := "   0: 0100007F:1F90 0100007F:C1B4 01 00000000:00000000 00:00000000 00000000     0        0 21422 1 0000 100 0\n"

	if !listeningOnPort([]byte(listening), 22) {
		t.Error("a LISTEN socket on port 22 should be detected")
	}
	if listeningOnPort([]byte(established), 22) {
		t.Error("an unrelated established socket must not count")
	}
	// Port 22 present, but the socket is connected rather than listening.
	if listeningOnPort([]byte(strings.Replace(listening, " 0A ", " 01 ", 1)), 22) {
		t.Error("only the LISTEN state counts")
	}
	if listeningOnPort(nil, 22) {
		t.Error("empty proc output must not report a listener")
	}
	if listeningOnPort([]byte(listening), 8080) {
		t.Error("a different port must not match")
	}
}

func TestSSHdListeningIgnoresPartialFailure(t *testing.T) {
	// cat /proc/net/tcp /proc/net/tcp6 exits non-zero when IPv6 is off, having
	// already printed the IPv4 table.
	out := []byte("   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1\n")
	exec := func(string, []string) ([]byte, error) { return out, errors.New("exited with code 1") }
	listening, err := sshdListening(exec, "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !listening {
		t.Error("the IPv4 table alone should be enough to report a listener")
	}

	exec = func(string, []string) ([]byte, error) { return nil, errors.New("container not running") }
	if _, err := sshdListening(exec, "c1"); err == nil {
		t.Error("expected an error when the probe produced no output at all")
	}
}

func TestDescribeContainerVerdicts(t *testing.T) {
	listening := []byte("   0: 00000000:0016 00000000:0000 0A 0:0 00:0 0 0 0 1 1\n")
	quiet := []byte("   0: 00000000:1F90 00000000:0000 0A 0:0 00:0 0 0 0 1 1\n")
	f := &fakeDiagnoser{
		inspect: map[string]container.InspectResponse{
			"c1": ttyContainer(&container.State{Status: "running", Running: true}, sshBinding("2200")),
		},
		logs: map[string]string{"c1": "supervisor[ssh.service]: main process exited before READY=1 (code=1)\nsystemd: Startup finished\n"},
	}

	got := describeContainer(f, func(string, []string) ([]byte, error) { return listening, nil }, "c1")
	if !strings.Contains(got, "sshListening=yes") || !strings.Contains(got, "not forwarding it") {
		t.Errorf("a healthy container should blame host port forwarding, got:\n%s", got)
	}

	got = describeContainer(f, func(string, []string) ([]byte, error) { return quiet, nil }, "c1")
	if !strings.Contains(got, "sshListening=no") || !strings.Contains(got, "sshd did not start") {
		t.Errorf("a container without sshd should blame the container, got:\n%s", got)
	}
	if !strings.Contains(got, "ssh-related output") || !strings.Contains(got, "ssh.service") {
		t.Errorf("ssh log lines should be surfaced when sshd is missing, got:\n%s", got)
	}

	// A probe that cannot run must not produce a confident verdict.
	got = describeContainer(f, func(string, []string) ([]byte, error) { return nil, errors.New("daemon busy") }, "c1")
	if !strings.Contains(got, "sshListening=unknown") {
		t.Errorf("expected an unknown probe result, got:\n%s", got)
	}
	if strings.Contains(got, "sshd did not start") || strings.Contains(got, "not forwarding it") {
		t.Errorf("no verdict should be given without a probe result, got:\n%s", got)
	}
}
