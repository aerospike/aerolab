package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

type confAdjustCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Path        string          `short:"p" long:"path" description:"Path to aerospike.conf on the remote nodes" default:"/etc/aerospike/aerospike.conf"`
	Command     string          `short:"c" long:"command" description:"command to run, get|set|create|delete" webchoice:"create,set,delete,get"`
	Key         string          `short:"k" long:"key" description:"the key to work on; eg 'namespace bar.storage-engine device.write-block-size'" webrequired:"true"`
	Values      []string        `short:"v" long:"value" description:"value to set a key to when using set option; can be specified multiple times"`
	parallelThreadsCmd
}

func (c *confAdjustCmd) Execute(args []string) error {
	if earlyProcessV2(nil, true) {
		return nil
	}
	if c.Command == "get" {
		c.Values = []string{}
	}
	if len(args) < 1 {
		if c.Command != "" || c.Key != "" || len(c.Values) > 0 {
			args = append([]string{c.Command, c.Key}, c.Values...)
		} else {
			c.help("E0")
			return nil
		}
	}
	command := args[0]
	path := ""
	if len(args) > 1 {
		path = args[1]
	} else if command != "get" {
		c.help("E1")
		return nil
	}
	setValues := []string{""}

	switch command {
	case "get":
		if len(args) > 2 {
			c.help("E2")
			return nil
		}
	case "delete":
		if len(args) != 2 {
			c.help("E3")
			return nil
		}
	case "set":
		if len(os.Args) > 2 {
			setValues = args[2:]
		}
	case "create":
		if len(args) != 2 {
			c.help("E4")
			return nil
		}
	}

	log.Println("Running conf.adjust")
	err := c.Nodes.ExpandNodes(c.ClusterName.String())
	if err != nil {
		return err
	}
	nodes, err := c.Nodes.Translate(c.ClusterName.String())
	if err != nil {
		return err
	}

	returns := parallelize.MapLimit(nodes, c.ParallelThreads, func(node int) error {
		out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"cat", c.Path}}, []int{node})
		if err != nil {
			nout := ""
			for _, n := range out {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("error on cluster %s: %s: %s", c.ClusterName, nout, err)
		}
		fileContents := bytes.NewReader(out[0])
		// edit actual file contents
		s, err := aeroconf.Parse(fileContents)
		if err != nil {
			return err
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
			if len(nodes) > 1 {
				prefix = fmt.Sprintf("(%v) ", node)
			}
			if path != "" {
				for j, i := range pathn {
					if sa.Type(i) == aeroconf.ValueString {
						if len(pathn) > j+1 {
							return fmt.Errorf("key item '%s' is a string not a stanza", i)
						}
						vals, err := sa.GetValues(i)
						if err != nil {
							return fmt.Errorf("could not get values for '%s'", i)
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
						return nil
					} else {
						sa = sa.Stanza(i)
						if sa == nil {
							return errors.New("stanza not found")
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
					return errors.New("stanza not found")
				}
			}
			err = sa.Delete(pathn[len(pathn)-1])
			if err != nil {
				return err
			}
		case "set":
			for _, i := range pathn[0 : len(pathn)-1] {
				sa = sa.Stanza(i)
				if sa == nil {
					return errors.New("stanza not found")
				}
			}
			err = sa.SetValues(pathn[len(pathn)-1], aeroconf.SliceToValues(setValues))
			if err != nil {
				return err
			}
		case "create":
			for _, i := range pathn {
				if sa.Stanza(i) == nil {
					err = sa.NewStanza(i)
					if err != nil {
						return err
					}
				}
				sa = sa.Stanza(i)
			}
		}
		if command != "get" {
			var buf bytes.Buffer
			err = s.Write(&buf, "", "    ", true)
			if err != nil {
				return err
			}
			contents := buf.Bytes()
			fileContents = bytes.NewReader(contents)
			// edit end
			err = b.CopyFilesToClusterReader(c.ClusterName.String(), []fileListReader{{filePath: c.Path, fileContents: fileContents, fileSize: len(contents)}}, []int{node})
			if err != nil {
				return err
			}
		}
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", nodes[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}
	log.Println("Done")
	return nil
}

func (c *confAdjustCmd) help(debugtext string) {
	debug(debugtext)
	comm := path.Base(os.Args[0]) + " conf adjust"
	fmt.Printf("\nUsage: %s [OPTIONS] {command} {path} [set-value1] [set-value2] [...set-valueX]\n", comm)
	fmt.Println("\n" + `[OPTIONS]
	-n, --name=  Cluster name (default: mydc)
	-l, --nodes= Nodes list, comma separated. Empty=ALL
	-p, --path=  Path to aerospike configuration file (default: /etc/aerospike/aerospike.conf)
	 --threads=  Number of parallel threads to run on (default: 50)`)
	fmt.Println("\n" + `COMMANDS:
	get    - get configuration/stanza and print to stdout
	delete - delete configuration/stanza
	set    - set configuration parameter
	create - create a new stanza`)
	fmt.Println("\n" + `PATH: path.to.item or path.to.stanza, e.g. network.heartbeat`)
	fmt.Println("\n" + `SET-VALUE: for the 'set' command - used to specify value of parameter; leave empty to crete no-value param`)
	fmt.Println("\n" + `To specify a literal dot in the configuration path, use .. (double-dot)`)
	fmt.Printf("\n"+`EXAMPLES:
	%s -n mydc create network.heartbeat
	%s -n mydc set network.heartbeat.mode mesh
	%s -n mydc set network.heartbeat.mesh-seed-address-port "172.17.0.2 3000" "172.17.0.3 3000"
	%s -n mydc create service
	%s -n mydc set service.proto-fd-max 3000
	%s -n mydc get
	%s -n mydc get network.service
	`+"\n", comm, comm, comm, comm, comm, comm, comm)
}
