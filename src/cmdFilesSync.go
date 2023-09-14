package main

import (
	"errors"
	"log"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type filesSyncCmd struct {
	SourceClusterName TypeClusterName `short:"n" long:"source-name" description:"Source Cluster name" default:"mydc"`
	SourceNode        TypeNode        `short:"l" long:"source-node" description:"Source Node number" default:"1"`
	IsClientS         bool            `short:"c" long:"source-client" description:"set this to indicate source is client group"`
	DestClusterName   TypeClusterName `short:"d" long:"dest-name" description:"Source Cluster name" default:"mydc"`
	DestNodes         TypeNodes       `short:"o" long:"dest-nodes" description:"Destination nodes, comma separated; empty = all except source node" default:""`
	IsClientD         bool            `short:"C" long:"dest-client" description:"set this to indicate destination is client group"`
	Path              string          `short:"p" long:"path" description:"Path to sync"`
	parallelThreadsCmd
}

func (c *filesSyncCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running files.list")

	cList := make(map[string]bool)
	cListCluster, err := b.ClusterList()
	if err != nil {
		return err
	}

	b.WorkOnClients()
	cListClient, err := b.ClusterList()
	if err != nil {
		return err
	}
	b.WorkOnServers()
	for _, c := range cListCluster {
		cList[c] = false
	}
	for _, c := range cListClient {
		cList[c] = true
	}

	// check source cluster exists
	if (!c.IsClientS && !inslice.HasString(cListCluster, string(c.SourceClusterName))) || (c.IsClientS && !inslice.HasString(cListClient, string(c.SourceClusterName))) {
		return errors.New("source cluster does not exist")
	}

	// check destination cluster exists
	if (!c.IsClientD && !inslice.HasString(cListCluster, string(c.DestClusterName))) || (c.IsClientD && !inslice.HasString(cListClient, string(c.DestClusterName))) {
		return errors.New("destination cluster does not exist")
	}

	if c.IsClientS {
		b.WorkOnClients()
	}
	sourceNodes, err := b.NodeListInCluster(string(c.SourceClusterName))
	b.WorkOnServers()
	if err != nil {
		return err
	}

	destNodes := sourceNodes
	if string(c.SourceClusterName) != string(c.DestClusterName) {
		if c.IsClientD {
			b.WorkOnClients()
		}
		destNodes, err = b.NodeListInCluster(string(c.DestClusterName))
		b.WorkOnServers()
	}
	if err != nil {
		return err
	}

	// check source node exists
	if !inslice.HasInt(sourceNodes, c.SourceNode.Int()) {
		return errors.New("source node not found in cluster")
	}

	// build destination node list
	destNodeList := []int{}
	if c.DestNodes != "" {
		if c.IsClientD {
			b.WorkOnClients()
		}
		err = c.DestNodes.ExpandNodes(string(c.DestClusterName))
		b.WorkOnServers()
		if err != nil {
			return err
		}
	}
	if c.DestNodes == "" {
		if string(c.SourceClusterName) != string(c.DestClusterName) {
			destNodeList = destNodes
		} else {
			for _, d := range destNodes {
				if d == c.SourceNode.Int() {
					continue
				}
				destNodeList = append(destNodeList, d)
			}
		}
	} else {
		for _, i := range strings.Split(c.DestNodes.String(), ",") {
			d, err := strconv.Atoi(i)
			if err != nil {
				return err
			}
			if string(c.SourceClusterName) == string(c.DestClusterName) && d == c.SourceNode.Int() {
				return errors.New("source node is also specified as a destination node, that's not going to work")
			}
			destNodeList = append(destNodeList, d)
		}
	}

	if len(destNodeList) == 0 {
		return errors.New("destination node list is empty, is this a one-node cluster?")
	}

	// copy c.SourceCluster:c.SourceNode -> c.DestCluster:[destNodeList]

	dir, err := os.MkdirTemp("", "aerolab-synctmp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	a.opts.Files.Download.ClusterName = c.SourceClusterName
	a.opts.Files.Download.Nodes = TypeNodes(strconv.Itoa(c.SourceNode.Int()))
	a.opts.Files.Download.Files.Source = flags.Filename(c.Path)
	a.opts.Files.Download.Files.Destination = flags.Filename(dir)
	a.opts.Files.Download.IsClient = c.IsClientS
	a.opts.Files.Download.doLegacy = true
	err = a.opts.Files.Download.Execute(nil)
	if err != nil {
		return err
	}

	a.opts.Files.Upload.IsClient = c.IsClientD
	a.opts.Files.Upload.ClusterName = c.DestClusterName
	a.opts.Files.Upload.Nodes = TypeNodes(intSliceToString(destNodeList, ","))
	_, src := path.Split(strings.TrimSuffix(c.Path, "/"))
	a.opts.Files.Upload.Files.Source = flags.Filename(path.Join(dir, src))
	dst, _ := path.Split(strings.TrimSuffix(c.Path, "/"))
	a.opts.Files.Upload.Files.Destination = flags.Filename(dst)
	a.opts.Files.Upload.doLegacy = true
	a.opts.Files.Upload.ParallelThreads = c.ParallelThreads
	err = a.opts.Files.Upload.Execute(nil)
	if err != nil {
		return err
	}

	log.Print("Done")
	return nil
}

func intSliceToString(a []int, sep string) string {
	var c string
	for _, b := range a {
		c = c + strconv.Itoa(b) + sep
	}
	c = strings.TrimSuffix(c, sep)
	return c
}
