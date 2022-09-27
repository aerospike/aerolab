package main

import (
	"bytes"
	"errors"
	"log"
	"path"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type tlsCopyCmd struct {
	SourceClusterName      TypeClusterName `short:"s" long:"source" description:"Source Cluster name" default:"mydc"`
	SourceNode             TypeNode        `short:"l" long:"source-node" description:"Source node from which to copy the TLS certificates" default:"1"`
	DestinationClusterName TypeClusterName `short:"d" long:"destination" description:"Destination Cluster name." default:"client"`
	DestinationNodeList    TypeNodes       `short:"a" long:"destination-nodes" description:"List of destination nodes to copy the TLS certs to, comma separated. Empty=ALL." default:""`
	TlsName                string          `short:"t" long:"tls-name" description:"Common Name (tlsname)" default:"tls1"`
	Help                   helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *tlsCopyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running tls.copy")
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}

	if !inslice.HasString(clusterList, string(c.SourceClusterName)) {
		return errors.New("source Cluster not found")
	}
	if !inslice.HasString(clusterList, string(c.DestinationClusterName)) {
		return errors.New("destination Cluster not found")
	}

	err = c.DestinationNodeList.ExpandNodes(string(c.DestinationClusterName))
	if err != nil {
		return err
	}

	sourceClusterNodes, err := b.NodeListInCluster(string(c.SourceClusterName))
	if err != nil {
		return err
	}
	destClusterNodes, err := b.NodeListInCluster(string(c.DestinationClusterName))
	if err != nil {
		return err
	}

	nodesList := []int{}
	if c.DestinationNodeList == "" {
		nodesList = destClusterNodes
	} else {
		nodes := strings.Split(c.DestinationNodeList.String(), ",")
		for _, node := range nodes {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return err
			}
			nodesList = append(nodesList, nodeInt)
			if !inslice.HasInt(destClusterNodes, nodeInt) {
				return errors.New("destination Node does not exist in cluster")
			}
		}
	}

	if !inslice.HasInt(sourceClusterNodes, c.SourceNode.Int()) {
		return errors.New("source Node does not exist in cluster")
	}

	// nodesList has list of nodes to copy TLS cert to
	// we have: sourceClusterNodes, destClusterNodes, nodesList, and everything in conf struct

	out, err := b.RunCommands(string(c.SourceClusterName), [][]string{[]string{"ls", path.Join("/etc/aerospike/ssl/", c.TlsName)}}, []int{c.SourceNode.Int()})
	if err != nil {
		return err
	}
	files := strings.Split(string(out[0]), "\n")
	var fl []fileList
	for _, file := range files {
		file = strings.Trim(file, "\n\t\r ")
		if file == "" {
			continue
		}
		out, err := b.RunCommands(string(c.SourceClusterName), [][]string{[]string{"cat", path.Join("/etc/aerospike/ssl/", c.TlsName, file)}}, []int{c.SourceNode.Int()})
		if err != nil {
			return err
		}
		nout := out[0]
		fl = append(fl, fileList{path.Join("/etc/aerospike/ssl/", c.TlsName, file), bytes.NewReader(nout), len(nout)})
	}

	_, err = b.RunCommands(string(c.DestinationClusterName), [][]string{[]string{"rm", "-rf", path.Join("/etc/aerospike/ssl/", c.TlsName)}, []string{"mkdir", "-p", path.Join("/etc/aerospike/ssl/", c.TlsName)}}, nodesList)
	if err != nil {
		return err
	}

	err = b.CopyFilesToCluster(string(c.DestinationClusterName), fl, nodesList)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}
