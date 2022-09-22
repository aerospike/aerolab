package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type filesUploadCmd struct {
	filesDownloadCmd
}

func init() {
	addBackendSwitch("files.upload", "aws", &a.opts.Files.Upload.Aws)
}

func (c *filesUploadCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.Files.Source == "help" && c.Files.Destination == "" {
		return printHelp("")
	}
	if c.Files.Destination == "" {
		return printHelp("")
	}
	log.Print("Running files.upload")
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

	for _, node := range nodes {
		err = b.Upload(c.ClusterName, node, c.Files.Source, c.Files.Destination, c.Aws.Verbose)
		if err != nil {
			log.Printf("ERROR SRC=%s:%d MSG=%s", c.ClusterName, node, err)
		}
	}
	log.Print("Done")
	return nil
}
