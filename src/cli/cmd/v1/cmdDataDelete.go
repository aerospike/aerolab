package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
)

type DataDeleteCmd struct {
	ClusterName      TypeClusterName   `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Node             TypeNode          `short:"l" long:"node" description:"Node to run on" default:"1"`
	SeedNode         string            `short:"g" long:"seed-node" description:"Seed node IP:PORT" default:"127.0.0.1:3000"`
	Namespace        string            `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Set              string            `short:"s" long:"set" description:"Set name" default:"myset"`
	PkPrefix         string            `short:"p" long:"pk-prefix" description:"Prefix to add to primary key" default:""`
	PkStartNumber    int               `short:"a" long:"pk-start-number" description:"The start ID of the unique PK names" default:"1"`
	PkEndNumber      int               `short:"z" long:"pk-end-number" description:"The end ID of the unique PK names" default:"1000"`
	Durable          bool              `short:"D" long:"durable-delete" description:"Use durable deletes"`
	RunDirect        bool              `short:"d" long:"run-direct" description:"If set, will run directly from current machine"`
	UseMultiThreaded int               `short:"u" long:"multi-thread" description:"Number of threads for processing" default:"0"`
	User             string            `short:"U" long:"username" description:"Aerospike username" default:""`
	Pass             string            `short:"P" long:"password" description:"Aerospike password" default:""`
	Version          TypeClientVersion `short:"v" long:"version" description:"Aerospike library version: 8" default:"8" webchoice:"8"`
	AuthExternal     bool              `short:"Q" long:"auth-external" description:"Use external auth method"`
	TlsCaCert        string            `short:"y" long:"tls-ca-cert" description:"TLS CA certificate path" default:""`
	TlsClientCert    string            `short:"w" long:"tls-client-cert" description:"TLS client certificate path" default:""`
	TlsServerName    string            `short:"i" long:"tls-server-name" description:"TLS ServerName" default:""`
	AerolabVersion   string            `long:"aerolab-version" description:"Aerolab version to install on remote node if not present" default:"latest"`
	Prerelease       bool              `long:"aerolab-prerelease" description:"Install prerelease version of aerolab"`
	RunJson          string            `long:"run-json" hidden:"true"`
	Help             HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *DataDeleteCmd) Execute(args []string) error {
	cmd := []string{"data", "delete"}
	system, err := Initialize(&Init{InitBackend: !c.RunDirect, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.delete(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *DataDeleteCmd) delete(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Load JSON if provided
	if c.RunJson != "" {
		jf, err := os.ReadFile(c.RunJson)
		if err != nil {
			return fmt.Errorf("failed to read run-json file: %w", err)
		}
		err = json.Unmarshal(jf, c)
		if err != nil {
			return fmt.Errorf("failed to unmarshal run-json: %w", err)
		}
	}

	// If running in direct mode, execute the actual delete
	if c.RunDirect {
		logger.Info("Delete start")
		logger.Info("namespace=%s set=%s pk_start_key=%s%d pk_end_key=%s%d",
			c.Namespace, c.Set, c.PkPrefix, c.PkStartNumber, c.PkPrefix, c.PkEndNumber)

		var err error
		switch c.Version {
		case "8":
			err = c.delete8(args, logger)
		default:
			err = fmt.Errorf("aerospike client version %s not supported", c.Version)
		}

		if err == nil {
			logger.Info("Delete done")
		}
		return err
	}

	// Otherwise, unpack and run on remote node
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"data", "delete"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Adjust seed node for docker backend
	seedNode, err := c.checkSeedPort(system, inventory, logger)
	if err != nil {
		return err
	}
	c.SeedNode = seedNode

	logger.Info("Unpacking and deploying to remote node")
	c.RunDirect = true
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = c.unpack(system, inventory, logger, "delete", data)
	if err != nil {
		return err
	}

	logger.Info("Complete")
	return nil
}

func (c *DataDeleteCmd) unpack(system *System, inventory *backends.Inventory, logger *logger.Logger, cmd string, data []byte) error {
	// Get the instance to run on
	cluster := inventory.Instances.WithClusterName(c.ClusterName.String()).WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		return fmt.Errorf("cluster %s not found or has no running instances", c.ClusterName.String())
	}

	instance := cluster.WithNodeNo(c.Node.Int()).Describe()
	if len(instance) == 0 {
		return fmt.Errorf("node %d not found in cluster %s", c.Node.Int(), c.ClusterName.String())
	}
	inst := instance[0]

	// Check if aerolab is already installed
	checkOutput := inst.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"which", "aerolab"},
			SessionTimeout: 10 * time.Second,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	aerolabInstalled := checkOutput.Output.Err == nil

	// Install aerolab if not present
	if !aerolabInstalled {
		logger.Info("Installing aerolab on %s:%d", inst.ClusterName, inst.NodeNo)

		var alVer *string
		if c.AerolabVersion != "latest" {
			alVer = &c.AerolabVersion
		}
		var pre *bool
		if !c.Prerelease {
			pre = &c.Prerelease
		}

		installScript, err := aerolab.GetLinuxInstallScript("", alVer, pre)
		if err != nil {
			return fmt.Errorf("failed to get aerolab install script: %w", err)
		}

		// Upload install script
		conf, err := inst.GetSftpConfig("root")
		if err != nil {
			return fmt.Errorf("failed to get SFTP config: %w", err)
		}

		client, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("failed to create SFTP client: %w", err)
		}

		now := time.Now().Format("20060102150405")
		scriptPath := "/tmp/install-aerolab.sh." + now
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    scriptPath,
			Source:      bytes.NewReader(installScript),
			Permissions: 0755,
		})
		client.Close()
		if err != nil {
			return fmt.Errorf("failed to upload install script: %w", err)
		}

		// Execute install script
		logger.Info("Running aerolab installer on %s:%d", inst.ClusterName, inst.NodeNo)
		installOutput := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", scriptPath},
				Stdout:         os.Stdout,
				Stderr:         os.Stderr,
				SessionTimeout: 10 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		if installOutput.Output.Err != nil {
			return fmt.Errorf("failed to install aerolab: %w", installOutput.Output.Err)
		}
	} else {
		logger.Info("Aerolab already installed on %s:%d", inst.ClusterName, inst.NodeNo)
	}

	// Upload JSON config
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	client, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer client.Close()

	jsonName := "/tmp/aerolab-data-cmd." + shortuuid.New()
	err = client.WriteFile(true, &sshexec.FileWriter{
		DestPath:    jsonName,
		Source:      bytes.NewReader(data),
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to upload JSON config: %w", err)
	}
	client.Close()

	// Execute aerolab on remote node
	logger.Info("Executing data %s on remote node", cmd)
	runCommand := []string{"aerolab", "data", cmd, "--run-json=" + jsonName}

	output := inst.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        runCommand,
			Stdin:          nil,
			Stdout:         os.Stdout,
			Stderr:         os.Stderr,
			SessionTimeout: 1 * time.Hour,
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if output.Output.Err != nil {
		return fmt.Errorf("failed to execute data command on remote node: %w", output.Output.Err)
	}

	return nil
}

func (c *DataDeleteCmd) checkSeedPort(system *System, inventory *backends.Inventory, logger *logger.Logger) (string, error) {
	// For non-docker backends, return as-is
	if system.Opts.Config.Backend.Type != string(backends.BackendTypeDocker) {
		return c.SeedNode, nil
	}

	// If custom seed node provided, use it
	if c.SeedNode != "127.0.0.1:3000" {
		return c.SeedNode, nil
	}

	// For docker, check if node has exposed ports
	cluster := inventory.Instances.WithClusterName(c.ClusterName.String()).WithState(backends.LifeCycleStateRunning)
	instance := cluster.WithNodeNo(c.Node.Int()).Describe()
	if len(instance) == 0 {
		return c.SeedNode, nil
	}

	inst := instance[0]
	// Check for docker-specific exposed ports in tags
	if exposedPorts, ok := inst.Tags["aerolab.docker.expose-ports"]; ok && exposedPorts != "" {
		return "127.0.0.1:" + exposedPorts, nil
	}

	return c.SeedNode, nil
}
