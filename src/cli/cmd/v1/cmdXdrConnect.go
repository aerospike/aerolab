package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type XdrConnectCmd struct {
	SourceClusterName       TypeClusterName `short:"S" long:"source" description:"Source Cluster name" default:"mydc"`
	DestinationClusterNames TypeClusterName `short:"D" long:"destinations" description:"Destination Cluster names, comma separated" default:"destdc"`
	IsConnector             bool            `short:"c" long:"connector" description:"Set to indicate that the destination is a client connector, not a cluster"`
	Version                 TypeXDRVersion  `short:"V" long:"xdr-version" description:"Specify aerospike xdr configuration version (4|5|auto)" default:"auto" webchoice:"auto,5,4"`
	Restart                 TypeYesNo       `short:"T" long:"restart-source" description:"Restart source nodes after connecting (y/n)" default:"y" webchoice:"y,n"`
	Namespaces              string          `short:"M" long:"namespaces" description:"Comma-separated list of namespaces to connect" default:"test"`
	CustomDestinationPort   int             `short:"P" long:"destination-port" description:"Optionally specify a custom destination port for the xdr connection"`
	ParallelThreads         int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help                    HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *XdrConnectCmd) Execute(args []string) error {
	cmd := []string{"xdr", "connect"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.connect(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *XdrConnectCmd) connect(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if c.Restart != "n" && c.Restart != "y" {
		return errors.New("restart-source option only accepts 'y' or 'n'")
	}

	namespaces := strings.Split(c.Namespaces, ",")
	destinations := strings.Split(c.DestinationClusterNames.String(), ",")

	// Get source cluster
	sourceCluster := inventory.Instances.WithClusterName(c.SourceClusterName.String()).WithState(backends.LifeCycleStateRunning)
	if sourceCluster == nil || sourceCluster.Count() == 0 {
		return fmt.Errorf("source cluster %s not found or has no running instances", c.SourceClusterName.String())
	}

	// Get destination IPs for each destination cluster
	destIpList := make(map[string][]string)
	for _, destination := range destinations {
		var destCluster backends.Instances
		if c.IsConnector {
			// For connectors, filter by client tag
			destCluster = inventory.Instances.WithClusterName(destination).WithState(backends.LifeCycleStateRunning).WithTags(map[string]string{"aerolab.old.type": "client"})
		} else {
			destCluster = inventory.Instances.WithClusterName(destination).WithState(backends.LifeCycleStateRunning)
		}

		if destCluster == nil || destCluster.Count() == 0 {
			return fmt.Errorf("destination cluster %s not found or has no running instances", destination)
		}

		var destIps []string
		for _, inst := range destCluster.Describe() {
			// For docker backend, check for exposed ports
			if system.Opts.Config.Backend.Type == string(backends.BackendTypeDocker) {
				if exposedPorts, ok := inst.Tags["aerolab.docker.expose-ports"]; ok && exposedPorts != "" {
					destIps = append(destIps, inst.IP.Private+" "+exposedPorts)
				} else {
					destIps = append(destIps, inst.IP.Private)
				}
			} else {
				destIps = append(destIps, inst.IP.Private)
			}
		}
		destIpList[destination] = destIps
	}

	// Create /opt/aerospike/xdr directory on all source nodes
	logger.Info("Creating XDR directory on source nodes")
	var hasErr error
	parallelize.ForEachLimit(sourceCluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		output := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"mkdir", "-p", "/opt/aerospike/xdr"},
				SessionTimeout: time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: failed to create XDR directory: %w", inst.ClusterName, inst.NodeNo, output.Output.Err))
		}
	})
	if hasErr != nil {
		return hasErr
	}

	// Configure XDR on each source node
	logger.Info("Configuring XDR on source nodes")
	parallelize.ForEachLimit(sourceCluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		err := c.configureXdrOnNode(inst, destIpList, destinations, namespaces, logger)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", inst.ClusterName, inst.NodeNo, err))
		}
	})
	if hasErr != nil {
		return hasErr
	}

	// Restart source cluster if requested
	if c.Restart == "y" {
		logger.Info("Restarting source cluster nodes")
		restartCmd := &AerospikeRestartCmd{
			ClusterName: c.SourceClusterName,
			Threads:     c.ParallelThreads,
		}
		_, err := restartCmd.RestartAerospike(system, inventory, logger, args, "restart")
		if err != nil {
			return fmt.Errorf("failed to restart source cluster: %w", err)
		}
	} else {
		logger.Info("Aerospike on source has NOT been restarted, changes not yet in effect")
	}

	return nil
}

func (c *XdrConnectCmd) configureXdrOnNode(inst *backends.Instance, destIpList map[string][]string, destinations []string, namespaces []string, logger *logger.Logger) error {
	// Determine XDR version
	xdrVersion := c.Version.String()
	if xdrVersion == "auto" {
		// Check aerospike version
		output := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"cat", "/opt/aerolab.aerospike.version"},
				SessionTimeout: 10 * time.Second,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to detect aerospike version: %w", output.Output.Err)
		}
		version := strings.TrimSpace(string(output.Output.Stdout))
		if strings.HasPrefix(version, "4.") || strings.HasPrefix(version, "3.") {
			xdrVersion = "4"
		} else {
			xdrVersion = "5"
		}
	}

	if c.IsConnector && xdrVersion == "4" {
		return errors.New("for connector setup, only use server versions 5+")
	}

	// Read aerospike.conf
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	client, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer client.Close()

	var buf bytes.Buffer
	err = client.ReadFile(&sshexec.FileReader{
		SourcePath:  "/etc/aerospike/aerospike.conf",
		Destination: &buf,
	})
	if err != nil {
		return fmt.Errorf("failed to read aerospike.conf: %w", err)
	}

	// Parse configuration
	confData := buf.String()

	// Add basic XDR stanza if not present
	if !strings.Contains(confData, "xdr {") {
		if xdrVersion == "5" {
			confData += "\nxdr {\n\n}\n"
		} else {
			confData += "\nxdr {\n    enable-xdr true\n    xdr-digestlog-path /opt/aerospike/xdr/digestlog 1G\n}\n"
		}
	}

	// Modify configuration using string manipulation (similar to original implementation)
	finalConf, err := c.modifyXdrConfig(confData, xdrVersion, destIpList, destinations, namespaces)
	if err != nil {
		return fmt.Errorf("failed to modify XDR config: %w", err)
	}

	// Write back configuration
	err = client.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/etc/aerospike/aerospike.conf",
		Source:      strings.NewReader(finalConf),
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to write aerospike.conf: %w", err)
	}

	return nil
}

func (c *XdrConnectCmd) modifyXdrConfig(conf string, xdrVersion string, destIpList map[string][]string, destinations []string, namespaces []string) (string, error) {
	dcStanzaName := "datacenter"
	nodeAddressPort := "dc-node-address-port"
	if xdrVersion == "5" {
		dcStanzaName = "dc"
		nodeAddressPort = "node-address-port"
	}

	// Split config into lines
	confs := strings.Split(conf, "\n")
	for i := range confs {
		confs[i] = strings.Trim(confs[i], "\r")
	}

	// Find XDR stanza boundaries and existing DCs
	xdrStart := -1
	xdrEnd := -1
	lvl := 0
	var xdrDcs []string

	for i := 0; i < len(confs); i++ {
		if strings.Contains(confs[i], "xdr {") {
			xdrStart = i
			lvl = 1
		} else if strings.Contains(confs[i], "{") && xdrStart != -1 {
			lvl += strings.Count(confs[i], "{")
		} else if strings.Contains(confs[i], "}") && xdrStart != -1 {
			lvl -= strings.Count(confs[i], "}")
		}

		if strings.Contains(confs[i], dcStanzaName+" ") && xdrStart != -1 && strings.HasSuffix(confs[i], "{") {
			tmp := strings.Split(confs[i], " ")
			for j := 0; j < len(tmp); j++ {
				if strings.Contains(tmp[j], dcStanzaName) && j+1 < len(tmp) {
					xdrDcs = append(xdrDcs, tmp[j+1])
					break
				}
			}
		}

		if lvl == 0 && xdrStart != -1 {
			xdrEnd = i
			break
		}
	}

	if xdrStart == -1 || xdrEnd == -1 {
		return "", fmt.Errorf("could not find XDR stanza in config")
	}

	// Build DC configuration to add
	dcToAdd := ""
	dc2namespace := make(map[string][]string)

	for _, dest := range destinations {
		// Check if DC already exists
		found := false
		for _, existingDc := range xdrDcs {
			if dest == existingDc {
				found = true
				break
			}
		}

		if !found {
			usePort := "3000"
			if c.IsConnector {
				usePort = "8901"
			}
			if c.CustomDestinationPort != 0 {
				usePort = strconv.Itoa(c.CustomDestinationPort)
			}

			dcToAdd += fmt.Sprintf("\n\t%s %s {\n", dcStanzaName, dest)
			if c.IsConnector {
				dcToAdd += "\t\tconnector true\n"
			}

			dstClusterIps := destIpList[dest]
			for _, ip := range dstClusterIps {
				if strings.Contains(ip, " ") {
					dcToAdd += fmt.Sprintf("\t\t%s %s\n", nodeAddressPort, ip)
				} else {
					dcToAdd += fmt.Sprintf("\t\t%s %s %s\n", nodeAddressPort, ip, usePort)
				}

				if xdrVersion == "5" {
					if _, ok := dc2namespace[dest]; !ok {
						dc2namespace[dest] = []string{}
					}
					for _, nspace := range namespaces {
						alreadyAdded := false
						for _, existing := range dc2namespace[dest] {
							if existing == nspace {
								alreadyAdded = true
								break
							}
						}
						if !alreadyAdded {
							dc2namespace[dest] = append(dc2namespace[dest], nspace)
							dcToAdd += fmt.Sprintf("\t\tnamespace %s {\n\t\t}\n", nspace)
						}
					}
				}
			}
			dcToAdd += "\t}\n"
		}
	}

	// Insert DC configuration
	confsx := confs[:xdrEnd]
	confsy := confs[xdrEnd:]
	if len(dcToAdd) > 0 {
		confsx = append(confsx, strings.Split(dcToAdd, "\n")...)
	}
	confsx = append(confsx, confsy...)

	// For XDR v4, update namespaces
	if xdrVersion == "4" {
		confsx = c.updateNamespacesForXDRv4(confsx, namespaces, destinations)
	}

	return strings.Join(confsx, "\n"), nil
}

func (c *XdrConnectCmd) updateNamespacesForXDRv4(confs []string, namespaces []string, destinations []string) []string {
	for _, targetNs := range namespaces {
		nsName := ""
		nsLoc := -1
		lvl := 0
		hasEnableXdr := false
		var hasDcList []string

		for j := 0; j < len(confs); j++ {
			if strings.HasPrefix(confs[j], "namespace ") {
				nsLoc = j
				atmp := strings.Split(confs[j], " ")
				if len(atmp) > 1 {
					nsName = atmp[1]
				}
				lvl = 1
			} else if strings.Contains(confs[j], "{") && nsLoc != -1 {
				lvl += strings.Count(confs[j], "{")
			} else if strings.Contains(confs[j], "}") && nsLoc != -1 {
				lvl -= strings.Count(confs[j], "}")
			} else if strings.Contains(confs[j], "enable-xdr true") && !strings.Contains(confs[j], "-enable-xdr true") && nsLoc != -1 && targetNs == nsName {
				hasEnableXdr = true
			} else if strings.Contains(confs[j], "xdr-remote-datacenter ") && nsLoc != -1 && targetNs == nsName {
				tmp := strings.Split(confs[j], " ")
				for k := 0; k < len(tmp); k++ {
					if strings.Contains(tmp[k], "xdr-remote-datacenter") && k+1 < len(tmp) {
						hasDcList = append(hasDcList, tmp[k+1])
						break
					}
				}
			}

			if lvl == 0 && nsLoc != -1 && nsName == targetNs {
				// Add enable-xdr if not present
				if !hasEnableXdr {
					confs[nsLoc] = confs[nsLoc] + "\nenable-xdr true"
				}

				// Add remote datacenters if not present
				for _, dc := range destinations {
					found := false
					for _, existingDc := range hasDcList {
						if dc == existingDc {
							found = true
							break
						}
					}
					if !found {
						confs[nsLoc] = confs[nsLoc] + fmt.Sprintf("\nxdr-remote-datacenter %s", dc)
					}
				}

				hasDcList = hasDcList[:0]
				nsLoc = -1
				nsName = ""
				hasEnableXdr = false
			}
		}
	}
	return confs
}
