package bdocker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Aerolab reaches a container over 127.0.0.1 on the host port that docker or
// podman published for the container's port 22. A refused connection there has
// three possible causes that look identical from the outside: the container is
// not running, sshd is not listening inside a container that is running, or the
// container engine's virtual machine is not forwarding the published port to
// the host. Podman blurs the first case further by reporting the configured
// port mapping in the container list whether or not the container runs, so the
// port resolves fine and the connection is refused anyway.
//
// These helpers separate the three by asking the daemon what the container is
// doing and asking the container itself whether sshd is listening - over the
// exec API, which does not depend on sshd. That turns a wall of identical
// "connection refused" lines into a verdict.

// containerLogTailLines is how much of a container's output to quote. Enough
// to catch an init system failing on startup, short enough to stay readable
// inside an error message.
const containerLogTailLines = 20

// containerLogSSHLines caps how many ssh-related log lines are pulled out of
// the container's output when sshd turns out not to be listening.
const containerLogSSHLines = 6

// containerLogFetchLines is how far back the log is read. Larger than what is
// quoted, because the ssh unit's own lines are usually further up than the
// tail: an init system logs sshd early and then goes quiet.
const containerLogFetchLines = 200

// containerLogFetchBytes bounds the read, so a chatty container cannot stall
// the error path.
const containerLogFetchBytes = 64 * 1024

// containerExecTimeout bounds the in-container probe. It reads one proc file,
// so anything slower means the container is not answering at all.
const containerExecTimeout = 15 * time.Second

// sshDiagnosticsAfter is how long the ssh-ready wait tolerates failures before
// reporting what the containers are doing. Long enough that a normally slow
// sshd never triggers it, short enough that a stuck run says why within the
// first minute instead of at the end of the budget.
const sshDiagnosticsAfter = 45 * time.Second

// containersDescribedOnFailure caps how many containers a single error
// describes. A failing cluster fails the same way on every node, so the first
// few carry the diagnosis and the rest would only bury it.
const containersDescribedOnFailure = 3

// sshdListenPort is the in-container port aerolab expects sshd on.
const sshdListenPort = 22

// containerDiagnoser is the slice of the docker client the diagnostics need,
// so they can be exercised without a daemon.
type containerDiagnoser interface {
	containerInspector
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
}

// containerExec runs a command inside a container and returns its stdout. It is
// a function rather than a method so the probe can be tested without a daemon,
// and so callers that have no client (or a container that is not running) can
// pass nil to skip it.
type containerExec func(id string, cmd []string) ([]byte, error)

// dockerExec adapts the docker client to containerExec. Output is returned even
// when the command reports a non-zero exit, because a partly-successful read
// still answers the question being asked.
func dockerExec(cli *client.Client) containerExec {
	return func(id string, cmd []string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), containerExecTimeout)
		defer cancel()
		out := new(bytes.Buffer)
		code, err := ExecWithCLI(ctx, cli, id, cmd, nil, nil, out, io.Discard, false)
		if err != nil {
			return out.Bytes(), err
		}
		if code != 0 {
			return out.Bytes(), fmt.Errorf("exited with code %d", code)
		}
		return out.Bytes(), nil
	}
}

// stoppedContainers returns the subset of ids that the daemon no longer
// reports as running, including any that have gone away entirely (a container
// created with auto-remove is deleted as soon as it exits). Containers we
// cannot inspect are left out: an API error says nothing about the container
// itself, and treating it as dead would abort a wait that might still succeed.
func stoppedContainers(cli containerInspector, ids []string) []string {
	stopped := []string{}
	for _, id := range ids {
		inspected, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				stopped = append(stopped, id)
			}
			continue
		}
		if inspected.Container.State == nil || !inspected.Container.State.Running {
			stopped = append(stopped, id)
		}
	}
	return stopped
}

// describeContainers renders what the given containers are actually doing,
// ready to be appended to an error message. exec may be nil to skip the
// in-container probe.
func describeContainers(cli containerDiagnoser, exec containerExec, ids []string) string {
	parts := []string{}
	for i, id := range ids {
		if i == containersDescribedOnFailure {
			parts = append(parts, fmt.Sprintf("(and %d more container(s) not shown)", len(ids)-i))
			break
		}
		parts = append(parts, describeContainer(cli, exec, id))
	}
	return strings.Join(parts, "\n")
}

// describeContainer reports one container's lifecycle state, whether sshd is
// listening inside it, its published host ports, and the tail of its output -
// then states which of the three failure modes those facts add up to.
func describeContainer(cli containerDiagnoser, exec containerExec, id string) string {
	inspected, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return fmt.Sprintf("container %s: no longer exists, so it exited and was auto-removed", shortContainerID(id))
		}
		return fmt.Sprintf("container %s: could not be inspected: %s", shortContainerID(id), err)
	}
	c := inspected.Container
	running := c.State != nil && c.State.Running
	sb := new(strings.Builder)
	fmt.Fprintf(sb, "container %s:", shortContainerID(id))
	if state := c.State; state != nil {
		fmt.Fprintf(sb, " state=%s", state.Status)
		if !state.Running {
			fmt.Fprintf(sb, " exitCode=%d", state.ExitCode)
		}
		if state.OOMKilled {
			sb.WriteString(" oomKilled=true")
		}
		if state.Error != "" {
			fmt.Fprintf(sb, " error=%q", state.Error)
		}
	} else {
		sb.WriteString(" state=unknown")
	}
	if c.RestartCount > 0 {
		fmt.Fprintf(sb, " restarts=%d", c.RestartCount)
	}

	listening, probed := false, false
	if running && exec != nil {
		var err error
		listening, err = sshdListening(exec, id)
		switch {
		case err != nil:
			fmt.Fprintf(sb, " sshListening=unknown(%s)", err)
		case listening:
			probed = true
			sb.WriteString(" sshListening=yes")
		default:
			probed = true
			sb.WriteString(" sshListening=no")
		}
	}
	fmt.Fprintf(sb, " hostPorts=%s", describeHostBindings(c.NetworkSettings))
	if verdict := containerVerdict(running, probed, listening); verdict != "" {
		fmt.Fprintf(sb, "\n  %s", verdict)
	}

	logs := containerLogText(cli, id, c.Config != nil && c.Config.Tty)
	// When sshd is missing, the reason is in the ssh unit's own log lines, and
	// those are normally further up than the tail worth quoting.
	if probed && !listening {
		if ssh := grepLines(logs, "ssh", containerLogSSHLines); ssh != "" {
			fmt.Fprintf(sb, "\n  ssh-related output:\n%s", ssh)
		}
	}
	if tail := tailLines(logs, containerLogTailLines); tail != "" {
		fmt.Fprintf(sb, "\n  last output:\n%s", tail)
	}
	return sb.String()
}

// containerVerdict turns the observed facts into the one sentence the reader
// needs. It stays silent when the probe did not run, rather than guessing.
func containerVerdict(running, probed, listening bool) string {
	if !running {
		return "the container is not running, so no host port is published for it and every connection is refused"
	}
	if !probed {
		return ""
	}
	if listening {
		return "sshd IS listening inside the container, so the container is healthy and the host cannot reach its published port: the docker/podman virtual machine is not forwarding it (restarting the docker/podman machine usually fixes this)"
	}
	return "nothing is listening on port 22 inside the container, so sshd did not start; this is a problem inside the container, not with host port forwarding"
}

// sshdListening asks the container whether anything is listening on TCP port 22
// by reading /proc/net/tcp in its own network namespace. This goes over the
// docker/podman exec API, which does not depend on sshd, so it tells apart "no
// sshd" from "the host cannot reach a working sshd". Reading proc avoids
// depending on ss, netstat or lsof, none of which a minimal distro image has.
func sshdListening(exec containerExec, id string) (bool, error) {
	out, err := exec(id, []string{"cat", "/proc/net/tcp", "/proc/net/tcp6"})
	// A container with IPv6 disabled has no /proc/net/tcp6, so cat reports a
	// failure while still having printed the IPv4 table we care about.
	if err != nil && len(bytes.TrimSpace(out)) == 0 {
		return false, err
	}
	return listeningOnPort(out, sshdListenPort), nil
}

// listeningOnPort reports whether /proc/net/tcp content holds a socket in the
// LISTEN state on the given port.
func listeningOnPort(procNetTCP []byte, port uint16) bool {
	want := fmt.Sprintf(":%04X", port)
	for _, line := range strings.Split(string(procNetTCP), "\n") {
		// Columns: sl local_address rem_address st ... where local_address is
		// HEXADDR:HEXPORT and st 0A is TCP_LISTEN.
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		if f[3] == "0A" && strings.HasSuffix(strings.ToUpper(f[1]), want) {
			return true
		}
	}
	return false
}

// describeHostBindings renders the host side of a container's port mappings as
// the daemon sees it, which is what aerolab dials.
func describeHostBindings(ns *container.NetworkSettings) string {
	if ns == nil || len(ns.Ports) == 0 {
		return "none"
	}
	parts := []string{}
	for port, bindings := range ns.Ports {
		if len(bindings) == 0 {
			parts = append(parts, fmt.Sprintf("%s->unpublished", port))
			continue
		}
		for _, bind := range bindings {
			host := bind.HostPort
			if bind.HostIP.IsValid() {
				host = bind.HostIP.String() + ":" + bind.HostPort
			}
			parts = append(parts, fmt.Sprintf("%s->%s", port, host))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// containerLogText returns the recent output of a container. Anything that goes
// wrong here yields an empty string: this runs on an error path and must never
// replace the failure it is explaining.
func containerLogText(cli containerDiagnoser, id string, tty bool) string {
	rc, err := cli.ContainerLogs(context.Background(), id, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(containerLogFetchLines),
	})
	if err != nil || rc == nil {
		return ""
	}
	defer rc.Close()
	out := new(bytes.Buffer)
	limited := io.LimitReader(rc, containerLogFetchBytes)
	if tty {
		// A container started with a TTY has a single unframed stream.
		io.Copy(out, limited) //nolint:errcheck
	} else {
		stdcopy.StdCopy(out, out, limited) //nolint:errcheck
	}
	return out.String()
}

// logLines splits container output into non-blank lines with the carriage
// returns a TTY stream carries stripped.
func logLines(logs string) []string {
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(logs, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// tailLines returns the last n lines of container output, indented for quoting.
func tailLines(logs string, n int) string {
	lines := logLines(logs)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return indent(lines)
}

// grepLines returns up to n lines of container output containing match,
// indented for quoting. The last ones are kept: they are the most recent
// attempt at whatever is being looked for.
func grepLines(logs, match string, n int) string {
	hits := []string{}
	for _, line := range logLines(logs) {
		if strings.Contains(strings.ToLower(line), match) {
			hits = append(hits, line)
		}
	}
	if len(hits) > n {
		hits = hits[len(hits)-n:]
	}
	return indent(hits)
}

func indent(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return "    " + strings.Join(lines, "\n    ")
}

// shortContainerID trims a container ID to the 12 characters docker and podman
// print, so error messages match what the user sees in `docker ps`.
func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// diagnoseStopped describes any of the given containers that are not running,
// prefixed with a newline so it can be appended to an error. It returns an
// empty string when they are all running, so a message about something else
// does not grow a pointless "everything is fine" tail. A stopped container
// cannot be exec'd into, so no probe is attempted.
func (s *b) diagnoseStopped(cli containerDiagnoser, ids []string) string {
	stopped := stoppedContainers(cli, ids)
	if len(stopped) == 0 {
		return ""
	}
	return "\n" + describeContainers(cli, nil, stopped)
}

// describeInstanceContainers renders diagnostics for containers grouped by
// zone, prefixed with a newline so it can be appended to an error message.
func (s *b) describeInstanceContainers(byZone map[string][]string) string {
	parts := []string{}
	for zone, ids := range byZone {
		cli, err := s.getDockerClient(zone)
		if err != nil {
			continue
		}
		if d := describeContainers(cli, dockerExec(cli), ids); d != "" {
			parts = append(parts, d)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n" + strings.Join(parts, "\n")
}
