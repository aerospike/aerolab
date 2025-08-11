# Commands

## Command with subcommands definition

Define the main command as so:
```go
type ClusterCmd struct {
    Create ClusterCreateCmd `command:"create" subcommands-optional:"true" description:"Create a cluster"`
    List ClusterListCmd `command:"list" subcommands-optional:"true" description:"List clusters"`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// this is required to display help if `aerolab cluster` command is called without any subcommands
func (c *ClusterCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}
```

The available tags are:
* `command` - the command name
* `subcommands-optional` - must be true
* `description` - description of the command
* `webicon` - icon for the command in the web interface
* `invwebforce` - whether running this command will force the web interface to refresh
* `webhidden` - whether the command is hidden from the web interface
* `hidden` - whether the command is hidden from the help output

## Command definition

Start by adding a struct definition. See example below.

```go
type ClusterCreateCmd struct {
    Name string `short:"n" long:"name" description:"Name of the cluster" default:"mydc"`
    Count int `short:"c" long:"count" description:"Number of nodes to create" default:"1"`
    GCP ClusterCreateCmdGcp `group:"GCP Backend" description:"backend-gcp"`
    AWS ClusterCreateCmdAws `group:"AWS Backend" description:"backend-aws"`
    Docker ClusterCreateCmdDocker `group:"Docker Backend" description:"backend-docker"`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ClusterCreateCmdGcp struct {
    Zone string `short:"z" long:"zone" description:"Zone to create the cluster in" default:"us-central1-a"`
}

type ClusterCreateCmdAws struct {
    Region string `short:"r" long:"region" description:"Region to create the cluster in" default:"us-east-1"`
}

type ClusterCreateCmdDocker struct {
    Network string `short:"n" long:"network" description:"Network to create the cluster in" default:"default"`
}
```

Explanation:
* the struct needs parameters which will be called, with tags defining what those parameters are
* for backend parameter groups, define the `group` tag with friendly backend name, and the `description` tag with `backend-<backend-name>`
  * backends which are not currently enabled, will have their options hidden automatically if this definition is followed
  * optional `namespace` tag can be specified on a group, all command-line switches will be prefixed with the namespace
* available tags for non-backend-special parameters:
  * `short` - short flag
  * `long` - long flag
  * `description` - description of the parameter
  * `default` - default value of the parameter
  * `hidden` - whether the parameter is hidden from the help output
  * `webhidden` - whether the parameter is hidden from the web interface
  * `simplemode` - whether the parameter is a simple mode parameter
  * `group` - group of the parameter - if group is defined, the parameter will be displayed in help in it's own section
  * `namespace` - if specified on a group, all command-line switches will be prefixed with the namespace

## Adding the command to the CLI

Add the command to the `cmd.go` file in order to make it available to the CLI.

## Help function

* the `Help` subcommand definition must be present in all commands, as it will allow for appending the `help` word at the end of the command to get it's usage

## Command execution

The command execution is done by defining the `Execute` function of the command struct. It is advised to perform system and backend init here, with basic logging, and then to call the actual command function. All exits from the Execute function will be handled by the `Error` function, which will log the error if needed and exit with the appropriate code. Example below:

```go
func (c *ClusterCreateCmd) Execute(args []string) error {
    // initialize the system with the backend, and check for available aerolab upgrade
	cmd := []string{"cluster", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

    // run the actual command function, passing it the system, command name and arguments
	err = c.ClusterCreate(system, cmd, args, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

    // log the completion of the command and exit
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}
```

Actual command function should be defined as per the below example.
* it should be checking if system is set and initialize it if not
* it should pass the inventory argument to the init function
* it should NOT be checking for upgrades (this should only be done in the Execute function so it only happens once)

```go
func (c *ClusterCreateCmd) ClusterCreate(system *System, cmd []string, args []string, inventory *backends.Inventory) error {
    if system == nil {
        var err error
        system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, cmd, c, args...)
        if err != nil {
            return err
        }
    }
    // if we intend to use the inventory
    if inventory == nil {
        inventory = system.Backend.GetInventory()
    }
}
```

## Initialize features

* InitBackend - whether to initialize the backend (set if the command requires it)
* UpgradeCheck - check for available aerolab upgrade and print the upgrade message if needed
* ExistingInventory - pass the inventory to the init function - if this is set, the system Init function will set the inventory to the passed one, and will not try to fetch it from the backend

## Flow

* the function being called as Execute:
  * should only happen if the function is called directly from the CLI; internal calls should be done via the command function, not Execute
  * Execute will initialize the system, perform basic logging, and call the command function itself, passing it the system parameter, command name and arguments
  * the command function itself will then continue without reinitializing the system, as it was already set by the Execute function
* the command is called as the command function itself (c.ClusterCreate) instead of via Execute (called internally from another function):
  * the caller may decide to pass the initialized system, in which case the command function will share the system.Opts state and logger with the caller
  * the caller may decide to not pass the system (set to nil):
    * the command function will initialize the system itself as needed
    * if the Inventory was passed to the command function, it will pass it to the system Init. This avoids re-fetching the inventory from the backend
    * if the Inventory was not passed to the command function, the system Init will fetch the inventory from the backend for this call

## Cmd Tags

* instances:
    * `aerolab.type` - instance type, values:
      * `template.create` - instance is a temp instance used for creating templates and should not otherwise exist
      * values from `aerolab.image.type`
    * `aerolab.soft.version` - software version, values:
      * `5.1.0.1` - aerospike version
      * `1.0.0` - AMS version
      * etc
    * `aerolab.custom.image` - Docker: if set, the instance is a custom image and attach should use docker's exec
* images:
    * `aerolab.image.type` - image type, values:
      * `aerospike` - aerospike image
      * `ams` - AMS image
      * etc
    * `aerolab.soft.version` - software version, values:
      * `5.1.0.1` - aerospike version
      * `1.0.0` - AMS version
      * etc
