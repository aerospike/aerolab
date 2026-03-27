//go:build nowebui

package cmd

import "fmt"

type WebUICmd struct {
	AgiMonitorEnable bool                `long:"agi-monitor-enable" description:"Enable built-in AGI monitor for auto-sizing and spot rotation"`
	AgiMonitor       AgiMonitorConfigCmd `group:"AGI Monitor" namespace:"agi-monitor" description:"AGI Monitor configuration (requires --agi-monitor-enable)"`
	Exec             WebUIExecCmd        `command:"exec" subcommands-optional:"true" description:"Execute command (internal use)" hidden:"true"`
	Help             HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *WebUICmd) Execute(args []string) error {
	return fmt.Errorf("WebUI is not available in this build")
}

type WebUIExecCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *WebUIExecCmd) Execute(args []string) error {
	return fmt.Errorf("WebUI is not available in this build")
}
