package cmd

type ConfigCmd struct {
	Backend  ConfigBackendCmd  `command:"backend" subcommands-optional:"true" description:"Show or change backend" webicon:"fas fa-vials"`
	Defaults ConfigDefaultsCmd `command:"defaults" subcommands-optional:"true" description:"Show or change defaults in the configuration file" webicon:"fas fa-arrow-right-to-city"`
	Aws      ConfigAwsCmd      `command:"aws" subcommands-optional:"true" description:"AWS-only related management commands" webicon:"fa-brands fa-aws"`
	Docker   ConfigDockerCmd   `command:"docker" subcommands-optional:"true" description:"DOCKER-only related management commands" webicon:"fa-brands fa-docker"`
	Gcp      ConfigGcpCmd      `command:"gcp" subcommands-optional:"true" description:"GCP-only related management commands" webicon:"fa-brands fa-google"`
	EnvVars  ConfigEnvVarsCmd  `command:"env-vars" subcommands-optional:"true" description:"Show the environment variables and how they are set" webicon:"fas fa-gear"`
	Migrate  ConfigMigrateCmd  `command:"migrate" subcommands-optional:"true" description:"Migrate the configuration to the new AeroLab v8" webicon:"fas fa-arrow-right-to-city"`
	Help     HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}
