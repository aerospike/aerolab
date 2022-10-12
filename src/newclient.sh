function help() {
    echo "\nUsage:\n  ./newclient.sh Name command-name 'command description'"
    echo "\nPay attention to UpperLower case of Name and command-name."
    echo "\nEx: ./newclient.sh Tools tools 'aerospike-tools'\n"
    exit 1
}

[ "$1" = "" ] && help
[ "$2" = "" ] && help
[ "$3" = "" ] && help
[ "$1" = "-h" ] && help
[ "$1" = "--help" ] && help
[ "$1" = "help" ] && help

client=$1
cmd=$2
description=$3

sed -i.bak -e "s/\/\/ NEW_CLIENTS_CREATE/${client} clientCreate${client}Cmd \`command:\"${cmd}\" subcommands-optional:\"true\" description:\"${description}\"\`\n\/\/ NEW_CLIENTS_CREATE/g" \
  -e "s/\/\/ NEW_CLIENTS_ADD/${client} clientAdd${client}Cmd \`command:\"${cmd}\" subcommands-optional:\"true\" description:\"${description}\"\`\n\/\/ NEW_CLIENTS_ADD/g" \
  -e "s/\/\/ NEW_CLIENTS_BACKEND/addBackendSwitch(\"client.create.${cmd}\", \"aws\", \&a.opts.Client.Create.${client}.Aws)\naddBackendSwitch(\"client.create.${cmd}\", \"docker\", \&a.opts.Client.Create.${client}.Docker)\naddBackendSwitch(\"client.grow.${cmd}\", \"aws\", \&a.opts.Client.Grow.${client}.Aws)\naddBackendSwitch(\"client.grow.${cmd}\", \"docker\", \&a.opts.Client.Grow.${client}.Docker)\n\n\/\/ NEW_CLIENTS_BACKEND/g" \
  cmdClientCreateGrow.go

cat <<EOF > cmdClientDo${client}.go
package main

import (
	"errors"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreate${client}Cmd struct {
	clientCreateBaseCmd
	aerospikeVersionCmd
	chDirCmd
}

type clientAdd${client}Cmd struct {
    // TODO below switches are examples, adjust to your needs
	ClientName  TypeClientName \`short:"n" long:"group-name" description:"Client group name" default:"client"\`
	Machines    TypeMachines   \`short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""\`
	StartScript flags.Filename \`short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"\`
	Help        helpCmd        \`command:"help" subcommands-optional:"true" description:"Print help"\`
}

func (c *clientCreate${client}Cmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	machines, err := c.createBase(args, "${cmd}")
	if err != nil {
		return err
	}
    // TODO copy Create switches to Add switches and run the Add command
	a.opts.Client.Add.${client}.ClientName = c.ClientName
	a.opts.Client.Add.${client}.StartScript = c.StartScript
	a.opts.Client.Add.${client}.Machines = TypeMachines(intSliceToString(machines, ","))
	return a.opts.Client.Add.${client}.add${client}(args)
}

func (c *clientAdd${client}Cmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.add${client}(args)
}

func (c *clientAdd${client}Cmd) add${client}(args []string) error {
	b.WorkOnClients()
	// TODO CODE HERE
	return errors.New("NOT IMPLEMENTED YET")
}
EOF

go fmt cmdClientCreateGrow.go
go fmt cmdClientDo${client}.go
