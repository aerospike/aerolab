package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

type confNamespaceMemoryCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Path            string          `short:"p" long:"path" description:"Path to aerospike.conf on the remote nodes" default:"/etc/aerospike/aerospike.conf"`
	Namespace       string          `short:"m" long:"namespace" description:"Name of the namespace to adjust" default:"test"`
	MemPct          int             `short:"r" long:"mem-pct" description:"The percentage of RAM to use for the namespace memory" default:"50"`
	ParallelThreads int             `long:"threads" description:"Run on this many nodes in parallel" default:"50"`
	Help            helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confNamespaceMemoryCmd) Execute(args []string) error {
	if earlyProcessV2(nil, true) {
		return nil
	}
	log.Println("Running conf.namespace-memory")
	err := c.Nodes.ExpandNodes(c.ClusterName.String())
	if err != nil {
		return err
	}
	nodes, err := c.Nodes.Translate(c.ClusterName.String())
	if err != nil {
		return err
	}

	returns := parallelize.MapLimit(nodes, c.ParallelThreads, func(node int) error {
		// TODO get memory size and calculate percentage into memSizeGb variable
		out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"free", "-b"}}, []int{node})
		if err != nil {
			nout := ""
			for _, n := range out {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("error on cluster %s: %s: %s", c.ClusterName, nout, err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(out[0]))
		memSizeGb := 0
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "Mem:") {
				continue
			}
			memBytes := strings.Fields(line)
			if len(memBytes) < 2 {
				return errors.New("memory line corrupt from free -m")
			}
			memSizeGb, err = strconv.Atoi(memBytes[1])
			if err != nil {
				return fmt.Errorf("could not get memory from %s: %s", memBytes[1], err)
			}
			memSizeGb = memSizeGb / 1024 / 1024
		}
		if memSizeGb == 0 {
			return errors.New("could not find memory size from free -b")
		}
		memSizeGb = memSizeGb * c.MemPct / 100 / 1024
		if memSizeGb == 0 {
			return errors.New("percentage would result in memory size 0")
		}
		// get config file
		out, err = b.RunCommands(c.ClusterName.String(), [][]string{{"cat", c.Path}}, []int{node})
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
		if s.Type("namespace "+c.Namespace) == aeroconf.ValueNil {
			return errors.New("namespace not found")
		}
		s.Stanza("namespace "+c.Namespace).SetValue("memory-size", strconv.Itoa(memSizeGb)+"G")
		// write file back
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
