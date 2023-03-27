package main

import (
	"fmt"
	"log"
	"os"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientConfigureRestGatewayCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	Seed       string         `short:"s" long:"seed" description:"seed IP to change to"`
	User       string         `long:"user" description:"connect policy username"`
	Pass       string         `long:"pass" description:"connect policy password"`
	Help       helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureRestGatewayCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running client.configure.rest-gateway")
	b.WorkOnClients()

	script := c.UpdateScript()
	f, err := os.CreateTemp(string(a.opts.Config.Backend.TmpDir), "aerolab-rest-gw")
	if err != nil {
		return err
	}
	fn := f.Name()
	_, err = f.WriteString(script)
	f.Close()
	if err != nil {
		return err
	}
	a.opts.Files.Upload.IsClient = true
	a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
	a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
	a.opts.Files.Upload.Files.Source = flags.Filename(fn)
	a.opts.Files.Upload.Files.Destination = "/opt/reconfigure-gw.sh"
	err = a.opts.Files.Upload.runUpload(nil)
	if err != nil {
		return err
	}

	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	nargs := []string{"/bin/bash", "/opt/reconfigure-gw.sh"}
	err = a.opts.Attach.Client.run(nargs)
	if err != nil {
		return err
	}
	a.opts.Attach.Client.Detach = true
	nargs = []string{"/bin/bash", "/opt/autoload/01-restgw.sh"}
	err = a.opts.Attach.Client.run(nargs)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *clientConfigureRestGatewayCmd) UpdateScript() string {
	ret := "set -e\n"
	if c.Seed != "" {
		ret = ret + fmt.Sprintf("sed -i 's/^SEED=.*/SEED=%s/g' /opt/autoload/01-restgw.sh\n", c.Seed)
	}
	if c.User != "" {
		ret = ret + fmt.Sprintf("sed -i 's/^AUTH_USER=.*/AUTH_USER=%s/g' /opt/autoload/01-restgw.sh\n", c.User)
	}
	if c.Pass != "" {
		ret = ret + fmt.Sprintf("sed -i 's/^AUTH_PASS=.*/AUTH_PASS=%s/g' /opt/autoload/01-restgw.sh\n", c.Pass)
	}
	ret = ret + "chmod 755 /opt/autoload/01-restgw.sh\nset +e; pkill -9 java || exit 0\n"
	return ret
}
