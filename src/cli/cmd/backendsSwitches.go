package cmd

import (
	"slices"
	"strings"

	"github.com/rglonek/go-flags"
)

func ShowHideBackend(parser *flags.Parser, enabledBackends []string) {
	hideGroup(parser.Groups(), enabledBackends)
	hideGroup(parser.Command.Groups(), enabledBackends)
	hideForCommand(parser.Command, enabledBackends)
}

func hideForCommand(command *flags.Command, enabledBackends []string) {
	for _, c := range command.Commands() {
		hideForCommand(c, enabledBackends)
		hideGroup(c.Groups(), enabledBackends)
	}
}

func hideGroup(groups []*flags.Group, enabledBackends []string) {
	for _, g := range groups {
		subGroups := g.Groups()
		if len(subGroups) > 0 {
			hideGroup(subGroups, enabledBackends)
		}
		if !strings.HasPrefix(g.LongDescription, "backend-") {
			continue
		}
		backendName := strings.TrimPrefix(g.LongDescription, "backend-")
		if !slices.Contains(enabledBackends, backendName) {
			g.Hidden = true
		}
	}
}
