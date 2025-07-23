package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

type clusterDestroyCmd struct {
	clusterStopCmd
	Force    bool `short:"f" long:"force" description:"force stop before destroy" webdisable:"true" webset:"true"`
	Parallel bool `short:"p" long:"parallel" description:"if destroying many clusters at once, set this to destroy in parallel"`
	clusterStartStopDestroyCmd
}

func (c *clusterDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.doDestroy("cluster", args)
}

func (c *clusterDestroyCmd) doDestroy(typeName string, args []string) error {
	_ = args
	log.Println("Running " + typeName + ".destroy")
	err := c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClusterName), c.Nodes.String(), typeName)
	if err != nil {
		return err
	}
	nodeCount := 0
	for _, n := range nodes {
		nodeCount += len(n)
	}
	var nerr error
	nerrLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	mu := 15
	if !c.Parallel {
		mu = 1
	}
	maxUnits := make(chan int, mu)
	if !c.Force {
		for {
			reader := bufio.NewReader(os.Stdin)
			backendString := "Backend: docker"
			if a.opts.Config.Backend.Type == "aws" {
				backendString = fmt.Sprintf("Backend: AWS Region: %s", a.opts.Config.Backend.Region)
				if a.opts.Config.Backend.AWSProfile != "" {
					backendString = fmt.Sprintf("%s AWSProfile: %s", backendString, a.opts.Config.Backend.Region)
				}
			}
			if a.opts.Config.Backend.Type == "gcp" {
				backendString = fmt.Sprintf("Backend: GCP Project: %s", a.opts.Config.Backend.Project)
			}
			fmt.Println("\n" + backendString)
			fmt.Printf("Are you sure you want to destroy clusters [%s] (%d nodes) (y/n)? ", strings.Join(cList, ", "), nodeCount)

			yesno, err := reader.ReadString('\n')
			if err != nil {
				logExit(err)
			}

			yesno = strings.ToLower(strings.TrimSpace(yesno))

			if yesno == "y" || yesno == "yes" {
				break
			} else if yesno == "n" || yesno == "no" {
				fmt.Println("Aborting")
				return nil
			}
		}
	}
	for _, ClusterName := range cList {
		maxUnits <- 1
		wg.Add(1)
		go func(ClusterName string) {
			defer wg.Done()
			defer func() {
				<-maxUnits
			}()
			if a.opts.Config.Backend.Type == "docker" {
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
