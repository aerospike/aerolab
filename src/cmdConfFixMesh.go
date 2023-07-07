package main

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
)

type confFixMeshCmd struct {
	aerospikeStartCmd
}

func (c *confFixMeshCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running conf.fixMesh")

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
	clusterIps, err := b.GetClusterNodeIps(string(c.ClusterName))
	if err != nil {
		return err
	}
	nodeList, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}

	nip, err := b.GetNodeIpMap(string(c.ClusterName), false)
	if err != nil {
		return err
	}
	// fix config if needed, read custom config file path if needed
	if c.ParallelThreads == 1 || len(nodeList) == 1 {
		for _, i := range nodeList {
			err = c.fixIt(i, nip, clusterIps, nodeList)
			if err != nil {
				return err
			}
		}
	} else {
		parallel := make(chan int, c.ParallelThreads)
		hasError := make(chan bool, len(nodeList))
		wait := new(sync.WaitGroup)
		for _, node := range nodeList {
			parallel <- 1
			wait.Add(1)
			go c.fixItParallel(node, nip, clusterIps, nodeList, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}
	}
	log.Print("Done")
	return nil
}

func (c *confFixMeshCmd) fixItParallel(i int, nip map[int]string, clusterIps []string, nodeList []int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.fixIt(i, nip, clusterIps, nodeList)
	if err != nil {
		log.Printf("ERROR from node %d: %s", i, err)
		hasError <- true
	}
}

func (c *confFixMeshCmd) fixIt(i int, nip map[int]string, clusterIps []string, nodeList []int) error {
	if _, ok := nip[i]; !ok {
		return nil
	}
	if nip[i] == "" {
		return nil
	}
	if nip[i] == "N/A" {
		return nil
	}
	files := []fileList{}
	var r [][]string
	r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
	nr, err := b.RunCommands(string(c.ClusterName), r, []int{i})
	if err != nil {
		return fmt.Errorf("cluster=%s node=%v nodeIP=%v RunCommands error=%s", string(c.ClusterName), i, nip[i], err)
	}
	// nr has contents of aerospike.conf
	newconf, err := fixAerospikeConfig(string(nr[0]), "", "mesh", clusterIps, nodeList)
	if err != nil {
		return err
	}
	files = append(files, fileList{"/etc/aerospike/aerospike.conf", strings.NewReader(newconf), len(newconf)})
	if len(files) > 0 {
		err := b.CopyFilesToCluster(string(c.ClusterName), files, []int{i})
		if err != nil {
			return fmt.Errorf("cluster=%s node=%v nodeIP=%v CopyFilesToCluster error=%s", string(c.ClusterName), i, nip[i], err)
		}
	}
	return nil
}
