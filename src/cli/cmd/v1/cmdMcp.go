package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	aerolabmcp "github.com/aerospike/aerolab/pkg/mcp"
)

// McpCmd runs the aerolab MCP (Model Context Protocol) server. It exposes
// every aerolab subcommand as an MCP tool via self-introspection of the
// command tree and executes each tool call as a subprocess of this binary.
//
// Two transports are supported: stdio (default, for local agents like
// Claude Desktop and Cursor) and streamable HTTP (for remote agents).
// Optional bearer-token auth is enforced on the HTTP transport.
type McpCmd struct {
	Transport      string   `long:"transport" description:"MCP transport: stdio|http" default:"stdio" webchoice:"stdio,http"`
	Addr           string   `long:"addr" description:"HTTP listen address (used when --transport=http)" default:"localhost:9190"`
	AuthToken      string   `long:"auth-token" env:"AEROLAB_MCP_AUTH_TOKEN" description:"Optional bearer token required by HTTP transport. If empty, no auth is enforced"`
	Profile        string   `long:"profile" description:"Tool profile controlling what operations are permitted" default:"standard" webchoice:"read-only,standard,admin"`
	Binary         string   `long:"binary" description:"Path to the aerolab binary invoked for tool execution (defaults to the current executable)"`
	InitBackend    bool     `long:"init-backend" description:"Initialize the configured backend on startup so dynamic choices (zones, instance types, VPCs) can be resolved"`
	TimeoutSec     int      `long:"timeout" description:"Default per-call subprocess timeout in seconds (0 = no timeout)" default:"600"`
	MaxOutputBytes int      `long:"max-output-bytes" description:"Maximum captured bytes of stdout+stderr returned per call" default:"1048576"`
	SessionTimeout string   `long:"session-timeout" description:"Idle timeout for HTTP streaming sessions (Go duration, e.g. 30m)" default:"30m"`
	MCPEnv         []string `long:"mcp-env" description:"Extra KEY=VAL env vars passed to every tool subprocess (repeatable)" hidden:"true"`
	Help           HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *McpCmd) Execute(args []string) error {
	cmd := []string{"mcp"}
	// Stdio transport must keep stdin/stdout exclusively for JSON-RPC;
	// anything that accidentally lands on stdout will corrupt the frame
	// stream. Pin the stdlib log package to stderr before Initialize
	// runs so background goroutines (e.g. scriptlog cleanup in
	// initialize.go) cannot leak to stdout even if someone later flips
	// log's default output.
	if strings.ToLower(c.Transport) == "stdio" {
		log.SetOutput(os.Stderr)
	}
	system, err := Initialize(&Init{InitBackend: c.InitBackend, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	if strings.ToLower(c.Transport) != "stdio" {
		system.Logger.Info("Running %s", strings.Join(cmd, "."))
	}

	return Error(c.runServer(system), system, cmd, c, args)
}

func (c *McpCmd) runServer(system *System) error {
	profile, err := aerolabmcp.ParseProfile(c.Profile)
	if err != nil {
		return err
	}

	binary := c.Binary
	if binary == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve binary path: %w", err)
		}
		binary, err = filepath.EvalSymlinks(exe)
		if err != nil {
			binary = exe
		}
	}

	sessionTimeout := 30 * time.Minute
	if c.SessionTimeout != "" {
		d, err := time.ParseDuration(c.SessionTimeout)
		if err != nil {
			return fmt.Errorf("invalid --session-timeout %q: %w", c.SessionTimeout, err)
		}
		sessionTimeout = d
	}

	runnerEnv, err := validateMCPEnv(c.MCPEnv)
	if err != nil {
		return err
	}

	root := BuildCommandTree(&Commands{})
	// Dynamic choice resolution requires an initialized backend.
	choiceSystem := system
	if !c.InitBackend || system == nil || system.Backend == nil {
		choiceSystem = nil
	}
	tree := convertTreeForMCP(root, choiceSystem, "")

	registry := &aerolabmcp.Registry{
		Root: []*aerolabmcp.Command{tree},
		Help: aerolabmcp.RenderHelpFromFactory(func() any { return &Commands{} }),
		Run: &aerolabmcp.Runner{
			Binary:         binary,
			DefaultTimeout: time.Duration(c.TimeoutSec) * time.Second,
			MaxOutputBytes: c.MaxOutputBytes,
			Env:            runnerEnv,
		},
		Gate: aerolabmcp.NewGate(profile),
	}

	cfg := aerolabmcp.Config{
		Transport:      aerolabmcp.Transport(strings.ToLower(c.Transport)),
		Addr:           c.Addr,
		AuthToken:      c.AuthToken,
		Profile:        profile,
		SessionTimeout: sessionTimeout,
		Implementation: mcpImplementation(),
		Registry:       registry,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return aerolabmcp.Serve(ctx, cfg)
}

// validateMCPEnv checks each --mcp-env entry looks like KEY=VAL with a
// non-empty key, then returns the validated slice ready to hand to
// aerolabmcp.Runner.Env. Empty input returns nil (the runner skips env
// merging entirely when both Env and per-call overrides are empty).
func validateMCPEnv(entries []string) ([]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --mcp-env %q: expected KEY=VALUE", e)
		}
		out = append(out, e)
	}
	return out, nil
}

// mcpImplementation returns the server Implementation advertised to
// clients. Version falls back to "dev" when build info is unavailable.
func mcpImplementation() *aerolabmcp.Implementation {
	_, _, _, friendly := GetAerolabVersion()
	if friendly == "" {
		friendly = "dev"
	}
	return &aerolabmcp.Implementation{Name: "aerolab-mcp", Version: friendly}
}

// convertTreeForMCP converts the CommandInfo tree from BuildCommandTree
// into the MCP package's neutral representation. parentPath is the
// slash-separated path of the parent (or "" for the root).
func convertTreeForMCP(c *CommandInfo, system *System, parentPath string) *aerolabmcp.Command {
	if c == nil {
		return nil
	}
	path := c.Path
	if path == "" && parentPath == "" {
		path = c.Name
	}
	out := &aerolabmcp.Command{
		Name:        c.Name,
		DisplayName: c.DisplayName,
		Path:        path,
		Description: c.Description,
		Hidden:      c.Hidden,
		Destructive: c.InvWebForce,
		Parameters:  convertParamsForMCP(c, system),
	}
	for _, child := range c.Children {
		if child == nil {
			continue
		}
		cc := convertTreeForMCP(child, system, path)
		if cc != nil {
			out.Children = append(out.Children, cc)
		}
	}
	return out
}

// safeResolveChoices wraps ResolveDynamicChoices with a panic recover so
// a buggy List() on one parameter cannot take down the MCP server.
// Errors are logged at Debug level (via system.Logger when available)
// and the parameter then falls back to free-text input.
func safeResolveChoices(system *System, cmdPath string, p ParameterInfo) (vals, labels []string) {
	defer func() {
		if r := recover(); r != nil {
			vals = nil
			labels = nil
			if system != nil && system.Logger != nil {
				system.Logger.Debug("mcp: dynamic choices for %s/%s panicked: %v", cmdPath, p.Name, r)
			}
		}
	}()
	v, l, err := ResolveDynamicChoices(system, cmdPath, p.Name, p.ChoicesMethod, p.Namespace, p.FieldName)
	if err != nil {
		if system != nil && system.Logger != nil {
			system.Logger.Debug("mcp: dynamic choices for %s/%s failed: %v", cmdPath, p.Name, err)
		}
		return nil, nil
	}
	return v, l
}

// convertParamsForMCP converts a command's []ParameterInfo into
// []aerolabmcp.Param, resolving dynamic choices where defined.
func convertParamsForMCP(c *CommandInfo, system *System) []aerolabmcp.Param {
	if c == nil {
		return nil
	}
	out := make([]aerolabmcp.Param, 0, len(c.Parameters))
	for _, p := range c.Parameters {
		if p.Hidden || p.WebHidden {
			continue
		}
		param := aerolabmcp.Param{
			Name:         p.Name,
			Short:        p.Short,
			Long:         p.Long,
			Description:  p.Description,
			Type:         p.Type,
			Default:      p.Default,
			Required:     p.Required,
			Choices:      p.Choices,
			ChoiceLabels: p.ChoiceLabels,
			IsSlice:      p.IsSlice,
			IsPositional: p.IsPositional,
			IsFile:       p.IsFile,
			Optional:     p.Optional,
			Group:        p.Group,
			Namespace:    p.Namespace,
		}
		if p.ChoicesMethod != "" && system != nil && system.Backend != nil {
			vals, labels := safeResolveChoices(system, c.Path, p)
			if len(vals) > 0 {
				param.Choices = vals
				param.ChoiceLabels = labels
			}
		}
		out = append(out, param)
	}
	return out
}
