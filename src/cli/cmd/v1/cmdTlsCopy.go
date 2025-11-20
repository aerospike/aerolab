package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type TlsCopyCmd struct {
	SourceClusterName      TypeClusterName `short:"s" long:"source" description:"Source cluster name" default:"mydc"`
	SourceNode             TypeNode        `short:"l" long:"source-node" description:"Source node from which to copy the TLS certificates" default:"1"`
	DestinationClusterName TypeClusterName `short:"d" long:"destination" description:"Destination cluster name" default:"client"`
	DestinationNodeList    TypeNodes       `short:"a" long:"destination-nodes" description:"List of destination nodes to copy the TLS certs to, comma separated. Empty=ALL." default:""`
	TlsName                string          `short:"t" long:"tls-name" description:"Common Name (tlsname)" default:"tls1"`
	Threads                int             `long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help                   HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TlsCopyCmd) Execute(args []string) error {
	cmd := []string{"tls", "copy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.CopyTLS(system, system.Backend.GetInventory(), system.Logger, args, "copy")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Copied TLS certificates to %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *TlsCopyCmd) CopyTLS(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"tls", action}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Validate source cluster and node
	sourceCluster := inventory.Instances.WithClusterName(c.SourceClusterName.String())
	if sourceCluster == nil {
		return nil, fmt.Errorf("source cluster %s not found", c.SourceClusterName.String())
	}

	sourceCluster = sourceCluster.WithState(backends.LifeCycleStateRunning)
	if sourceCluster.Count() == 0 {
		return nil, fmt.Errorf("no running instances found in source cluster %s", c.SourceClusterName.String())
	}

	sourceInstance := sourceCluster.WithNodeNo(c.SourceNode.Int()).Describe()
	if len(sourceInstance) == 0 {
		return nil, fmt.Errorf("source node %d not found or not running in cluster %s", c.SourceNode.Int(), c.SourceClusterName.String())
	}

	// Validate destination cluster
	destCluster := inventory.Instances.WithClusterName(c.DestinationClusterName.String())
	if destCluster == nil {
		return nil, fmt.Errorf("destination cluster %s not found", c.DestinationClusterName.String())
	}

	destCluster = destCluster.WithState(backends.LifeCycleStateRunning)
	if destCluster.Count() == 0 {
		return nil, fmt.Errorf("no running instances found in destination cluster %s", c.DestinationClusterName.String())
	}

	// Filter destination nodes if specified
	if c.DestinationNodeList.String() != "" {
		nodes, err := expandNodeNumbers(c.DestinationNodeList.String())
		if err != nil {
			return nil, err
		}
		destCluster = destCluster.WithNodeNo(nodes...)
		if destCluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some destination nodes in %s not found or not running", c.DestinationNodeList.String())
		}
	}

	destInstances := destCluster.Describe()

	logger.Info("Copying TLS certificates from %s:%d to %d nodes in cluster %s",
		c.SourceClusterName.String(), c.SourceNode.Int(),
		destInstances.Count(), c.DestinationClusterName.String())

	// Read certificates from source node
	certFiles, err := c.readCertificatesFromSource(sourceInstance[0], logger)
	if err != nil {
		return nil, err
	}

	// Upload certificates to destination nodes
	err = c.uploadCertificatesToDestination(destInstances, certFiles, logger)
	if err != nil {
		return nil, err
	}

	return destInstances, nil
}

// readCertificatesFromSource reads certificate files from the source node
func (c *TlsCopyCmd) readCertificatesFromSource(sourceInstance *backends.Instance, logger *logger.Logger) (map[string][]byte, error) {
	logger.Info("Reading certificates from source node %s:%d", sourceInstance.ClusterName, sourceInstance.NodeNo)

	// Get SFTP config
	conf, err := sourceInstance.GetSftpConfig("root")
	if err != nil {
		return nil, fmt.Errorf("failed to get SFTP config from source: %w", err)
	}

	// Create SFTP client
	client, err := sshexec.NewSftp(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client for source: %w", err)
	}
	defer client.Close()

	// List files in the SSL directory
	output := sourceInstance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"ls", path.Join("/etc/aerospike/ssl", c.TlsName)},
			SessionTimeout: 30 * time.Second,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if output.Output.Err != nil {
		return nil, fmt.Errorf("failed to list SSL directory on source: %s", output.Output.Err)
	}

	files := strings.Split(strings.TrimSpace(string(output.Output.Stdout)), "\n")
	certFiles := make(map[string][]byte)

	// Read each certificate file
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}

		var buf bytes.Buffer
		err = client.ReadFile(&sshexec.FileReader{
			SourcePath:  path.Join("/etc/aerospike/ssl", c.TlsName, file),
			Destination: &buf,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s from source: %w", file, err)
		}

		certFiles[file] = buf.Bytes()
		logger.Debug("Read certificate file: %s (%d bytes)", file, buf.Len())
	}

	if len(certFiles) == 0 {
		return nil, fmt.Errorf("no certificate files found in /etc/aerospike/ssl/%s on source node", c.TlsName)
	}

	logger.Info("Successfully read %d certificate files from source", len(certFiles))
	return certFiles, nil
}

// uploadCertificatesToDestination uploads certificates to destination nodes
func (c *TlsCopyCmd) uploadCertificatesToDestination(destInstances backends.InstanceList, certFiles map[string][]byte, logger *logger.Logger) error {
	logger.Info("Uploading certificates to %d destination nodes", destInstances.Count())

	// Get SFTP configs for all destination instances
	confs, err := destInstances.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP configs for destinations: %w", err)
	}

	type uploadTask struct {
		conf     *sshexec.ClientConf
		instance *backends.Instance
	}

	tasks := make([]uploadTask, len(confs))
	for i, conf := range confs {
		tasks[i] = uploadTask{
			conf:     conf,
			instance: destInstances.Describe()[i],
		}
	}

	var hasErr error
	parallelize.ForEachLimit(tasks, c.Threads, func(task uploadTask) {
		instance := task.instance

		// Remove old directory and create new one
		output := instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"rm", "-rf", path.Join("/etc/aerospike/ssl", c.TlsName)},
				SessionTimeout: 30 * time.Second,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		if output.Output.Err != nil {
			logger.Error("Failed to remove old SSL directory on %s:%d: %s", instance.ClusterName, instance.NodeNo, output.Output.Err)
			hasErr = errors.New("some nodes failed to remove old SSL directory")
			return
		}

		// Create SSL directory
		output = instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"mkdir", "-p", path.Join("/etc/aerospike/ssl", c.TlsName)},
				SessionTimeout: 30 * time.Second,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		if output.Output.Err != nil {
			logger.Error("Failed to create SSL directory on %s:%d: %s", instance.ClusterName, instance.NodeNo, output.Output.Err)
			hasErr = errors.New("some nodes failed to create SSL directory")
			return
		}

		// Upload certificate files
		client, err := sshexec.NewSftp(task.conf)
		if err != nil {
			logger.Error("Failed to create SFTP client for %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed SFTP connection")
			return
		}
		defer client.Close()

		for filename, content := range certFiles {
			err = client.WriteFile(true, &sshexec.FileWriter{
				DestPath:    path.Join("/etc/aerospike/ssl", c.TlsName, filename),
				Source:      bytes.NewReader(content),
				Permissions: 0644,
			})
			if err != nil {
				logger.Error("Failed to upload %s to %s:%d: %s", filename, instance.ClusterName, instance.NodeNo, err)
				hasErr = errors.New("some nodes failed to upload certificates")
				return
			}
		}

		logger.Debug("Uploaded %d certificate files to %s:%d", len(certFiles), instance.ClusterName, instance.NodeNo)
	})

	return hasErr
}
