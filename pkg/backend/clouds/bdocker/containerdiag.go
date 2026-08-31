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
// podman published for the container's port 22. Nothing listens on that host
// port unless the container is running, so a container that died on startup
// looks exactly like a healthy one whose sshd is slow: every connection is
// refused. Podman makes this worse than docker by reporting the configured
// port mapping in the container list whether or not the container is running,
// so the port resolves fine and the only way to tell the two apart is to ask
// the daemon what the container is doing. These helpers do that, so a failure
// says "the container exited with code 1, here is its output" instead of
// repeating "connection refused" until the caller's budget runs out.

// containerLogTailLines is how much of a container's output to quote. Enough
// to catch an init system failing on startup, short enough to stay readable
// inside an error message.
const containerLogTailLines = 20

// containerLogTailBytes bounds the read, so a chatty container cannot stall
// the error path.
const containerLogTailBytes = 16 * 1024

// sshDiagnosticsAfter is how long the ssh-ready wait tolerates failures before
// reporting what the containers are doing. Long enough that a normally slow
// sshd never triggers it, short enough that a stuck run says why within the
// first minute instead of at the end of the budget.
const sshDiagnosticsAfter = 45 * time.Second

// containersDescribedOnFailure caps how many containers a single error
// describes. A failing cluster fails the same way on every node, so the first
// few carry the diagnosis and the rest would only bury it.
const containersDescribedOnFailure = 3

// containerDiagnoser is the slice of the docker client the diagnostics need,
// so they can be exercised without a daemon.
type containerDiagnoser interface {
	containerInspector
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
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
// ready to be appended to an error message.
func describeContainers(cli containerDiagnoser, ids []string) string {
	parts := []string{}
	for i, id := range ids {
		if i == containersDescribedOnFailure {
			parts = append(parts, fmt.Sprintf("(and %d more container(s) not shown)", len(ids)-i))
			break
		}
		parts = append(parts, describeContainer(cli, id))
	}
	return strings.Join(parts, "\n")
}

// describeContainer reports one container's lifecycle state, published host
// ports and the tail of its output.
func describeContainer(cli containerDiagnoser, id string) string {
	inspected, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return fmt.Sprintf("container %s: no longer exists, so it exited and was auto-removed", shortContainerID(id))
		}
		return fmt.Sprintf("container %s: could not be inspected: %s", shortContainerID(id), err)
	}
	c := inspected.Container
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
	fmt.Fprintf(sb, " hostPorts=%s", describeHostBindings(c.NetworkSettings))
	tty := c.Config != nil && c.Config.Tty
	if logs := containerLogTail(cli, id, tty); logs != "" {
		fmt.Fprintf(sb, "\n  last output:\n%s", logs)
	}
	return sb.String()
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

// containerLogTail returns the last few lines the container wrote, indented
// for quoting. Anything that goes wrong here yields an empty string: this runs
// on an error path and must never replace the failure it is explaining.
func containerLogTail(cli containerDiagnoser, id string, tty bool) string {
	rc, err := cli.ContainerLogs(context.Background(), id, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(containerLogTailLines),
	})
	if err != nil || rc == nil {
		return ""
	}
	defer rc.Close()
	out := new(bytes.Buffer)
	limited := io.LimitReader(rc, containerLogTailBytes)
	if tty {
		// A container started with a TTY has a single unframed stream.
		io.Copy(out, limited) //nolint:errcheck
	} else {
		stdcopy.StdCopy(out, out, limited) //nolint:errcheck
	}
	return indentLogTail(out.String())
}

// indentLogTail trims and indents container output, keeping at most the last
// containerLogTailLines lines in case the daemon ignored the tail request.
func indentLogTail(logs string) string {
	logs = strings.ReplaceAll(logs, "\r\n", "\n")
	lines := []string{}
	for _, line := range strings.Split(logs, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, "    "+strings.TrimRight(line, "\r"))
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > containerLogTailLines {
		lines = lines[len(lines)-containerLogTailLines:]
	}
	return strings.Join(lines, "\n")
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
// does not grow a pointless "everything is fine" tail.
func (s *b) diagnoseStopped(cli containerDiagnoser, ids []string) string {
	stopped := stoppedContainers(cli, ids)
	if len(stopped) == 0 {
		return ""
	}
	return "\n" + describeContainers(cli, stopped)
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
		if d := describeContainers(cli, ids); d != "" {
			parts = append(parts, d)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n" + strings.Join(parts, "\n")
}
