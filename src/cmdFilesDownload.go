package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type filesRestCmd struct {
	Source      string
	Destination string
}

type filesDownloadCmd struct {
	ClusterName string              `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       string              `short:"l" long:"nodes" description:"Node number(s), comma-separated. Default=ALL" default:""`
	Aws         filesDownloadCmdAws `no-flag:"true"`
	Files       filesRestCmd        `positional-args:"true"`
}

type filesDownloadCmdAws struct {
	Verbose bool `short:"v" long:"verbose" description:"do not run scp in quiet mode"`
}

func init() {
	addBackendSwitch("files.download", "aws", &a.opts.Files.Download.Aws)
}

func (c *filesDownloadCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.Files.Source == "help" && c.Files.Destination == "" {
		return printHelp("If more than one node is specified, files will be downloaded to {Destination}/{nodeNumber}/")
	}
	if c.Files.Destination == "" {
		return printHelp("If more than one node is specified, files will be downloaded to {Destination}/{nodeNumber}/")
	}
	log.Print("Running files.download")
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, c.ClusterName) {
		err = fmt.Errorf("cluster does not exist: %s", c.ClusterName)
		return err
	}

	var nodes []int
	nodesList, err := b.NodeListInCluster(c.ClusterName)
	if err != nil {
		return err
	}
	if c.Nodes == "" {
		nodes = nodesList
	} else {
		for _, nodeString := range strings.Split(c.Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return err
			}
			if !inslice.HasInt(nodesList, nodeInt) {
				return fmt.Errorf("node %d does not exist in cluster", nodeInt)
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) == 0 {
		err = errors.New("found 0 nodes in cluster")
		return err
	}

	dst := c.Files.Destination
	for _, node := range nodes {
		if len(nodes) > 1 {
			dst = path.Join(c.Files.Destination, strconv.Itoa(node)) + "/"
			os.MkdirAll(dst, 0755)
		}
		err = b.Download(c.ClusterName, node, c.Files.Source, dst, c.Aws.Verbose)
		if err != nil {
			log.Printf("ERROR SRC=%s:%d MSG=%s", c.ClusterName, node, err)
		}
	}
	log.Print("Done")
	return nil
}
