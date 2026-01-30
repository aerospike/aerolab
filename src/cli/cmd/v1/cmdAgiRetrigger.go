package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/aerospike/aerolab/pkg/utils/diff"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

// AgiRetriggerCmd re-runs the ingest process on an AGI instance.
// It compares the current configuration with new parameters,
// shows the diff, and restarts the ingest service.
//
// The command can update:
//   - SFTP source parameters
//   - S3 source parameters
//   - Time range filtering
//   - Custom source name
//   - Patterns file
//   - Log level settings
type AgiRetriggerCmd struct {
	ClusterName TypeAgiClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`

	// Source options
	ClusterSource TypeClusterName `long:"source-cluster" description:"Cluster name to use as the source for logs"`
	LocalSource   flags.Filename  `long:"source-local" description:"Get logs from a local directory; Docker: use 'bind:/path' prefix to bind-mount instead of copying"`

	// SFTP source options
	SftpEnable  *bool   `long:"source-sftp-enable" description:"Enable SFTP source"`
	SftpThreads *int    `long:"source-sftp-threads" description:"Number of concurrent downloader threads"`
	SftpHost    *string `long:"source-sftp-host" description:"SFTP host"`
	SftpPort    *int    `long:"source-sftp-port" description:"SFTP port"`
	SftpUser    *string `long:"source-sftp-user" description:"SFTP user"`
	SftpPass    *string `long:"source-sftp-pass" description:"SFTP password (supports ENV::VAR_NAME)" webtype:"password"`
	SftpKey     *string `long:"source-sftp-key" description:"Key file for SFTP login"`
	SftpPath    *string `long:"source-sftp-path" description:"Path on SFTP to download logs from"`
	SftpRegex   *string `long:"source-sftp-regex" description:"Regex to filter files to download"`

	// S3 source options
	S3Enable  *bool   `long:"source-s3-enable" description:"Enable S3 source"`
	S3Threads *int    `long:"source-s3-threads" description:"Number of concurrent downloader threads"`
	S3Region  *string `long:"source-s3-region" description:"AWS region where S3 bucket is located"`
	S3Bucket  *string `long:"source-s3-bucket" description:"S3 bucket name"`
	S3KeyID   *string `long:"source-s3-key-id" description:"AWS access key ID (supports ENV::VAR_NAME)"`
	S3Secret  *string `long:"source-s3-secret-key" description:"AWS secret key (supports ENV::VAR_NAME)" webtype:"password"`
	S3Path    *string `long:"source-s3-path" description:"Path prefix in S3 bucket"`
	S3Regex   *string `long:"source-s3-regex" description:"Regex to filter files to download"`

	// Time range options
	TimeRanges     *bool   `long:"ingest-timeranges-enable" description:"Enable time range filtering"`
	TimeRangesFrom *string `long:"ingest-timeranges-from" description:"Time range start (format: 2006-01-02T15:04:05Z07:00)"`
	TimeRangesTo   *string `long:"ingest-timeranges-to" description:"Time range end (format: 2006-01-02T15:04:05Z07:00)"`

	// Custom options
	CustomSourceName *string         `long:"ingest-custom-source-name" description:"Custom source name to display in Grafana"`
	PatternsFile     *flags.Filename `long:"ingest-patterns-file" description:"Custom patterns YAML file for log ingest"`
	IngestLogLevel   *int            `long:"ingest-log-level" description:"Log level: 1=CRITICAL,2=ERROR,3=WARN,4=INFO,5=DEBUG,6=DETAIL"`
	IngestCpuProfile *bool           `long:"ingest-cpu-profiling" description:"Enable CPU profiling for ingest"`

	Force bool    `long:"force" description:"Do not ask for confirmation, just continue" webdisable:"true" webset:"true"`
	Help  HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi run-ingest.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiRetriggerCmd) Execute(args []string) error {
	cmd := []string{"agi", "run-ingest"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Retrigger(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// Retrigger re-runs the ingest process on the AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiRetriggerCmd) Retrigger(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "run-ingest"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Process ENV:: variables
	c.processEnvVariables()

	// Validate S3 path if S3 is enabled
	if c.S3Enable != nil && *c.S3Enable && c.S3Path != nil && *c.S3Path == "" {
		return errors.New("S3 path cannot be left empty when S3 source is enabled")
	}

	// Validate local files exist (handle bind: prefix for local source)
	for _, k := range []*string{(*string)(c.SftpKey), (*string)(c.PatternsFile)} {
		if k != nil && *k != "" {
			if _, err := os.Stat(*k); err != nil {
				return fmt.Errorf("could not access %s: %w", *k, err)
			}
		}
	}
	// Handle local source separately to support bind: prefix
	if c.LocalSource != "" {
		localPath := string(c.LocalSource)
		if isBindMountSource(localPath) {
			localPath = getBindMountPath(localPath)
		}
		if _, err := os.Stat(localPath); err != nil {
			return fmt.Errorf("could not access %s: %w", localPath, err)
		}
	}

	// Bind mount is only supported on Docker
	backendType := system.Opts.Config.Backend.Type
	if isBindMountSource(string(c.LocalSource)) && backendType != "docker" {
		return fmt.Errorf("bind mount (bind:/path) for --source-local is only supported on Docker backend")
	}

	// Parse time ranges
	var tfrom, tto time.Time
	if c.TimeRangesFrom != nil && *c.TimeRangesFrom != "" {
		var err error
		tfrom, err = time.Parse("2006-01-02T15:04:05Z07:00", *c.TimeRangesFrom)
		if err != nil {
			tfrom, err = time.Parse("2006/01/02 15:04:05 GMT", *c.TimeRangesFrom+" GMT")
			if err != nil {
				return fmt.Errorf("from time range invalid: %w", err)
			}
		}
	}
	if c.TimeRangesTo != nil && *c.TimeRangesTo != "" {
		var err error
		tto, err = time.Parse("2006-01-02T15:04:05Z07:00", *c.TimeRangesTo)
		if err != nil {
			tto, err = time.Parse("2006/01/02 15:04:05 GMT", *c.TimeRangesTo+" GMT")
			if err != nil {
				return fmt.Errorf("to time range invalid: %w", err)
			}
		}
	}

	// Get AGI instance
	instance := inventory.Instances.WithClusterName(string(c.ClusterName)).WithState(backends.LifeCycleStateRunning)
	if instance.Count() == 0 {
		return fmt.Errorf("AGI instance %s not found or not running", c.ClusterName)
	}
	inst := instance.Describe()[0]

	// Check if AGI is installed
	outputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"cat", "/opt/agi-installed"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})
	if len(outputs) == 0 || outputs[0].Output.Err != nil || len(outputs[0].Output.Stdout) == 0 {
		return errors.New("instance is missing file `/opt/agi-installed`, most likely it is still starting")
	}

	// Check if ingest is already running
	outputs = backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "cat /opt/agi/ingest.pid 2>/dev/null"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})
	if len(outputs) > 0 && outputs[0].Output.Err == nil && len(outputs[0].Output.Stdout) > 0 {
		pid := strings.TrimSpace(string(outputs[0].Output.Stdout))
		checkOutputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", fmt.Sprintf("ls /proc | egrep '^%s$'", pid)},
				SessionTimeout: time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if len(checkOutputs) > 0 && checkOutputs[0].Output.Err == nil {
			return errors.New("ingest already running")
		}
	}

	// Read current config from remote via SFTP (avoids terminal control character issues)
	sftpConf, err := inst.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}
	sftpCli, err := sshexec.NewSftp(sftpConf)
	if err != nil {
		return fmt.Errorf("could not create SFTP client: %w", err)
	}
	defer sftpCli.Close()

	var configBuf bytes.Buffer
	err = sftpCli.ReadFile(&sshexec.FileReader{
		SourcePath:  "/opt/agi/ingest.yaml",
		Destination: &configBuf,
	})
	if err != nil {
		return fmt.Errorf("could not read ingest.yaml: %w", err)
	}

	oldConfig := configBuf.String()
	conf, err := ingest.MakeConfigReader(true, strings.NewReader(oldConfig), true)
	if err != nil {
		return fmt.Errorf("could not unmarshal current config: %w", err)
	}

	// Update configuration with new parameters
	c.updateConfig(conf, tfrom, tto)

	// Handle cluster source
	if c.ClusterSource != "" {
		if c.LocalSource != "" {
			return errors.New("local source cannot be specified when using --source-cluster")
		}
		// Get logs from source cluster (using similar logic to AgiCreateCmd)
		localSource, err := c.getLogsFromCluster(system, inventory, logger)
		if err != nil {
			return fmt.Errorf("failed to get logs from cluster %s: %w", c.ClusterSource, err)
		}
		c.LocalSource = flags.Filename(localSource)
		defer os.RemoveAll(string(localSource))
	}

	// Validate S3 and SFTP credentials not redacted
	if conf.Downloader.S3Source != nil && conf.Downloader.S3Source.Enabled && conf.Downloader.S3Source.SecretKey == "<redacted>" {
		return errors.New("S3 source is enabled, but SecretKey has been redacted by the previous run; update the secret key value")
	}
	if conf.Downloader.SftpSource != nil && conf.Downloader.SftpSource.Enabled {
		if conf.Downloader.SftpSource.KeyFile == "<redacted>" || conf.Downloader.SftpSource.Password == "<redacted>" {
			return errors.New("SFTP source is enabled, but the password or keyFile is in <redacted> state, provide one and set the other to an empty string")
		}
		if conf.Downloader.SftpSource.KeyFile == "" && conf.Downloader.SftpSource.Password == "" {
			return errors.New("SFTP source is enabled, but no authentication method has been provided")
		}
	}

	// Marshal new config
	var encBuf bytes.Buffer
	var encBufPretty bytes.Buffer
	enc := yaml.NewEncoder(&encBuf)
	enc.SetIndent(2)
	encPretty := yaml.NewEncoder(&encBufPretty)
	encPretty.SetIndent(2)

	// Redact secrets for display
	var s3secret, sftpSecret, keySecret string
	if conf.Downloader.S3Source != nil {
		s3secret = conf.Downloader.S3Source.SecretKey
		if conf.Downloader.S3Source.SecretKey != "" {
			conf.Downloader.S3Source.SecretKey = "<redacted>"
		}
	}
	if conf.Downloader.SftpSource != nil {
		sftpSecret = conf.Downloader.SftpSource.Password
		keySecret = conf.Downloader.SftpSource.KeyFile
		if conf.Downloader.SftpSource.Password != "" {
			conf.Downloader.SftpSource.Password = "<redacted>"
		}
		if conf.Downloader.SftpSource.KeyFile != "" {
			conf.Downloader.SftpSource.KeyFile = "<redacted>"
		}
	}

	err = encPretty.Encode(conf)

	// Restore secrets
	if conf.Downloader.S3Source != nil {
		conf.Downloader.S3Source.SecretKey = s3secret
	}
	if conf.Downloader.SftpSource != nil {
		conf.Downloader.SftpSource.Password = sftpSecret
		conf.Downloader.SftpSource.KeyFile = keySecret
	}

	if err != nil {
		return fmt.Errorf("could not marshal new config to yaml: %w", err)
	}
	err = enc.Encode(conf)
	if err != nil {
		return fmt.Errorf("could not marshal new config to yaml: %w", err)
	}

	newConfigPretty := encBufPretty.Bytes()
	newConfig := encBuf.Bytes()

	// Show diff and ask for confirmation
	fmt.Println(string(diff.Diff("old", []byte(oldConfig), "new", newConfigPretty)))

	if c.ClusterSource != "" {
		fmt.Println("AGI from Source Cluster:", c.ClusterSource)
	}
	if c.LocalSource != "" && c.ClusterSource == "" {
		fmt.Println("AGI from Local Source:", c.LocalSource)
	}

	if !c.Force && IsInteractive() {
		ans, quitting, err := choice.Choice("Are you sure you want to continue?", choice.Items{
			choice.Item("Yes"),
			choice.Item("No"),
		})
		if err != nil {
			return err
		}
		if quitting || ans == "No" {
			fmt.Println("Aborting")
			return nil
		}
	}

	// Upload new configuration
	confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		// Upload patterns file if specified
		if c.PatternsFile != nil && *c.PatternsFile != "" {
			patternData, err := os.ReadFile(string(*c.PatternsFile))
			if err != nil {
				return fmt.Errorf("could not read patterns file: %w", err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/patterns.yaml",
				Source:      bytes.NewReader(patternData),
				Permissions: 0644,
			})
			if err != nil {
				return fmt.Errorf("failed to upload patterns file: %w", err)
			}
		}

		// Upload SFTP key if specified
		if c.SftpKey != nil && *c.SftpKey != "" {
			keyData, err := os.ReadFile(*c.SftpKey)
			if err != nil {
				return fmt.Errorf("could not read sftp key file: %w", err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/sftp.key",
				Source:      bytes.NewReader(keyData),
				Permissions: 0600,
			})
			if err != nil {
				return fmt.Errorf("failed to upload sftp key file: %w", err)
			}
		}

		// Upload new config
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/opt/agi/ingest.yaml",
			Source:      bytes.NewReader(newConfig),
			Permissions: 0644,
		})
		if err != nil {
			return fmt.Errorf("could not upload configuration to instance: %w", err)
		}
	}

	// Upload local source if specified
	if c.LocalSource != "" {
		logger.Info("Uploading local source files")
		err = c.uploadLocalSource(inst, logger)
		if err != nil {
			return fmt.Errorf("failed to upload local source to remote: %w", err)
		}
	}

	// Remove steps.json and restart ingest
	outputs = backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "rm -f /opt/agi/ingest/steps.json; systemctl start agi-ingest"},
			SessionTimeout: 2 * time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})
	if len(outputs) > 0 && outputs[0].Output.Err != nil {
		return fmt.Errorf("could not start ingest system: %s: %s", outputs[0].Output.Err, string(outputs[0].Output.Stdout))
	}

	logger.Info("Ingest retriggered successfully")
	return nil
}

// processEnvVariables processes ENV::VAR_NAME patterns in configuration fields.
func (c *AgiRetriggerCmd) processEnvVariables() {
	fields := []*string{c.SftpUser, c.SftpPass, c.S3KeyID, c.S3Secret}
	for _, field := range fields {
		if field != nil && strings.HasPrefix(*field, "ENV::") {
			envVar := strings.TrimPrefix(*field, "ENV::")
			val := os.Getenv(envVar)
			*field = val
		}
	}
}

// updateConfig updates the ingest configuration with new parameters.
func (c *AgiRetriggerCmd) updateConfig(conf *ingest.Config, tfrom, tto time.Time) {
	// Update SFTP source
	if conf.Downloader.SftpSource == nil {
		conf.Downloader.SftpSource = &ingest.SftpSource{}
	}
	if c.SftpEnable != nil {
		conf.Downloader.SftpSource.Enabled = *c.SftpEnable
	}
	if c.SftpThreads != nil {
		conf.Downloader.SftpSource.Threads = *c.SftpThreads
	}
	if c.SftpHost != nil {
		conf.Downloader.SftpSource.Host = *c.SftpHost
	}
	if c.SftpPort != nil {
		conf.Downloader.SftpSource.Port = *c.SftpPort
	}
	if c.SftpUser != nil {
		conf.Downloader.SftpSource.Username = *c.SftpUser
	}
	if c.SftpPass != nil {
		conf.Downloader.SftpSource.Password = *c.SftpPass
	}
	if c.SftpKey != nil {
		conf.Downloader.SftpSource.KeyFile = "/opt/agi/sftp.key"
	}
	if c.SftpPath != nil {
		conf.Downloader.SftpSource.PathPrefix = *c.SftpPath
	}
	if c.SftpRegex != nil {
		conf.Downloader.SftpSource.SearchRegex = *c.SftpRegex
	}

	// Update S3 source
	if conf.Downloader.S3Source == nil {
		conf.Downloader.S3Source = &ingest.S3Source{}
	}
	if c.S3Enable != nil {
		conf.Downloader.S3Source.Enabled = *c.S3Enable
	}
	if c.S3Threads != nil {
		conf.Downloader.S3Source.Threads = *c.S3Threads
	}
	if c.S3Region != nil {
		conf.Downloader.S3Source.Region = *c.S3Region
	}
	if c.S3Bucket != nil {
		conf.Downloader.S3Source.BucketName = *c.S3Bucket
	}
	if c.S3KeyID != nil {
		conf.Downloader.S3Source.KeyID = *c.S3KeyID
	}
	if c.S3Secret != nil {
		conf.Downloader.S3Source.SecretKey = *c.S3Secret
	}
	if c.S3Path != nil {
		conf.Downloader.S3Source.PathPrefix = *c.S3Path
	}
	if c.S3Regex != nil {
		conf.Downloader.S3Source.SearchRegex = *c.S3Regex
	}

	// Update time ranges
	if c.TimeRanges != nil {
		conf.IngestTimeRanges.Enabled = *c.TimeRanges
	}
	if c.TimeRangesFrom != nil {
		conf.IngestTimeRanges.From = tfrom
	}
	if c.TimeRangesTo != nil {
		conf.IngestTimeRanges.To = tto
	}

	// Update other options
	if c.CustomSourceName != nil {
		conf.CustomSourceName = *c.CustomSourceName
	}
	if c.PatternsFile != nil {
		conf.PatternsFile = "/opt/agi/patterns.yaml"
	}
	if c.IngestLogLevel != nil {
		conf.LogLevel = *c.IngestLogLevel
	}
	if c.IngestCpuProfile != nil {
		if *c.IngestCpuProfile {
			conf.CPUProfilingOutputFile = "/opt/agi/cpu.ingest.pprof"
		} else {
			conf.CPUProfilingOutputFile = ""
		}
	}

	// Enable read-only input mode when using bind mount
	if isBindMountSource(string(c.LocalSource)) {
		conf.Directories.ReadOnlyInput = true
	}
}

// getLogsFromCluster retrieves logs from a source cluster (simplified version).
func (c *AgiRetriggerCmd) getLogsFromCluster(system *System, inventory *backends.Inventory, logger *logger.Logger) (string, error) {
	// This uses the same logic as AgiCreateCmd.getLogsFromCluster
	// For brevity, we create a temp AgiCreateCmd and call its method
	createCmd := &AgiCreateCmd{
		ClusterSource: c.ClusterSource,
	}
	return createCmd.getLogsFromCluster(system, inventory, logger)
}

// uploadLocalSource uploads local source files to the AGI instance.
// If the source uses bind mount (bind:/path), this function skips the upload
// since the files are already available via the bind mount.
func (c *AgiRetriggerCmd) uploadLocalSource(inst *backends.Instance, logger *logger.Logger) error {
	// Skip upload if using bind mount - files are already available via mount
	if isBindMountSource(string(c.LocalSource)) {
		logger.Info("Skipping file upload - using bind mount for local source")
		return nil
	}

	confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}

	sourcePath := string(c.LocalSource)
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("could not stat local source: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		if stat.IsDir() {
			// Create input directory
			_ = cli.RawClient().MkdirAll("/opt/agi/files/input")
			err = uploadDirectory(cli, sourcePath, "/opt/agi/files/input/")
			if err != nil {
				return fmt.Errorf("failed to upload directory: %w", err)
			}
		} else {
			// Upload single file
			f, err := os.Open(sourcePath)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer f.Close()

			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/files/input/" + stat.Name(),
				Source:      f,
				Permissions: 0644,
			})
			if err != nil {
				return fmt.Errorf("failed to upload file: %w", err)
			}
		}
	}

	return nil
}
