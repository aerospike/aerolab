package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	flags "github.com/rglonek/go-flags"
)

// SimpleModeAction indicates whether a rule shows or hides an item.
type SimpleModeAction int

const (
	SimpleModeShow SimpleModeAction = iota
	SimpleModeHide
)

// SimpleModeRule represents a single rule from the simple mode config file.
type SimpleModeRule struct {
	Action  SimpleModeAction
	Pattern string // normalized to lowercase, e.g. "cluster.create.name" or "all" or "agi.*"
}

// SimpleModeConfig holds the parsed simple mode configuration.
type SimpleModeConfig struct {
	Rules        []SimpleModeRule
	ForceEnabled bool // from AEROLAB_FORCE_SIMPLE_MODE
}

// simpleModeAlwaysAllowed is a set of top-level commands that are always allowed
// even in forced simple mode to prevent locking users out.
var simpleModeAlwaysAllowed = map[string]bool{
	"config":     true,
	"webui":      true,
	"help":       true,
	"version":    true,
	"completion": true,
	"upgrade":    true,
}

// LoadSimpleModeConfig reads AEROLAB_SIMPLE_MODE and AEROLAB_FORCE_SIMPLE_MODE
// environment variables and returns a SimpleModeConfig. Returns nil if neither
// env var is set.
func LoadSimpleModeConfig() (*SimpleModeConfig, error) {
	filePath := os.Getenv("AEROLAB_SIMPLE_MODE")
	forceStr := os.Getenv("AEROLAB_FORCE_SIMPLE_MODE")
	forceEnabled := strings.EqualFold(forceStr, "true") || forceStr == "1"

	if filePath == "" && !forceEnabled {
		return nil, nil
	}

	config := &SimpleModeConfig{
		ForceEnabled: forceEnabled,
	}

	if filePath != "" {
		rules, err := parseSimpleModeFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load simple mode config from %s: %w", filePath, err)
		}
		config.Rules = rules
	}

	return config, nil
}

// parseSimpleModeFile reads and parses the simple mode configuration file.
func parseSimpleModeFile(filePath string) ([]SimpleModeRule, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rules []SimpleModeRule
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip inline comments
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		if len(line) < 2 {
			return nil, fmt.Errorf("line %d: invalid rule (too short): %q", lineNum, line)
		}

		var action SimpleModeAction
		switch line[0] {
		case '+':
			action = SimpleModeShow
		case '-':
			action = SimpleModeHide
		default:
			return nil, fmt.Errorf("line %d: rule must start with '+' or '-': %q", lineNum, line)
		}

		pattern := strings.ToLower(strings.TrimSpace(line[1:]))
		if pattern == "" {
			return nil, fmt.Errorf("line %d: empty pattern after +/-", lineNum)
		}

		rules = append(rules, SimpleModeRule{
			Action:  action,
			Pattern: pattern,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return rules, nil
}

// IsAllowed checks the rules to determine if the given path should be shown or
// hidden in simple mode. Returns nil if no rule matches (caller should use the
// struct tag default), a pointer to true if shown, or a pointer to false if hidden.
// The path should use dots as separators and be lowercase (e.g. "cluster.create.name").
func (c *SimpleModeConfig) IsAllowed(path string) *bool {
	if c == nil || len(c.Rules) == 0 {
		return nil
	}

	path = strings.ToLower(path)
	var result *bool

	for _, rule := range c.Rules {
		if simpleModePatternMatches(rule.Pattern, path) {
			val := rule.Action == SimpleModeShow
			result = &val
		}
	}

	return result
}

// simpleModePatternMatches checks if a pattern matches the given path.
// Patterns:
//   - "all" matches everything
//   - "cluster.create" matches exactly "cluster.create"
//   - "agi.*" matches "agi" and all paths that start with "agi."
func simpleModePatternMatches(pattern, path string) bool {
	if pattern == "all" {
		return true
	}

	// Wildcard: "agi.*" matches "agi" itself and anything under "agi."
	if before, ok := strings.CutSuffix(pattern, ".*"); ok {
		prefix := before
		return path == prefix || strings.HasPrefix(path, prefix+".")
	}

	// Exact match
	return pattern == path
}

// CheckCommandAllowed returns an error if the given command path is not allowed
// in simple mode. Only enforced when ForceEnabled is true.
// The path should use dots as separators (e.g. "cluster.create").
func (c *SimpleModeConfig) CheckCommandAllowed(path string) error {
	if c == nil || !c.ForceEnabled {
		return nil
	}

	// Always allow built-in essential commands
	topLevel := strings.ToLower(path)
	if idx := strings.Index(topLevel, "."); idx >= 0 {
		topLevel = topLevel[:idx]
	}
	if simpleModeAlwaysAllowed[topLevel] {
		return nil
	}

	result := c.IsAllowed(strings.ToLower(path))
	if result == nil {
		// No override rule; check if the struct-tag default would allow it.
		// When force is enabled and there IS a config file with rules, commands
		// not mentioned default to their struct tag (which is "allowed" unless
		// simplemode:"false"). If the config has an -ALL rule, this will be
		// overridden. So just return nil here and let the struct-tag default stand.
		return nil
	}
	if !*result {
		return fmt.Errorf("command '%s' is not available in simple mode", path)
	}
	return nil
}

// CheckParameterAllowed checks if a specific parameter was changed from its
// default when it is hidden in simple mode. Returns an error if a hidden
// parameter was explicitly set. Only enforced when ForceEnabled is true.
func (c *SimpleModeConfig) CheckParameterAllowed(cmdPath string, paramLong string, structTagSimpleMode bool) error {
	if c == nil || !c.ForceEnabled {
		return nil
	}

	paramPath := strings.ToLower(cmdPath + "." + paramLong)
	result := c.IsAllowed(paramPath)

	// Determine effective simple mode: config override or struct tag default
	allowed := structTagSimpleMode
	if result != nil {
		allowed = *result
	}

	if !allowed {
		return fmt.Errorf("parameter '--%s' is not available in simple mode", paramLong)
	}
	return nil
}

// ApplyToCommandTree walks the CommandInfo tree and overrides SimpleMode fields
// based on the config rules. This is used by the WebUI to customize the command
// tree before serving it to the frontend.
// The root node is the synthetic "aerolab" node; paths in the config file start
// from the first-level commands (e.g. "cluster", not "aerolab.cluster"), so we
// skip the root and start recursion from its children with an empty parent path.
func (c *SimpleModeConfig) ApplyToCommandTree(root *CommandInfo) {
	if c == nil || len(c.Rules) == 0 {
		return
	}
	for _, child := range root.Children {
		applySimpleModeToNode(c, child, "")
	}
}

func applySimpleModeToNode(config *SimpleModeConfig, node *CommandInfo, parentPath string) {
	path := node.Name
	if parentPath != "" {
		path = parentPath + "." + node.Name
	}

	// Check if config overrides this node's simpleMode
	if result := config.IsAllowed(path); result != nil {
		node.SimpleMode = *result
	}

	// Override parameters
	for i := range node.Parameters {
		paramName := node.Parameters[i].Long
		if paramName == "" {
			paramName = node.Parameters[i].FieldName
		}
		paramPath := path + "." + paramName
		if result := config.IsAllowed(paramPath); result != nil {
			node.Parameters[i].SimpleMode = *result
		}
	}

	// Recurse children
	for _, child := range node.Children {
		applySimpleModeToNode(config, child, path)
	}
}

// HideBlockedCommands walks the go-flags parser's command tree and sets
// Hidden=true on commands that are blocked by simple mode rules. This ensures
// the CLI help output only shows allowed commands. It also hides blocked
// options from help. The go-flags library still allows hidden commands to be
// invoked (they are just omitted from help); actual enforcement happens via
// CheckCommandAllowed after parsing.
func (c *SimpleModeConfig) HideBlockedCommands(parser *flags.Parser) {
	if c == nil || len(c.Rules) == 0 {
		return
	}
	hideBlockedInCommand(c, parser.Command, "")
}

func hideBlockedInCommand(config *SimpleModeConfig, cmd *flags.Command, parentPath string) {
	for _, sub := range cmd.Commands() {
		path := sub.Name
		if parentPath != "" {
			path = parentPath + "." + sub.Name
		}

		// Check if this command should be hidden
		result := config.IsAllowed(path)
		if result != nil && !*result {
			sub.Hidden = true
		}

		// Also hide blocked options within this command
		hideBlockedOptions(config, sub, path)

		// Recurse into subcommands
		hideBlockedInCommand(config, sub, path)
	}
}

func hideBlockedOptions(config *SimpleModeConfig, cmd *flags.Command, cmdPath string) {
	hideOptionsInGroup(config, cmd.Group, cmdPath)
}

func hideOptionsInGroup(config *SimpleModeConfig, grp *flags.Group, cmdPath string) {
	for _, opt := range grp.Options() {
		long := opt.LongName
		if long == "" {
			continue
		}
		paramPath := strings.ToLower(cmdPath + "." + long)
		result := config.IsAllowed(paramPath)
		// If the config explicitly hides this param, mark it hidden
		if result != nil && !*result {
			opt.Hidden = true
		}
	}
	// Recurse into sub-groups
	for _, sub := range grp.Groups() {
		hideOptionsInGroup(config, sub, cmdPath)
	}
}

// getActiveCommandPath walks the go-flags parser's Active command chain
// and returns the dot-separated command path (e.g. "cluster.create").
func getActiveCommandPath(parser *flags.Parser) string {
	var parts []string
	cmd := parser.Command.Active
	for cmd != nil {
		parts = append(parts, cmd.Name)
		cmd = cmd.Active
	}
	return strings.Join(parts, ".")
}

// checkParsedParameters walks the go-flags parser's active command chain,
// collects all blocked option flags, then scans the raw command-line args to
// detect if the user explicitly passed any of them. This avoids relying on
// go-flags' IsSet() which returns true for struct tag defaults too.
func (c *SimpleModeConfig) checkParsedParameters(parser *flags.Parser, rawArgs []string) error {
	if c == nil || !c.ForceEnabled {
		return nil
	}

	cmdPath := getActiveCommandPath(parser)
	if cmdPath == "" {
		return nil
	}

	// Always allow built-in essential commands
	topLevel := strings.ToLower(cmdPath)
	if idx := strings.Index(topLevel, "."); idx >= 0 {
		topLevel = topLevel[:idx]
	}
	if simpleModeAlwaysAllowed[topLevel] {
		return nil
	}

	// Collect all blocked flags from the active command chain
	var blocked []blockedFlag
	cmd := parser.Command.Active
	path := ""
	for cmd != nil {
		if path == "" {
			path = cmd.Name
		} else {
			path = path + "." + cmd.Name
		}
		c.collectBlockedFlags(cmd.Group, path, &blocked)
		cmd = cmd.Active
	}

	if len(blocked) == 0 {
		return nil
	}

	// Scan raw args for blocked flags
	return checkArgsForBlockedFlags(rawArgs, blocked)
}

// collectBlockedFlags walks a Group (and sub-groups) to find options blocked by
// simple mode, returning their flag patterns for matching against raw args.
type blockedFlag struct {
	longFlag    string // e.g. "--count"
	shortFlag   rune   // e.g. 'c', or 0 if none
	displayName string // e.g. "--count"
}

func (c *SimpleModeConfig) collectBlockedFlags(grp *flags.Group, cmdPath string, out *[]blockedFlag) {
	for _, opt := range grp.Options() {
		long := opt.LongNameWithNamespace()
		if long == "" {
			continue
		}

		// Determine if this parameter is allowed
		paramPath := strings.ToLower(cmdPath + "." + long)
		result := c.IsAllowed(paramPath)

		blocked := false
		if result != nil {
			blocked = !*result
		} else {
			// No config override; check the struct tag
			fieldTag := opt.Field().Tag
			if fieldTag.Get("simplemode") == "false" {
				blocked = true
			}
		}

		if blocked {
			*out = append(*out, blockedFlag{
				longFlag:    "--" + long,
				shortFlag:   opt.ShortName,
				displayName: "--" + long,
			})
		}
	}
	// Recurse into sub-groups
	for _, sub := range grp.Groups() {
		c.collectBlockedFlags(sub, cmdPath, out)
	}
}

// checkArgsForBlockedFlags scans raw command-line args for any blocked flags.
func checkArgsForBlockedFlags(args []string, blocked []blockedFlag) error {
	for _, arg := range args {
		if arg == "--" {
			break // everything after -- is positional
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		for _, bf := range blocked {
			// Check long flags: --count or --count=value
			if bf.longFlag != "" {
				if arg == bf.longFlag || strings.HasPrefix(arg, bf.longFlag+"=") {
					return fmt.Errorf("parameter '%s' is not available in simple mode (cannot be changed from default)", bf.displayName)
				}
			}
			// Check short flags: -c or -c5 or combined like -nc
			if bf.shortFlag != 0 && len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' {
				for _, r := range arg[1:] {
					if r == bf.shortFlag {
						return fmt.Errorf("parameter '%s' is not available in simple mode (cannot be changed from default)", bf.displayName)
					}
				}
			}
		}
	}
	return nil
}

// SimpleModePathFromSlash converts a slash-separated path (e.g. "cluster/create")
// to dot-separated (e.g. "cluster.create") for use with simple mode matching.
func SimpleModePathFromSlash(slashPath string) string {
	return strings.ReplaceAll(slashPath, "/", ".")
}
