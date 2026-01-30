package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type ClientStartCmd struct {
	ClientName      TypeClientName `short:"n" long:"group-name" description:"Client names, comma separated OR 'all' to affect all clients" default:"client"`
	Machines        TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	ParallelThreads int            `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help            HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ClientStopCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client names, comma separated OR 'all' to affect all clients" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	Help       HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ClientDestroyCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client names, comma separated OR 'all' to affect all clients" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	Parallel   bool           `short:"p" long:"parallel" description:"If destroying many clients at once, set this to destroy in parallel"`
	Force      bool           `short:"f" long:"force" description:"Force destroy without confirmation" webdisable:"true" webset:"true"`
	Help       HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientStartCmd) Execute(args []string) error {
	cmd := []string{"client", "start"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.startClients(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientStartCmd) startClients(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Get client instances
	clients, err := c.getClientInstances(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("no client instances found")
	}

	// For docker backend, set vm.max_map_count for elasticsearch
	if system.Opts.Config.Backend.Type == string(backends.BackendTypeDocker) {
		out, err := exec.Command("docker", "run", "--rm", "-i", "--privileged", "ubuntu:22.04", "sysctl", "-w", "vm.max_map_count=262144").CombinedOutput()
		if err != nil {
			logger.Warn("Workaround `sysctl -w vm.max_map_count=262144` for docker failed, elasticsearch clients might fail to start: %s: %s", err, string(out))
		}
	}

	// Start instances
	logger.Info("Starting %d client instances", len(clients))
	err = clients.Start(10 * time.Minute)
	if err != nil {
		return fmt.Errorf("failed to start instances: %w", err)
	}

	// Run startup scripts on each instance
	scriptErr := false
	parallelize.ForEachLimit(clients, c.ParallelThreads, func(inst *backends.Instance) {
		// Upload autoloader script
		autoloader := "[ ! -d /opt/autoload ] && exit 0; RET=0; for f in $(ls /opt/autoload |sort -n); do /bin/bash /opt/autoload/${f}; CRET=$?; if [ ${CRET} -ne 0 ]; then RET=${CRET}; fi; done; exit ${RET}"

		conf, err := inst.GetSftpConfig("root")
		if err != nil {
			logger.Error("Failed to get SFTP config for %s:%d: %s", inst.ClusterName, inst.NodeNo, err)
			return
		}

		client, err := sshexec.NewSftp(conf)
		if err != nil {
			logger.Error("Failed to create SFTP client for %s:%d: %s", inst.ClusterName, inst.NodeNo, err)
			return
		}

		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/usr/local/bin/autoloader.sh",
			Source:      strings.NewReader(autoloader),
			Permissions: 0755,
		})
		client.Close()
		if err != nil {
			logger.Warn("Could not upload /usr/local/bin/autoloader.sh to %s:%d, will not start scripts from /opt/autoload: %s", inst.ClusterName, inst.NodeNo, err)
		}

		// Run autoloader script
		output := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"/bin/bash", "/usr/local/bin/autoloader.sh"},
				SessionTimeout: 5 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			scriptErr = true
			logger.Error("Autoloader script returned error on %s:%d: %s (stdout: %s)", inst.ClusterName, inst.NodeNo, output.Output.Err, string(output.Output.Stdout))
		}

		// Run custom startup script
		output = inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"/bin/bash", "/usr/local/bin/start.sh"},
				SessionTimeout: 5 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			scriptErr = true
			logger.Error("Custom startup script returned error on %s:%d: %s (stdout: %s)", inst.ClusterName, inst.NodeNo, output.Output.Err, string(output.Output.Stdout))
		}
	})

	if scriptErr {
		return errors.New("some startup scripts returned errors")
	}

	return nil
}

func (c *ClientStopCmd) Execute(args []string) error {
	cmd := []string{"client", "stop"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.stopClients(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientStopCmd) stopClients(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Get client instances
	clients, err := c.getClientInstances(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("no client instances found")
	}

	// Stop instances
	logger.Info("Stopping %d client instances", len(clients))
	err = clients.Stop(false, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to stop instances: %w", err)
	}

	return nil
}

func (c *ClientDestroyCmd) Execute(args []string) error {
	cmd := []string{"client", "destroy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.destroyClients(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientDestroyCmd) destroyClients(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Get client instances
	clients, err := c.getClientInstances(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("no client instances found")
	}

	// Get list of unique cluster names
	clusterNames := make(map[string]bool)
	for _, client := range clients {
		clusterNames[client.ClusterName] = true
	}
	clusterList := []string{}
	for name := range clusterNames {
		clusterList = append(clusterList, name)
	}

	// Confirm destruction unless forced
	if !c.Force && IsInteractive() {
		for {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("Are you sure you want to destroy clients [%s] (y/n)? ", strings.Join(clusterList, ", "))

			yesno, err := reader.ReadString('\n')
			if err != nil {
				return err
			}

			yesno = strings.ToLower(strings.TrimSpace(yesno))

			if yesno == "y" || yesno == "yes" {
				break
			} else if yesno == "n" || yesno == "no" {
				logger.Info("Aborting")
				return nil
			}
		}
	}

	// For docker backend, stop instances first
	if system.Opts.Config.Backend.Type == string(backends.BackendTypeDocker) {
		logger.Info("Stopping instances (docker backend)")
		err = clients.Stop(false, 10*time.Minute)
		if err != nil {
			logger.Warn("Failed to stop some instances: %s", err)
		}
	}

	// Destroy instances
	logger.Info("Destroying %d client instances", len(clients))

	if c.Parallel {
		// Parallel destruction
		var hasErr error
		var mu sync.Mutex
		var wg sync.WaitGroup
		maxConcurrent := 15
		sem := make(chan struct{}, maxConcurrent)

		for _, client := range clients {
			wg.Add(1)
			sem <- struct{}{}
			go func(client *backends.Instance) {
				defer wg.Done()
				defer func() { <-sem }()

				err := backends.InstanceList{client}.Terminate(10 * time.Minute)
				if err != nil {
					mu.Lock()
					hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", client.ClusterName, client.NodeNo, err))
					mu.Unlock()
				}
			}(client)
		}
		wg.Wait()

		if hasErr != nil {
			return hasErr
		}
	} else {
		// Sequential destruction
		err = clients.Terminate(10 * time.Minute)
		if err != nil {
			return fmt.Errorf("failed to destroy instances: %w", err)
		}
	}

	return nil
}

// Helper function to get client instances based on filter criteria
func (c *ClientStartCmd) getClientInstances(inventory *backends.Inventory, clientName string, machines string) (backends.InstanceList, error) {
	return getClientInstancesHelper(inventory, clientName, machines)
}

func (c *ClientStopCmd) getClientInstances(inventory *backends.Inventory, clientName string, machines string) (backends.InstanceList, error) {
	return getClientInstancesHelper(inventory, clientName, machines)
}

func (c *ClientDestroyCmd) getClientInstances(inventory *backends.Inventory, clientName string, machines string) (backends.InstanceList, error) {
	return getClientInstancesHelper(inventory, clientName, machines)
}

func getClientInstancesHelper(inventory *backends.Inventory, clientName string, machines string) (backends.InstanceList, error) {
	// Get all client instances
	clients := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating)

	// Filter by client name
	if clientName != "all" && clientName != "ALL" {
		names := strings.Split(clientName, ",")
		var filtered backends.InstanceList
		for _, name := range names {
			filtered = append(filtered, clients.WithClusterName(name).Describe()...)
		}
		clients = filtered
	}

	// Filter by machine numbers
	if machines != "" && machines != "all" && machines != "ALL" {
		machineNums := []int{}
		for _, nodeString := range strings.Split(machines, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return nil, fmt.Errorf("invalid machine number: %s", nodeString)
			}
			machineNums = append(machineNums, nodeInt)
		}
		clients = clients.WithNodeNo(machineNums...)
	}

	return clients.Describe(), nil
}
