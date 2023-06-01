package main

import (
	"errors"
	"log"
	"sync"
)

type clusterDestroyCmd struct {
	clusterStopCmd
	Docker   clusterDestroyCmdDocker `no-flag:"true"`
	Parallel bool                    `short:"p" long:"parallel" description:"if destroying many clusters at once, set this to destroy in parallel"`
	clusterStartStopDestroyCmd
}

type clusterDestroyCmdDocker struct {
	Force bool `short:"f" long:"force" description:"force stop before destroy"`
}

func init() {
	addBackendSwitch("cluster.destroy", "docker", &a.opts.Cluster.Destroy.Docker)
}

func (c *clusterDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running cluster.destroy")
	err := c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClusterName), c.Nodes.String())
	if err != nil {
		return err
	}
	var nerr error
	nerrLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	mu := 15
	if !c.Parallel {
		mu = 1
	}
	maxUnits := make(chan int, mu)
	for _, ClusterName := range cList {
		maxUnits <- 1
		wg.Add(1)
		go func(ClusterName string) {
			defer wg.Done()
			defer func() {
				<-maxUnits
			}()
			if c.Docker.Force && a.opts.Config.Backend.Type == "docker" {
				b.ClusterStop(ClusterName, nodes[ClusterName])
			}
			err = b.ClusterDestroy(ClusterName, nodes[ClusterName])
			if err != nil {
				nerrLock.Lock()
				if nerr == nil {
					nerr = err
				} else {
					nerr = errors.New(nerr.Error() + "\n" + err.Error())
				}
				nerrLock.Unlock()
			}
		}(ClusterName)
	}
	wg.Wait()
	if nerr != nil {
		return nerr
	}
	log.Println("Done")
	return nil
}
