package bdocker_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/citrusleaf/aerolab/pkg/backend/clouds/bdocker"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const execTestImage = "ubuntu:24.04"

// newTestContainer starts a long-lived container to exec into, or skips the test
// if no usable Docker daemon is reachable.
func newTestContainer(t *testing.T) (*client.Client, string) {
	t.Helper()
	cli, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker: %v", err)
	}
	ctx := context.Background()
	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		t.Skipf("no docker: %v", err)
	}
	if _, err := cli.ImageInspect(ctx, execTestImage); err != nil {
		r, err := cli.ImagePull(ctx, execTestImage, client.ImagePullOptions{})
		if err != nil {
			t.Skipf("cannot pull %s: %v", execTestImage, err)
		}
		io.Copy(io.Discard, r) //nolint:errcheck
		r.Close()              //nolint:errcheck
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image: execTestImage,
			Cmd:   []string{"sleep", "300"},
			Tty:   false,
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		cli.ContainerRemove(context.Background(), created.ID, client.ContainerRemoveOptions{Force: true}) //nolint:errcheck
		cli.Close()                                                                                       //nolint:errcheck
	})
	return cli, created.ID
}

func TestExecWithCLI(t *testing.T) {
	cli, id := newTestContainer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("splits stdout and stderr", func(t *testing.T) {
		var out, errb bytes.Buffer
		code, err := bdocker.ExecWithCLI(ctx, cli, id,
			[]string{"/bin/sh", "-c", "echo to-stdout; echo to-stderr >&2"},
			nil, nil, &out, &errb, false)
		if err != nil || code != 0 {
			t.Fatalf("code=%d err=%v", code, err)
		}
		if got := out.String(); got != "to-stdout\n" {
			t.Errorf("stdout = %q, want %q", got, "to-stdout\n")
		}
		if got := errb.String(); got != "to-stderr\n" {
			t.Errorf("stderr = %q, want %q", got, "to-stderr\n")
		}
	})

	t.Run("propagates exit code", func(t *testing.T) {
		var out, errb bytes.Buffer
		code, err := bdocker.ExecWithCLI(ctx, cli, id,
			[]string{"/bin/sh", "-c", "echo bye; exit 42"},
			nil, nil, &out, &errb, false)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if code != 42 {
			t.Errorf("exit code = %d, want 42", code)
		}
		if got := out.String(); got != "bye\n" {
			t.Errorf("stdout = %q, want %q", got, "bye\n")
		}
	})

	t.Run("captures large output without truncation", func(t *testing.T) {
		var out, errb bytes.Buffer
		code, err := bdocker.ExecWithCLI(ctx, cli, id,
			[]string{"/bin/sh", "-c", "for i in $(seq 1 20000); do echo line-$i; done"},
			nil, nil, &out, &errb, false)
		if err != nil || code != 0 {
			t.Fatalf("code=%d err=%v", code, err)
		}
		lines := strings.Count(out.String(), "\n")
		if lines != 20000 {
			t.Errorf("stdout lines = %d, want 20000", lines)
		}
	})

	t.Run("pipes stdin", func(t *testing.T) {
		var out, errb bytes.Buffer
		stdin := io.NopCloser(strings.NewReader("hello-from-stdin\n"))
		code, err := bdocker.ExecWithCLI(ctx, cli, id,
			[]string{"/bin/cat"},
			nil, stdin, &out, &errb, false)
		if err != nil || code != 0 {
			t.Fatalf("code=%d err=%v", code, err)
		}
		if got := out.String(); got != "hello-from-stdin\n" {
			t.Errorf("stdout = %q, want %q", got, "hello-from-stdin\n")
		}
	})

	t.Run("tty merges streams raw", func(t *testing.T) {
		var out, errb bytes.Buffer
		code, err := bdocker.ExecWithCLI(ctx, cli, id,
			[]string{"/bin/sh", "-c", "echo tty-out; echo tty-err >&2"},
			nil, nil, &out, &errb, true)
		if err != nil || code != 0 {
			t.Fatalf("code=%d err=%v", code, err)
		}
		got := out.String()
		if !strings.Contains(got, "tty-out") || !strings.Contains(got, "tty-err") {
			t.Errorf("tty stdout = %q, want both streams merged", got)
		}
		if errb.Len() != 0 {
			t.Errorf("tty stderr = %q, want empty", errb.String())
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		cctx, ccancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer ccancel()
		var out, errb bytes.Buffer
		start := time.Now()
		code, err := bdocker.ExecWithCLI(cctx, cli, id,
			[]string{"sleep", "30"},
			nil, nil, &out, &errb, false)
		if err == nil {
			t.Fatalf("want error, got code=%d", code)
		}
		if elapsed := time.Since(start); elapsed > 10*time.Second {
			t.Errorf("took %v, want prompt cancellation", elapsed)
		}
	})
}
