package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/logger"
)

type ConfAdjustCmd struct {
	ClusterName     TypeClusterName   `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes         `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Path            string            `short:"p" long:"path" description:"Path to aerospike.conf on the remote nodes" default:"/etc/aerospike/aerospike.conf"`
	Command         string            `short:"c" long:"command" description:"command to run, get|set|create|delete" webchoice:"create,set,delete,get"`
	Key             string            `short:"k" long:"key" description:"the key to work on; eg 'namespace bar.storage-engine device.write-block-size'" webrequired:"true"`
	Values          []string          `short:"v" long:"value" description:"value to set a key to when using set option; can be specified multiple times"`
	ParallelThreads int               `long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help            ConfAdjustHelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ConfAdjustHelpCmd struct{}

func (c *ConfAdjustHelpCmd) Execute(args []string) error {
	return c.help("")
}

func (c *ConfAdjustHelpCmd) help(msg string) error {
	helpStr := fmt.Sprintf(`COMMANDS:
	get    - get configuration/stanza and print to stdout
	delete - delete configuration/stanza
	set    - set configuration parameter
	create - create a new stanza
	
PATH: path.to.item or path.to.stanza, e.g. network.heartbeat
SET-VALUE: for the 'set' command - used to specify value of parameter; leave empty to crete no-value param
To specify a literal dot in the configuration path, use .. (double-dot)
EXAMPLES:
	%s -n mydc create network.heartbeat
	%s -n mydc set network.heartbeat.mode mesh
	%s -n mydc set network.heartbeat.mesh-seed-address-port "172.17.0.2 3000" "172.17.0.3 3000"
	%s -n mydc create service
	%s -n mydc set service.proto-fd-max 3000
	%s -n mydc get
	%s -n mydc get network.service

`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	return PrintHelp(true, helpStr)
}

func (c *ConfAdjustCmd) Execute(args []string) error {
	cmd := []string{"conf", "adjust"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.Adjust(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ConfAdjustCmd) Adjust(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"conf", "adjust"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	instances := inventory.Instances.WithState(backends.LifeCycleStateRunning).WithClusterName(c.ClusterName.String())
	if c.Nodes != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return err
		}
		instances = instances.WithNodeNo(nodes...)
		if instances.Count() != len(nodes) {
			return fmt.Errorf("some nodes not found: %s", c.Nodes.String())
		}
	}

	if c.Command == "get" {
		c.Values = []string{}
	}
	if len(args) < 1 {
		if c.Command != "" || c.Key != "" || len(c.Values) > 0 {
			args = append([]string{c.Command, c.Key}, c.Values...)
		} else {
			c.Help.help("Zero args provided and no values are set")
			return nil
		}
	}
	command := args[0]
	path := ""
	if len(args) > 1 {
		path = args[1]
	} else if command != "get" {
		c.Help.help("Command provided requires more arguments")
		return nil
	}
	setValues := []string{""}

	switch command {
	case "get":
		if len(args) > 2 {
			c.Help.help("Get command does not accept value parameters")
			return nil
		}
	case "delete":
		if len(args) != 2 {
			c.Help.help("Invalid argument count for delete command")
			return nil
		}
	case "set":
		if len(os.Args) > 2 {
			setValues = args[2:]
		}
	case "create":
		if len(args) != 2 {
			c.Help.help("Invalid argument count for create command")
			return nil
		}
	}
	var hasErr bool
	parallelize.ForEachLimit(instances.Describe(), c.ParallelThreads, func(i *backends.Instance) {
		conf, err := i.GetSftpConfig("root")
		if err != nil {
			logger.Error("Failed to adjust configuration for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
		client, err := sshexec.NewSftp(conf)
		if err != nil {
			logger.Error("Failed to adjust configuration for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
		defer client.Close()
		var buf bytes.Buffer
		err = client.ReadFile(&sshexec.FileReader{
			SourcePath:  c.Path,
			Destination: &buf,
		})
		if err != nil {
			logger.Error("Failed to adjust configuration for node %s: %s", i.Name, err)
			hasErr = true
			return
		}

		// edit actual file contents
		s, err := aeroconf.Parse(&buf)
		if err != nil {
			logger.Error("failed to parse configuration", err)
			hasErr = true
			return
		}

		sa := s
		path = strings.ReplaceAll(path, "..", "±§±§±")
		pathn := strings.Split(path, ".")
		for i := range pathn {
			pathn[i] = strings.ReplaceAll(pathn[i], "±§±§±", ".")
		}
		if pathn[0] == "" && len(pathn) > 1 {
			pathn = pathn[1:]
		}
		switch command {
		case "get":
			prefix := ""
			if len(instances.Describe()) > 1 {
				prefix = fmt.Sprintf("(%v) ", i.NodeNo)
			}
			if path != "" {
				for j, i := range pathn {
					if sa.Type(i) == aeroconf.ValueString {
						if len(pathn) > j+1 {
							logger.Error("key item '%s' is a string not a stanza", i)
							hasErr = true
							return
						}
						vals, err := sa.GetValues(i)
						if err != nil {
							logger.Error("could not get values for '%s'", i)
							hasErr = true
							return
						}
						valstring := ""
						for _, vv := range vals {
							if valstring != "" {
								valstring = valstring + " "
							}
							valstring = valstring + *vv
						}
						fmt.Printf("%s%s %s\n", prefix, i, valstring)
						c.Values = append(c.Values, valstring)
						return
					} else {
						sa = sa.Stanza(i)
						if sa == nil {
							logger.Error("stanza not found")
							hasErr = true
							return
						}
					}
				}
			}
			var buf bytes.Buffer
			sa.Write(&buf, prefix, "    ", true)
			fmt.Print(buf.String())
		case "delete":
			for _, i := range pathn[0 : len(pathn)-1] {
				sa = sa.Stanza(i)
				if sa == nil {
					logger.Error("stanza not found")
					hasErr = true
					return
				}
			}
			err = sa.Delete(pathn[len(pathn)-1])
			if err != nil {
				logger.Error("failed to delete stanza", err)
				hasErr = true
				return
			}
		case "set":
			for _, i := range pathn[0 : len(pathn)-1] {
				sa = sa.Stanza(i)
				if sa == nil {
					logger.Error("stanza not found")
					hasErr = true
					return
				}
			}
			err = sa.SetValues(pathn[len(pathn)-1], aeroconf.SliceToValues(setValues))
			if err != nil {
				logger.Error("failed to set values", err)
				hasErr = true
				return
			}
		case "create":
			for _, i := range pathn {
				if sa.Stanza(i) == nil {
					err = sa.NewStanza(i)
					if err != nil {
						logger.Error("failed to create stanza", err)
						hasErr = true
						return
					}
				}
				sa = sa.Stanza(i)
			}
		}
		var newConfig []byte
		if command != "get" {
			var buf bytes.Buffer
			err = s.Write(&buf, "", "    ", true)
			if err != nil {
				logger.Error("failed to write file", err)
				hasErr = true
				return
			}
			newConfig = buf.Bytes()
			// edit end
		}

		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath: c.Path,
			Source:   bytes.NewReader(newConfig),
		})
		if err != nil {
			logger.Error("Failed to adjust configuration for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
	})
	if hasErr {
		return errors.New("some nodes failed to adjust configuration")
	}
	return nil
}
