package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
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
			err = c.fixIt(i, nip, clusterIps)
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
			go c.fixItParallel(node, nip, clusterIps, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}
	}
	log.Print("Done")
	return nil
}

func (c *confFixMeshCmd) fixItParallel(i int, nip map[int]string, clusterIps []string, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.fixIt(i, nip, clusterIps)
	if err != nil {
		log.Printf("ERROR from node %d: %s", i, err)
		hasError <- true
	}
}

func (c *confFixMeshCmd) fixIt(i int, nip map[int]string, clusterIps []string) error {
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
	newconf, err := fixAerospikeConfig(string(nr[0]), "", "mesh", clusterIps)
	if err != nil {
		return err
	}
	newconf, err = c.fixAccessAddress(newconf, nip[i])
	if err != nil {
		log.Printf("WARNING: Could not fix access-address: %s", err)
	}
	files = append(files, fileList{"/etc/aerospike/aerospike.conf", newconf, len(newconf)})
	if len(files) > 0 {
		err := b.CopyFilesToCluster(string(c.ClusterName), files, []int{i})
		if err != nil {
			return fmt.Errorf("cluster=%s node=%v nodeIP=%v CopyFilesToCluster error=%s", string(c.ClusterName), i, nip[i], err)
		}
	}
	return nil
}

func (c *confFixMeshCmd) fixAccessAddress(old string, newIp string) (new string, err error) {
	conf, err := aeroconf.Parse(strings.NewReader(old))
	if err != nil {
		return old, err
	}
	s := conf.Stanza("network")
	if s == nil {
		return old, errors.New("network stanza not found")
	}
	s = s.Stanza("service")
	if s == nil {
		return old, errors.New("network.service stanza not found")
	}
	if s.Type("access-address") == aeroconf.ValueString {
		vals, err := s.GetValues("access-address")
		if err != nil {
			return old, err
		}
		for i, val := range vals {
			if val == nil || strings.HasPrefix(*val, "127.") {
				continue
			}
			valIP := net.ParseIP(*val)
			if valIP.IsPrivate() {
				vals[i] = &newIp
			}
		}
	}
	if s.Type("alternate-access-address") == aeroconf.ValueString {
		vals, err := s.GetValues("alternate-access-address")
		if err != nil {
			return old, err
		}
		for i, val := range vals {
			if val == nil || strings.HasPrefix(*val, "127.") {
				continue
			}
			valIP := net.ParseIP(*val)
			if valIP.IsPrivate() {
				vals[i] = &newIp
			}
		}
	}
	var buf bytes.Buffer
	err = conf.Write(&buf, "", "    ", true)
	if err != nil {
		return old, err
	}
	return buf.String(), nil
}
