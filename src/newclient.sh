function help() {
    echo "Usage: ./newclient.sh Name"
    echo "Ensure 'Name' starts from uppercase letter and the rest are lowercase"
    exit 1
}

[ "$1" = "" ] && help
[ "$1" = "-h" ] && help
[ "$1" = "--help" ] && help
[ "$1" = "help" ] && help

client=$1
cmd=$2
description=$3

sed -i.bak -e "s/\/\/ NEW_CLIENTS_CREATE/${client} clientCreate${client}Cmd \`command:"${cmd}" subcommands-optional:"true" description:"${description}"\`\n\/\/ NEW_CLIENTS_CREATE/g" cmdClientCreateGrow.go

cat <<EOF > cmdClientDo${client}.go
package main

import (
	"errors"

	"github.com/jessevdk/go-flags"
)

type clientCreate${client}Cmd struct {
	clientCreateBaseCmd
	aerospikeVersionCmd
	chDirCmd
}

type clientAdd${client}Cmd struct {
    // TODO below switches are examples, adjust to your needs
	ClientName  TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	Help        helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreate${client}Cmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	machines, err := c.createBase(args)
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
