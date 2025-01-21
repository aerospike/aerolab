package main

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aerospike/aerolab/parallelize"
)

type aerospikeIsStableCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Namespace        string          `short:"m" long:"namespace" description:"Namespace to change" default:"test"`
	Wait             bool            `short:"w" long:"wait" description:"If set, will wait in a loop until the cluster is stable, and then return"`
	WaitTimeout      int             `short:"o" long:"wait-timeout" description:"If set, will timeout if the cluster doesn't become stable by this many seconds"`
	IgnoreMigrations bool            `short:"i" long:"ignore-migrations" description:"If set, will ignore migrations when checking if cluster is stable"`
	IgnoreClusterKey bool            `short:"k" long:"ignore-cluster-key" description:"If set, will not check if the cluster key matches on all nodes in the cluster"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *aerospikeIsStableCmd) Execute(args []string) error {
	if earlyProcessV2(nil, true) {
		return nil
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
	// scripts
	waitScript := fmt.Sprintf(`timeout=%d
	start_time=$(date +%%s)
	while (( timeout == 0 || $(date +%%s) - start_time < timeout )); do
		RET=$(asinfo -v 'cluster-stable:size=%d;ignore-migrations=%t;namespace=%s' 2>&1)
		if [ $? -eq 0 ]; then
			echo ${RET}
			exit 0
		fi
		sleep 1
	done
	echo "${RET}"
	exit 1
	`, c.WaitTimeout, len(nodes), c.IgnoreMigrations, c.Namespace)
	noWaitCmd := []string{"asinfo", "-v", fmt.Sprintf("cluster-stable:size=%d;ignore-migrations=%t;namespace=%s", len(nodes), c.IgnoreMigrations, c.Namespace)}

	firstLoop := true
	keysLock := new(sync.Mutex)
	for c.WaitTimeout == 0 || time.Since(startTime) < time.Duration(c.WaitTimeout)*time.Second {
		log.Println("aerospike.is-stable: Getting cluster keys")
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
			out, err := b.RunCommands(c.ClusterName.String(), [][]string{cmd}, []int{node})
			if len(out) == 0 {
				out = [][]byte{{'-'}}
			}
			if err != nil {
				return fmt.Errorf("%s: %s", err, string(out[0]))
			}
			keysLock.Lock()
			clusterKeys = append(clusterKeys, string(out[0]))
			keysLock.Unlock()
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

		if !c.IgnoreClusterKey {
			for _, k := range clusterKeys {
				if clusterKeys[0] != k {
					same = false
					break
				}
			}
		}

		if same {
			log.Print("Cluster Stable")
			return nil
		}

		if !c.Wait {
			return errors.New("cluster not stable")
		}
		time.Sleep(time.Second)
	}
	return errors.New("timeout reached, cluster unstable")
}
