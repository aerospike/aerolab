package main

import (
	"errors"
	"fmt"
	"log"
	"sync"
)

type clusterStartCmd struct {
	clusterStopCmd
	NoFixMesh       bool `short:"f" long:"no-fix-mesh" description:"Set to avoid running conf-fix-mesh"`
	NoStart         bool `short:"s" long:"no-start" description:"Set to prevent Aerospike from starting on cluster-start"`
	ParallelThreads int  `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"50"`
	clusterStartStopDestroyCmd
}

func (c *clusterStartCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running cluster.start")
	err := c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClusterName), c.Nodes.String())
	if err != nil {
		return err
	}
	var nerr error
	for _, ClusterName := range cList {
		err = b.ClusterStart(ClusterName, nodes[ClusterName])
		if err != nil {
			if nerr == nil {
				nerr = err
			} else {
				nerr = errors.New(nerr.Error() + "\n" + err.Error())
			}
		}
	}
	if nerr != nil {
		return nerr
	}

	if !c.NoFixMesh {
		for _, ClusterName := range cList {
			a.opts.Conf.FixMesh.ClusterName = TypeClusterName(ClusterName)
			a.opts.Conf.FixMesh.Nodes = TypeNodes(intSliceToString(nodes[ClusterName], ","))
			a.opts.Conf.FixMesh.ParallelThreads = c.ParallelThreads
			e := a.opts.Conf.FixMesh.Execute(nil)
			if e != nil {
				return e
			}
		}
	}
	if !c.NoStart {
		for _, ClusterName := range cList {
			a.opts.Aerospike.Start.ClusterName = TypeClusterName(ClusterName)
			a.opts.Aerospike.Start.Nodes = TypeNodes(intSliceToString(nodes[ClusterName], ","))
			a.opts.Aerospike.Start.ParallelThreads = c.ParallelThreads
			e := a.opts.Aerospike.Start.Execute(nil)
			if e != nil {
				return e
			}
		}
	}

	if c.ParallelThreads == 1 {
		for _, ClusterName := range cList {
			err = c.finishStart(ClusterName, nodes[ClusterName])
			if err != nil {
				return err
			}
		}
	} else {
		parallel := make(chan int, c.ParallelThreads)
		hasError := make(chan bool, len(nodes))
		wait := new(sync.WaitGroup)
		for _, ClusterName := range cList {
			for _, node := range nodes[ClusterName] {
				parallel <- 1
				wait.Add(1)
				go c.finishStartParallel(ClusterName, node, parallel, wait, hasError)
			}
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}

	}
	log.Println("Done")
	return nil
}

func (c *clusterStartCmd) finishStartParallel(ClusterName string, node int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.finishStart(ClusterName, []int{node})
	if err != nil {
		log.Printf("ERROR from cluster %s node %d: %s", ClusterName, node, err)
		hasError <- true
	}
}

func (c *clusterStartCmd) finishStart(ClusterName string, nodes []int) error {
	autoloader := "[ ! -d /opt/autoload ] && exit 0; RET=0; for f in $(ls /opt/autoload |sort -n); do /bin/bash /opt/autoload/${f}; CRET=$?; if [ ${CRET} -ne 0 ]; then RET=${CRET}; fi; done; exit ${RET}"
	err := b.CopyFilesToCluster(ClusterName, []fileList{{"/usr/local/bin/autoloader.sh", autoloader, len(autoloader)}}, nodes)
	if err != nil {
		log.Printf("Could not upload /usr/local/bin/autoloader.sh, will not start scripts from /opt/autoload: %s", err)
	}
	out, err := b.RunCommands(ClusterName, [][]string{{"/bin/bash", "/usr/local/bin/autoloader.sh"}}, nodes)
	if err != nil {
		nout := ""
		for _, n := range out {
			nout = nout + "\n" + string(n)
		}
		return fmt.Errorf("could not run autoload scripts: %s: %s", err, nout)
	}
	return nil
}
