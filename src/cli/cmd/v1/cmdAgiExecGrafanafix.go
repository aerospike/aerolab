package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/grafanafix"
)

// AgiExecGrafanaFixCmd runs the Grafana helper service.
// This is a hidden command that runs inside AGI instances, not called by users directly.
// It configures Grafana with the AGI datasource plugin and dashboards, and periodically saves annotations.
type AgiExecGrafanaFixCmd struct {
	YamlFile string  `short:"y" long:"yaml" description:"Path to YAML config file" default:"/opt/agi/grafanafix.yaml"`
	Help     HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs the Grafana helper service.
// This function performs the following:
//  1. Stops Grafana if running
//  2. Applies early patches to grafana.ini (anonymous auth, timeout settings, etc.)
//  3. Installs the JSON datasource plugin for AGI
//  4. Starts Grafana
//  5. Imports dashboards and sets home dashboard
//  6. Loads existing annotations
//  7. Enters a loop that periodically saves annotations
//
// The function runs indefinitely, saving annotations every 5 minutes.
//
// Returns:
//   - error: nil on success (never returns under normal operation), or an error describing what failed
func (c *AgiExecGrafanaFixCmd) Execute(args []string) error {
	cmd := []string{"agi", "exec", "grafanafix"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Ensure /opt/agi directory exists
	os.MkdirAll("/opt/agi", 0755)

	// Write PID file for process management
	os.WriteFile("/opt/agi/grafanafix.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/grafanafix.pid")

	// Load configuration
	conf := new(grafanafix.GrafanaFix)
	yamlFile := c.YamlFile
	if _, err := os.Stat(yamlFile); !os.IsNotExist(err) {
		f, err := os.Open(yamlFile)
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
		conf, err = grafanafix.MakeConfig(true, f, true)
		f.Close()
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
	} else {
		// Use defaults if file doesn't exist
		var err error
		conf, err = grafanafix.MakeConfig(true, nil, true)
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
	}

	// Stop Grafana if running
	log.Print("Stopping grafana")
	time.Sleep(1 * time.Second)
	exec.Command("service", "grafana-server", "stop").CombinedOutput()
	time.Sleep(1 * time.Second)

	// Wait for grafana to fully stop
	waited := 0
	for {
		_, err = exec.Command("pidof", "grafana").CombinedOutput()
		if err != nil {
			// Grafana has stopped
			break
		}
		exec.Command("service", "grafana-server", "stop").CombinedOutput()
		time.Sleep(time.Second)
		waited++
		if waited > 60 {
			break
		}
	}

	// Apply early patch to grafana.ini and install datasource plugin
	log.Print("Early patch")
	err = grafanafix.EarlySetup("/etc/grafana/grafana.ini", "/etc/grafana/provisioning", "/var/lib/grafana/plugins", "", 0)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Start Grafana
	log.Print("Starting grafana")
	out, err := exec.Command("service", "grafana-server", "start").CombinedOutput()
	if err != nil {
		errstr := fmt.Sprintf("%s\n%s", string(out), err)
		var pid []byte
		retries := 0
		for {
			pid, _ = os.ReadFile("/var/run/grafana-server.pid")
			if len(pid) > 0 {
				break
			}
			if retries > 59 {
				return Error(errors.New(errstr), system, cmd, c, args)
			}
			retries++
			time.Sleep(time.Second)
		}
		pidi, err := strconv.Atoi(strings.TrimSpace(string(pid)))
		if err != nil {
			return Error(fmt.Errorf("(%s): %s", err, errstr), system, cmd, c, args)
		}
		_, err = os.FindProcess(pidi)
		if err != nil {
			return Error(fmt.Errorf("(%s): %s", err, errstr), system, cmd, c, args)
		}
	}

	// Run grafanafix (this function runs indefinitely)
	log.Print("Running grafanafix")
	grafanafix.Run(conf)

	// This should never be reached under normal operation
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

