//go:build !noaerolabmcp

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
	Transport              string   `long:"transport" description:"MCP transport: stdio|http" default:"stdio" webchoice:"stdio,http"`
	Addr                   string   `long:"addr" description:"HTTP listen address (used when --transport=http)" default:"localhost:9190"`
	AuthToken              string   `long:"auth-token" env:"AEROLAB_MCP_AUTH_TOKEN" description:"Optional bearer token required by HTTP transport. If empty, no auth is enforced"`
	Profile                string   `long:"profile" description:"Tool profile controlling what operations are permitted" default:"standard" webchoice:"read-only,standard,admin"`
	Binary                 string   `long:"binary" description:"Path to the aerolab binary invoked for tool execution (defaults to the current executable)"`
	InitBackend            bool     `long:"init-backend" description:"Initialize the configured backend on startup so dynamic choices (zones, instance types, VPCs) can be resolved"`
	TimeoutSec             int      `long:"timeout" description:"Default per-call subprocess timeout in seconds (0 = no timeout)" default:"600"`
	MaxOutputBytes         int      `long:"max-output-bytes" description:"Maximum captured bytes of stdout+stderr returned per call" default:"1048576"`
	SessionTimeout         string   `long:"session-timeout" description:"Idle timeout for HTTP streaming sessions (Go duration, e.g. 30m)" default:"30m"`
	MCPEnv                 []string `long:"mcp-env" description:"Extra KEY=VAL env vars passed to every tool subprocess (repeatable)" hidden:"true"`
	DisableForceJSONOutput bool     `long:"mcp-disable-force-json" description:"Disable the MCP-side default of --output=json for read-style commands. By default the MCP server advertises and injects json output so agents can parse results more reliably; use this flag to fall back to each command's native default (usually table)."`
	ForceSimpleMode        bool     `long:"force-simple-mode" description:"Force simple mode: hide blocked tools/parameters from the advertised MCP schema and refuse blocked calls"`
	SimpleModeConfig       string   `long:"simple-mode-config" description:"Path to a simple mode rules file (overrides AEROLAB_SIMPLE_MODE env var)"`
	Help                   HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
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

	if err := c.applySimpleModeFlags(system); err != nil {
		return Error(err, system, cmd, c, args)
	}

	return Error(c.runServer(system), system, cmd, c, args)
}

// applySimpleModeFlags folds the --simple-mode-config and --force-simple-mode
// CLI flags into the system's SimpleModeConfig. --simple-mode-config takes
// precedence over AEROLAB_SIMPLE_MODE (its rules replace any env-loaded rules
// on the active config); --force-simple-mode sets ForceEnabled and also
// exports AEROLAB_FORCE_SIMPLE_MODE so every tool subprocess inherits the
// same contract (same trick cmdWebUI uses).
func (c *McpCmd) applySimpleModeFlags(system *System) error {
	if c.SimpleModeConfig != "" {
		rules, err := parseSimpleModeFile(c.SimpleModeConfig)
		if err != nil {
			return fmt.Errorf("failed to load simple mode config from %s: %w", c.SimpleModeConfig, err)
		}
		if system.SimpleModeConfig == nil {
			system.SimpleModeConfig = &SimpleModeConfig{}
		}
		system.SimpleModeConfig.Rules = rules
		os.Setenv("AEROLAB_SIMPLE_MODE", c.SimpleModeConfig)
	}
	if c.ForceSimpleMode {
		if system.SimpleModeConfig == nil {
			system.SimpleModeConfig = &SimpleModeConfig{ForceEnabled: true}
		} else {
			system.SimpleModeConfig.ForceEnabled = true
		}
		os.Setenv("AEROLAB_FORCE_SIMPLE_MODE", "true")
	}
	return nil
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
	// Apply simple mode overrides so a --simple-mode-config file can flip
	// SimpleMode flags on individual commands/parameters before we filter
	// the tree and before the runtime gate evaluates incoming tool calls.
	if system.SimpleModeConfig != nil {
		system.SimpleModeConfig.ApplyToCommandTree(root)
	}
	// Dynamic choice resolution requires an initialized backend.
	choiceSystem := system
	if !c.InitBackend || system == nil || system.Backend == nil {
		choiceSystem = nil
	}
	// When simple mode is forced, drop blocked commands and parameters from
	// the tree we advertise over MCP so agents cannot even see them.
	forceSimple := system.SimpleModeConfig != nil && system.SimpleModeConfig.ForceEnabled
	tree := convertTreeForMCP(root, choiceSystem, "", !c.DisableForceJSONOutput, forceSimple)

	registry := &aerolabmcp.Registry{
		Root: []*aerolabmcp.Command{tree},
		Help: aerolabmcp.RenderHelpFromFactory(func() any { return &Commands{} }),
		Run: &aerolabmcp.Runner{
			Binary:         binary,
			DefaultTimeout: time.Duration(c.TimeoutSec) * time.Second,
			MaxOutputBytes: c.MaxOutputBytes,
			Env:            runnerEnv,
		},
		Gate:                   aerolabmcp.NewGate(profile),
		DisableForceJSONOutput: c.DisableForceJSONOutput,
	}
	// When simple mode is forced, reject blocked commands and blocked
	// parameters at the MCP layer so agents get a clear error without
	// paying for a subprocess fork (which would also ultimately reject).
	if forceSimple {
		registry.SimpleModeGate = newSimpleModeGate(system.SimpleModeConfig, root)
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
// slash-separated path of the parent (or "" for the root). When
// forceJSONOutput is true, read-style `--output` parameters advertise
// "json" as their default in the schema description so agents see the
// value the runner will ultimately inject for them. When forceSimpleMode
// is true, nodes with SimpleMode=false are dropped from the resulting
// tree (with the exception of paths whose top-level segment is in
// simpleModeAlwaysAllowed, which are kept so users can always reach
// `config`, `help`, `version`, etc.).
func convertTreeForMCP(c *CommandInfo, system *System, parentPath string, forceJSONOutput bool, forceSimpleMode bool) *aerolabmcp.Command {
	if c == nil {
		return nil
	}
	path := c.Path
	if path == "" && parentPath == "" {
		path = c.Name
	}
	// Drop whole command subtrees blocked in forced simple mode, unless
	// the command's top-level segment is in simpleModeAlwaysAllowed.
	if forceSimpleMode && !c.SimpleMode && !simpleModeAllowsPath(path) {
		return nil
	}
	out := &aerolabmcp.Command{
		Name:        c.Name,
		DisplayName: c.DisplayName,
		Path:        path,
		Description: c.Description,
		Hidden:      c.Hidden,
		Destructive: c.InvWebForce,
		Parameters:  convertParamsForMCP(c, system, forceJSONOutput, forceSimpleMode),
	}
	for _, child := range c.Children {
		if child == nil {
			continue
		}
		cc := convertTreeForMCP(child, system, path, forceJSONOutput, forceSimpleMode)
		if cc != nil {
			out.Children = append(out.Children, cc)
		}
	}
	return out
}

// simpleModeAllowsPath returns true when the top-level segment of a
// slash-separated command path is in simpleModeAlwaysAllowed. Used during
// tree conversion to preserve commands that must remain reachable even
// when a blanket `-all` rule is in effect (e.g. `config`, `help`, `mcp`).
func simpleModeAllowsPath(path string) bool {
	if path == "" {
		return false
	}
	top := path
	if idx := strings.Index(top, "/"); idx >= 0 {
		top = top[:idx]
	}
	return simpleModeAlwaysAllowed[strings.ToLower(top)]
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
// []aerolabmcp.Param, resolving dynamic choices where defined. When
// forceJSONOutput is true, a hint is appended to the description of every
// read-style `--output` parameter so agents inspecting the schema see
// that MCP auto-injects --output=json. The Default field itself is left
// untouched — the runtime injection in pkg/mcp relies on the original
// default to decide whether to inject (so commands already defaulting
// to json-indent are not silently downgraded to json).
func convertParamsForMCP(c *CommandInfo, system *System, forceJSONOutput bool, forceSimpleMode bool) []aerolabmcp.Param {
	if c == nil {
		return nil
	}
	out := make([]aerolabmcp.Param, 0, len(c.Parameters))
	for _, p := range c.Parameters {
		if p.Hidden || p.WebHidden {
			continue
		}
		// In forced simple mode, drop parameters whose effective
		// SimpleMode (struct tag default, overridden by any simple
		// mode rule file via ApplyToCommandTree) is false.
		if forceSimpleMode && !p.SimpleMode {
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
		if forceJSONOutput && aerolabmcp.ShouldForceJSONOutputParam(param) {
			hint := "MCP auto-injects --output=" + aerolabmcp.ForcedJSONOutputValue + " when omitted; pass explicitly to override."
			if param.Description == "" {
				param.Description = hint
			} else {
				param.Description = param.Description + " " + hint
			}
		}
		out = append(out, param)
	}
	return out
}

// mcpSimpleModeGate is the cmd-package adapter that lets pkg/mcp enforce
// simple-mode rules without importing the CLI's reflection machinery. It
// holds the (already-applied) CommandInfo tree so parameter lookups use
// the effective SimpleMode bit (struct tag default, overridden by any
// rule file loaded via ApplyToCommandTree).
type mcpSimpleModeGate struct {
	cfg  *SimpleModeConfig
	root *CommandInfo
}

// newSimpleModeGate wires a SimpleModeGate bound to the given config and
// command tree. The caller must have already invoked
// cfg.ApplyToCommandTree(root) so per-parameter SimpleMode flags reflect
// both struct tags and rule-file overrides.
func newSimpleModeGate(cfg *SimpleModeConfig, root *CommandInfo) aerolabmcp.SimpleModeGate {
	return &mcpSimpleModeGate{cfg: cfg, root: root}
}

// CheckCommand returns an error if the slash-separated path is blocked
// by the active simple-mode rules. Paths whose top-level segment is in
// simpleModeAlwaysAllowed bypass the check (e.g. `help`, `config`, `mcp`).
func (g *mcpSimpleModeGate) CheckCommand(path string) error {
	if g == nil || g.cfg == nil || !g.cfg.ForceEnabled {
		return nil
	}
	return g.cfg.CheckCommandAllowed(SimpleModePathFromSlash(path))
}

// CheckArgs returns an error if any key in args refers to a parameter
// blocked by simple mode for the given command. Keys that don't resolve
// to a known parameter are left alone — the runner will reject them on
// its own. Boolean false values are treated as "not set" (matches CLI
// bool flag semantics: the flag is only emitted when true).
func (g *mcpSimpleModeGate) CheckArgs(path string, args map[string]any) error {
	if g == nil || g.cfg == nil || !g.cfg.ForceEnabled {
		return nil
	}
	if len(args) == 0 || g.root == nil {
		return nil
	}
	cmd := g.root.FindByPath(path)
	if cmd == nil {
		return nil
	}
	for key, val := range args {
		if b, ok := val.(bool); ok && !b {
			continue
		}
		p := findParamByLong(cmd.Parameters, key)
		if p == nil {
			continue
		}
		structTagAllowed := p.SimpleMode
		if err := g.cfg.CheckParameterAllowed(SimpleModePathFromSlash(path), p.Long, structTagAllowed); err != nil {
			return err
		}
	}
	return nil
}

// findParamByLong returns the ParameterInfo whose Long flag (preferred)
// or Name matches the supplied key, case-insensitive. Returns nil when
// no parameter matches — a likely signal the caller passed an unknown
// flag or one of the extra control fields (confirm, timeout_sec, …).
func findParamByLong(params []ParameterInfo, key string) *ParameterInfo {
	for i := range params {
		if strings.EqualFold(params[i].Long, key) || strings.EqualFold(params[i].Name, key) {
			return &params[i]
		}
	}
	return nil
}
