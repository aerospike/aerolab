package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

type ClientConfigureAMSCmd struct {
	ClientName      TypeClientName  `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines        TypeMachines    `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	ConnectClusters TypeClusterName `short:"s" long:"clusters" description:"Comma-separated list of clusters to configure as source for this AMS"`
	ConnectClients  TypeClientName  `short:"S" long:"clients" description:"Comma-separated list of (graph) clients to configure as source for this AMS"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientConfigureAMSCmd) Execute(args []string) error {
	cmd := []string{"client", "configure", "ams"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.configureAMS(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientConfigureAMSCmd) configureAMS(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "configure", "ams"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Check that at least one connection type is specified
	if c.ConnectClusters.String() == "" && c.ConnectClients.String() == "" {
		return fmt.Errorf("either --clusters or --clients must be specified")
	}

	// Get client instances (AMS instances to configure)
	clients, err := getClientInstancesHelper(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("no client instances found")
	}

	// Parse connections
	clusterNodes, err := c.parseConnections(inventory, c.ConnectClusters.String(), false)
	if err != nil {
		return err
	}
	clientNodes, err := c.parseConnections(inventory, c.ConnectClients.String(), true)
	if err != nil {
		return err
	}

	// Build target lists
	asdTargets := []string{}
	nodeTargets := []string{}
	clientTargets := []string{}

	for _, nodes := range clusterNodes {
		for _, node := range nodes {
			asdTargets = append(asdTargets, node+":9145")
			nodeTargets = append(nodeTargets, node+":9100")
		}
	}

	for _, nodes := range clientNodes {
		for _, node := range nodes {
			clientTargets = append(clientTargets, node+":9090")
		}
	}

	// Configure each AMS client
	for _, client := range clients {
		logger.Info("Configuring AMS on %s:%d", client.ClusterName, client.NodeNo)

		// Build sed commands
		commands := []string{}

		if len(asdTargets) > 0 {
			ips := "'" + strings.Join(asdTargets, "','") + "'"
			commands = append(commands, fmt.Sprintf("sed -i.bakAsd -E \"s/.*TODO_ASD_TARGETS/      - targets: [%s] #TODO_ASD_TARGETS/g\" /etc/prometheus/prometheus.yml", ips))
		}

		if len(nodeTargets) > 0 {
			nips := "'" + strings.Join(nodeTargets, "','") + "'"
			commands = append(commands, fmt.Sprintf("sed -i.bakNode -E \"s/.*TODO_ASDN_TARGETS/      - targets: [%s] #TODO_ASDN_TARGETS/g\" /etc/prometheus/prometheus.yml", nips))
		}

		if len(clientTargets) > 0 {
			cips := "'" + strings.Join(clientTargets, "','") + "'"
			commands = append(commands, fmt.Sprintf("sed -i.bakClient -E \"s/.*TODO_CLIENT_TARGETS/      - targets: [%s] #TODO_CLIENT_TARGETS/g\" /etc/prometheus/prometheus.yml", cips))
		}

		// Execute sed commands
		if len(commands) > 0 {
			fullCmd := strings.Join(commands, " && ")
			output := client.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        []string{"bash", "-c", fullCmd},
					SessionTimeout: time.Minute,
				},
				Username:       "root",
				ConnectTimeout: 30 * time.Second,
			})

			if output.Output.Err != nil {
				return fmt.Errorf("failed to configure prometheus on %s:%d: %w (stdout: %s, stderr: %s)",
					client.ClusterName, client.NodeNo, output.Output.Err, output.Output.Stdout, output.Output.Stderr)
			}
		}

		// Restart Prometheus
		logger.Info("Restarting Prometheus on %s:%d", client.ClusterName, client.NodeNo)
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", "kill -HUP $(pidof prometheus) || systemctl restart prometheus"},
				SessionTimeout: time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			logger.Warn("Failed to restart Prometheus on %s:%d: %s", client.ClusterName, client.NodeNo, output.Output.Err)
			logger.Warn("You may need to manually restart Prometheus: systemctl restart prometheus")
		} else {
			logger.Info("Successfully configured AMS on %s:%d", client.ClusterName, client.NodeNo)
		}
	}

	logger.Info("To access Grafana, visit the client IP on port 3000 from your browser")
	logger.Info("Run 'aerolab client list' to get IPs. Username:Password is admin:admin")
	logger.Info("NOTE: Remember to install the aerospike-prometheus-exporter on Aerospike server nodes: aerolab cluster add exporter")

	return nil
}

// parseConnections parses cluster or client names and returns IP addresses
func (c *ClientConfigureAMSCmd) parseConnections(inventory *backends.Inventory, names string, isClient bool) (map[string][]string, error) {
	if names == "" {
		return nil, nil
	}

	nameList := strings.Split(names, ",")
	result := make(map[string][]string)

	for _, name := range nameList {
		name = strings.TrimSpace(name)
		var instances []*backends.Instance

		if isClient {
			instances = inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(name).WithState(backends.LifeCycleStateRunning).Describe()
		} else {
			instances = inventory.Instances.WithClusterName(name).WithState(backends.LifeCycleStateRunning).Describe()
		}

		if len(instances) == 0 {
			return nil, fmt.Errorf("%s '%s' not found or has no running instances", map[bool]string{true: "client", false: "cluster"}[isClient], name)
		}

		ips := []string{}
		for _, inst := range instances {
			ip := inst.IP.Private
			if ip == "" {
				ip = inst.IP.Public
			}
			ips = append(ips, ip)
		}
		result[name] = ips
	}

	return result, nil
}
