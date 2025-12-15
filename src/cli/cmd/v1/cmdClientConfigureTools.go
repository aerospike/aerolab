package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type ClientConfigureToolsCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	ConnectAMS TypeClientName `short:"m" long:"ams" default:"ams" description:"AMS client machine name"`
	Threads    int            `short:"t" long:"threads" description:"Number of parallel threads" default:"10"`
	Help       HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientConfigureToolsCmd) Execute(args []string) error {
	cmd := []string{"client", "configure", "tools"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.configureTools(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientConfigureToolsCmd) configureTools(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "configure", "tools"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Get AMS client instances to find Loki endpoint
	logger.Info("Finding AMS instance: %s", c.ConnectAMS.String())
	amsInstances := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(c.ConnectAMS.String()).WithState(backends.LifeCycleStateRunning).Describe()
	if len(amsInstances) == 0 {
		return fmt.Errorf("AMS client '%s' not found or has no running instances", c.ConnectAMS.String())
	}

	// Get Loki endpoint (IP:3100) from first AMS instance
	lokiIP := amsInstances[0].IP.Private
	if lokiIP == "" {
		lokiIP = amsInstances[0].IP.Public
	}
	lokiEndpoint := lokiIP + ":3100"

	if len(amsInstances) > 1 {
		logger.Warn("Found more than 1 AMS machine, will point log consolidator at the first one: %s", lokiEndpoint)
	}
	logger.Info("Using Loki endpoint: %s", lokiEndpoint)

	// Get tools client instances
	toolsClients, err := getClientInstancesHelper(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(toolsClients) == 0 {
		return fmt.Errorf("no tools client instances found")
	}

	logger.Info("Configuring Promtail on %d tools client machines", len(toolsClients))

	// Configure each tools client in parallel
	errs := parallelize.MapLimit(toolsClients, c.Threads, func(client *backends.Instance) error {
		return c.configureToolsClient(client, lokiEndpoint, logger)
	})

	// Check for errors
	var hasError bool
	for i, err := range errs {
		if err != nil {
			logger.Error("Node %s:%d returned error: %s", toolsClients[i].ClusterName, toolsClients[i].NodeNo, err)
			hasError = true
		}
	}

	if hasError {
		return errors.New("some nodes returned errors")
	}

	logger.Info("Successfully configured Promtail on all tools clients")
	return nil
}

// configureToolsClient configures Promtail on a single tools client instance
func (c *ClientConfigureToolsCmd) configureToolsClient(client *backends.Instance, lokiEndpoint string, logger *logger.Logger) error {
	logger.Debug("Configuring Promtail on %s:%d", client.ClusterName, client.NodeNo)

	// Detect architecture
	isArm := client.Architecture == backends.ArchitectureARM64

	// Get SFTP client
	conf, err := client.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}
	sftpClient, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// 1. Store Loki IP
	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/opt/asbench-grafana.ip",
		Source:      strings.NewReader(lokiEndpoint),
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to upload Loki IP file: %w", err)
	}

	// 2. Upload Promtail installation script
	installScript := c.generatePromtailInstallScript(isArm)
	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/opt/install-promtail.sh",
		Source:      strings.NewReader(installScript),
		Permissions: 0755,
	})
	if err != nil {
		return fmt.Errorf("failed to upload Promtail install script: %w", err)
	}

	// 3. Upload Promtail configuration script
	configScript := c.generatePromtailConfigScript()
	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/opt/configure-promtail.sh",
		Source:      strings.NewReader(configScript),
		Permissions: 0755,
	})
	if err != nil {
		return fmt.Errorf("failed to upload Promtail config script: %w", err)
	}

	sftpClient.Close()

	// 4. Execute installation script
	output := client.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"/bin/bash", "/opt/install-promtail.sh"},
			SessionTimeout: 5 * time.Minute,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})
	if output.Output.Err != nil {
		return fmt.Errorf("failed to install Promtail: %w (stdout: %s, stderr: %s)", output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	// 5. Execute configuration script
	output = client.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"/bin/bash", "/opt/configure-promtail.sh"},
			SessionTimeout: time.Minute,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})
	if output.Output.Err != nil {
		return fmt.Errorf("failed to configure Promtail: %w (stdout: %s, stderr: %s)", output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	// 6. Create systemd service and enable/start Promtail
	output = client.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"/bin/bash", "-c", "systemctl daemon-reload && systemctl enable promtail && systemctl restart promtail"},
			SessionTimeout: time.Minute,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})
	if output.Output.Err != nil {
		return fmt.Errorf("failed to enable and start Promtail: %w (stdout: %s, stderr: %s)", output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	logger.Debug("Successfully configured Promtail on %s:%d", client.ClusterName, client.NodeNo)
	return nil
}

// generatePromtailInstallScript generates the installation script for Promtail
func (c *ClientConfigureToolsCmd) generatePromtailInstallScript(isArm bool) string {
	arch := "amd64"
	if isArm {
		arch = "arm64"
	}

	return fmt.Sprintf(`#!/bin/bash
set -e

# Check if Promtail is already installed
if [ -f /usr/bin/promtail ]; then
	echo "Promtail already installed"
else
	apt-get update
	apt-get -y install unzip wget
	cd /root
	wget -q https://github.com/grafana/loki/releases/download/v3.3.0/promtail-linux-%s.zip
	unzip -q promtail-linux-%s.zip
	mv promtail-linux-%s /usr/bin/promtail
	chmod 755 /usr/bin/promtail
	echo "Promtail binary installed"
fi

# Create necessary directories
mkdir -p /etc/promtail /var/promtail /var/log

# Create systemd service file
cat <<'PROMTAILSVC' > /usr/lib/systemd/system/promtail.service
[Unit]
Description=Promtail Log Aggregation System
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/bin/promtail -config.file=/etc/promtail/promtail.yaml -log-config-reverse-order
Restart=always
RestartSec=5
StandardOutput=append:/var/log/promtail.log
StandardError=append:/var/log/promtail.log

[Install]
WantedBy=multi-user.target
PROMTAILSVC

echo "Promtail systemd service created"
`, arch, arch, arch)
}

// generatePromtailConfigScript generates the configuration script for Promtail
func (c *ClientConfigureToolsCmd) generatePromtailConfigScript() string {
	return `#!/bin/bash
cat <<'EOF' > /etc/promtail/promtail.yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0
positions:
  filename: /var/promtail/positions.yaml
clients:
  - url: http://$(cat /opt/asbench-grafana.ip)/loki/api/v1/push
scrape_configs:
  - job_name: asbench
    static_configs:
      - targets:
          - localhost
        labels:
          job: asbench
          __path__: /var/log/asbench_*.log
          host: $(hostname)
    pipeline_stages:
      - match:
          selector: '{job="asbench"}'
          stages:
          - regex:
              source: filename
              expression: "/var/log/asbench_(?P<instance>.*)\\.log"
          - labels:
              instance:
EOF
echo "Promtail configured successfully"
`
}
