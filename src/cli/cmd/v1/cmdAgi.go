package cmd

// AgiCmd is the main AGI (Aerospike Grafana Integration) command structure.
// AGI provides a template-based approach where templates have all software pre-installed
// (aerospike, grafana, plugin, proxy, ingest tools), enabling fast instance creation.
type AgiCmd struct {
	Template  AgiTemplateCmd  `command:"template" subcommands-optional:"true" description:"AGI template management" webicon:"fas fa-file-image"`
	Create    AgiCreateCmd    `command:"create" subcommands-optional:"true" description:"Create AGI instance" webicon:"fas fa-plus"`
	List      AgiListCmd      `command:"list" subcommands-optional:"true" description:"List AGI instances" webicon:"fas fa-list"`
	Start     AgiStartCmd     `command:"start" subcommands-optional:"true" description:"Start AGI instance" webicon:"fas fa-play"`
	Stop      AgiStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop AGI instance" webicon:"fas fa-stop"`
	Status    AgiStatusCmd    `command:"status" subcommands-optional:"true" description:"Show AGI status" webicon:"fas fa-info-circle"`
	Details   AgiDetailsCmd   `command:"details" subcommands-optional:"true" description:"Show ingest details" webicon:"fas fa-magnifying-glass"`
	Destroy   AgiDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy AGI instance" webicon:"fas fa-trash"`
	Delete    AgiDeleteCmd    `command:"delete" subcommands-optional:"true" description:"Destroy instance and volume" webicon:"fas fa-trash-can"`
	Attach    AgiAttachCmd    `command:"attach" subcommands-optional:"true" description:"Attach to AGI shell" webicon:"fas fa-terminal"`
	Open      AgiOpenCmd      `command:"open" subcommands-optional:"true" description:"Open AGI in browser" webicon:"fas fa-globe"`
	AddToken  AgiAddTokenCmd  `command:"add-auth-token" subcommands-optional:"true" description:"Add auth token" webicon:"fas fa-key"`
	Relabel   AgiRelabelCmd   `command:"change-label" subcommands-optional:"true" description:"Change label" webicon:"fas fa-tag"`
	Retrigger AgiRetriggerCmd `command:"run-ingest" subcommands-optional:"true" description:"Retrigger ingest" webicon:"fas fa-rotate"`
	Share     AgiShareCmd     `command:"share" subcommands-optional:"true" description:"Share via SSH key" webicon:"fas fa-share"`
	Monitor   AgiMonitorCmd   `command:"monitor" subcommands-optional:"true" description:"Monitor system" webicon:"fas fa-gauge"`
	Exec      AgiExecCmd      `command:"exec" subcommands-optional:"true" description:"AGI subsystems" hidden:"true"`
	Help      HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// Note: The following commands are implemented in their respective files:
// - AgiCreateCmd is implemented in cmdAgiCreate.go
// - AgiListCmd is implemented in cmdAgiList.go
// - AgiStartCmd is implemented in cmdAgiStart.go
// - AgiStopCmd is implemented in cmdAgiStop.go
// - AgiDestroyCmd is implemented in cmdAgiDestroy.go
// - AgiDeleteCmd is implemented in cmdAgiDelete.go
// - AgiStatusCmd is implemented in cmdAgiStatus.go
// - AgiDetailsCmd is implemented in cmdAgiDetails.go
// - AgiAttachCmd is implemented in cmdAgiAttach.go
// - AgiOpenCmd is implemented in cmdAgiOpen.go
// - AgiAddTokenCmd is implemented in cmdAgiAddToken.go
// - AgiRelabelCmd is implemented in cmdAgiRelabel.go
// - AgiRetriggerCmd is implemented in cmdAgiRetrigger.go
// - AgiShareCmd is implemented in cmdAgiShare.go

// Note: AgiMonitorCmd, AgiMonitorCreateCmd, and AgiMonitorListenCmd are defined in cmdAgiMonitor.go

// AgiMonitorCmd is the AGI monitor system command structure.
// It provides commands for creating monitor instances and running the monitor listener.
// See cmdAgiMonitor.go for detailed struct definitions.
type AgiMonitorCmd struct {
	Create AgiMonitorCreateCmd `command:"create" subcommands-optional:"true" description:"Create monitor instance" webicon:"fas fa-plus"`
	Listen AgiMonitorListenCmd `command:"listen" subcommands-optional:"true" description:"Start monitor listener" webicon:"fas fa-headphones"`
	Help   HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiMonitorCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// AgiExecCmd is the hidden AGI exec subsystem command structure
// These commands are run inside AGI instances, not by users directly
type AgiExecCmd struct {
	Plugin       AgiExecPluginCmd       `command:"plugin" subcommands-optional:"true" description:"Run plugin backend"`
	GrafanaFix   AgiExecGrafanaFixCmd   `command:"grafanafix" subcommands-optional:"true" description:"Run Grafana helper"`
	Ingest       AgiExecIngestCmd       `command:"ingest" subcommands-optional:"true" description:"Run ingest service"`
	Proxy        AgiExecProxyCmd        `command:"proxy" subcommands-optional:"true" description:"Run web proxy"`
	IngestStatus AgiExecIngestStatusCmd `command:"ingest-status" subcommands-optional:"true" description:"Get ingest status"`
	IngestDetail AgiExecIngestDetailCmd `command:"ingest-detail" subcommands-optional:"true" description:"Get ingest details"`
	Simulate     AgiExecSimulateCmd     `command:"simulate" subcommands-optional:"true" description:"Simulate spot termination"`
	Help         HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// Note: The following exec commands are implemented in their respective files:
// - AgiExecPluginCmd is implemented in cmdAgiExecPlugin.go
// - AgiExecGrafanaFixCmd is implemented in cmdAgiExecGrafanafix.go
// - AgiExecIngestCmd is implemented in cmdAgiExecIngest.go
// - AgiExecProxyCmd is implemented in cmdAgiExecProxy.go
// - AgiExecIngestStatusCmd is implemented in cmdAgiExec.go
// - AgiExecIngestDetailCmd is implemented in cmdAgiExec.go
// - AgiExecSimulateCmd is implemented in cmdAgiExec.go
