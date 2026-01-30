package cmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/agi/plugin"
)

// AgiExecPluginCmd runs the Grafana plugin backend service.
// This is a hidden command that runs inside AGI instances, not called by users directly.
// The plugin provides a JSON datasource for Grafana that queries data from Aerospike.
type AgiExecPluginCmd struct {
	YamlFile string  `short:"y" long:"yaml" description:"Path to YAML config file" default:"/opt/agi/plugin.yaml"`
	Help     HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs the plugin backend service.
// It loads configuration from the YAML file (with environment variable overrides),
// initializes the plugin, writes a PID file for process management,
// and starts the HTTP server for Grafana to connect to.
//
// The plugin listens on the configured address (default: 0.0.0.0:8850) and serves:
//   - /metrics - Available metrics for Grafana queries
//   - /metric-payload-options - Metric configuration options
//   - /query - Main query endpoint for Grafana panels
//   - /variable - Variable queries for Grafana templates
//   - /tag-keys - Available tag keys for filtering
//   - /tag-values - Tag values for specific keys
//   - /histogram - Histogram data queries
//   - /shutdown - Graceful shutdown endpoint
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiExecPluginCmd) Execute(args []string) error {
	cmd := []string{"agi", "exec", "plugin"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Ensure /opt/agi directory exists
	os.MkdirAll("/opt/agi", 0755)

	// Write PID file for process management
	os.WriteFile("/opt/agi/plugin.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/plugin.pid")

	// Load configuration from YAML file with environment variable overrides
	yamlFile := c.YamlFile
	if _, err := os.Stat(yamlFile); os.IsNotExist(err) {
		yamlFile = "" // Use defaults if file doesn't exist
	}
	conf, err := plugin.MakeConfig(true, yamlFile, true)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Initialize plugin with database connection
	p, err := plugin.Init(conf)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Starting plugin HTTP server on %s:%d", conf.Service.ListenAddress, conf.Service.ListenPort)

	// Start HTTP server (blocks until shutdown)
	err = p.Listen()

	// Cleanup
	p.Close()

	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

