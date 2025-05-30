package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type filesRestDownloadCmd struct {
	Source      string
	Destination flags.Filename `webtype:"download"`
}

type filesRestUploadCmd struct {
	Source      flags.Filename
	Destination string
}

type filesUploadDownloadCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Node number(s), comma-separated. Default=ALL" default:""`
	IsClient    bool            `short:"c" long:"client" description:"set this to run the command against client groups instead of clusters"`
	parallelThreadsCmd
	Aws      filesDownloadCmdAws `no-flag:"true"`
	Gcp      filesDownloadCmdAws `no-flag:"true"`
	doLegacy bool                // set to do legacy if non-legacy fails
}

type filesDownloadCmd struct {
	filesUploadDownloadCmd
	Files filesRestDownloadCmd `positional-args:"true"`
}

type filesDownloadCmdAws struct {
	Verbose bool `short:"v" long:"verbose" description:"do not run scp in quiet mode"`
	Legacy  bool `short:"o" long:"legacy" description:"enable legacy scp mode"`
}

func init() {
	addBackendSwitch("files.download", "aws", &a.opts.Files.Download.Aws)
	addBackendSwitch("files.download", "gcp", &a.opts.Files.Download.Gcp)
}

func (c *filesDownloadCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	if c.Files.Destination == "-" {
		return fmt.Errorf("downloading through a zip stream to stdout not currently supported, please use `logs get` with a custom file path instead")
	}
	if string(c.Files.Source) == "help" && string(c.Files.Destination) == "" {
		return printHelp("If more than one node is specified, files will be downloaded to {Destination}/{nodeNumber}/\n\n")
	}
	if string(c.Files.Destination) == "" {
		return printHelp("If more than one node is specified, files will be downloaded to {Destination}/{nodeNumber}/\n\n")
	}
	if b == nil {
		return logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		return logFatal("Could not init backend: %s", err)
	}
	log.Print("Running files.download")
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

	dst := string(c.Files.Destination)
	verbose := c.Aws.Verbose
	legacy := c.Aws.Legacy
	if a.opts.Config.Backend.Type == "gcp" {
		verbose = c.Gcp.Verbose
		legacy = c.Gcp.Legacy
	}

	if c.ParallelThreads == 1 || len(nodes) == 1 {
		for _, node := range nodes {
			if len(nodes) > 1 {
				dst = path.Join(string(c.Files.Destination), strconv.Itoa(node))
				os.MkdirAll(dst, 0755)
			}
			err = c.get(node, dst, verbose, legacy)
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
			dst = path.Join(string(c.Files.Destination), strconv.Itoa(node))
			os.MkdirAll(dst, 0755)
			go c.getParallel(node, dst, verbose, legacy, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get files from %d nodes", len(hasError))
		}
	}
	log.Print("Done")
	return nil
}

func (c *filesDownloadCmd) get(node int, dst string, verbose bool, legacy bool) error {
	err := b.Download(string(c.ClusterName), node, string(c.Files.Source), dst, verbose, legacy)
	if err != nil {
		if !c.doLegacy {
			log.Printf("ERROR SRC=%s:%d MSG=%s", string(c.ClusterName), node, err)
		} else {
			log.Printf("ERROR SRC=%s:%d MSG=%s ACTION=switching legacy mode to %t and retrying", string(c.ClusterName), node, err, !legacy)
			err = b.Download(string(c.ClusterName), node, string(c.Files.Source), dst, verbose, !legacy)
			if err != nil {
				log.Printf("ERROR SRC=%s:%d MSG=%s ACTION=giving up", string(c.ClusterName), node, err)
			}
		}
	}
	return nil
}

func (c *filesDownloadCmd) getParallel(node int, dst string, verbose bool, legacy bool, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.get(node, dst, verbose, legacy)
	if err != nil {
		log.Printf("ERROR getting logs from node %d: %s", node, err)
		hasError <- true
	}
}
