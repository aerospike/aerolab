package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Implementation re-exports the SDK's server Implementation record so
// callers do not need to import the SDK directly.
type Implementation = sdkmcp.Implementation

// Transport identifies the MCP transport used by this process.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
)

// Config is the full configuration consumed by Serve/NewServer. Most
// callers populate this from the cmdMcp.go McpCmd flags.
type Config struct {
	// Transport selects stdio or http. An empty string defaults to stdio.
	Transport Transport

	// Addr is the listen address when Transport == http
	// (e.g. "localhost:9190").
	Addr string

	// AuthToken, when non-empty, is required as a Bearer credential on
	// every HTTP request.
	AuthToken string

	// Profile controls gating of destructive commands.
	Profile Profile

	// SessionTimeout is the idle timeout for streamable HTTP sessions.
	SessionTimeout time.Duration

	// Logger receives server-level log events. When nil, a default
	// discarding logger is used.
	Logger *slog.Logger

	// Implementation is the server's advertised name/version.
	Implementation *Implementation

	// Registry holds the command tree, runner, help, and gate.
	Registry *Registry
}

// NewServer builds an *sdkmcp.Server with all aerolab tools registered.
// The caller is responsible for driving the selected transport; Serve is
// a convenience that does this in one call.
func NewServer(cfg Config) (*sdkmcp.Server, error) {
	if cfg.Registry == nil {
		return nil, errors.New("mcp: config.Registry is required")
	}
	if cfg.Implementation == nil {
		cfg.Implementation = &sdkmcp.Implementation{Name: "aerolab-mcp", Version: "dev"}
	}

	opts := &sdkmcp.ServerOptions{
		Instructions: defaultInstructions,
		Logger:       cfg.Logger,
	}
	server := sdkmcp.NewServer(cfg.Implementation, opts)

	// A single registration set is shared between the generic and
	// per-leaf registrations so a later auto-tool with the same name
	// cannot overwrite an earlier one (or vice-versa).
	seen := map[string]struct{}{}
	RegisterGenericToolsWith(server, cfg.Registry, seen)
	RegisterAutoToolsWith(server, cfg.Registry, seen)
	return server, nil
}

// addUnique wraps server.AddTool with a de-duplication check against
// seen. It returns true when the tool was actually registered. When a
// duplicate is encountered, the registration is skipped and a warning
// goes to the logger (falling back to stderr).
func addUnique(server *sdkmcp.Server, seen map[string]struct{}, logger *slog.Logger, tool *sdkmcp.Tool, handler sdkmcp.ToolHandler) bool {
	if server == nil || tool == nil {
		return false
	}
	if seen != nil {
		if _, dup := seen[tool.Name]; dup {
			msg := fmt.Sprintf("mcp: duplicate tool registration skipped: %q", tool.Name)
			if logger != nil {
				logger.Warn(msg)
			} else {
				fmt.Fprintln(os.Stderr, msg)
			}
			return false
		}
		seen[tool.Name] = struct{}{}
	}
	server.AddTool(tool, handler)
	return true
}

// Serve runs the configured server on the selected transport and blocks
// until the context is cancelled or the transport fails. It is safe to
// call from McpCmd.Execute.
func Serve(ctx context.Context, cfg Config) error {
	server, err := NewServer(cfg)
	if err != nil {
		return err
	}

	switch Transport(strings.ToLower(string(cfg.Transport))) {
	case "", TransportStdio:
		return server.Run(ctx, &sdkmcp.StdioTransport{})
	case TransportHTTP:
		return serveHTTP(ctx, server, cfg)
	}
	return fmt.Errorf("mcp: unknown transport %q", cfg.Transport)
}

// maxHTTPBodyBytes caps every request body. 1 MiB is comfortably larger
// than any JSON-RPC frame the MCP spec asks clients to send while still
// preventing slowloris-body or accidental large uploads from tying up a
// server goroutine.
const maxHTTPBodyBytes = 1 << 20

// httpIdleTimeout closes idle streamable-HTTP sessions after two minutes
// to reclaim file descriptors. ReadTimeout and WriteTimeout are left
// unset because streamable HTTP holds connections open by design.
const httpIdleTimeout = 2 * time.Minute

// serveHTTP mounts the streamable handler at /mcp and starts an HTTP
// listener on cfg.Addr. Graceful shutdown on ctx cancel.
func serveHTTP(ctx context.Context, server *sdkmcp.Server, cfg Config) error {
	streamOpts := &sdkmcp.StreamableHTTPOptions{
		Logger:         cfg.Logger,
		SessionTimeout: cfg.SessionTimeout,
	}
	var handler http.Handler = sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return server },
		streamOpts,
	)
	handler = http.MaxBytesHandler(handler, maxHTTPBodyBytes)

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.Handle("/mcp/", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	var root http.Handler = mux
	if cfg.AuthToken != "" {
		root = BearerMiddleware(cfg.AuthToken)(root)
		msg := "WARNING: aerolab mcp is serving bearer auth over plain HTTP; tokens travel in clear. Terminate TLS at a reverse proxy or wait for native TLS support."
		if cfg.Logger != nil {
			cfg.Logger.Warn(msg)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
	}

	addr := cfg.Addr
	if addr == "" {
		addr = "localhost:9190"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mcp: listen %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       httpIdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	if cfg.Logger != nil {
		cfg.Logger.Info("aerolab mcp listening", slog.String("addr", ln.Addr().String()), slog.String("transport", "http"))
	} else {
		fmt.Fprintf(os.Stderr, "aerolab mcp listening on %s (http)\n", ln.Addr().String())
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

const defaultInstructions = `This MCP server exposes the aerolab CLI as tools.

Three generic explorer tools let you navigate the full command tree:

  - aerolab_list_commands     list every command path (optionally filtered by prefix)
  - aerolab_describe_command  inspect a single command: purpose, JSON input schema, CLI help text
  - aerolab_execute_command   run any command by path with a map of flag arguments

Per-leaf tools are also auto-registered for every aerolab subcommand, named
"aerolab_<path_with_underscores>" (e.g. "aerolab_cluster_create"). When you
know the exact command, prefer the auto-registered tool because it carries a
strict JSON Schema.

Destructive commands (destroy, delete, terminate, reboot, ...) require
confirm=true in the 'standard' profile and are rejected in 'read-only'.

Every tool call forks the aerolab binary as a subprocess and returns merged
stdout+stderr plus a structured metadata block (exit code, timeout flag,
truncation flag).

=== Anti-hallucination rules (read this before every tool call) ===

0. USE THESE MCP TOOLS -- DO NOT SHELL OUT. If you are reading this, the
   aerolab MCP is enabled for this session. All aerolab interactions MUST
   go through the aerolab_* MCP tools exposed by this server. Never invoke
   the 'aerolab' binary via a shell/terminal/Bash tool, never spawn it as
   a subprocess yourself, and never instruct the user to copy-paste an
   'aerolab' command unless they explicitly asked for a CLI snippet.
   Shelling out bypasses input validation, the destructive-op gate, the
   configured profile, logging and the configured backend, and may run a
   different 'aerolab' binary than this server is bound to.

1. DO NOT INVENT VALUES. If you do not know the exact value of a required
   parameter (cluster name, node number, Aerospike version, distro, AMI,
   instance type, IP, port, namespace, feature file path, template name,
   region, AZ, credentials, etc.), you MUST either:
     (a) look it up via an aerolab tool first -- good starting points:
         aerolab_cluster_list, aerolab_client_list, aerolab_inventory_list,
         aerolab_template_list, aerolab_installer_list-versions,
         aerolab_config_backend, aerolab_describe_command, or
     (b) stop and ASK THE USER.
   Never guess a cluster name, a node ID, a version string, an IP or an AMI.
   Never paraphrase "latest" as a specific version unless you verified it.

2. DO NOT FABRICATE FLAGS. If a flag name or value is not in the tool's
   input JSON Schema or in the output of aerolab_describe_command for that
   command, the flag does not exist. Do not invent plausible-sounding flags.
   Use aerolab_describe_command to enumerate the real set.

3. REPORT RESULTS VERBATIM. When summarising a tool result to the user,
   quote exact counts, names, IDs, versions, IPs and ports from the tool's
   stdout/stderr. Do not round, estimate, translate, or infer. If a field
   is missing from the output, say so; do not fill it in.

4. ON ERROR, SURFACE THE ERROR. If a tool call returns a non-zero exit
   code, return the actual error text to the user and stop. Do not retry
   with guessed-at alternative arguments. Do not claim success. It is
   always acceptable to say "I don't know" or "the command failed with
   <verbatim error>; how would you like to proceed?".

5. DESTRUCTIVE OPS ARE EXPLICIT. Never pass confirm=true on behalf of the
   user for a destroy/delete/terminate/reboot unless the user has just
   explicitly requested that exact destructive action. The confirm flag is
   a guardrail, not a default.

6. PREFER READ-ONLY DISCOVERY BEFORE WRITES. Before any mutating call,
   confirm state with a read-only tool (list, describe, status). This is
   cheap and eliminates most hallucinated-parameter bugs.

When in doubt: look it up, or ask. Don't guess.`
