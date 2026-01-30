package cmd

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type LogsGetCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Journal     bool            `short:"j" long:"journal" description:"Attempt to get logs from journald instead of log files"`
	LogLocation string          `short:"p" long:"path" description:"Aerospike log file path" default:"/var/log/aerospike.log"`
	Destination string          `short:"d" long:"destination" description:"Destination directory (will be created if doesn't exist)" default:"./logs/" webtype:"download"`
	Force       bool            `short:"f" long:"force" description:"set to not be asked whether to override existing files" webdisable:"true" webset:"true"`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Tail        []string        `description:"Optionally, specify the command to execute to get the logs instead of log files/journalctl"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *LogsGetCmd) Execute(args []string) error {
	cmd := []string{"logs", "get"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.GetLogs(system, system.Backend.GetInventory(), system.Logger, args, "get")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Downloaded logs from %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *LogsGetCmd) GetLogs(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"logs", action}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClusterName.String() == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var instances backends.InstanceList
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.GetLogs(system, inventory, logger, args, action)
			if err != nil {
				return nil, err
			}
			instances = append(instances, inst...)
		}
		return instances, nil
	}
	cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if cluster == nil {
		return nil, fmt.Errorf("cluster %s not found", c.ClusterName.String())
	}
	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return nil, err
		}
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No running instances found for cluster %s", c.ClusterName.String())
		return nil, nil
	}

	// Clean up log location
	c.LogLocation = strings.Trim(c.LogLocation, "\r\n\t ")
	if len(c.Tail) == 0 {
		c.Tail = args
	}
	if c.Threads < 1 {
		return nil, errors.New("thread count must be 1+")
	}

	logger.Info("Getting logs from %d nodes", cluster.Count())

	// Create destination directory if it doesn't exist
	if c.Destination != "-" {
		if _, err := os.Stat(c.Destination); err != nil {
			err = os.MkdirAll(c.Destination, 0755)
			if err != nil {
				return nil, fmt.Errorf("failed to create destination directory: %w", err)
			}
		} else if !c.Force {
			// Check for existing files and ask for confirmation
			entries, _ := os.ReadDir(c.Destination)
			_, logf := path.Split(c.LogLocation)
			ask := false
			for _, ee := range entries {
				if strings.HasPrefix(ee.Name(), c.ClusterName.String()+"-") && strings.HasSuffix(ee.Name(), "."+strings.TrimLeft(logf, ".")) {
					ask = true
					break
				}
			}
			if ask && IsInteractive() {
				reader := bufio.NewReader(os.Stdin)
				fmt.Print("Directory exists and existing files will be overwritten, continue download (y/n)? ")

				yesno, err := reader.ReadString('\n')
				if err != nil {
					return nil, fmt.Errorf("failed to read input: %w", err)
				}

				yesno = strings.ToLower(strings.TrimSpace(yesno))

				if yesno != "y" && yesno != "yes" {
					fmt.Println("Aborting")
					return nil, nil
				}
			}

			c.Destination = path.Join(c.Destination, c.ClusterName.String())
		}
	}

	// Get logs from all instances
	if c.Threads == 1 || c.Destination == "-" {
		var w *zip.Writer
		if c.Destination == "-" {
			w = zip.NewWriter(os.Stdout)
			defer w.Close()
		}
		for _, instance := range cluster.Describe() {
			err := c.getLogFromInstance(instance, w, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to get logs from node %d: %w", instance.NodeNo, err)
			}
		}
	} else {
		// Parallel processing
		var hasError bool
		var mu sync.Mutex
		parallelize.ForEachLimit(cluster.Describe(), c.Threads, func(instance *backends.Instance) {
			err := c.getLogFromInstance(instance, nil, logger)
			if err != nil {
				mu.Lock()
				hasError = true
				mu.Unlock()
				logger.Error("Failed to get logs from node %d: %s", instance.NodeNo, err)
			}
		})
		if hasError {
			return nil, fmt.Errorf("failed to get logs from some nodes")
		}
	}

	return cluster.Describe(), nil
}

func (c *LogsGetCmd) getLogFromInstance(instance *backends.Instance, w *zip.Writer, logger *logger.Logger) error {
	if w == nil {
		return c.getLogLocal(instance, logger)
	}

	// Create filename
	fn := "node" + strconv.Itoa(instance.NodeNo)
	if len(c.Tail) > 0 {
		fn = fn + "." + c.Tail[0] + ".log"
	} else if c.Journal {
		fn = fn + ".journald.log"
	} else {
		_, logf := path.Split(c.LogLocation)
		fn = fn + "." + strings.TrimLeft(logf, ".")
	}

	// Create file in zip
	f, err := w.Create(fn)
	if err != nil {
		return fmt.Errorf("failed to create zip file entry: %w", err)
	}

	// Get log content
	content, err := c.getLogContent(instance, logger)
	if err != nil {
		return err
	}

	// Write content to zip file
	_, err = f.Write(content)
	if err != nil {
		return fmt.Errorf("failed to write to zip file: %w", err)
	}

	return nil
}

func (c *LogsGetCmd) getLogLocal(instance *backends.Instance, logger *logger.Logger) error {
	// Create filename
	fn := path.Join(c.Destination, instance.ClusterName+"-"+strconv.Itoa(instance.NodeNo))
	if len(c.Tail) > 0 {
		fn = fn + "." + c.Tail[0] + ".log"
	} else if c.Journal {
		fn = fn + ".journald.log"
	} else {
		_, logf := path.Split(c.LogLocation)
		fn = fn + "." + strings.TrimLeft(logf, ".")
	}

	// Ensure the directory exists
	dir := path.Dir(fn)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create local file
	file, err := os.OpenFile(fn, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Get log content
	content, err := c.getLogContent(instance, logger)
	if err != nil {
		return err
	}

	// Write content to local file
	_, err = file.Write(content)
	if err != nil {
		return fmt.Errorf("failed to write to local file: %w", err)
	}

	return nil
}

func (c *LogsGetCmd) getLogContent(instance *backends.Instance, logger *logger.Logger) ([]byte, error) {
	var command []string
	if c.Journal || len(c.Tail) > 0 {
		command = []string{"journalctl", "-u", "aerospike", "--no-pager"}
		if len(c.Tail) > 0 {
			command = c.Tail
		}
	} else {
		command = []string{"cat", c.LogLocation}
	}

	// Execute command and capture output
	var buf bytes.Buffer
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        command,
			Stdin:          nil,
			Stdout:         &buf,
			Stderr:         &buf,
			SessionTimeout: time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	// Check for errors
	if output.Output.Err != nil {
		return nil, fmt.Errorf("command failed: %s (%s) (%s)", output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	return buf.Bytes(), nil
}
