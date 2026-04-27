//go:build !noagi

package cmd

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/aerospike/aerolab/pkg/agi/plugin"
)

// AgiExecPluginCmd runs the Grafana plugin backend service.
// This is a hidden command that runs inside AGI instances, not called by users directly.
// The plugin provides a JSON datasource for Grafana that queries data from the
// embedded DB (pkg/agi/db).
type AgiExecPluginCmd struct {
	YamlFile string `short:"y" long:"yaml" description:"Path to YAML config file" default:"/opt/agi/plugin.yaml"`
	// sharedDB, if non-nil, is used instead of opening a new Pebble
	// handle. It is set by cmdAgiExecService when the plugin runs in
	// the same process as the ingest pipeline; the db is then owned
	// by the service command and plugin's Close does not close it.
	sharedDB *db.DB
	Help     HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs the plugin backend service.
// It loads configuration from the YAML file (with environment variable overrides),
// initializes the plugin, writes a PID file for process management,
// and starts the HTTP server for Grafana to connect to.
//
// The plugin listens on the configured address (default: 127.0.0.1:8851) and serves:
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
	//nolint:errcheck
	os.MkdirAll("/opt/agi", 0755)

	// Write PID file for process management
	//nolint:errcheck
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

	// Initialize plugin with database connection.
	var p *plugin.Plugin
	if c.sharedDB != nil {
		p, err = plugin.InitWithDB(conf, c.sharedDB)
	} else {
		p, err = plugin.Init(conf)
	}
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Install a signal handler so SIGTERM/SIGINT flush the HTTP
	// server's in-flight work before the process is killed. Without
	// this a systemd `stop` would kill the process while a query is
	// still writing its response, and — when WAL is off — any still
	// buffered db writes from the plugin's label refresher would be
	// lost. Delivery is idempotent; once per signal is enough.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig, ok := <-sigCh
		if !ok {
			return
		}
		system.Logger.Info("Plugin: received %s, draining HTTP server", sig)
		p.Shutdown()
	}()

	system.Logger.Info("Starting plugin HTTP server on %s:%d", conf.Service.ListenAddress, conf.Service.ListenPort)

	// Start HTTP server (blocks until shutdown)
	err = p.Listen()

	// Stop forwarding signals once the server has returned.
	signal.Stop(sigCh)
	close(sigCh)

	// Cleanup
	p.Close()

	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}
