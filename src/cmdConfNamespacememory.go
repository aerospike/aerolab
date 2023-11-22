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
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Path        string          `short:"p" long:"path" description:"Path to aerospike.conf on the remote nodes" default:"/etc/aerospike/aerospike.conf"`
	Namespace   string          `short:"m" long:"namespace" description:"Name of the namespace to adjust" default:"test"`
	MemPct      int             `short:"r" long:"mem-pct" description:"The percentage of RAM to use for the namespace memory" default:"50"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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
		sysSizeGb := memSizeGb / 1024
		memSizeGb = memSizeGb * c.MemPct / 100 / 1024
		if memSizeGb == 0 {
			return errors.New("percentage would result in memory size 0")
		}
		// get asd version
		vno, err := b.RunCommands(c.ClusterName.String(), [][]string{{"cat", "/opt/aerolab.aerospike.version"}}, []int{node})
		if err != nil {
			nout := ""
			for _, n := range vno {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("error on cluster %s: %s: %s", c.ClusterName, nout, err)
		}
		is7 := true
		if VersionCheck(string(vno[0]), "7.0.0.0") > 0 {
			is7 = false
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
		if !is7 {
			log.Printf("Processing NodeVersion %s NodeNumber %d TotalRamGb %d memory-size=%dG", string(vno[0]), node, sysSizeGb, memSizeGb)
			s.Stanza("namespace "+c.Namespace).SetValue("memory-size", strconv.Itoa(memSizeGb)+"G")
		} else {
			if s.Stanza("namespace "+c.Namespace).Type("storage-engine memory") != aeroconf.ValueStanza {
				log.Printf("WARN Skipping NodeVersion %s NodeNumber %d storage-engine is not memory", string(vno[0]), node)
				return nil
			}
			if s.Stanza("namespace "+c.Namespace).Stanza("storage-engine memory").Type("device") != aeroconf.ValueNil {
				log.Printf("WARN Skipping NodeVersion %s NodeNumber %d device backing configured for storage-engine", string(vno[0]), node)
				return nil
			}
			if s.Stanza("namespace "+c.Namespace).Stanza("storage-engine memory").Type("file") != aeroconf.ValueNil {
				log.Printf("WARN Skipping NodeVersion %s NodeNumber %d file backing configured for storage-engine", string(vno[0]), node)
				return nil
			}
			log.Printf("Processing NodeVersion %s NodeNumber %d TotalRamGb %d data-size=%dG", string(vno[0]), node, sysSizeGb, memSizeGb)
			s.Stanza("namespace "+c.Namespace).Stanza("storage-engine memory").SetValue("data-size", strconv.Itoa(memSizeGb)+"G")
		}
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
