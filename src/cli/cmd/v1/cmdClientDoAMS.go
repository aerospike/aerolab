package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers"
	"github.com/aerospike/aerolab/pkg/utils/installers/grafana"
	"github.com/aerospike/aerolab/pkg/utils/installers/prometheus"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

type ClientCreateAMSCmd struct {
	ClientCreateNoneCmd
	GrafanaVersion    string          `long:"grafana-version" description:"Grafana version to install" default:"12.4.3"`
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

	// Resolve (or auto-create) the AMS template image and point the base client
	// creation at it, so Prometheus/Grafana/Loki are not reinstalled from scratch
	// on every AMS client.
	if err := c.resolveAMSTemplate(system, inventory, logger, args); err != nil {
		return err
	}

	// If a template was just auto-created, resolveAMSTemplate refreshed the
	// backend inventory. Re-fetch the local inventory handle so the downstream
	// instance lookup finds the newly-created image (with its correct
	// Architecture). Without this, the image lookup misses, a placeholder
	// Image{} with zero-value Architecture (= amd64) is fabricated, and Docker
	// refuses to run an arm64 template on an arm64 host because the platform
	// passed to the daemon is linux/amd64.
	inventory = system.Backend.GetInventory()

	// Create base client first. The AMS template already has the base tools
	// (curl/wget/vim/git/jq/unzip/zip) baked in by buildAMSTemplateInstallScript,
	// so skip the per-instance install step to avoid redundant apt/yum work.
	baseCmd := &ClientCreateBaseCmd{
		ClientCreateNoneCmd: c.ClientCreateNoneCmd,
		skipBaseInstall:     true,
	}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Configure AMS on each client (Prometheus, Grafana, and Loki are already
	// baked into the template image; this applies per-instance scrape targets
	// and custom dashboards, then restarts services to pick up the config).
	logger.Info("Configuring AMS clients (template pre-installed Prometheus, Grafana, Loki)")

	for _, client := range clients.Describe() {
		err := c.installAMS(system, client, logger, clusterNodes, clientNodes, customDashboards)
		if err != nil {
			return fmt.Errorf("failed to configure AMS on %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		logger.Info("Successfully configured AMS on %s:%d", client.ClusterName, client.NodeNo)
		logger.Info("Username:Password is admin:admin")

		// Prefer the backend-computed AccessURL (Docker derives it from actual
		// runtime port mappings, which is the only reliable source when
		// auto-increment was used to pick a free host port; cloud backends
		// already build the public-IP URL there). Fall back to a
		// best-effort computation if for any reason AccessURL is empty.
		accessURL := client.AccessURL
		if accessURL == "" {
			if system.Opts.Config.Backend.Type == "docker" {
				accessURL = "http://localhost:3000"
			} else {
				accessURL = fmt.Sprintf("http://%s:3000", client.IP.Public)
			}
		}
		logger.Info("Access Grafana at: %s", accessURL)
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

// installAMS installs and configures complete AMS stack with minimal SSH calls.
// The client instance is expected to have been booted from an AMS template image
// that already has Prometheus, Grafana, and Loki pre-installed. This function
// only applies per-instance configuration (Prometheus scrape targets, dashboards,
// and service restart).
func (c *ClientCreateAMSCmd) installAMS(system *System, client *backends.Instance, logger *logger.Logger, clusterNodes, clientNodes map[string][]string, customDashboards []CustomAMSDashboard) error {
	logger.Info("Building AMS per-instance configuration script for %s:%d", client.ClusterName, client.NodeNo)

	// Step 1: Build per-instance configuration script (scrape targets only)
	fullScript, err := c.buildAMSInstanceConfigScript(clusterNodes, clientNodes)
	if err != nil {
		return fmt.Errorf("failed to build AMS configuration script: %w", err)
	}

	// Step 2: Upload script via SFTP
	conf, err := client.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}
	conf.MaxRetries = c.MaxRetries
	conf.RetrySleep = c.RetrySleep

	sftpClient, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}

	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/opt/aerolab/scripts/install-ams-complete.sh",
		Source:      strings.NewReader(fullScript),
		Permissions: 0755,
	})
	sftpClient.Close()
	if err != nil {
		return fmt.Errorf("failed to upload installation script: %w", err)
	}

	// Step 3: Execute per-instance configuration in ONE SSH call
	logger.Info("Applying AMS configuration on %s:%d", client.ClusterName, client.NodeNo)

	var stdout, stderr *os.File
	var stdin io.ReadCloser
	terminal := false
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
		terminal = true
	}

	scriptPath := "/opt/aerolab/scripts/install-ams-complete.sh"
	execDetail := sshexec.ExecDetail{
		Command:        []string{"bash", scriptPath},
		SessionTimeout: 30 * time.Minute,
		Terminal:       terminal,
	}
	if system.logLevel >= 5 {
		execDetail.Stdin = stdin
		execDetail.Stdout = stdout
		execDetail.Stderr = stderr
	}

	output := client.Exec(&backends.ExecInput{
		ExecDetail:     execDetail,
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
		MaxRetries:     c.MaxRetries,
		RetrySleep:     c.RetrySleep,
	})

	if output.Output.Err != nil {
		// Save script failure to local machine for debugging
		failure := scriptlog.NewScriptFailureWithPath(
			client.ClusterName,
			client.NodeNo,
			scriptPath,
			[]byte(fullScript),
			output.Output.Stdout,
			output.Output.Stderr,
			output.Output.Err,
		)
		logPath, saveErr := scriptlog.SaveFailure(failure)
		if saveErr != nil {
			return fmt.Errorf("installation failed: %w (stdout: %s, stderr: %s) (also failed to save logs: %v)", output.Output.Err, output.Output.Stdout, output.Output.Stderr, saveErr)
		}
		return fmt.Errorf("%s", scriptlog.FormatError(logPath, client.ClusterName, client.NodeNo, output.Output.Err))
	}

	// Step 4: Install dashboards (parallel downloads for speed)
	logger.Info("Installing dashboards on %s:%d", client.ClusterName, client.NodeNo)
	err = c.installDashboards(system, client, customDashboards, logger)
	if err != nil {
		return fmt.Errorf("failed to install dashboards: %w", err)
	}

	// Step 5: Final service restart in ONE call. Loki is not auto-started on
	// first boot from the template (the Docker service shim refuses
	// `restart` on a stopped service), so start it explicitly if a restart
	// is not possible.
	logger.Info("Restarting services on %s:%d", client.ClusterName, client.NodeNo)
	serviceRestartCmd := "systemctl daemon-reload; systemctl restart prometheus; systemctl restart grafana-server; sleep 3; systemctl restart loki 2>/dev/null || systemctl start loki"
	output = client.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", serviceRestartCmd},
			SessionTimeout: 2 * time.Minute,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
		MaxRetries:     c.MaxRetries,
		RetrySleep:     c.RetrySleep,
	})

	if output.Output.Err != nil {
		// Save script failure to local machine for debugging
		failure := scriptlog.NewScriptFailureWithPath(
			client.ClusterName,
			client.NodeNo,
			"inline:service-restart",
			[]byte(serviceRestartCmd),
			output.Output.Stdout,
			output.Output.Stderr,
			output.Output.Err,
		)
		logPath, saveErr := scriptlog.SaveFailure(failure)
		if saveErr != nil {
			return fmt.Errorf("failed to start services: %w (also failed to save logs: %v)", output.Output.Err, saveErr)
		}
		return fmt.Errorf("%s", scriptlog.FormatError(logPath, client.ClusterName, client.NodeNo, output.Output.Err))
	}

	return nil
}

// buildAMSTemplateInstallScript builds the installation script that is baked
// into an AMS template image. It installs Prometheus, Grafana (with plugins
// and generic datasource provisioning), and Loki, and enables the systemd
// services without starting them. Per-instance configuration (Prometheus
// scrape targets and dashboards) is handled separately when the instance
// is created from the template.
func (c *ClientCreateAMSCmd) buildAMSTemplateInstallScript(archStr string) ([]byte, error) {
	var script bytes.Buffer

	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n")
	script.WriteString("echo 'Starting AMS template installation...'\n\n")

	// Bake the base client tools (curl/wget/vim/git/jq/unzip/zip) so that
	// `client create ams`, which is the only consumer of this template,
	// can skip the per-instance `client create base` step entirely.
	script.WriteString("# ===== Installing base client tools =====\n")
	baseScript, err := installers.GetInstallScript(baseInstallSoftware(false), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get base install script: %w", err)
	}
	script.Write(baseScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\necho 'Base client tools installed'\n\n")

	script.WriteString("# ===== Installing Prometheus =====\n")
	var promVersion *string
	if c.PrometheusVersion != "latest" && c.PrometheusVersion != "" {
		promVersion = &c.PrometheusVersion
	}
	prometheusScript, err := prometheus.GetLinuxInstallScript(promVersion, nil, true, false) // enable but don't start yet
	if err != nil {
		return nil, fmt.Errorf("failed to get Prometheus install script: %w", err)
	}
	script.Write(prometheusScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\necho 'Prometheus installed'\n\n")

	// Install Grafana (from installer package)
	script.WriteString("# ===== Installing Grafana =====\n")
	grafanaVersion := c.GrafanaVersion
	if grafanaVersion == "latest" {
		grafanaVersion = ""
	}
	grafanaScript, err := grafana.GetInstallScript(grafanaVersion, true, false) // enable but don't start yet
	if err != nil {
		return nil, fmt.Errorf("failed to get Grafana install script: %w", err)
	}
	script.Write(grafanaScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\necho 'Grafana installed'\n\n")

	// Configure Grafana (static: plugins, datasource provisioning, systemd override)
	script.WriteString("# ===== Configuring Grafana =====\n")
	script.WriteString(c.buildGrafanaConfig())
	script.WriteString("\necho 'Grafana configured'\n\n")

	// Install Loki
	script.WriteString("# ===== Installing Loki =====\n")
	script.WriteString(c.buildLokiInstallForArch(archStr))
	script.WriteString("\necho 'Loki installed'\n\n")

	// Bake static content that was previously fetched per-instance from GitHub:
	// Prometheus rules file and skeleton edits, Grafana dashboards directory,
	// and the Grafana dashboard provisioning file.
	script.WriteString("# ===== Baking static Prometheus/Grafana content =====\n")
	script.WriteString(c.buildPrometheusTemplateSkeleton())
	script.WriteString("mkdir -p /var/lib/grafana/dashboards\n")
	script.WriteString("(wget -q -O /etc/grafana/provisioning/dashboards/all.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/dashboards/all.yaml || { sleep 1; wget -q -O /etc/grafana/provisioning/dashboards/all.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/dashboards/all.yaml; })\n")
	script.WriteString("echo 'Static Prometheus/Grafana content baked'\n\n")

	// Enable systemd services (but don't start them - first start happens on instance creation)
	script.WriteString("# ===== Enabling systemd services =====\n")
	script.WriteString("systemctl enable loki\n")
	script.WriteString("systemctl enable grafana-server\n")
	script.WriteString("systemctl enable prometheus\n")
	script.WriteString("echo 'Services enabled'\n\n")

	script.WriteString("echo 'AMS template installation complete!'\n")
	return script.Bytes(), nil
}

// buildAMSInstanceConfigScript builds the per-instance configuration script
// run on top of a client instance booted from an AMS template. It only
// configures Prometheus scrape targets; dashboards and service restart are
// handled separately in installAMS.
func (c *ClientCreateAMSCmd) buildAMSInstanceConfigScript(clusterNodes, clientNodes map[string][]string) (string, error) {
	var script bytes.Buffer

	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n")
	script.WriteString("echo 'Applying AMS per-instance configuration...'\n\n")

	script.WriteString("# ===== Configuring Prometheus scrape targets =====\n")
	script.WriteString(c.buildPrometheusTargetsConfig(clusterNodes, clientNodes))
	script.WriteString("\necho 'Prometheus scrape targets configured'\n\n")

	script.WriteString("echo 'AMS per-instance configuration complete!'\n")
	return script.String(), nil
}

// buildPrometheusTemplateSkeleton generates the user-agnostic Prometheus
// setup commands that are baked into the AMS template image: download of the
// aerospike_rules.yaml file and the structural edits to prometheus.yml that
// add the rule_files entry, rename the local node job, and seed the three
// scrape-config stubs (#TODO_ASD_TARGETS / #TODO_ASDN_TARGETS /
// #TODO_CLIENT_TARGETS) that per-instance configuration later fills in.
func (c *ClientCreateAMSCmd) buildPrometheusTemplateSkeleton() string {
	var script bytes.Buffer

	// Download aerospike rules (with retry)
	script.WriteString("(wget -q -O /etc/prometheus/aerospike_rules.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/prometheus/aerospike_rules.yml || { sleep 1; wget -q -O /etc/prometheus/aerospike_rules.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/prometheus/aerospike_rules.yml; })\n")
	script.WriteString("sed -i.bak -E 's/^rule_files:/rule_files:\\n  - \"\\/etc\\/prometheus\\/aerospike_rules.yaml\"/g' /etc/prometheus/prometheus.yml\n")
	script.WriteString("sed -i.bak -E 's/- job_name: node/- job_name: nodelocal/g' /etc/prometheus/prometheus.yml\n")
	script.WriteString("sed -i.bak -E 's/^scrape_configs:/scrape_configs:\\n  - job_name: aerospike\\n    static_configs:\\n      - targets: [] #TODO_ASD_TARGETS\\n  - job_name: node\\n    static_configs:\\n      - targets: [] #TODO_ASDN_TARGETS\\n  - job_name: clients\\n    static_configs:\\n      - targets: [] #TODO_CLIENT_TARGETS\\n/g' /etc/prometheus/prometheus.yml\n")

	return script.String()
}

// buildPrometheusTargetsConfig generates the per-instance Prometheus scrape
// target sed edits against the skeleton baked in by
// buildPrometheusTemplateSkeleton. Targets are filled in for the
// #TODO_ASD_TARGETS / #TODO_ASDN_TARGETS / #TODO_CLIENT_TARGETS placeholders.
func (c *ClientCreateAMSCmd) buildPrometheusTargetsConfig(clusterNodes, clientNodes map[string][]string) string {
	var script bytes.Buffer

	if len(clusterNodes) > 0 {
		asdTargets := []string{}
		nodeTargets := []string{}
		for _, nodes := range clusterNodes {
			for _, node := range nodes {
				asdTargets = append(asdTargets, node+":9145")
				nodeTargets = append(nodeTargets, node+":9100")
			}
		}
		fmt.Fprintf(&script, "sed -i.bakAsd -E \"s/.*TODO_ASD_TARGETS/      - targets: ['%s'] #TODO_ASD_TARGETS/g\" /etc/prometheus/prometheus.yml\n", strings.Join(asdTargets, "','"))        //nolint:errcheck
		fmt.Fprintf(&script, "sed -i.bakAsdNode -E \"s/.*TODO_ASDN_TARGETS/      - targets: ['%s'] #TODO_ASDN_TARGETS/g\" /etc/prometheus/prometheus.yml\n", strings.Join(nodeTargets, "','")) //nolint:errcheck
	}

	if len(clientNodes) > 0 {
		clientTargets := []string{}
		for _, nodes := range clientNodes {
			for _, node := range nodes {
				clientTargets = append(clientTargets, node+":9090")
			}
		}
		fmt.Fprintf(&script, "sed -i.bakGraph -E \"s/.*TODO_CLIENT_TARGETS/      - targets: ['%s'] #TODO_CLIENT_TARGETS/g\" /etc/prometheus/prometheus.yml\n", strings.Join(clientTargets, "','")) //nolint:errcheck
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

# Configure datasources (with retry)
(wget -q -O /etc/grafana/provisioning/datasources/all.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/datasources/all.yaml || { sleep 1; wget -q -O /etc/grafana/provisioning/datasources/all.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/datasources/all.yaml; })
echo -e '  - name: Loki\n    type: loki\n    access: proxy\n    url: http://localhost:3100\n    jsonData:\n      maxLines: 1000\n' >> /etc/grafana/provisioning/datasources/all.yaml
sed -i.bak 's/prometheus:9090/127.0.0.1:9090/g' /etc/grafana/provisioning/datasources/all.yaml
`
}

// buildLokiInstallForArch generates Loki installation commands for the given
// architecture string (either "amd64" or "arm64").
func (c *ClientCreateAMSCmd) buildLokiInstallForArch(arch string) string {
	if arch != "arm64" {
		arch = "amd64"
	}

	return fmt.Sprintf(`# Install Loki
cd /root
(wget -q https://github.com/grafana/loki/releases/download/v3.3.0/loki-linux-%[1]s.zip || { sleep 1; wget -q https://github.com/grafana/loki/releases/download/v3.3.0/loki-linux-%[1]s.zip; })
unzip -q loki-linux-%[1]s.zip
mv loki-linux-%[1]s /usr/bin/loki
(wget -q https://github.com/grafana/loki/releases/download/v3.3.0/logcli-linux-%[1]s.zip || { sleep 1; wget -q https://github.com/grafana/loki/releases/download/v3.3.0/logcli-linux-%[1]s.zip; })
unzip -q logcli-linux-%[1]s.zip
mv logcli-linux-%[1]s /usr/bin/logcli
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
`, arch)
}

// installDashboards installs Grafana dashboards on a running AMS client
// instance. The AMS template image already ships the default dashboards
// and the dashboard provisioning file baked in, so this function only
// handles the per-instance bits: honoring the `--dashboards` replace
// semantics (remove baked-in defaults when the flag is set without the
// `+` prefix) and installing any custom dashboards.
func (c *ClientCreateAMSCmd) installDashboards(system *System, client *backends.Instance, customDashboards []CustomAMSDashboard, logger *logger.Logger) error {
	wantDefaults := c.Dashboards == "" || strings.HasPrefix(string(c.Dashboards), "+")

	if !wantDefaults {
		// `--dashboards <path>` (no `+` prefix) means "replace defaults":
		// wipe the dashboards that the template baked in so the custom set
		// stands alone.
		if c.DebugDashboards {
			logger.Debug("Removing baked-in default dashboards to honor --dashboards replace semantics")
		}
		rmCmd := "rm -f /var/lib/grafana/dashboards/*.json"
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", rmCmd},
				SessionTimeout: 30 * time.Second,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
			MaxRetries:     c.MaxRetries,
			RetrySleep:     c.RetrySleep,
		})
		if output.Output.Err != nil {
			failure := scriptlog.NewScriptFailureWithPath(
				client.ClusterName,
				client.NodeNo,
				"inline:dashboard-replace-cleanup",
				[]byte(rmCmd),
				output.Output.Stdout,
				output.Output.Stderr,
				output.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				return fmt.Errorf("failed to remove baked-in default dashboards: %w (also failed to save logs: %v)", output.Output.Err, saveErr)
			}
			return fmt.Errorf("%s", scriptlog.FormatError(logPath, client.ClusterName, client.NodeNo, output.Output.Err))
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

// installAMSDefaultDashboards downloads the default aerospike-monitoring
// dashboards from GitHub onto the given instance. It is called once per
// AMS template image at template-build time to bake the dashboards into
// the image; per-instance `client create ams` invocations reuse the
// baked set.
func (c *ClientCreateAMSCmd) installAMSDefaultDashboards(client *backends.Instance, logger *logger.Logger) error {
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
		mkdirScript := strings.Join(mkdirCmds, " && ")
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", mkdirScript},
				SessionTimeout: time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
			MaxRetries:     c.MaxRetries,
			RetrySleep:     c.RetrySleep,
		})
		if output.Output.Err != nil {
			// Save script failure to local machine for debugging
			failure := scriptlog.NewScriptFailureWithPath(
				client.ClusterName,
				client.NodeNo,
				"inline:dashboard-mkdir",
				[]byte(mkdirScript),
				output.Output.Stdout,
				output.Output.Stderr,
				output.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				return fmt.Errorf("failed to create dashboard directories: %w (also failed to save logs: %v)", output.Output.Err, saveErr)
			}
			return fmt.Errorf("%s", scriptlog.FormatError(logPath, client.ClusterName, client.NodeNo, output.Output.Err))
		}
	}

	// Download dashboards in parallel (this is the expensive part, so we parallelize it)
	if c.DebugDashboards {
		logger.Debug("Downloading %d dashboards in parallel", len(dashboards))
	}

	errors := parallelize.MapLimit(dashboards, c.ParallelSSHThreads, func(dashboard []string) error {
		for tries := range 3 {
			output := client.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        dashboard,
					SessionTimeout: time.Minute,
				},
				Username:       "root",
				ConnectTimeout: 30 * time.Second,
				MaxRetries:     c.MaxRetries,
				RetrySleep:     c.RetrySleep,
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
		dirScript := strings.Join(dirCmds, " && ")
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", dirScript},
				SessionTimeout: 30 * time.Second,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
			MaxRetries:     c.MaxRetries,
			RetrySleep:     c.RetrySleep,
		})
		if output.Output.Err != nil {
			// Save script failure to local machine for debugging
			failure := scriptlog.NewScriptFailureWithPath(
				client.ClusterName,
				client.NodeNo,
				"inline:custom-dashboard-dirs",
				[]byte(dirScript),
				output.Output.Stdout,
				output.Output.Stderr,
				output.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				return fmt.Errorf("failed to create directories: %w (also failed to save logs: %v)", output.Output.Err, saveErr)
			}
			return fmt.Errorf("%s", scriptlog.FormatError(logPath, client.ClusterName, client.NodeNo, output.Output.Err))
		}
	}

	// Upload files via SFTP (we can't parallelize SFTP, but we can batch the uploads)
	conf, err := client.GetSftpConfig("root")
	if err != nil {
		return err
	}
	conf.MaxRetries = c.MaxRetries
	conf.RetrySleep = c.RetrySleep
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

// resolveAMSTemplate finds an AMS template image in the inventory and points
// the base client creation at it. If no template is found for the target
// architecture, one is auto-created via `client template ams create`.
// If more than one template exists, the highest generation is used and a
// warning is logged recommending `client template ams cleanup`.
//
// If the caller has explicitly pre-set c.AWS.ImageID / c.GCP.ImageName /
// c.Docker.ImageName, no template resolution is performed (manual override).
func (c *ClientCreateAMSCmd) resolveAMSTemplate(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	backendType := system.Opts.Config.Backend.Type

	// If an explicit image was specified by the user, don't override it.
	switch backendType {
	case "aws":
		if c.AWS.ImageID != "" {
			logger.Info("AWS image override already set (%s); skipping AMS template resolution", c.AWS.ImageID)
			return nil
		}
	case "gcp":
		if c.GCP.ImageName != "" {
			logger.Info("GCP image override already set (%s); skipping AMS template resolution", c.GCP.ImageName)
			return nil
		}
	case "docker":
		if c.Docker.ImageName != "" {
			logger.Info("Docker image override already set (%s); skipping AMS template resolution", c.Docker.ImageName)
			return nil
		}
	}

	arch, archStr, err := c.resolveAMSArch(system)
	if err != nil {
		return err
	}

	images := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "ams"}).WithArchitecture(arch)
	var templateName string
	// resolvedImage points at the AMS template we end up using, so we can
	// size the client's root volume to be at least as large as the backing
	// snapshot. AWS rejects launching from a 30GiB snapshot onto a 20GiB
	// root volume (InvalidBlockDeviceMapping); GCP behaves the same way.
	var resolvedImage *backends.Image
	switch images.Count() {
	case 0:
		logger.Info("No AMS template found for %s; creating one...", archStr)
		templateCreate := &ClientTemplateAMSCreateCmd{
			GrafanaVersion:     c.GrafanaVersion,
			PrometheusVersion:  c.PrometheusVersion,
			Distro:             "ubuntu",
			DistroVersion:      "24.04",
			Arch:               archStr,
			Timeout:            30,
			Owner:              c.Owner,
			DisablePublicIP:    c.AWS.DisablePublicIP,
			GCPDisablePublicIP: c.GCP.DisablePublicIP,
			GCPVPC:             string(c.GCP.VPC),
			GCPSubnet:          c.GCP.Subnet,
			MaxRetries:         c.MaxRetries,
			RetrySleep:         c.RetrySleep,
		}
		name, err := templateCreate.CreateTemplate(system, inventory, logger.WithPrefix("[template] "), args, false)
		if err != nil {
			return fmt.Errorf("failed to auto-create AMS template: %w", err)
		}
		// Refresh inventory so the newly created image is visible to downstream
		// instance creation, then resolve the image's canonical inventory name.
		// (The Docker backend stores images with a `:latest` tag suffix, which
		// `CreateTemplate` does not include in the value it returns. Using the
		// raw returned name would cause `WithName` exact-match lookups in
		// `InstancesCreateCmd` to miss and fall through to a placeholder
		// `Image{}` with a zero-value Architecture, which in turn makes Docker
		// try to run the arm64 image as linux/amd64.)
		system.Backend.ForceRefreshInventory()
		refreshed := system.Backend.GetInventory()
		templateName = resolveCanonicalAMSTemplateName(refreshed, arch, name)
		if templateName == "" {
			return fmt.Errorf("auto-created AMS template %q not visible in inventory after refresh", name)
		}
		imgs := refreshed.Images.
			WithTags(map[string]string{"aerolab.image.type": "ams"}).
			WithArchitecture(arch).
			WithName(templateName).
			Describe()
		if len(imgs) > 0 {
			resolvedImage = imgs[0]
		}
	case 1:
		resolvedImage = images.Describe()[0]
		templateName = resolvedImage.Name
		logger.Info("Using AMS template: %s", templateName)
		logger.Info("Using existing AMS template. To refresh the template and get the latest dashboards, run: aerolab client template ams refresh")
	default:
		// More than one template: pick the highest generation.
		best := images.Describe()[0]
		bestGen, _ := strconv.Atoi(best.Tags["aerolab.ams.generation"])
		for _, img := range images.Describe()[1:] {
			gen, err := strconv.Atoi(img.Tags["aerolab.ams.generation"])
			if err != nil {
				continue
			}
			if gen > bestGen {
				best = img
				bestGen = gen
			}
		}
		resolvedImage = best
		templateName = best.Name
		names := []string{}
		for _, img := range images.Describe() {
			gen := img.Tags["aerolab.ams.generation"]
			if gen == "" {
				gen = "?"
			}
			names = append(names, img.Name+" (generation="+gen+")")
		}
		sort.Strings(names)
		logger.Warn("Found %d AMS templates for %s: %s. Using the highest generation (%s, generation=%d). Run `aerolab client template ams cleanup` to remove the superseded ones.", images.Count(), archStr, strings.Join(names, ", "), templateName, bestGen)
		logger.Info("Using existing AMS template. To refresh the template and get the latest dashboards, run: aerolab client template ams refresh")
	}

	// Point the base client creation at the resolved template image and
	// propagate the resolved architecture so the base client create command
	// creates a matching-platform instance (otherwise Docker, whose default
	// platform is linux/amd64, would refuse to run an arm64 template image
	// on an arm64 host).
	switch backendType {
	case "aws":
		c.AWS.ImageID = templateName
	case "gcp":
		c.GCP.ImageName = templateName
	case "docker":
		c.Docker.ImageName = templateName
	}
	if c.Arch == "" {
		c.Arch = archStr
	}

	// Ensure the client's root volume is at least as large as the AMS
	// template snapshot. Without this, the default 20GiB --disk on cloud
	// backends fails with InvalidBlockDeviceMapping because the AMS
	// template is built with a 30GiB root volume.
	if resolvedImage != nil && resolvedImage.Size > 0 {
		templateSizeGiB := int64(resolvedImage.Size / backends.StorageGiB)
		if templateSizeGiB > 0 {
			switch backendType {
			case "aws":
				updated, changed, err := ensureRootDiskMinSizeGiB(c.AWS.Disks, templateSizeGiB)
				if err != nil {
					return fmt.Errorf("could not adjust --aws-disk for AMS template: %w", err)
				}
				if changed {
					logger.Info("Resizing AWS root volume to %dGiB to match AMS template snapshot", templateSizeGiB)
					c.AWS.Disks = updated
				}
			case "gcp":
				updated, changed, err := ensureRootDiskMinSizeGiB(c.GCP.Disks, templateSizeGiB)
				if err != nil {
					return fmt.Errorf("could not adjust --gcp-disk for AMS template: %w", err)
				}
				if changed {
					logger.Info("Resizing GCP root volume to %dGiB to match AMS template snapshot", templateSizeGiB)
					c.GCP.Disks = updated
				}
			}
		}
	}
	return nil
}

// ensureRootDiskMinSizeGiB returns disks with the first ("root") entry's
// size= field bumped to at least minGiB. Returns the (possibly updated)
// slice and a flag indicating whether a change was made. Disks without an
// explicit size= attribute (Docker bind mounts, cloud disks letting the
// backend pick a default) are left untouched. Subsequent ("data") disks
// are not modified.
func ensureRootDiskMinSizeGiB(disks []string, minGiB int64) ([]string, bool, error) {
	if len(disks) == 0 || minGiB <= 0 {
		return disks, false, nil
	}
	parts := strings.Split(disks[0], ",")
	changed := false
	for i, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 || strings.ToLower(strings.TrimSpace(kv[0])) != "size" {
			continue
		}
		size, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("invalid size in disk %q: %w", disks[0], err)
		}
		if size < minGiB {
			parts[i] = fmt.Sprintf("size=%d", minGiB)
			changed = true
		}
		break
	}
	if !changed {
		return disks, false, nil
	}
	out := make([]string, len(disks))
	copy(out, disks)
	out[0] = strings.Join(parts, ",")
	return out, true, nil
}

// resolveCanonicalAMSTemplateName finds the canonical inventory name for an
// AMS template image we just auto-created. It accepts the name that
// `CreateTemplate` returned (e.g. `ams-tmpl-xxx`) and matches it against
// inventory entries for the given architecture, tolerating backend-imposed
// tag suffixes like Docker's `:latest`. Returns "" if no match was found.
func resolveCanonicalAMSTemplateName(inventory *backends.Inventory, arch backends.Architecture, createdName string) string {
	images := inventory.Images.
		WithTags(map[string]string{"aerolab.image.type": "ams"}).
		WithArchitecture(arch).
		Describe()
	for _, img := range images {
		if img.Name == createdName || strings.HasPrefix(img.Name, createdName+":") {
			return img.Name
		}
	}
	return ""
}

// resolveAMSArch determines the target architecture for the AMS client,
// mirroring the AGI create logic (backend-specific: AWS/GCP derive from
// InstanceType when possible, Docker uses backend config or runtime, falling
// back to amd64).
func (c *ClientCreateAMSCmd) resolveAMSArch(system *System) (backends.Architecture, string, error) {
	var arch backends.Architecture

	// Explicit user override via --arch always wins.
	if c.Arch != "" {
		if err := arch.FromString(c.Arch); err != nil {
			return arch, "", fmt.Errorf("invalid architecture %q: %w", c.Arch, err)
		}
		return arch, arch.String(), nil
	}

	switch system.Opts.Config.Backend.Type {
	case "docker":
		ar := system.Opts.Config.Backend.Arch
		if ar == "" {
			ar = runtime.GOARCH
		}
		arch.FromString(ar) //nolint:errcheck
	case "aws":
		if c.AWS.InstanceType != "" {
			itypes, err := system.Backend.GetInstanceTypes(backends.BackendTypeAWS)
			if err == nil {
				for _, i := range itypes {
					if i.Name == string(c.AWS.InstanceType) && len(i.Arch) > 0 {
						arch.FromString(i.Arch[0].String()) //nolint:errcheck
						break
					}
				}
			}
		}
	case "gcp":
		if c.GCP.InstanceType != "" {
			itypes, err := system.Backend.GetInstanceTypes(backends.BackendTypeGCP)
			if err == nil {
				for _, i := range itypes {
					if i.Name == string(c.GCP.InstanceType) && len(i.Arch) > 0 {
						arch.FromString(i.Arch[0].String()) //nolint:errcheck
						break
					}
				}
			}
		}
	}

	if arch.String() == "" {
		arch.FromString("amd64") //nolint:errcheck
	}
	return arch, arch.String(), nil
}
