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
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Path            string          `short:"p" long:"path" description:"Path to aerospike.conf on the remote nodes" default:"/etc/aerospike/aerospike.conf"`
	ParallelThreads int             `long:"threads" description:"Run on this many nodes in parallel" default:"50"`
}

func (c *confAdjustCmd) Execute(args []string) error {
	if earlyProcessV2(nil, true) {
		return nil
	}
	if len(args) < 2 {
		c.help()
		return nil
	}
	command := args[0]
	path := args[1]
	setValues := []string{""}

	switch command {
	case "delete":
		if len(args) != 2 {
			c.help()
			return nil
		}
	case "set":
		if len(os.Args) > 2 {
			setValues = args[2:]
		}
	case "create":
		if len(args) != 2 {
			c.help()
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
		pathn := strings.Split(path, ".")
		if pathn[0] == "" && len(pathn) > 1 {
			pathn = pathn[1:]
		}
		switch command {
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

func (c *confAdjustCmd) help() {
	comm := path.Base(os.Args[0]) + " conf adjust"
	fmt.Printf("\nUsage: %s [OPTIONS] {command} {path} [set-value1] [set-value2] [...set-valueX]\n", comm)
	fmt.Println("\n" + `[OPTIONS]
	-n, --name=  Cluster name (default: mydc)
	-l, --nodes= Nodes list, comma separated. Empty=ALL
	-p, --path=  Path to aerospike configuration file (default: /etc/aerospike/aerospike.conf)
	 --threads=  Number of parallel threads to run on (default: 50)`)
	fmt.Println("\n" + `COMMANDS:
	delete - delete configuration/stanza
	set    - set configuration parameter
	create - create a new stanza`)
	fmt.Println("\n" + `PATH: path.to.item or path.to.stanza, e.g. network.heartbeat`)
	fmt.Println("\n" + `SET-VALUE: for the 'set' command - used to specify value of parameter; leave empty to crete no-value param`)
	fmt.Printf("\n"+`EXAMPLES:
	%s -n mydc create network.heartbeat
	%s -n mydc set network.heartbeat.mode mesh
	%s -n mydc set network.heartbeat.mesh-seed-address-port "172.17.0.2 3000" "172.17.0.3 3000"
	%s -n mydc create service
	%s -n mydc set service.proto-fd-max 3000
	`+"\n", comm, comm, comm, comm, comm)
}