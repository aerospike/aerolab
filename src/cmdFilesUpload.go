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
	if earlyProcessV2(args, false) {
		return nil
	}
	if c.Files.Source == "help" && c.Files.Destination == "" {
		return printHelp("")
	}
	if c.Files.Destination == "" {
		return printHelp("")
	}
	if b == nil {
		logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		logFatal("Could not init backend: %s", err)
	}
	log.Print("Running files.upload")
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", string(c.ClusterName))
		return err
	}

	var nodes []int
	nodesList, err := b.NodeListInCluster(string(c.ClusterName))
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
		err = b.Upload(string(c.ClusterName), node, string(c.Files.Source), string(c.Files.Destination), c.Aws.Verbose)
		if err != nil {
			log.Printf("ERROR SRC=%s:%d MSG=%s", string(c.ClusterName), node, err)
		}
	}
	log.Print("Done")
	return nil
}