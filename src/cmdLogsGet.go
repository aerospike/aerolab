package main

import (
	"archive/zip"
	"bufio"
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

type logsGetCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Client      bool            `short:"c" long:"client" description:"Set to indicate that this is a client group, not an aerospike cluster"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Journal     bool            `short:"j" long:"journal" description:"Attempt to get logs from journald instead of log files"`
	LogLocation string          `short:"p" long:"path" description:"Aerospike log file path" default:"/var/log/aerospike.log"`
	Destination flags.Filename  `short:"d" long:"destination" description:"Destination directory (will be created if doesn't exist)" default:"./logs/" webtype:"download"`
	Force       bool            `short:"f" long:"force" description:"set to not be asked whether to override existing files" webdisable:"true" webset:"true"`
	parallelThreadsCmd
	Tail []string       `description:"Optionally, specify the command to execute to get the logs instead of log files/journalctl"`
	Help logsGetCmdHelp `command:"help" subcommands-optional:"true" description:"Print help"`
}

type logsGetCmdHelp struct{}

func (c *logsGetCmdHelp) Execute(args []string) error {
	return printHelp("In order to specify a non-journal custom command to execute for log gathering, provide inline tail, for example: aerolab logs get -n myclient -- docker logs aerospike-graph\n\n")
}

func (c *logsGetCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if len(c.Tail) == 0 {
		c.Tail = args
	}
	if c.ParallelThreads < 1 {
		return errors.New("thread count must be 1+")
	}
	log.Print("Running logs.get")
	nnn := "cluster"
	if c.Client {
		b.WorkOnClients()
		nnn = "client group"
	}
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf(nnn+" does not exist: %s", string(c.ClusterName))
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
				return fmt.Errorf("node %d does not exist in "+nnn, nodeInt)
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) == 0 {
		err = errors.New("found 0 nodes in " + nnn)
		return err
	}

	if c.Destination != "-" {
		if _, err := os.Stat(string(c.Destination)); err != nil {
			err = os.MkdirAll(string(c.Destination), 0755)
			if err != nil {
				return err
			}
		} else if !c.Force {
			entries, _ := os.ReadDir(string(c.Destination))
			_, logf := path.Split(c.LogLocation)
			ask := false
			for _, ee := range entries {
				if strings.HasPrefix(ee.Name(), string(c.ClusterName)+"-") && strings.HasSuffix(ee.Name(), "."+strings.TrimLeft(logf, ".")) {
					ask = true
					break
				}
			}
			if ask {
				for {
					reader := bufio.NewReader(os.Stdin)
					fmt.Print("Directory exists and existing files will be overwritten, continue download (y/n)? ")

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
		}

		c.Destination = flags.Filename(path.Join(string(c.Destination), string(c.ClusterName)))
	}

	if c.ParallelThreads == 1 || c.Destination == "-" {
		var w *zip.Writer
		if c.Destination == "-" {
			w = zip.NewWriter(os.Stdout)
			defer w.Close()
		}
		for _, node := range nodes {
			err = c.get(node, w)
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
			go c.getParallel(node, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}
	}
	log.Print("Done")
	return nil
}

func (c *logsGetCmd) getParallel(node int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.get(node, nil)
	if err != nil {
		log.Printf("ERROR getting logs from node %d: %s", node, err)
		hasError <- true
	}
}

func (c *logsGetCmd) get(node int, w *zip.Writer) error {
	if w == nil {
		return c.getLocal(node)
	}

	fn := strconv.Itoa(node)
	if len(c.Tail) > 0 {
		fn = fn + "." + c.Tail[0] + ".log"
	} else if c.Journal {
		fn = fn + ".journald.log"
	} else {
		_, logf := path.Split(c.LogLocation)
		fn = fn + "." + strings.TrimLeft(logf, ".")
	}

	f, err := w.Create(fn)
	if err != nil {
		return err
	}
	if c.Journal || len(c.Tail) > 0 {
		command := []string{"journalctl", "-u", "aerospike", "--no-pager"}
		if len(c.Tail) > 0 {
			command = c.Tail
		}
		err := b.RunCustomOut(string(c.ClusterName), node, command, os.Stdin, f, f, false, nil)
		if err != nil {
			return fmt.Errorf("journalctl error: %s", err)
		}
		return nil
	}

	command := []string{"cat", c.LogLocation}
	err = b.RunCustomOut(string(c.ClusterName), node, command, os.Stdin, f, f, false, nil)
	if err != nil {
		return fmt.Errorf("log cat error: %s", err)
	}
	return nil
}

func (c *logsGetCmd) getLocal(node int) error {
	fn := string(c.Destination) + "-" + strconv.Itoa(node)
	if len(c.Tail) > 0 {
		fn = fn + "." + c.Tail[0] + ".log"
	} else if c.Journal {
		fn = fn + ".journald.log"
	} else {
		_, logf := path.Split(c.LogLocation)
		fn = fn + "." + strings.TrimLeft(logf, ".")
	}
	f, err := os.OpenFile(fn, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if c.Journal || len(c.Tail) > 0 {
		command := []string{"journalctl", "-u", "aerospike", "--no-pager"}
		if len(c.Tail) > 0 {
			command = c.Tail
		}
		err = b.RunCustomOut(string(c.ClusterName), node, command, os.Stdin, f, f, false, nil)
		if err != nil {
			return fmt.Errorf("journalctl error: %s", err)
		}
		return nil
	}

	command := []string{"cat", c.LogLocation}
	err = b.RunCustomOut(string(c.ClusterName), node, command, os.Stdin, f, f, false, nil)
	if err != nil {
		return fmt.Errorf("log cat error: %s", err)
	}
	return nil
}
