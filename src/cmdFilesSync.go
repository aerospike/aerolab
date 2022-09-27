package main

import (
	"errors"
	"log"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	"github.com/jessevdk/go-flags"
)

type filesSyncCmd struct {
	SourceClusterName TypeClusterName `short:"n" long:"source-name" description:"Source Cluster name" default:"mydc"`
	SourceNode        TypeNode        `short:"l" long:"source-node" description:"Source Node number" default:"1"`
	DestClusterName   TypeClusterName `short:"d" long:"dest-name" description:"Source Cluster name" default:"mydc"`
	DestNodes         TypeNodes       `short:"o" long:"dest-nodes" description:"Destination nodes, comma separated; empty = all except source node" default:""`
	Path              string          `short:"p" long:"path" description:"Path to sync"`
}

func (c *filesSyncCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running files.list")

	cList, err := b.ClusterList()
	if err != nil {
		return err
	}

	// check source cluster exists
	if !inslice.HasString(cList, string(c.SourceClusterName)) {
		return errors.New("source cluster does not exist")
	}

	// check destination cluster exists
	if !inslice.HasString(cList, string(c.DestClusterName)) {
		return errors.New("destination cluster does not exist")
	}

	sourceNodes, err := b.NodeListInCluster(string(c.SourceClusterName))
	if err != nil {
		return err
	}

	destNodes := sourceNodes
	if string(c.SourceClusterName) != string(c.DestClusterName) {
		destNodes, err = b.NodeListInCluster(string(c.DestClusterName))
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
	err = c.DestNodes.ExpandNodes(string(c.DestClusterName))
	if err != nil {
		return err
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
	err = a.opts.Files.Download.Execute(nil)
	if err != nil {
		return err
	}

	a.opts.Files.Upload.ClusterName = c.DestClusterName
	a.opts.Files.Upload.Nodes = TypeNodes(intSliceToString(destNodeList, ","))
	_, src := path.Split(strings.TrimSuffix(c.Path, "/"))
	a.opts.Files.Upload.Files.Source = flags.Filename(path.Join(dir, src))
	dst, _ := path.Split(strings.TrimSuffix(c.Path, "/"))
	a.opts.Files.Upload.Files.Destination = flags.Filename(dst)
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
