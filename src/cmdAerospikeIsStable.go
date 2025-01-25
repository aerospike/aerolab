package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/parallelize"
)

type NodeRange []int

func (n *NodeRange) UnmarshalFlag(value string) error {
	if value == "" {
		return nil
	}
	for _, v := range strings.Split(value, ",") {
		if strings.Contains(v, "-") {
			ab := strings.Split(v, "-")
			if len(ab) != 2 {
				return errors.New("invalid range format")
			}
			a, err := strconv.Atoi(ab[0])
			if err != nil {
				return err
			}
			b, err := strconv.Atoi(ab[1])
			if err != nil {
				return err
			}
			start := a
			end := b
			if start > end {
				start = b
				end = a
			}
			for i := start; i <= end; i++ {
				*n = append(*n, i)
			}
			continue
		}
		x, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*n = append(*n, x)
	}
	return nil
}

type aerospikeIsStableCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Nodes            NodeRange       `short:"l" long:"nodes" description:"Only consider the given nodes, e.g. --nodes=1-4,7,8"`
	Namespace        string          `short:"m" long:"namespace" description:"Namespace to change" default:"test"`
	Wait             bool            `short:"w" long:"wait" description:"If set, will wait in a loop until the cluster is stable, and then return"`
	WaitTimeout      int             `short:"o" long:"wait-timeout" description:"If set, will timeout if the cluster doesn't become stable by this many seconds"`
	IgnoreMigrations bool            `short:"i" long:"ignore-migrations" description:"If set, will ignore migrations when checking if cluster is stable"`
	IgnoreClusterKey bool            `short:"k" long:"ignore-cluster-key" description:"If set, will not check if the cluster key matches on all nodes in the cluster"`
	Verbose          bool            `short:"v" long:"verbose" description:"Enable verbose logging"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *aerospikeIsStableCmd) Execute(args []string) error {
	if earlyProcessV2(nil, true) {
		return nil
	}
	if c.WaitTimeout != 0 {
		c.Wait = true
	}
	startTime := time.Now()
	log.Println("Running aerospike.is-stable")
	// get node count
	log.Println("aerospike.is-stable: Getting cluster size")
	nodes, err := b.NodeListInCluster(c.ClusterName.String())
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return errors.New("cluster does not exists")
	}
	for _, node := range c.Nodes {
		if !slices.Contains(nodes, node) {
			return fmt.Errorf("selected node %d not found", node)
		}
	}
	if len(c.Nodes) > 0 {
		nodes = c.Nodes
	}
	// scripts
	waitScript := fmt.Sprintf(`debug=%t
	timeout=%d
	start_time=$(date +%%s)
	while (( timeout == 0 || $(date +%%s) - start_time < timeout )); do
		RET=$(asinfo -v 'cluster-stable:size=%d;ignore-migrations=%t;namespace=%s' 2>&1)
		if [ $? -eq 0 ]; then
			echo "AEROLAB-SUCCESS-CLUSTER-KEY:${RET}"
			exit 0
		fi
		[ "${debug}" == "true" ] && echo "${RET}"
		sleep 1
	done
	echo ${RET}
	exit 1
	`, c.Verbose, c.WaitTimeout, len(nodes), c.IgnoreMigrations, c.Namespace)
	noWaitCmd := []string{"asinfo", "-v", fmt.Sprintf("cluster-stable:size=%d;ignore-migrations=%t;namespace=%s", len(nodes), c.IgnoreMigrations, c.Namespace)}

	firstLoop := true
	keysLock := new(sync.Mutex)
	for c.WaitTimeout == 0 || time.Since(startTime) < time.Duration(c.WaitTimeout)*time.Second {
		if !firstLoop {
			log.Println("aerospike.is-stable: Cluster Key Mismatch, Getting cluster keys")
		} else {
			log.Println("aerospike.is-stable: Getting cluster keys")
		}
		clusterKeys := []string{} // lets reset
		// get all cluster keys
		returns := parallelize.MapLimit(nodes, c.ParallelThreads, func(node int) error {
			var cmd []string
			if c.Wait {
				if firstLoop {
					// upload wait script
					err = b.CopyFilesToCluster(string(c.ClusterName), []fileList{{filePath: "/opt/is-stable.sh", fileContents: waitScript, fileSize: len(waitScript)}}, []int{node})
					if err != nil {
						return err
					}
				}
				cmd = []string{"/bin/bash", "/opt/is-stable.sh"}
			} else {
				cmd = noWaitCmd
			}
			// run cmd; if error, ret the error;; if success, capture cluster key in clusterKeys (lock with keysLock)
			r, w, err := os.Pipe()
			if err != nil {
				return err
			}
			go func() {
				reader := bufio.NewReader(r)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						if err == io.EOF {
							break
						}
						fmt.Printf("aerospike.is-stable: Error reading from pipe: %v\n", err)
						break
					}
					if c.Verbose {
						log.Printf("aerospike.is-stable verbose: node:%d %s", node, strings.TrimPrefix(strings.TrimRight(line, "\r\n"), "AEROLAB-SUCCESS-CLUSTER-KEY:"))
					}
					if strings.HasPrefix(line, "AEROLAB-SUCCESS-CLUSTER-KEY:") {
						keysLock.Lock()
						clusterKeys = append(clusterKeys, strings.TrimRight(strings.Split(line, "-SUCCESS-CLUSTER-KEY:")[1], "\r\n "))
						keysLock.Unlock()
					}
					if !c.Wait {
						clusterKeys = append(clusterKeys, strings.TrimRight(line, "\r\n "))
					}
				}
				r.Close()
			}()
			err = b.RunCustomOut(c.ClusterName.String(), node, cmd, nil, w, w, false, nil)
			w.Close()
			if err != nil {
				return fmt.Errorf("node:%d %s", node, err)
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %d returned %s", nodes[i], ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
		firstLoop = false

		same := true

		if len(nodes) != len(clusterKeys) {
			same = false
		} else {
			if !c.IgnoreClusterKey {
				for _, k := range clusterKeys {
					if clusterKeys[0] != k {
						same = false
						break
					}
				}
			}
		}

		if same {
			log.Print("aerospike.is-stable: Cluster Stable")
			return nil
		}

		if !c.Wait {
			return errors.New("cluster not stable")
		}
		time.Sleep(time.Second)
	}
	return errors.New("timeout reached, cluster unstable")
}
