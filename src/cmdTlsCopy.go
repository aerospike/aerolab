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
	SourceClusterName      TypeClusterName `short:"s" long:"source" description:"Source Cluster name/Client Group" default:"mydc"`
	SourceNode             TypeNode        `short:"l" long:"source-node" description:"Source node from which to copy the TLS certificates" default:"1"`
	IsSourceClient         bool            `short:"c" long:"source-client" description:"set to indicate the source node is a client machine"`
	DestinationClusterName TypeClusterName `short:"d" long:"destination" description:"Destination Cluster name/Client Group" default:"client"`
	DestinationNodeList    TypeNodes       `short:"a" long:"destination-nodes" description:"List of destination nodes to copy the TLS certs to, comma separated. Empty=ALL." default:""`
	IsDestinationClient    bool            `short:"C" long:"destination-client" description:"set to indicate the destination cluster is a client group"`
	TlsName                string          `short:"t" long:"tls-name" description:"Common Name (tlsname)" default:"tls1"`
	Help                   helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *tlsCopyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running tls.copy")
	clusterList := make(map[string]bool)
	clusters, err := b.ClusterList()
	if err != nil {
		return err
	}
	b.WorkOnClients()
	clients, err := b.ClusterList()
	b.WorkOnServers()
	if err != nil {
		return err
	}
	for _, cc := range clusters {
		clusterList[cc] = false
	}
	for _, cc := range clients {
		clusterList[cc] = true
	}

	if (!c.IsSourceClient && !inslice.HasString(clusters, string(c.SourceClusterName))) || (c.IsSourceClient && !inslice.HasString(clients, string(c.SourceClusterName))) {
		return errors.New("source Cluster not found")
	}
	if (!c.IsDestinationClient && !inslice.HasString(clusters, string(c.DestinationClusterName))) || (c.IsDestinationClient && !inslice.HasString(clients, string(c.DestinationClusterName))) {
		return errors.New("destination Cluster not found")
	}

	if c.IsDestinationClient {
		b.WorkOnClients()
	}
	err = c.DestinationNodeList.ExpandNodes(string(c.DestinationClusterName))
	b.WorkOnServers()
	if err != nil {
		return err
	}

	if c.IsSourceClient {
		b.WorkOnClients()
	}
	sourceClusterNodes, err := b.NodeListInCluster(string(c.SourceClusterName))
	b.WorkOnServers()
	if err != nil {
		return err
	}
	if c.IsDestinationClient {
		b.WorkOnClients()
	}
	destClusterNodes, err := b.NodeListInCluster(string(c.DestinationClusterName))
	if err != nil {
		return err
	}
	b.WorkOnServers()

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

	if c.IsSourceClient {
		b.WorkOnClients()
	}
	out, err := b.RunCommands(string(c.SourceClusterName), [][]string{{"ls", path.Join("/etc/aerospike/ssl/", c.TlsName)}}, []int{c.SourceNode.Int()})
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
		out, err := b.RunCommands(string(c.SourceClusterName), [][]string{{"cat", path.Join("/etc/aerospike/ssl/", c.TlsName, file)}}, []int{c.SourceNode.Int()})
		if err != nil {
			return err
		}
		nout := out[0]
		fl = append(fl, fileList{path.Join("/etc/aerospike/ssl/", c.TlsName, file), bytes.NewReader(nout), len(nout)})
	}
	b.WorkOnServers()
	if c.IsDestinationClient {
		b.WorkOnClients()
	}
	_, err = b.RunCommands(string(c.DestinationClusterName), [][]string{{"rm", "-rf", path.Join("/etc/aerospike/ssl/", c.TlsName)}, {"mkdir", "-p", path.Join("/etc/aerospike/ssl/", c.TlsName)}}, nodesList)
	if err != nil {
		return err
	}

	err = b.CopyFilesToCluster(string(c.DestinationClusterName), fl, nodesList)
	if err != nil {
		return err
	}
	b.WorkOnServers()
	log.Print("Done")
	return nil
}
