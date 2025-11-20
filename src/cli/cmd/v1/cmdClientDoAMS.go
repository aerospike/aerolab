package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/grafana"
	"github.com/aerospike/aerolab/pkg/utils/installers/prometheus"
	"github.com/rglonek/logger"
)

type ClientCreateAMSCmd struct {
	ClientCreateNoneCmd
	GrafanaVersion    string `long:"grafana-version" description:"Grafana version to install" default:"latest"`
	PrometheusVersion string `long:"prometheus-version" description:"Prometheus version to install" default:"latest"`
}

func (c *ClientCreateAMSCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "ams"}
	} else {
		cmd = []string{"client", "create", "ams"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.createAMSClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateAMSCmd) createAMSClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "ams"
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Install Prometheus and Grafana
	logger.Info("Installing AMS (Prometheus and Grafana)")

	// Get created instances
	clients := system.Backend.GetInventory().Instances.
		WithTags(map[string]string{"aerolab.old.type": "client"}).
		WithClusterName(c.ClientName.String()).
		WithState(backends.LifeCycleStateRunning)

	if clients.Count() == 0 {
		return fmt.Errorf("no running client instances found after creation")
	}

	clientList := clients.Describe()
	for _, client := range clientList {
		// Install Prometheus
		var promVersion *string
		if c.PrometheusVersion != "latest" {
			promVersion = &c.PrometheusVersion
		}
		prometheusScript, err := prometheus.GetLinuxInstallScript(promVersion, nil, true, true)
		if err != nil {
			logger.Warn("Failed to get Prometheus installer for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		conf, err := client.GetSftpConfig("root")
		if err != nil {
			logger.Warn("Failed to get SFTP config for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		sftpClient, err := sshexec.NewSftp(conf)
		if err != nil {
			logger.Warn("Failed to create SFTP client for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install-prometheus.sh",
			Source:      strings.NewReader(string(prometheusScript)),
			Permissions: 0755,
		})
		if err != nil {
			sftpClient.Close()
			logger.Warn("Failed to upload Prometheus installer to %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Install Grafana
		grafanaVersion := c.GrafanaVersion
		if grafanaVersion == "latest" {
			grafanaVersion = "latest"
		}
		grafanaScript, err := grafana.GetInstallScript(grafanaVersion, true, true)
		if err != nil {
			sftpClient.Close()
			logger.Warn("Failed to get Grafana installer for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install-grafana.sh",
			Source:      strings.NewReader(string(grafanaScript)),
			Permissions: 0755,
		})
		sftpClient.Close()
		if err != nil {
			logger.Warn("Failed to upload Grafana installer to %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Execute Prometheus installer
		logger.Info("Installing Prometheus on %s:%d", client.ClusterName, client.NodeNo)
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install-prometheus.sh"},
				SessionTimeout: 15 * time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			logger.Warn("Failed to install Prometheus on %s:%d: %s", client.ClusterName, client.NodeNo, output.Output.Err)
			continue
		}

		// Execute Grafana installer
		logger.Info("Installing Grafana on %s:%d", client.ClusterName, client.NodeNo)
		output = client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install-grafana.sh"},
				SessionTimeout: 15 * time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			logger.Warn("Failed to install Grafana on %s:%d: %s", client.ClusterName, client.NodeNo, output.Output.Err)
		} else {
			logger.Info("Successfully installed AMS on %s:%d", client.ClusterName, client.NodeNo)
		}
	}

	return nil
}
