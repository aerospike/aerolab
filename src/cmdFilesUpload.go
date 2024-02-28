package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
)

type filesUploadCmd struct {
	filesDownloadCmd
}

func init() {
	addBackendSwitch("files.upload", "aws", &a.opts.Files.Upload.Aws)
	addBackendSwitch("files.upload", "gcp", &a.opts.Files.Upload.Gcp)
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
		return logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		return logFatal("Could not init backend: %s", err)
	}
	return c.runUpload(args)
}

func (c *filesUploadCmd) runUpload(args []string) error {
	_ = args
	log.Print("Running files.upload")
	if c.IsClient {
		b.WorkOnClients()
	}
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", string(c.ClusterName))
		return err
	}

	var nodes []int
	err = c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	nodesList, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}
	if c.Nodes == "" {
		nodes = nodesList
	} else {
		for _, nodeString := range strings.Split(c.Nodes.String(), ",") {
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

	verbose := c.Aws.Verbose
	legacy := c.Aws.Legacy
	if a.opts.Config.Backend.Type == "gcp" {
		verbose = c.Gcp.Verbose
		legacy = c.Gcp.Legacy
	}

	if c.ParallelThreads == 1 || len(nodes) == 1 {
		for _, node := range nodes {
			err = c.put(node, verbose, legacy)
			if err != nil {
				return err
			}
		}
	} else {
		parallel := make(chan int, c.ParallelThreads)
		hasError := make(chan bool, len(nodes))
		wait := new(sync.WaitGroup)
		for _, node := range nodes {
			parallel <- 1
			wait.Add(1)
			go c.putParallel(node, verbose, legacy, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to upload files to %d nodes", len(hasError))
		}
	}

	log.Print("Done")
	return nil
}

func (c *filesUploadCmd) put(node int, verbose bool, legacy bool) error {
	err := b.Upload(string(c.ClusterName), node, string(c.Files.Source), string(c.Files.Destination), verbose, legacy)
	if err != nil {
		if !c.doLegacy {
			log.Printf("ERROR SRC=%s:%d MSG=%s", string(c.ClusterName), node, err)
		} else {
			log.Printf("ERROR SRC=%s:%d MSG=%s ACTION=switching legacy mode to %t and retrying", string(c.ClusterName), node, err, !legacy)
			err = b.Upload(string(c.ClusterName), node, string(c.Files.Source), string(c.Files.Destination), verbose, !legacy)
			if err != nil {
				log.Printf("ERROR SRC=%s:%d MSG=%s ACTION=giving up", string(c.ClusterName), node, err)
			}
		}
	}
	return nil
}

func (c *filesUploadCmd) putParallel(node int, verbose bool, legacy bool, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.put(node, verbose, legacy)
	if err != nil {
		log.Printf("ERROR getting logs from node %d: %s", node, err)
		hasError <- true
	}
}
