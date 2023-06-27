package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
)

type clientStartCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client names, comma separated OR 'all' to affect all clusters" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	Help       helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
	clientStartStopDestroyCmd
}

type clientStopCmd struct {
	clientStartCmd
}

type clientDestroyCmd struct {
	clientStartCmd
	Parallel bool `short:"p" long:"parallel" description:"if destroying many clients at once, set this to destroy in parallel"`
	Force    bool `short:"f" long:"force" description:"force stop before destroy"`
}

func (c *clientStartCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.runStart(args)
}

func (c *clientStartCmd) runStart(args []string) error {
	log.Println("Running client.start")
	b.WorkOnClients()
	err := c.Machines.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClientName), c.Machines.String())
	if err != nil {
		return err
	}
	if a.opts.Config.Backend.Type == "docker" {
		out, err := exec.Command("docker", "run", "--rm", "-i", "--privileged", "ubuntu:22.04", "sysctl", "-w", "vm.max_map_count=262144").CombinedOutput()
		if err != nil {
			fmt.Println("Workaround `sysctl -w vm.max_map_count=262144` for docker failed, elasticsearch clients might fail to start...")
			fmt.Println(err)
			fmt.Println(string(out))
		}
	}
	var nerr error
	scriptErr := false
	for _, ClusterName := range cList {
		err = b.ClusterStart(ClusterName, nodes[ClusterName])
		if err != nil {
			if nerr == nil {
				nerr = err
			} else {
				nerr = errors.New(nerr.Error() + "\n" + err.Error())
			}
		}
		if err == nil {
			// generic startup scripts
			autoloader := "[ ! -d /opt/autoload ] && exit 0; RET=0; for f in $(ls /opt/autoload |sort -n); do /bin/bash /opt/autoload/${f}; CRET=$?; if [ ${CRET} -ne 0 ]; then RET=${CRET}; fi; done; exit ${RET}"
			err = b.CopyFilesToCluster(ClusterName, []fileList{{"/usr/local/bin/autoloader.sh", strings.NewReader(autoloader), len(autoloader)}}, nodes[ClusterName])
			if err != nil {
				log.Printf("Could not upload /usr/local/bin/autoloader.sh, will not start scripts from /opt/autoload: %s", err)
			}
			out, err := b.RunCommands(ClusterName, [][]string{{"/bin/bash", "/usr/local/bin/autoloader.sh"}}, nodes[ClusterName])
			if err != nil {
				scriptErr = true
				prt := ""
				for i, o := range out {
					prt = prt + "\n ---- " + strconv.Itoa(i) + " ----\n" + string(o)
				}
				log.Printf("Some startup sripts returned an error (%s). Outputs:%s", err, prt)
			}
			// custom startup script
			out, err = b.RunCommands(ClusterName, [][]string{{"/bin/bash", "/usr/local/bin/start.sh"}}, nodes[ClusterName])
			if err != nil {
				scriptErr = true
				prt := ""
				for i, o := range out {
					prt = prt + "\n ---- " + strconv.Itoa(i) + " ----\n" + string(o)
				}
				log.Printf("Some custom startup sripts returned an error (%s). Outputs:%s", err, prt)
			}
		}
	}
	if nerr != nil {
		return nerr
	}
	log.Println("Done")
	if scriptErr {
		return errors.New("SOME SCRIPTS RETURNED ERRORS")
	}
	return nil
}

func (c *clientDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running client.destroy")
	b.WorkOnClients()
	err := c.Machines.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClientName), c.Machines.String())
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
	if !c.Force {
		for {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("Are you sure you want to destroy clients [%s] (y/n)? ", strings.Join(cList, ", "))

			yesno, err := reader.ReadString('\n')
			if err != nil {
				log.Fatal(err)
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

func (c *clientStopCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.runStop(args)
}

func (c *clientStopCmd) runStop(args []string) error {
	b.WorkOnClients()
	log.Println("Running client.stop")
	err := c.Machines.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClientName), c.Machines.String())
	if err != nil {
		return err
	}
	var nerr error
	for _, ClusterName := range cList {
		err = b.ClusterStop(ClusterName, nodes[ClusterName])
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
	log.Println("Done")
	return nil
}

type clientStartStopDestroyCmd struct {
}

func (c *clientStartStopDestroyCmd) getBasicData(clusterName string, Nodes string) (cList []string, nodes map[string][]int, err error) {
	// check cluster exists
	clusterList, err := b.ClusterList()
	if err != nil {
		return nil, nil, err
	}
	if clusterName != "all" && clusterName != "ALL" {
		cList = strings.Split(clusterName, ",")
	} else {
		cList = clusterList
	}
	for _, clusterName = range cList {
		if !inslice.HasString(clusterList, clusterName) {
			err = fmt.Errorf("client group does not exist: %s", clusterName)
			return nil, nil, err
		}
	}
	nodes = make(map[string][]int)
	var nodesC []int
	if Nodes == "" || Nodes == "all" || Nodes == "ALL" {
		for _, clusterName = range cList {
			nodesC, err = b.NodeListInCluster(clusterName)
			if err != nil {
				return nil, nil, err
			}
			nodes[clusterName] = nodesC
		}
	} else {
		for _, nodeString := range strings.Split(Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return nil, nil, err
			}
			nodesC = append(nodesC, nodeInt)
		}
		for _, clusterName = range cList {
			nodes[clusterName] = nodesC
		}
	}
	for _, clusterName = range cList {
		if len(nodes[clusterName]) == 0 {
			err = errors.New("found 0 machines in client group")
			return nil, nil, err
		}
	}
	return
}
