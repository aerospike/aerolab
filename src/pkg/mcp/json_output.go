package mcp

import "strings"

// ForcedJSONOutputValue is the value the MCP server injects for the
// `--output` flag of read-style aerolab commands (cluster list, client
// list, inventory list, …) when the caller did not specify a value.
//
// JSON is chosen over json-indent so responses stay compact — LLM clients
// parse either fine but json-indent roughly doubles token usage on large
// inventories.
const ForcedJSONOutputValue = "json"

// ShouldForceJSONOutputParam reports whether p is an `--output` flag that
// accepts "json" as a value and would benefit from having its default
// flipped to "json" for MCP callers.
//
// The detection is conservative: it only matches long-flag "output", and
// requires the parameter to advertise a JSON-capable value either via its
// Choices list or via its description (the latter is where aerolab
// currently lists the accepted formats). Parameters already defaulting to
// a JSON-family value (json / json-indent / jq) are excluded so we never
// "downgrade" json-indent to json.
func ShouldForceJSONOutputParam(p Param) bool {
	if p.Long != "output" {
		return false
	}
	if !paramAdvertisesJSON(p) {
		return false
	}
	if isJSONFamilyDefault(p.Default) {
		return false
	}
	return true
}

// ShouldForceJSONOutput returns the CLI long-flag name to inject (almost
// always "output") and true when cmd has an output parameter eligible for
// auto-injection. Returns ("", false) when the caller should not inject.
func ShouldForceJSONOutput(cmd *Command) (string, bool) {
	if cmd == nil {
		return "", false
	}
	for _, p := range cmd.Parameters {
		if ShouldForceJSONOutputParam(p) {
			return p.Long, true
		}
	}
	return "", false
}

func paramAdvertisesJSON(p Param) bool {
	for _, c := range p.Choices {
		if strings.EqualFold(strings.TrimSpace(c), "json") {
			return true
		}
	}
	// aerolab output flags list accepted values in the description
	// ("Output format (text, table, json, json-indent, ...)") rather than
	// via webchoice, so fall back to a substring check.
	return strings.Contains(strings.ToLower(p.Description), "json")
}

func isJSONFamilyDefault(def string) bool {
	switch strings.ToLower(strings.TrimSpace(def)) {
	case "json", "json-indent", "jq":
		return true
	}
	return false
}

// maybeInjectJSONOutput sets args[<outputFlag>] = "json" when reg permits
// the injection (DisableForceJSONOutput is false) and cmd has a matching
// output parameter that the caller has not already set. It always returns
// the (possibly mutated) args map; a nil input is allocated on demand.
//
// A nil registry is treated as "injection enabled" so in-package tests
// that construct a handler without a Registry pointer still exercise the
// default behaviour.
func maybeInjectJSONOutput(reg *Registry, cmd *Command, args map[string]any) map[string]any {
	if reg != nil && reg.DisableForceJSONOutput {
		return args
	}
	flag, ok := ShouldForceJSONOutput(cmd)
	if !ok {
		return args
	}
	if args == nil {
		args = map[string]any{}
	}
	if _, alreadySet := args[flag]; alreadySet {
		return args
	}
	args[flag] = ForcedJSONOutputValue
	return args
}
