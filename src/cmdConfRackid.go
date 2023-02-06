package main

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

type confRackIdCmd struct {
	aerospikeStartCmd
	RackId     string `short:"i" long:"id" description:"Rack ID to use" default:"0"`
	Namespaces string `short:"m" long:"namespaces" description:"comma-separated list of namespaces to modify; empty=all" default:""`
}

func (c *confRackIdCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running conf.rackid")

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", string(c.ClusterName))
		return err
	}

	err = c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	// get cluster IPs and node list
	nodeList, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}

	// limit to only the nodeList list of nodes from c.Nodes
	nodes := []int{}
	if c.Nodes.String() == "" {
		nodes = nodeList
	} else {
		for _, node := range strings.Split(c.Nodes.String(), ",") {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return fmt.Errorf("node list could not be converted to integer: %s", err)
			}
			if !inslice.HasInt(nodeList, nodeInt) {
				return fmt.Errorf("node %d not found", nodeInt)
			}
			nodes = append(nodes, nodeInt)
		}
	}

	namespaces := []string{}
	if c.Namespaces != "" {
		namespaces = strings.Split(c.Namespaces, ",")
	}
	foundns := 0

	// fix config if needed, read custom config file path if needed
	for _, i := range nodes {
		files := []fileList{}
		var r [][]string
		r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
		var nr [][]byte
		nr, err = b.RunCommands(string(c.ClusterName), r, []int{i})
		if err != nil {
			return fmt.Errorf("cluster=%s node=%v RunCommands error=%s", string(c.ClusterName), i, err)
		}
		cc, err := aeroconf.Parse(bytes.NewReader(nr[0]))
		if err != nil {
			return fmt.Errorf("config parse failure: %s", err)
		}
		// modify rack-id in given/all namespaces
		for _, key := range cc.ListKeys() {
			if strings.HasPrefix(key, "namespace ") && cc.Type(key) == aeroconf.ValueStanza {
				ns := strings.Split(key, " ")
				if len(ns) < 2 && ns[1] == "" {
					log.Printf("stanza namespace does not have a name, skipping: %s", key)
					continue
				}
				if len(namespaces) == 0 || inslice.HasString(namespaces, strings.Trim(ns[1], "\r\t\n ")) {
					cc.Stanza(key).SetValue("rack-id", c.RackId)
					foundns++
				}
			}
		}
		if foundns < len(namespaces) {
			return fmt.Errorf("not all listed namespaces were found, or no namespaces found at all")
		}
		// modify end
		buf := new(bytes.Buffer)
		cc.Write(buf, "", "    ", true)
		newconf := buf.String()
		files = append(files, fileList{"/etc/aerospike/aerospike.conf", strings.NewReader(newconf), len(newconf)})
		if len(files) > 0 {
			err := b.CopyFilesToCluster(string(c.ClusterName), files, []int{i})
			if err != nil {
				return fmt.Errorf("cluster=%s node=%v CopyFilesToCluster error=%s", string(c.ClusterName), i, err)
			}
		}
	}

	log.Print("Done, do not forget to restart the aerospike service")
	return nil
}
