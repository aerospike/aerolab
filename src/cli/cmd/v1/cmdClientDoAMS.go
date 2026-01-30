package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/grafana"
	"github.com/aerospike/aerolab/pkg/utils/installers/prometheus"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

type ClientCreateAMSCmd struct {
	ClientCreateNoneCmd
	GrafanaVersion    string          `long:"grafana-version" description:"Grafana version to install" default:"latest"`
	PrometheusVersion string          `long:"prometheus-version" description:"Prometheus version to install" default:"latest"`
	ConnectClusters   TypeClusterName `short:"s" long:"clusters" description:"Comma-separated list of clusters to configure as source for this AMS"`
	ConnectClients    TypeClientName  `short:"S" long:"clients" description:"Comma-separated list of (graph) clients to configure as source for this AMS"`
	Dashboards        flags.Filename  `long:"dashboards" description:"Dashboards list file, see https://github.com/aerospike/aerolab/blob/master/docs/usage/monitoring/dashboards.md"`
	DebugDashboards   bool            `long:"debug-dashboards" description:"Enable debug output for dashboard installation"`
}

type CustomAMSDashboard struct {
	Destination string  `yaml:"destination"`
	FromFile    *string `yaml:"fromFile,omitempty"`
	FromUrl     *string `yaml:"fromUrl,omitempty"`
}

type GitHubDirEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int    `json:"size"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
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

	// Auto-expose port 3000 for Docker backend if not already exposed
	// Use +3000:3000 to auto-increment host port if 3000 is already in use
	if system.Opts.Config.Backend.Type == "docker" {
		hasPort3000 := false
		for _, port := range c.Docker.ExposePorts {
			if strings.Contains(port, ":3000") {
				hasPort3000 = true
				break
			}
		}
		if !hasPort3000 {
			system.Logger.Info("Auto-exposing port 3000 for Grafana access (auto-increment if port in use)")
			c.Docker.ExposePorts = append([]string{"+3000:3000"}, c.Docker.ExposePorts...)
		}
	}

	defer UpdateDiskCache(system)()
	err = c.createAMSClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateAMSCmd) createAMSClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "ams"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "ams"
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

	// Load custom dashboards
	customDashboards, err := c.loadCustomDashboards()
	if err != nil {
		return err
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Install AMS on each client
	logger.Info("Installing AMS (Prometheus, Grafana, and Loki)")

	for _, client := range clients.Describe() {
		err := c.installAMS(system, client, logger, clusterNodes, clientNodes, customDashboards)
		if err != nil {
			logger.Warn("Failed to install AMS on %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		logger.Info("Successfully installed AMS on %s:%d", client.ClusterName, client.NodeNo)
		logger.Info("Username:Password is admin:admin")

		// Determine access URL
		var accessHost string
		var accessPort string = "3000"
		if system.Opts.Config.Backend.Type == "docker" {
			accessHost = "localhost"
			for _, port := range c.Docker.ExposePorts {
				if strings.Contains(port, ":3000") {
					parts := strings.Split(strings.TrimPrefix(port, "+"), ":")
					if len(parts) == 2 {
						accessPort = parts[0]
						break
					}
				}
			}
		} else {
			accessHost = client.IP.Public
		}
		logger.Info("Access Grafana at: http://%s:%s", accessHost, accessPort)
		logger.Info("NOTE: Remember to install the aerospike-prometheus-exporter on Aerospike server nodes: aerolab cluster add exporter")
	}

	return nil
}

// parseConnections parses cluster or client names and returns IP addresses
func (c *ClientCreateAMSCmd) parseConnections(inventory *backends.Inventory, names string, isClient bool) (map[string][]string, error) {
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

// loadCustomDashboards loads custom dashboards from file
func (c *ClientCreateAMSCmd) loadCustomDashboards() ([]CustomAMSDashboard, error) {
	dashboards := []CustomAMSDashboard{}

	if c.Dashboards != "" {
		data, err := os.ReadFile(strings.TrimPrefix(string(c.Dashboards), "+"))
		if err != nil {
			return nil, fmt.Errorf("failed to read dashboards file: %w", err)
		}
		err = yaml.Unmarshal(data, &dashboards)
		if err != nil {
			return nil, fmt.Errorf("failed to parse dashboards file: %w", err)
		}
	}

	// Always add asbench dashboard
	asbenchURL := "https://raw.githubusercontent.com/aerospike/aerolab/master/scripts/asbench2.json"
	dashboards = append(dashboards, CustomAMSDashboard{
		FromUrl:     &asbenchURL,
		Destination: "/var/lib/grafana/dashboards/asbench.json",
	})

	return dashboards, nil
}

// installAMS installs and configures complete AMS stack with minimal SSH calls
func (c *ClientCreateAMSCmd) installAMS(system *System, client *backends.Instance, logger *logger.Logger, clusterNodes, clientNodes map[string][]string, customDashboards []CustomAMSDashboard) error {
	logger.Info("Building complete installation script for %s:%d", client.ClusterName, client.NodeNo)

	// Step 1: Build ONE complete installation script
	fullScript, err := c.buildCompleteInstallScript(client, clusterNodes, clientNodes)
	if err != nil {
		return fmt.Errorf("failed to build installation script: %w", err)
	}

	// Step 2: Upload script via SFTP
	conf, err := client.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	sftpClient, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}

	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/tmp/install-ams-complete.sh",
		Source:      strings.NewReader(fullScript),
		Permissions: 0755,
	})
	sftpClient.Close()
	if err != nil {
		return fmt.Errorf("failed to upload installation script: %w", err)
	}

	// Step 3: Execute complete installation in ONE SSH call
	logger.Info("Installing Prometheus, Grafana, and Loki on %s:%d (this may take 10-15 minutes)", client.ClusterName, client.NodeNo)

	var stdout, stderr *os.File
	var stdin io.ReadCloser
	terminal := false
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
		terminal = true
	}

	output := client.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "/tmp/install-ams-complete.sh"},
			Stdin:          stdin,
			Stdout:         stdout,
			Stderr:         stderr,
			SessionTimeout: 30 * time.Minute,
			Terminal:       terminal,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})

	if output.Output.Err != nil {
		return fmt.Errorf("installation failed: %w (stdout: %s, stderr: %s)", output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	// Step 4: Install dashboards (parallel downloads for speed)
	logger.Info("Installing dashboards on %s:%d", client.ClusterName, client.NodeNo)
	err = c.installDashboards(system, client, customDashboards, logger)
	if err != nil {
		return fmt.Errorf("failed to install dashboards: %w", err)
	}

	// Step 5: Final service restart in ONE call
	logger.Info("Starting services on %s:%d", client.ClusterName, client.NodeNo)
	output = client.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "systemctl daemon-reload; systemctl restart prometheus; systemctl restart grafana-server; sleep 3; systemctl restart loki"},
			SessionTimeout: 2 * time.Minute,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})

	if output.Output.Err != nil {
		return fmt.Errorf("failed to start services: %w", output.Output.Err)
	}

	return nil
}

// buildCompleteInstallScript builds ONE complete script for all installations
func (c *ClientCreateAMSCmd) buildCompleteInstallScript(client *backends.Instance, clusterNodes, clientNodes map[string][]string) (string, error) {
	var script bytes.Buffer

	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n")
	script.WriteString("echo 'Starting AMS installation...'\n\n")

	// Part 1: Prometheus installation (from installer package)
	script.WriteString("# ===== Installing Prometheus =====\n")
	var promVersion *string
	if c.PrometheusVersion != "latest" && c.PrometheusVersion != "" {
		promVersion = &c.PrometheusVersion
	}
	prometheusScript, err := prometheus.GetLinuxInstallScript(promVersion, nil, true, false) // enable but don't start yet
	if err != nil {
		return "", fmt.Errorf("failed to get Prometheus install script: %w", err)
	}
	script.Write(prometheusScript)
	script.WriteString("\necho 'Prometheus installed'\n\n")

	// Part 2: Grafana installation (from installer package)
	script.WriteString("# ===== Installing Grafana =====\n")
	grafanaVersion := c.GrafanaVersion
	if grafanaVersion == "latest" {
		grafanaVersion = ""
	}
	grafanaScript, err := grafana.GetInstallScript(grafanaVersion, true, false) // enable but don't start yet
	if err != nil {
		return "", fmt.Errorf("failed to get Grafana install script: %w", err)
	}
	script.Write(grafanaScript)
	script.WriteString("\necho 'Grafana installed'\n\n")

	// Part 3: Configure Prometheus
	script.WriteString("# ===== Configuring Prometheus =====\n")
	script.WriteString(c.buildPrometheusConfig(clusterNodes, clientNodes))
	script.WriteString("\necho 'Prometheus configured'\n\n")

	// Part 4: Configure Grafana
	script.WriteString("# ===== Configuring Grafana =====\n")
	script.WriteString(c.buildGrafanaConfig())
	script.WriteString("\necho 'Grafana configured'\n\n")

	// Part 5: Install Loki
	script.WriteString("# ===== Installing Loki =====\n")
	script.WriteString(c.buildLokiInstall(client))
	script.WriteString("\necho 'Loki installed'\n\n")

	// Part 6: Setup autostart scripts
	script.WriteString("# ===== Setting up autostart scripts =====\n")
	/*script.WriteString(`mkdir -p /opt/autoload
	echo 'service prometheus start' > /opt/autoload/01-prometheus
	echo 'service grafana-server start' > /opt/autoload/02-grafana
	echo 'service loki start' > /opt/autoload/03-loki
	chmod 755 /opt/autoload/*
	`)*/
	script.WriteString("systemctl enable loki\n")
	script.WriteString("systemctl enable grafana-server\n")
	script.WriteString("systemctl enable prometheus\n")
	script.WriteString("echo 'Autostart scripts created'\n\n")

	script.WriteString("echo 'AMS installation complete!'\n")
	return script.String(), nil
}

// buildPrometheusConfig generates Prometheus configuration commands
func (c *ClientCreateAMSCmd) buildPrometheusConfig(clusterNodes, clientNodes map[string][]string) string {
	var script bytes.Buffer

	// Download aerospike rules
	script.WriteString("wget -q -O /etc/prometheus/aerospike_rules.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/prometheus/aerospike_rules.yml\n")
	script.WriteString("sed -i.bak -E 's/^rule_files:/rule_files:\\n  - \"\\/etc\\/prometheus\\/aerospike_rules.yaml\"/g' /etc/prometheus/prometheus.yml\n")
	script.WriteString("sed -i.bak -E 's/- job_name: node/- job_name: nodelocal/g' /etc/prometheus/prometheus.yml\n")

	// Add scrape configs
	script.WriteString("sed -i.bak -E 's/^scrape_configs:/scrape_configs:\\n  - job_name: aerospike\\n    static_configs:\\n      - targets: [] #TODO_ASD_TARGETS\\n  - job_name: node\\n    static_configs:\\n      - targets: [] #TODO_ASDN_TARGETS\\n  - job_name: clients\\n    static_configs:\\n      - targets: [] #TODO_CLIENT_TARGETS\\n/g' /etc/prometheus/prometheus.yml\n")

	// Configure targets
	if len(clusterNodes) > 0 {
		asdTargets := []string{}
		nodeTargets := []string{}
		for _, nodes := range clusterNodes {
			for _, node := range nodes {
				asdTargets = append(asdTargets, node+":9145")
				nodeTargets = append(nodeTargets, node+":9100")
			}
		}
		script.WriteString(fmt.Sprintf("sed -i.bakAsd -E \"s/.*TODO_ASD_TARGETS/      - targets: ['%s'] #TODO_ASD_TARGETS/g\" /etc/prometheus/prometheus.yml\n", strings.Join(asdTargets, "','")))
		script.WriteString(fmt.Sprintf("sed -i.bakAsdNode -E \"s/.*TODO_ASDN_TARGETS/      - targets: ['%s'] #TODO_ASDN_TARGETS/g\" /etc/prometheus/prometheus.yml\n", strings.Join(nodeTargets, "','")))
	}

	if len(clientNodes) > 0 {
		clientTargets := []string{}
		for _, nodes := range clientNodes {
			for _, node := range nodes {
				clientTargets = append(clientTargets, node+":9090")
			}
		}
		script.WriteString(fmt.Sprintf("sed -i.bakGraph -E \"s/.*TODO_CLIENT_TARGETS/      - targets: ['%s'] #TODO_CLIENT_TARGETS/g\" /etc/prometheus/prometheus.yml\n", strings.Join(clientTargets, "','")))
	}

	return script.String()
}

// buildGrafanaConfig generates Grafana configuration commands
func (c *ClientCreateAMSCmd) buildGrafanaConfig() string {
	return `# Configure Grafana
chmod 664 /etc/grafana/grafana.ini
sed -i.bak -E 's/^\[paths\]$/[paths]\nprovisioning = \/etc\/grafana\/provisioning/g' /etc/grafana/grafana.ini

# Create systemd override to run Grafana as root (avoids permission issues)
mkdir -p /etc/systemd/system/grafana-server.service.d
cat <<'GRAFANAOVERRIDE' > /etc/systemd/system/grafana-server.service.d/override.conf
[Service]
User=root
Group=root
GRAFANAOVERRIDE

# Ensure Grafana directories exist and have proper permissions
mkdir -p /var/lib/grafana /var/log/grafana /run/grafana
chown -R root:root /var/lib/grafana /var/log/grafana /run/grafana /etc/grafana
chmod 755 /var/lib/grafana /var/log/grafana /run/grafana

# Install plugins
grafana-cli plugins install camptocamp-prometheus-alertmanager-datasource
grafana-cli plugins install grafana-polystat-panel

# Configure datasources
wget -q -O /etc/grafana/provisioning/datasources/all.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/datasources/all.yaml
echo -e '  - name: Loki\n    type: loki\n    access: proxy\n    url: http://localhost:3100\n    jsonData:\n      maxLines: 1000\n' >> /etc/grafana/provisioning/datasources/all.yaml
sed -i.bak 's/prometheus:9090/127.0.0.1:9090/g' /etc/grafana/provisioning/datasources/all.yaml
`
}

// buildLokiInstall generates Loki installation commands
func (c *ClientCreateAMSCmd) buildLokiInstall(client *backends.Instance) string {
	arch := "amd64"
	if client.Architecture == backends.ArchitectureARM64 {
		arch = "arm64"
	}

	return fmt.Sprintf(`# Install Loki
cd /root
wget -q https://github.com/grafana/loki/releases/download/v3.3.0/loki-linux-%s.zip
unzip -q loki-linux-%s.zip
mv loki-linux-%s /usr/bin/loki
wget -q https://github.com/grafana/loki/releases/download/v3.3.0/logcli-linux-%s.zip
unzip -q logcli-linux-%s.zip
mv logcli-linux-%s /usr/bin/logcli
chmod 755 /usr/bin/logcli /usr/bin/loki
mkdir -p /etc/loki /data-logs/loki

cat <<'LOKIEOF' > /etc/loki/loki.yaml
auth_enabled: false
server:
  http_listen_port: 3100
  grpc_listen_port: 9096
common:
  path_prefix: /data-logs/loki
  storage:
    filesystem:
      chunks_directory: /data-logs/loki/chunks
      rules_directory: /data-logs/loki/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory
schema_config:
  configs:
    - from: 2020-10-24
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
analytics:
  reporting_enabled: false
LOKIEOF

# Create Loki systemd service
cat <<'LOKISVCEOF' > /usr/lib/systemd/system/loki.service
[Unit]
Description=Loki Log Aggregation System
After=network.target grafana-server.service
Wants=grafana-server.service

[Service]
Type=simple
User=root
ExecStart=/usr/bin/loki -config.file=/etc/loki/loki.yaml -log-config-reverse-order
Restart=always
RestartSec=5
WorkingDirectory=/data-logs/loki
StandardOutput=append:/var/log/loki.log
StandardError=append:/var/log/loki.log

[Install]
WantedBy=multi-user.target
LOKISVCEOF

systemctl daemon-reload
`, arch, arch, arch, arch, arch, arch)
}

// installDashboards downloads and installs Grafana dashboards
func (c *ClientCreateAMSCmd) installDashboards(system *System, client *backends.Instance, customDashboards []CustomAMSDashboard, logger *logger.Logger) error {
	// Create base directories in ONE call
	output := client.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "wget -q -O /etc/grafana/provisioning/dashboards/all.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/dashboards/all.yaml && mkdir -p /var/lib/grafana/dashboards"},
			SessionTimeout: 2 * time.Minute,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})
	if output.Output.Err != nil {
		return fmt.Errorf("failed to setup dashboard directories: %w", output.Output.Err)
	}

	// Install default dashboards if not disabled
	if c.Dashboards == "" || strings.HasPrefix(string(c.Dashboards), "+") {
		if c.DebugDashboards {
			logger.Debug("Installing default dashboards from aerospike-monitoring GitHub")
		}
		err := c.installDefaultDashboards(client, logger)
		if err != nil {
			return err
		}
	}

	// Install custom dashboards
	if len(customDashboards) > 0 {
		if c.DebugDashboards {
			logger.Debug("Installing %d custom dashboards", len(customDashboards))
		}
		err := c.installCustomDashboards(client, customDashboards, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

// installDefaultDashboards downloads default dashboards from GitHub
func (c *ClientCreateAMSCmd) installDefaultDashboards(client *backends.Instance, logger *logger.Logger) error {
	baseURL := "https://api.github.com/repos/aerospike/aerospike-monitoring/contents/"
	currentPath := "config/grafana/dashboards"

	mkdirs, dashboards, err := c.githubBuildDashboardList(baseURL, currentPath, currentPath)
	if err != nil {
		return fmt.Errorf("failed to list GitHub dashboards: %w", err)
	}

	// Create all directories in ONE call
	if len(mkdirs) > 0 {
		mkdirCmds := []string{}
		for _, mkdir := range mkdirs {
			mkdirCmds = append(mkdirCmds, strings.Join(mkdir, " "))
		}
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", strings.Join(mkdirCmds, " && ")},
				SessionTimeout: time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to create dashboard directories: %w", output.Output.Err)
		}
	}

	// Download dashboards in parallel (this is the expensive part, so we parallelize it)
	if c.DebugDashboards {
		logger.Debug("Downloading %d dashboards in parallel", len(dashboards))
	}

	errors := parallelize.MapLimit(dashboards, c.ParallelSSHThreads, func(dashboard []string) error {
		for tries := 0; tries < 3; tries++ {
			output := client.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        dashboard,
					SessionTimeout: time.Minute,
				},
				Username:       "root",
				ConnectTimeout: 30 * time.Second,
			})
			if output.Output.Err == nil {
				return nil
			}
			if tries < 2 {
				time.Sleep(time.Second)
			}
		}
		return fmt.Errorf("failed to download dashboard after 3 tries")
	})

	for _, err := range errors {
		if err != nil {
			return err
		}
	}

	return nil
}

// installCustomDashboards installs custom dashboards from URLs or files
func (c *ClientCreateAMSCmd) installCustomDashboards(client *backends.Instance, dashboards []CustomAMSDashboard, logger *logger.Logger) error {
	// Download/read all dashboard contents
	type dashFile struct {
		path    string
		content []byte
	}
	files := []dashFile{}

	for _, dashboard := range dashboards {
		var content []byte
		var err error

		if dashboard.FromUrl != nil && *dashboard.FromUrl != "" {
			if c.DebugDashboards {
				logger.Debug("Downloading custom dashboard from: %s", *dashboard.FromUrl)
			}
			resp, err := http.Get(*dashboard.FromUrl)
			if err != nil {
				return fmt.Errorf("failed to download dashboard from %s: %w", *dashboard.FromUrl, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				return fmt.Errorf("URL %s returned status %d", *dashboard.FromUrl, resp.StatusCode)
			}
			content, err = io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read dashboard: %w", err)
			}
		} else if dashboard.FromFile != nil && *dashboard.FromFile != "" {
			if c.DebugDashboards {
				logger.Debug("Loading custom dashboard from file: %s", *dashboard.FromFile)
			}
			content, err = os.ReadFile(*dashboard.FromFile)
			if err != nil {
				return fmt.Errorf("failed to read dashboard file: %w", err)
			}
		}

		files = append(files, dashFile{
			path:    dashboard.Destination,
			content: content,
		})
	}

	// Create all needed directories in ONE call
	dirCmds := []string{}
	dirs := make(map[string]bool)
	for _, file := range files {
		dir, _ := path.Split(file.path)
		if dir != "/var/lib/grafana/dashboards" && dir != "/var/lib/grafana/dashboards/" && !dirs[dir] {
			dirCmds = append(dirCmds, "mkdir -p "+dir)
			dirs[dir] = true
		}
	}
	if len(dirCmds) > 0 {
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", strings.Join(dirCmds, " && ")},
				SessionTimeout: 30 * time.Second,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to create directories: %w", output.Output.Err)
		}
	}

	// Upload files via SFTP (we can't parallelize SFTP, but we can batch the uploads)
	conf, err := client.GetSftpConfig("root")
	if err != nil {
		return err
	}
	sftpClient, err := sshexec.NewSftp(conf)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	for _, file := range files {
		if c.DebugDashboards {
			logger.Debug("Uploading custom dashboard to: %s", file.path)
		}
		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    file.path,
			Source:      bytes.NewReader(file.content),
			Permissions: 0644,
		})
		if err != nil {
			return fmt.Errorf("failed to upload dashboard to %s: %w", file.path, err)
		}
	}

	return nil
}

// githubBuildDashboardList recursively lists dashboards from GitHub
func (c *ClientCreateAMSCmd) githubBuildDashboardList(baseURL, basePath, currentPath string) (mkdirs [][]string, dashboards [][]string, err error) {
	entries := []GitHubDirEntry{}

	req, err := http.NewRequest("GET", baseURL+currentPath, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Add("Accept", "application/vnd.github+json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, nil, fmt.Errorf("GET '%s': status code %d", baseURL+currentPath, response.StatusCode)
	}

	err = json.NewDecoder(response.Body).Decode(&entries)
	if err != nil {
		return nil, nil, err
	}

	for _, entry := range entries {
		if entry.Size != 0 && strings.HasSuffix(entry.Name, ".json") && entry.Type == "file" {
			newPath := strings.TrimPrefix(entry.Path, basePath)
			dashboards = append(dashboards, []string{"wget", "-q", "-O", "/var/lib/grafana/dashboards" + newPath, entry.DownloadURL})
		} else if entry.Size == 0 && entry.DownloadURL == "" && entry.Type == "dir" {
			newPath := strings.TrimPrefix(entry.Path, basePath)
			mkdirs = append(mkdirs, []string{"mkdir", "-p", "/var/lib/grafana/dashboards" + newPath})

			newMkdirs, newDashboards, err := c.githubBuildDashboardList(baseURL, basePath, entry.Path)
			if err != nil {
				return nil, nil, err
			}
			dashboards = append(dashboards, newDashboards...)
			mkdirs = append(mkdirs, newMkdirs...)
		}
	}

	return mkdirs, dashboards, nil
}
