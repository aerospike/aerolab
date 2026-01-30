package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InventoryMigrateCmd struct {
	DryRun     bool    `long:"dry-run" description:"Show what would be migrated without making changes"`
	Yes        bool    `short:"y" long:"yes" description:"Skip confirmation prompt and proceed with migration"`
	Force      bool    `long:"force" description:"Force re-migration of already migrated resources"`
	Verbose    bool    `short:"v" long:"verbose" description:"Show detailed migration information"`
	SSHKeyPath string  `long:"ssh-key-path" description:"Path to old v7 SSH keys directory (default: ~/aerolab-keys/)"`
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryMigrateCmd) Execute(args []string) error {
	cmd := []string{"inventory", "migrate"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.InventoryMigrate(system, cmd, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

/*
================================================================================

	OLD AEROLAB NAMING AND TAGGING REFERENCE

================================================================================

--------------------------------------------------------------------------------

	DOCKER

--------------------------------------------------------------------------------

Docker Instance Container Naming:
  - Servers: "aerolab-{clusterName}_{nodeNumber}" (e.g., "aerolab-mydc_1")
  - Clients: "aerolab_c-{clusterName}_{nodeNumber}" (e.g., "aerolab_c-myclient_1")

Docker Container Labels:
  - "aerolab.client.type"  = client type (e.g., "graph", "tools", "ams", "vscode", etc.)
  - "owner"                = owner name
  - "agiLabel"             = AGI label for the container
  - Custom labels via --docker-label flag (key=value format)

Docker Template Image Naming:
  - New format: "aerolab-{distro}_{version}_{arch}:{aerospikeVersion}"
    Example: "aerolab-ubuntu_22.04_amd64:7.0.0"
  - Old format: "aerolab-{distro}_{version}:{aerospikeVersion}"
    Example: "aerolab-ubuntu_22.04:7.0.0"

Docker Networks:
  - User-defined names (not aerolab-managed)
  - No specific labels/tags

--------------------------------------------------------------------------------

	AWS

--------------------------------------------------------------------------------

AWS Instance Tags - Servers:
  - "UsedBy"                             = "aerolab4"
  - "Aerolab4ClusterName"                = cluster name
  - "Aerolab4NodeNumber"                 = node number (string)
  - "Aerolab4OperatingSystem"            = distro name (e.g., "ubuntu")
  - "Aerolab4OperatingSystemVersion"     = distro version (e.g., "22.04")
  - "Aerolab4AerospikeVersion"           = Aerospike version (e.g., "7.0.0")
  - "Name"                               = "aerolab4-{clusterName}-{nodeNumber}"
  - "Arch"                               = "amd" or "arm"
  - "aerolab4expires"                    = expiry timestamp (RFC3339 format)
  - "telemetry"                          = telemetry UUID
  - "Aerolab4CostPerHour"                = cost per hour (float string)
  - "Aerolab4CostSoFar"                  = running cost total (float string)
  - "Aerolab4CostStartTime"              = cost tracking start time (unix timestamp)
  - "owner"                              = owner name
  - "aerolab7spot"                       = "true" for spot instances

AWS Instance Tags - Clients:
  - "UsedBy"                                  = "aerolab4client"
  - "Aerolab4clientClusterName"               = client cluster name
  - "Aerolab4clientNodeNumber"                = node number (string)
  - "Aerolab4clientOperatingSystem"           = distro name
  - "Aerolab4clientOperatingSystemVersion"    = distro version
  - "Aerolab4clientAerospikeVersion"          = Aerospike version (if applicable)
  - "Aerolab4clientType"                      = client type (e.g., "tools", "graph")

AWS AGI-Specific Instance Tags:
  - "aerolab7agiav"     = Aerospike version for AGI
  - "aerolab4features"  = feature flags (integer bitmask as string)
  - "aerolab4ssl"       = "true"/"false" for SSL enabled
  - "agiLabel"          = AGI label string
  - "agiinstance"       = instance type used
  - "aginodim"          = "true"/"false" for NoDIM mode
  - "termonpow"         = "true"/"false" for terminate on poweroff
  - "isspot"            = "true"/"false" for spot instance
  - "agiSrcLocal"       = local source string
  - "agiSrcSftp"        = SFTP source string
  - "agiSrcS3"          = S3 source string
  - "agiDomain"         = AGI domain for Route53

AWS Template AMI Naming:
  - "aerolab4-template-{distro}_{version}_{aerospikeVersion}_{arch}"
    Example: "aerolab4-template-ubuntu_22.04_7.0.0_amd"

AWS Volume (EFS) Tags:
  - "Name"           = volume name
  - "UsedBy"         = "aerolab7"
  - "lastUsed"       = last used timestamp (RFC3339 format)
  - "expireDuration" = expiry duration string (e.g., "24h0m0s")
  - "aerolab7owner"  = owner name
  - "agiLabel"       = AGI label
  - Custom tags via --tag flag (key=value format)

AWS Security Groups Naming:
  - "AeroLabServer-{vpcSuffix}"          (e.g., "AeroLabServer-0abc1234")
  - "AeroLabClient-{vpcSuffix}"          (e.g., "AeroLabClient-0abc1234")
  - "{customPrefix}-{vpcSuffix}"         (default: "aerolab-managed-external-{vpcSuffix}")
    Note: vpcSuffix is the VPC ID without the "vpc-" prefix

AWS SSH Key Naming:
  - "aerolab-{clusterName}_{region}"     (e.g., "aerolab-mydc_us-east-1")

--------------------------------------------------------------------------------

	GCP

--------------------------------------------------------------------------------

GCP Instance Labels - Servers:
  - "used_by"                            = "aerolab4"
  - "aerolab4cluster_name"               = cluster name
  - "aerolab4node_number"                = node number (string)
  - "aerolab4operating_system"           = distro name (e.g., "ubuntu")
  - "aerolab4operating_system_version"   = distro version (e.g., "22-04", dots replaced with dashes)
  - "aerolab4aerospike_version"          = Aerospike version (e.g., "7-0-0", dots replaced with dashes)
  - "arch"                               = "amd" or "arm"
  - "aerolab4expires"                    = expiry timestamp (RFC3339, lowercase, : replaced by _, + by -)
  - "telemetry"                          = telemetry UUID
  - "aerolab_cost_ph"                    = cost per hour (encoded float)
  - "aerolab_cost_sofar"                 = running cost total (encoded float)
  - "aerolab_cost_starttime"             = cost tracking start time (encoded unix timestamp)
  - "owner"                              = owner name
  - "isspot"                             = "true" for spot/preemptible instances

GCP Instance Labels - Clients:
  - "used_by"                                  = "aerolab4client"
  - "aerolab4client_name"                      = client cluster name
  - "aerolab4client_node_number"               = node number (string)
  - "aerolab4client_operating_system"          = distro name
  - "aerolab4client_operating_system_version"  = distro version
  - "aerolab4client_aerospike_version"         = Aerospike version
  - "aerolab4client_type"                      = client type

GCP Instance Naming:
  - "aerolab4-{clusterName}-{nodeNumber}"      (e.g., "aerolab4-mydc-1")

GCP Template Image Naming:
  - "aerolab4-template-{distro}-{version}-{aerospikeVersion}-{arch}"
    Example: "aerolab4-template-ubuntu-22-04-7-0-0-amd"
    Note: All dots and special chars replaced with dashes for GCP resource naming

GCP Network Tags (Compute Tags for Firewall Targeting):
  - "aerolab-server"              = tag for server instances
  - "aerolab-client"              = tag for client instances
  - "{firewallNamePrefix}"        = custom firewall tag (default: "aerolab-managed-external")

GCP Firewall Rules Naming:
  - "aerolab-managed-internal"    = internal communication rule (server<->client)
  - "{firewallNamePrefix}"        = external access rule (default: "aerolab-managed-external")

GCP Volume (Persistent Disk) Labels:
  - "usedby"          = "aerolab7"
  - "lastused"        = last used timestamp (lowercase, : replaced by _, + by -)
  - "expireduration"  = expiry duration (lowercase, . replaced by _)
  - "aerolab7owner"   = owner name
  - "agilabel{N}"     = AGI label (base32 encoded, chunked to 63 chars max per label)
    N = 0, 1, 2, ... for multi-chunk labels
  - "agiinstance"     = instance type
  - "aginodim"        = "true"/"false" for NoDIM mode
  - "termonpow"       = "true"/"false" for terminate on poweroff
  - "isspot"          = "true"/"false" for spot instance

GCP SSH Key Naming:
  - "aerolab-gcp-{clusterName}"   (e.g., "aerolab-gcp-mydc")

GCP Expiry System Resources:
  - Cloud Function: "aerolab-expiries"
  - Cloud Scheduler Job: "aerolab-expiries"
  - Storage Bucket: "aerolab-{projectId}" (e.g., "aerolab-my-project")

--------------------------------------------------------------------------------

	GCP LABEL ENCODING NOTES

--------------------------------------------------------------------------------

GCP labels have restrictions:
  - Max 63 characters per label value
  - Only lowercase letters, numbers, underscores, and dashes
  - Must start with a lowercase letter

For long values (like AGI labels), the gcplabels package uses:
  - Base32 encoding (no padding)
  - Lowercase conversion
  - Chunking to 63 chars with numeric suffix (agilabel0, agilabel1, etc.)

For timestamps and durations:
  - Colons (:) replaced with underscores (_)
  - Plus signs (+) replaced with dashes (-)
  - Periods (.) replaced with underscores (_)
  - All lowercase

--------------------------------------------------------------------------------

	EXPIRY SYSTEM TAGS SUMMARY

--------------------------------------------------------------------------------

AWS Expiry Tags:
  - Instances: "aerolab4expires" = RFC3339 timestamp
  - Volumes:   "lastUsed" = RFC3339, "expireDuration" = duration string

GCP Expiry Labels:
  - Instances: "aerolab4expires" = RFC3339 (encoded: lowercase, : -> _, + -> -)
  - Volumes:   "lastused" = RFC3339 (encoded), "expireduration" = duration (encoded)

================================================================================
*/

/*
================================================================================

	OLD SSH HANDLING, KEY NAMES, LOCATIONS, SCOPES, ETC

================================================================================

--------------------------------------------------------------------------------

	COMMON SSH KEY PATHS AND DIRECTORIES

--------------------------------------------------------------------------------

AeroLab Root Directory (AEROLAB_HOME):
  - Default: ~/.aerolab (${HOME}/.aerolab)
  - Override: AEROLAB_HOME environment variable
  - Used for: shared key override, caches, configs

SSH Key Storage Directory (SshKeyPath):
  - Default: ${HOME}/aerolab-keys/
  - Config: aerolab config backend -p PATH (or --key-path PATH)
  - Permissions: 0700 for directory, 0600 for key files
  - Created automatically if doesn't exist

Shared Key Override:
  - If ${AEROLAB_HOME}/sshkey exists, it's used for ALL clusters
  - Takes priority over per-cluster keys
  - Allows users to provide their own pre-existing SSH key
  - AWS uses name "manual-aerolab-agi-shared" for shared key
  - GCP uses name "sshkey" for shared key

--------------------------------------------------------------------------------

	AWS SSH KEY HANDLING

--------------------------------------------------------------------------------

AWS Key Naming Format:
  - Pattern: "aerolab-{clusterName}_{region}"
  - Example: "aerolab-mydc_us-east-1"
  - Note: Region is included to allow same cluster name in different regions

AWS Key Scope:
  - Per cluster + region combination
  - Each cluster in a specific region has its own SSH key
  - If cluster "mydc" exists in us-east-1 and eu-west-1, two keys exist

AWS Local Key Storage:
  - Path: {SshKeyPath}/aerolab-{clusterName}_{region}
  - Example: ~/aerolab-keys/aerolab-mydc_us-east-1
  - Contains: Private key only (no .pub file)
  - Permissions: 0600

AWS Cloud Key Storage:
  - Registered with EC2 Key Pairs service
  - Key pair name matches local naming convention
  - Verified with ec2.DescribeKeyPairs API

AWS Key Generation Process:
  - Uses AWS EC2 CreateKeyPair API
  - AWS generates the key pair server-side
  - Private key material returned and saved locally
  - Public key automatically available in EC2 for instance launch

AWS Key Usage:
  - input.KeyName is set when calling ec2.RunInstances
  - EC2 injects the public key into instance authorized_keys during launch
  - No separate public key file needed locally

AWS Template Keys:
  - Pattern: "aerolab-template{unixTimestamp}_{region}"
  - Example: "aerolab-template1704067200_us-east-1"
  - Created temporarily for template creation
  - Deleted immediately after template is created (d.killKey called in defer)

AWS Key Deletion:
  - Local file deleted with os.Remove()
  - AWS key pair deleted with ec2.DeleteKeyPair API
  - Both must succeed for complete cleanup

AWS Shared Key Behavior:
  - If ${AEROLAB_HOME}/sshkey exists:
    - Returns keyName="manual-aerolab-agi-shared"
    - Returns keyPath=${AEROLAB_HOME}/sshkey
    - Shared key is NOT deleted (killKey returns early)
    - makeKey returns early without creating new key

--------------------------------------------------------------------------------

	GCP SSH KEY HANDLING

--------------------------------------------------------------------------------

GCP Key Naming Format:
  - Pattern: "aerolab-gcp-{clusterName}"
  - Example: "aerolab-gcp-mydc"
  - Note: NO region in name (unlike AWS)

GCP Key Scope:
  - Per cluster only (NOT per region like AWS)
  - Same key used regardless of which zone instances are in

GCP Local Key Storage:
  - Private key: {SshKeyPath}/aerolab-gcp-{clusterName}
  - Public key: {SshKeyPath}/aerolab-gcp-{clusterName}.pub
  - Example: ~/aerolab-keys/aerolab-gcp-mydc
            ~/aerolab-keys/aerolab-gcp-mydc.pub
  - Permissions: 0600 for both files

GCP Cloud Key Storage:
  - NO registration with GCP (unlike AWS which uses Key Pairs)
  - Public key is NOT stored in any GCP service
  - Key exists only locally and in instance metadata

GCP Key Generation Process:
  - Locally generated using Go crypto/rsa
  - RSA 2048-bit key
  - Private key: PEM encoded (PKCS1)
  - Public key: OpenSSH authorized_keys format
  - No cloud API calls for key generation

GCP Key Usage:
  - Public key read from .pub file
  - Injected into instance metadata during creation:
    computepb.Items{
        Key:   "ssh-keys",
        Value: "root:{public_key_content}",
    }
  - GCP startup script enables root login via SSH

GCP Key Injection Format:
  - Metadata key: "ssh-keys"
  - Metadata value: "root:" + string(sshKeyPubContent)
  - Sets up SSH access for root user directly

GCP Key Deletion:
  - Private key: os.Remove(keyPath)
  - Public key: os.Remove(keyPath + ".pub")
  - No cloud API calls needed (key not registered anywhere)

GCP Shared Key Behavior:
  - If ${AEROLAB_HOME}/sshkey exists:
    - Returns keyName="sshkey"
    - Returns keyPath=${AEROLAB_HOME}/sshkey
    - Public key expected at ${AEROLAB_HOME}/sshkey.pub
    - Shared key is NOT deleted (killKey returns early)
    - makeKey returns early without creating new key

--------------------------------------------------------------------------------

	KEY FUNCTION SUMMARY

--------------------------------------------------------------------------------

Both AWS and GCP implement these functions:

GetKeyPath(clusterName string) (keyPath string, err error)
  - Returns path to private key for given cluster
  - Calls getKey() internally

getKey(clusterName string) (keyName, keyPath string, err error)
  - Checks for shared key override first
  - Returns existing key name and path
  - AWS: Also verifies key exists in EC2
  - GCP: Only verifies local file exists

makeKey(clusterName string) (keyName, keyPath string, err error)
  - Checks for shared key override first
  - Creates key if doesn't exist (calls getKey first)
  - AWS: Creates key via EC2 API, saves private key locally
  - GCP: Generates locally, saves both private and public keys

killKey(clusterName string) (keyName, keyPath string, err error)
  - Checks for shared key override (returns early if shared)
  - Deletes local key files
  - AWS: Also deletes key from EC2 Key Pairs

getKeyPathNoCheck(clusterName string) (keyPath string)  [AWS only]
  - Returns key path without validating existence in EC2
  - Used for quick lookups

--------------------------------------------------------------------------------

	MIGRATION CONSIDERATIONS

--------------------------------------------------------------------------------

When migrating from old aerolab to new:

1. AWS Keys:
   - Key naming convention may differ in new version
   - EC2 Key Pair names need to be tracked/updated
   - Local key files may need to be moved/renamed
   - Region is part of key identity

2. GCP Keys:
   - Key naming convention may differ in new version
   - Both .pub and private key files need migration
   - No cloud-side changes needed (keys not registered)

3. Shared Keys:
   - ${AEROLAB_HOME}/sshkey should continue working
   - Shared key path is hardcoded, not configurable

4. Key Directory:
   - SshKeyPath config needs to be preserved
   - Old keys in old directory should be migrated or
     config updated to point to existing directory

5. Cross-Region (AWS specific):
   - Same cluster name in different regions = different keys
   - Each region's key is independent

================================================================================
*/

/*
OLD TAG MIGRATION - SUMMARY and FLOW DETAILS

Migration Process:
1. Read old tags from all inventory items (instances, volumes, images)
2. Translate old tags to v8 format (see cmdInventoryMigrateImplementationPlan.md for full mapping)
3. Add migration marker tag (AEROLAB_V7_MIGRATED=true)
4. Copy SSH keys to {sshKeysDir}/old/ subdirectory
5. Update backend SSH key resolution to check for migrated instances

Key Tag Mappings:
- Server instances: Set aerolab.type = "aerospike"
- Client instances: Set aerolab.type from Aerolab4clientType (e.g., "tools", "graph", "ams", "vscode")
- Software version: Aerolab4AerospikeVersion/Aerolab4clientAerospikeVersion -> aerolab.soft.version
- Tags without v8 equivalent: Preserve with "v7-" prefix

Backend-Specific Behavior:
- AWS: Add new tags alongside old (never remove old tags)
- GCP: Add new tags, remove old only if 64-label limit would be exceeded
- Docker: NOT SUPPORTED (returns error immediately)

SSH Key Migration:
- Copy keys to {sshKeysDir}/old/{original-key-name}
- Backend updated to check AEROLAB_V7_MIGRATED tag and resolve old key paths
- Shared key (${AEROLAB_HOME}/sshkey) takes precedence, skip per-cluster migration

See cmdInventoryMigrateImplementationPlan.md for complete implementation details.
*/

func (c *InventoryMigrateCmd) InventoryMigrate(system *System, cmd []string, args []string, inventory *backends.Inventory) error {
	if system.Opts.Config.Backend.Type == "docker" {
		return fmt.Errorf("not supported: cannot migrate inventory for docker backend")
	}

	// Resolve SSH key paths
	sshKeyInfo, err := c.resolveSSHKeyPath()
	if err != nil {
		return fmt.Errorf("failed to resolve SSH key path: %w", err)
	}

	// Get the backend type
	var backendType backends.BackendType
	switch system.Opts.Config.Backend.Type {
	case "aws":
		backendType = backends.BackendTypeAWS
	case "gcp":
		backendType = backends.BackendTypeGCP
	default:
		return fmt.Errorf("unsupported backend type: %s", system.Opts.Config.Backend.Type)
	}

	// Get the aerolab version
	_, _, _, aerolabVersion := GetAerolabVersion()

	// Determine project name (default to "default" if empty)
	projectName := os.Getenv("AEROLAB_PROJECT")
	if projectName == "" {
		projectName = "default"
	}

	// Build migration input
	input := &backends.MigrateV7Input{
		Project:        projectName,
		DryRun:         c.DryRun,
		Force:          c.Force,
		SSHKeyInfo:     sshKeyInfo,
		AerolabVersion: aerolabVersion,
	}

	// Always do a discovery pass first to check for collisions
	discoveryInput := &backends.MigrateV7Input{
		Project:        projectName,
		DryRun:         true, // Discovery only
		Force:          c.Force,
		SSHKeyInfo:     sshKeyInfo,
		AerolabVersion: aerolabVersion,
	}

	discovery, err := system.Backend.MigrateV7Resources(backendType, discoveryInput)
	if err != nil {
		return fmt.Errorf("failed to discover v7 resources: %w", err)
	}

	totalResources := len(discovery.DryRunInstances) +
		len(discovery.DryRunVolumes) +
		len(discovery.DryRunImages) +
		len(discovery.DryRunFirewalls)

	if totalResources == 0 {
		system.Logger.Info("No v7 resources found to migrate.")
		return nil
	}

	// Check for old AGI instances and inform user
	agiInstanceCount, agiMonitorCount, agiMonitorNames := c.checkForOldAGIInstances(system, discovery)
	if agiInstanceCount > 0 {
		system.Logger.Info("")
		system.Logger.Info("=== AGI INSTANCES DETECTED ===")
		system.Logger.Info("Found %d AGI instance(s) that will be migrated.", agiInstanceCount)
		system.Logger.Info("")
		system.Logger.Info("AGI instances will be migrated with preserved tags for v8 compatibility.")
		system.Logger.Info("After migration:")
		system.Logger.Info("  - Use 'aerolab agi list' to see migrated AGI instances")
		system.Logger.Info("  - Use 'aerolab agi status -n NAME' to check AGI status")
		system.Logger.Info("  - Use 'aerolab agi stop/start' to manage instances")
		system.Logger.Info("  - AGI volumes (EFS/GCP Persistent Disk) will be recognized by v8")
		system.Logger.Info("")
		system.Logger.Info("Note: Some AGI features may require recreation for full v8 compatibility.")
		system.Logger.Info("")
	}
	if agiMonitorCount > 0 {
		system.Logger.Info("")
		system.Logger.Warn("=== AGI MONITOR INSTANCES REQUIRE RECREATION ===")
		system.Logger.Warn("Found %d AGI Monitor instance(s): %s", agiMonitorCount, strings.Join(agiMonitorNames, ", "))
		system.Logger.Info("")
		system.Logger.Warn("AGI Monitor instances run the aerolab binary internally and MUST be recreated.")
		system.Logger.Warn("The v7 binary on these instances is NOT compatible with v8 AGI instances.")
		system.Logger.Info("")
		system.Logger.Info("After migration, for each AGI Monitor listed above:")
		system.Logger.Info("  1. Note the monitor configuration (check cloud console or v7 aerolab)")
		system.Logger.Info("  2. Destroy: aerolab client destroy -n <monitor-name>")
		system.Logger.Info("  3. Recreate: aerolab agi monitor create -n <monitor-name> ...")
		system.Logger.Info("")
	}

	// Check for collisions with existing v8 resources
	collisions := c.checkForCollisions(system, discovery, inventory, backendType)
	if len(collisions) > 0 {
		system.Logger.Error("=== COLLISION DETECTED ===")
		system.Logger.Error("Cannot migrate: the following v7 resources would collide with existing v8 resources:")
		system.Logger.Error("")
		for _, collision := range collisions {
			system.Logger.Error("  %s", collision)
		}
		system.Logger.Error("")
		system.Logger.Error("Please resolve these collisions before migrating.")
		system.Logger.Error("Options:")
		system.Logger.Error("  1. Terminate the conflicting v8 resources")
		system.Logger.Error("  2. Terminate the conflicting v7 resources")
		system.Logger.Error("  3. Rename one of the conflicting clusters")
		return fmt.Errorf("migration aborted: %d collision(s) detected", len(collisions))
	}

	// Show summary
	serverCount := 0
	clientCount := 0
	for _, inst := range discovery.DryRunInstances {
		if inst.IsClient {
			clientCount++
		} else {
			serverCount++
		}
	}

	// If not dry-run and not --yes, prompt for confirmation
	if !c.DryRun && !c.Yes && IsInteractive() {
		system.Logger.Info("Found v7 resources to migrate:")
		system.Logger.Info("  Instances: %d (%d servers, %d clients)", len(discovery.DryRunInstances), serverCount, clientCount)
		system.Logger.Info("  Volumes: %d", len(discovery.DryRunVolumes))
		system.Logger.Info("  Images: %d", len(discovery.DryRunImages))
		system.Logger.Info("  Firewalls: %d", len(discovery.DryRunFirewalls))
		system.Logger.Info("  SSH Keys: %d", len(discovery.DryRunSSHKeys))
		system.Logger.Info("")

		if c.Verbose {
			c.printDryRunResults(system, discovery)
		}

		// Prompt for confirmation
		fmt.Print("Proceed with migration? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			return fmt.Errorf("migration cancelled by user")
		}
	}

	// For dry-run mode, just print the discovery results
	if c.DryRun {
		c.printDryRunResults(system, discovery)
		c.printExpiryDryRun(system, backendType, discovery)
		system.Logger.Info("Run without --dry-run to apply these changes.")
		return nil
	}

	// Run the migration
	result, err := system.Backend.MigrateV7Resources(backendType, input)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Print results
	c.printMigrationResults(system, result)

	// Install expiry system if any migrated resources have expiry set
	c.installExpiryIfNeeded(system, backendType, result)

	// Check for errors
	if len(result.Errors) > 0 {
		if result.InstancesMigrated > 0 || result.VolumesMigrated > 0 || result.ImagesMigrated > 0 || result.FirewallsMigrated > 0 {
			return fmt.Errorf("migration completed with %d errors (partial success)", len(result.Errors))
		}
		return fmt.Errorf("migration failed with %d errors", len(result.Errors))
	}

	return nil
}

// resolveSSHKeyPath resolves the SSH key paths for migration
func (c *InventoryMigrateCmd) resolveSSHKeyPath() (*backends.SSHKeyPathInfo, error) {
	info := &backends.SSHKeyPathInfo{}

	// Check for shared key in standard AEROLAB_HOME
	aerolabHome := os.Getenv("AEROLAB_HOME")
	if aerolabHome == "" {
		aerolabHome = filepath.Join(os.Getenv("HOME"), ".aerolab")
	}
	sharedKey := filepath.Join(aerolabHome, "sshkey")
	if _, err := os.Stat(sharedKey); err == nil {
		info.SharedKeyPath = sharedKey
	}

	// Resolve provided path or default
	if c.SSHKeyPath != "" {
		// Check if it's an aerolab home (has keys/ subdir or sshkey)
		keysSubdir := filepath.Join(c.SSHKeyPath, "keys")
		if _, err := os.Stat(keysSubdir); err == nil {
			info.KeysDir = keysSubdir
			info.IsAerolabHome = true
			// Also check for shared key in this home
			altShared := filepath.Join(c.SSHKeyPath, "sshkey")
			if _, err := os.Stat(altShared); err == nil {
				info.SharedKeyPath = altShared
			}
		} else {
			// Direct keys directory
			info.KeysDir = c.SSHKeyPath
		}
	} else {
		// Default
		info.KeysDir = filepath.Join(os.Getenv("HOME"), "aerolab-keys")
	}

	return info, nil
}

// checkForCollisions checks if any v7 resources would collide with existing v8 resources.
// Returns a list of collision descriptions, or empty slice if no collisions found.
//
// Note: When --force is used, already-migrated resources will appear in both the v7 discovery
// results AND the v8 inventory. This is expected - we detect these "self-collisions" by
// comparing resource IDs and skip them, allowing re-migration to proceed.
func (c *InventoryMigrateCmd) checkForCollisions(system *System, discovery *backends.MigrationResult, inventory *backends.Inventory, backendType backends.BackendType) []string {
	var collisions []string

	if inventory == nil {
		return collisions
	}

	// Build lookup maps for existing v8 resources
	// Key for instances: "clusterName:nodeNo:zone" (zone normalized to region for comparison)
	existingInstances := make(map[string]*backends.Instance)
	// Also build a set of existing instance IDs to detect self-collisions (same resource already migrated)
	existingInstanceIDs := make(map[string]bool)
	for _, inst := range inventory.Instances.Describe() {
		// Only check instances of the same backend type
		if inst.BackendType != backendType {
			continue
		}
		// Normalize zone to region for comparison
		zone := c.zoneToRegion(inst.ZoneName, backendType)
		key := fmt.Sprintf("%s:%d:%s", inst.ClusterName, inst.NodeNo, zone)
		existingInstances[key] = inst
		existingInstanceIDs[inst.InstanceID] = true
	}

	// Key for images: "imageName" (parsed from v7 name pattern)
	existingImages := make(map[string]*backends.Image)
	// Also build a set of existing image IDs to detect self-collisions
	existingImageIDs := make(map[string]bool)
	for _, img := range inventory.Images.Describe() {
		if img.BackendType != backendType {
			continue
		}
		existingImages[img.Name] = img
		existingImageIDs[img.ImageId] = true
	}

	// Key for volumes: "volumeName:zone"
	existingVolumes := make(map[string]*backends.Volume)
	// Also build a set of existing volume IDs to detect self-collisions
	existingVolumeIDs := make(map[string]bool)
	for _, vol := range inventory.Volumes.Describe() {
		if vol.BackendType != backendType {
			continue
		}
		zone := c.zoneToRegion(vol.ZoneName, backendType)
		key := fmt.Sprintf("%s:%s", vol.Name, zone)
		existingVolumes[key] = vol
		existingVolumeIDs[vol.FileSystemId] = true
	}

	// Check instance collisions
	for _, v7Inst := range discovery.DryRunInstances {
		zone := c.zoneToRegion(v7Inst.Zone, backendType)
		key := fmt.Sprintf("%s:%d:%s", v7Inst.ClusterName, v7Inst.NodeNo, zone)
		if existing, ok := existingInstances[key]; ok {
			// Check if this is a self-collision (same resource already migrated, being re-migrated with --force)
			// If the instance IDs match, it's the same resource and not a real collision
			if existingInstanceIDs[v7Inst.InstanceID] {
				continue // Same resource, skip - this allows --force to work on already-migrated resources
			}
			instType := "server"
			if v7Inst.IsClient {
				instType = "client"
			}
			collisions = append(collisions, fmt.Sprintf(
				"INSTANCE: v7 %s '%s' (cluster=%s, node=%d, zone=%s) collides with existing v8 instance '%s' (ID=%s)",
				instType, v7Inst.Name, v7Inst.ClusterName, v7Inst.NodeNo, v7Inst.Zone,
				existing.Name, existing.InstanceID,
			))
		}
	}

	// Check image collisions - for images, we check by the parsed attributes
	// since v7 and v8 may have different naming conventions
	for _, v7Img := range discovery.DryRunImages {
		// Check if an image with the same name exists
		if existing, ok := existingImages[v7Img.Name]; ok {
			// Check if this is a self-collision (same image already migrated)
			if existingImageIDs[v7Img.ImageID] {
				continue // Same resource, skip
			}
			collisions = append(collisions, fmt.Sprintf(
				"IMAGE: v7 image '%s' collides with existing v8 image '%s' (ID=%s)",
				v7Img.Name, existing.Name, existing.ImageId,
			))
		}
	}

	// Check standalone volume collisions (attached volumes inherit instance identity)
	for _, v7Vol := range discovery.DryRunVolumes {
		// Skip attached volumes - they're tied to instance identity
		if v7Vol.AttachedToInstance != "" {
			continue
		}
		zone := c.zoneToRegion(v7Vol.Zone, backendType)
		key := fmt.Sprintf("%s:%s", v7Vol.Name, zone)
		if existing, ok := existingVolumes[key]; ok {
			// Check if this is a self-collision (same volume already migrated)
			if existingVolumeIDs[v7Vol.VolumeID] {
				continue // Same resource, skip
			}
			collisions = append(collisions, fmt.Sprintf(
				"VOLUME: v7 volume '%s' (zone=%s) collides with existing v8 volume '%s' (ID=%s)",
				v7Vol.Name, v7Vol.Zone, existing.Name, existing.FileSystemId,
			))
		}
	}

	return collisions
}

// printDryRunResults prints detailed dry-run output
func (c *InventoryMigrateCmd) printDryRunResults(system *System, result *backends.MigrationResult) {
	system.Logger.Info("=== DRY RUN MODE - No changes will be made ===")
	system.Logger.Info("")

	// Summary
	serverCount := 0
	clientCount := 0
	for _, inst := range result.DryRunInstances {
		if inst.IsClient {
			clientCount++
		} else {
			serverCount++
		}
	}

	system.Logger.Info("Discovered v7 Resources:")
	system.Logger.Info("  Instances: %d (%d servers, %d clients)", len(result.DryRunInstances), serverCount, clientCount)
	system.Logger.Info("  Volumes: %d", len(result.DryRunVolumes))
	system.Logger.Info("  Images: %d", len(result.DryRunImages))
	system.Logger.Info("  Firewalls: %d", len(result.DryRunFirewalls))
	system.Logger.Info("  SSH Keys: %d", len(result.DryRunSSHKeys))
	system.Logger.Info("")

	if c.Verbose {
		// Instance details
		if len(result.DryRunInstances) > 0 {
			system.Logger.Info("Instance Migration Details:")
			for i, inst := range result.DryRunInstances {
				instType := "SERVER"
				if inst.IsClient {
					instType = "CLIENT"
				}
				system.Logger.Info("  [%d] Instance: %s (%s) - %s", i+1, inst.InstanceID, inst.Name, instType)
				system.Logger.Info("      Cluster: %s, Node: %d, Zone: %s", inst.ClusterName, inst.NodeNo, inst.Zone)
				system.Logger.Info("      Tags to ADD:")
				for k, v := range inst.TagsToAdd {
					system.Logger.Info("        %s = %s", k, v)
				}
				if inst.SSHKeyFrom != "" {
					system.Logger.Info("      SSH Key Migration:")
					system.Logger.Info("        FROM: %s", inst.SSHKeyFrom)
					system.Logger.Info("        TO:   %s", inst.SSHKeyTo)
				}
				system.Logger.Info("")
			}
		}

		// Volume details
		if len(result.DryRunVolumes) > 0 {
			system.Logger.Info("Volume Migration Details:")
			for i, vol := range result.DryRunVolumes {
				system.Logger.Info("  [%d] Volume: %s (%s) - %s", i+1, vol.VolumeID, vol.Name, vol.VolumeType)
				system.Logger.Info("      Zone: %s", vol.Zone)
				system.Logger.Info("      Tags to ADD:")
				for k, v := range vol.TagsToAdd {
					system.Logger.Info("        %s = %s", k, v)
				}
				system.Logger.Info("")
			}
		}

		// Image details
		if len(result.DryRunImages) > 0 {
			system.Logger.Info("Image Migration Details:")
			for i, img := range result.DryRunImages {
				system.Logger.Info("  [%d] Image: %s (%s)", i+1, img.ImageID, img.Name)
				if img.OSName != "" {
					system.Logger.Info("      Parsed: OS=%s, Version=%s, Arch=%s", img.OSName, img.OSVersion, img.Architecture)
				}
				system.Logger.Info("      Tags to ADD:")
				for k, v := range img.TagsToAdd {
					system.Logger.Info("        %s = %s", k, v)
				}
				system.Logger.Info("")
			}
		}

		// Firewall details
		if len(result.DryRunFirewalls) > 0 {
			system.Logger.Info("Firewall Migration Details:")
			for i, fw := range result.DryRunFirewalls {
				system.Logger.Info("  [%d] Firewall: %s (%s)", i+1, fw.FirewallID, fw.Name)
				system.Logger.Info("      Zone: %s, VPC: %s", fw.Zone, fw.VPCID)
				if fw.MigrationStatus == "skipped" {
					system.Logger.Info("      Status: SKIPPED - %s", fw.MigrationError)
				} else if len(fw.TagsToAdd) > 0 {
					system.Logger.Info("      Tags to ADD:")
					for k, v := range fw.TagsToAdd {
						system.Logger.Info("        %s = %s", k, v)
					}
				}
				system.Logger.Info("")
			}
		}

		// SSH Key details
		if len(result.DryRunSSHKeys) > 0 {
			system.Logger.Info("SSH Key Migration:")
			for i, key := range result.DryRunSSHKeys {
				system.Logger.Info("  [%d] %s", i+1, key.ClusterName)
				system.Logger.Info("      FROM: %s", key.FromPath)
				system.Logger.Info("      TO:   %s", key.ToPath)
				if key.Error != "" {
					system.Logger.Warn("      WARNING: %s", key.Error)
				}
			}
			system.Logger.Info("")
		}
	}

	// Final summary
	sshKeyWarnings := 0
	for _, key := range result.DryRunSSHKeys {
		if key.Error != "" {
			sshKeyWarnings++
		}
	}
	firewallSkipped := 0
	for _, fw := range result.DryRunFirewalls {
		if fw.MigrationStatus == "skipped" {
			firewallSkipped++
		}
	}
	system.Logger.Info("Summary:")
	system.Logger.Info("  Would migrate %d instances", len(result.DryRunInstances))
	system.Logger.Info("  Would migrate %d volumes", len(result.DryRunVolumes))
	system.Logger.Info("  Would migrate %d images", len(result.DryRunImages))
	if firewallSkipped > 0 {
		system.Logger.Info("  Would migrate %d firewalls (%d skipped - no label support)", len(result.DryRunFirewalls)-firewallSkipped, firewallSkipped)
	} else {
		system.Logger.Info("  Would migrate %d firewalls", len(result.DryRunFirewalls))
	}
	if sshKeyWarnings > 0 {
		system.Logger.Info("  Would copy %d SSH keys (%d not found)", len(result.DryRunSSHKeys), sshKeyWarnings)
	} else {
		system.Logger.Info("  Would copy %d SSH keys", len(result.DryRunSSHKeys))
	}
	system.Logger.Info("")
}

// printMigrationResults prints actual migration output
func (c *InventoryMigrateCmd) printMigrationResults(system *System, result *backends.MigrationResult) {
	system.Logger.Info("=== MIGRATION COMPLETE ===")
	system.Logger.Info("")

	// Count successes and failures
	instanceSuccess := 0
	instanceFail := 0
	for _, inst := range result.MigratedInstances {
		if inst.MigrationStatus == "success" {
			instanceSuccess++
		} else {
			instanceFail++
		}
	}

	volumeSuccess := 0
	volumeFail := 0
	for _, vol := range result.MigratedVolumes {
		if vol.MigrationStatus == "success" {
			volumeSuccess++
		} else {
			volumeFail++
		}
	}

	imageSuccess := 0
	imageFail := 0
	for _, img := range result.MigratedImages {
		if img.MigrationStatus == "success" {
			imageSuccess++
		} else {
			imageFail++
		}
	}

	firewallSuccess := 0
	firewallSkipped := 0
	firewallFail := 0
	for _, fw := range result.MigratedFirewalls {
		switch fw.MigrationStatus {
		case "success":
			firewallSuccess++
		case "skipped":
			firewallSkipped++
		default:
			firewallFail++
		}
	}

	sshKeySuccess := result.SSHKeysMigrated

	// Print summary
	system.Logger.Info("Migrated Resources:")
	if instanceFail == 0 {
		system.Logger.Info("  ✓ Instances: %d/%d migrated", instanceSuccess, len(result.MigratedInstances))
	} else {
		system.Logger.Warn("  ⚠ Instances: %d/%d migrated (%d failed)", instanceSuccess, len(result.MigratedInstances), instanceFail)
	}
	if volumeFail == 0 {
		system.Logger.Info("  ✓ Volumes: %d/%d migrated", volumeSuccess, len(result.MigratedVolumes))
	} else {
		system.Logger.Warn("  ⚠ Volumes: %d/%d migrated (%d failed)", volumeSuccess, len(result.MigratedVolumes), volumeFail)
	}
	if imageFail == 0 {
		system.Logger.Info("  ✓ Images: %d/%d migrated", imageSuccess, len(result.MigratedImages))
	} else {
		system.Logger.Warn("  ⚠ Images: %d/%d migrated (%d failed)", imageSuccess, len(result.MigratedImages), imageFail)
	}
	if firewallFail == 0 {
		if firewallSkipped > 0 {
			system.Logger.Info("  ✓ Firewalls: %d/%d migrated (%d skipped)", firewallSuccess, len(result.MigratedFirewalls), firewallSkipped)
		} else {
			system.Logger.Info("  ✓ Firewalls: %d/%d migrated", firewallSuccess, len(result.MigratedFirewalls))
		}
	} else {
		system.Logger.Warn("  ⚠ Firewalls: %d/%d migrated (%d failed, %d skipped)", firewallSuccess, len(result.MigratedFirewalls), firewallFail, firewallSkipped)
	}
	system.Logger.Info("  ✓ SSH Keys: %d copied", sshKeySuccess)
	system.Logger.Info("")

	if c.Verbose {
		// Instance details
		if len(result.MigratedInstances) > 0 {
			system.Logger.Info("Instance Details:")
			for _, inst := range result.MigratedInstances {
				if inst.MigrationStatus == "success" {
					system.Logger.Info("  ✓ %s (%s) - %d tags added", inst.InstanceID, inst.Name, len(inst.TagsAdded))
				} else {
					system.Logger.Error("  ✗ %s (%s) - %s", inst.InstanceID, inst.Name, inst.MigrationError)
				}
			}
			system.Logger.Info("")
		}

		// Volume details
		if len(result.MigratedVolumes) > 0 {
			system.Logger.Info("Volume Details:")
			for _, vol := range result.MigratedVolumes {
				if vol.MigrationStatus == "success" {
					system.Logger.Info("  ✓ %s (%s) - %d tags added", vol.VolumeID, vol.Name, len(vol.TagsAdded))
				} else {
					system.Logger.Error("  ✗ %s (%s) - %s", vol.VolumeID, vol.Name, vol.MigrationError)
				}
			}
			system.Logger.Info("")
		}

		// Image details
		if len(result.MigratedImages) > 0 {
			system.Logger.Info("Image Details:")
			for _, img := range result.MigratedImages {
				if img.MigrationStatus == "success" {
					system.Logger.Info("  ✓ %s - %d tags added", img.Name, len(img.TagsAdded))
				} else {
					system.Logger.Error("  ✗ %s - %s", img.Name, img.MigrationError)
				}
			}
			system.Logger.Info("")
		}

		// Firewall details
		if len(result.MigratedFirewalls) > 0 {
			system.Logger.Info("Firewall Details:")
			for _, fw := range result.MigratedFirewalls {
				switch fw.MigrationStatus {
				case "success":
					system.Logger.Info("  ✓ %s (%s) - %d tags added", fw.Name, fw.FirewallID, len(fw.TagsAdded))
				case "skipped":
					system.Logger.Info("  ⊘ %s (%s) - skipped", fw.Name, fw.FirewallID)
				default:
					system.Logger.Error("  ✗ %s (%s) - %s", fw.Name, fw.FirewallID, fw.MigrationError)
				}
			}
			system.Logger.Info("")
		}
	}

	// Print warnings
	if len(result.Errors) > 0 {
		system.Logger.Info("")
		system.Logger.Warn("Warnings/Errors:")
		for _, err := range result.Errors {
			system.Logger.Warn("  - %s", err.Error())
		}
	}

}

// printExpiryDryRun prints what expiry system installation would happen in dry-run mode
func (c *InventoryMigrateCmd) printExpiryDryRun(system *System, backendType backends.BackendType, result *backends.MigrationResult) {
	// Collect regions that have resources with expiry
	regionsWithExpiry := make(map[string]bool)

	// Check dry-run instances for expiry tags
	for _, inst := range result.DryRunInstances {
		// Check if any expiry-related tag would be set
		// AWS uses AEROLAB_EXPIRES, GCP uses aerolab-expires (native label)
		if inst.TagsToAdd["AEROLAB_EXPIRES"] != "" || inst.TagsToAdd["aerolab-expires"] != "" {
			region := c.zoneToRegion(inst.Zone, backendType)
			if region != "" {
				regionsWithExpiry[region] = true
			}
		}
	}

	// Check dry-run volumes for expiry tags
	for _, vol := range result.DryRunVolumes {
		// Volumes use the same expiry tags
		if vol.TagsToAdd["AEROLAB_EXPIRES"] != "" || vol.TagsToAdd["aerolab-expires"] != "" {
			region := c.zoneToRegion(vol.Zone, backendType)
			if region != "" {
				regionsWithExpiry[region] = true
			}
		}
	}

	if len(regionsWithExpiry) == 0 {
		return
	}

	// Convert to slice
	regions := make([]string, 0, len(regionsWithExpiry))
	for region := range regionsWithExpiry {
		regions = append(regions, region)
	}

	system.Logger.Info("Expiry System:")
	system.Logger.Info("  Would install v8 expiry system for regions: %s", strings.Join(regions, ", "))
	system.Logger.Info("  (Required for migrated resources with expiry set to auto-expire)")
	system.Logger.Info("")
}

// installExpiryIfNeeded installs the expiry system asynchronously if any migrated resources have expiry set
func (c *InventoryMigrateCmd) installExpiryIfNeeded(system *System, backendType backends.BackendType, result *backends.MigrationResult) {
	// Collect regions that have resources with expiry
	regionsWithExpiry := make(map[string]bool)

	// Check migrated instances for expiry tags
	for _, inst := range result.MigratedInstances {
		if inst.MigrationStatus != "success" {
			continue
		}
		// Check if any expiry-related tag was set
		// AWS uses AEROLAB_EXPIRES, GCP uses aerolab-expires (native label)
		if inst.TagsAdded["AEROLAB_EXPIRES"] != "" || inst.TagsAdded["aerolab-expires"] != "" {
			region := c.zoneToRegion(inst.Zone, backendType)
			if region != "" {
				regionsWithExpiry[region] = true
			}
		}
	}

	// Check migrated volumes for expiry tags
	for _, vol := range result.MigratedVolumes {
		if vol.MigrationStatus != "success" {
			continue
		}
		// Volumes use the same expiry tags
		if vol.TagsAdded["AEROLAB_EXPIRES"] != "" || vol.TagsAdded["aerolab-expires"] != "" {
			region := c.zoneToRegion(vol.Zone, backendType)
			if region != "" {
				regionsWithExpiry[region] = true
			}
		}
	}

	if len(regionsWithExpiry) == 0 {
		return
	}

	// Convert to slice
	regions := make([]string, 0, len(regionsWithExpiry))
	for region := range regionsWithExpiry {
		regions = append(regions, region)
	}

	system.Logger.Info("")
	system.Logger.Info("Installing v8 expiry system for migrated resources with expiry set...")
	system.Logger.Info("  Regions: %s", strings.Join(regions, ", "))

	// Check for v7 expiry system and warn user
	warnIfV7ExpiryInstalled(system.Backend, backendType, system.Logger)

	// Install expiry system in background, but wait for completion
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := system.Backend.ExpiryInstall(backendType, 15, 4, false, false, false, true, regions...)
		if err != nil {
			system.Logger.Error("Error installing expiry system: %s", err)
			system.Logger.Error("Migrated resources with expiry may not auto-expire.")
			system.Logger.Error("Run 'aerolab config expiry-install' to manually install the expiry system.")
		} else {
			system.Logger.Info("Expiry system installed successfully for regions: %s", strings.Join(regions, ", "))
		}
	}()
	wg.Wait()
}

// checkForOldAGIInstances checks the discovery results for old AGI instances
// and returns the count of AGI instances and AGI Monitor instances found,
// along with the list of AGI Monitor instance names (for user notification).
// AGI instances are detected by:
// - aerolab.type = "agi" in TagsToAdd (set during translation for v7 AGI instances)
// - Presence of preserved AGI-specific tags (agiLabel, agiinstance, etc.)
// AGI Monitor instances are detected by:
// - aerolab.client.type = "agimonitor" in TagsToAdd
// - Presence of agimUrl/agimZone tags
func (c *InventoryMigrateCmd) checkForOldAGIInstances(system *System, result *backends.MigrationResult) (agiCount int, agiMonitorCount int, agiMonitorNames []string) {
	for _, inst := range result.DryRunInstances {
		// Check for AGI instance (detected by aerolab.type = "agi" or preserved AGI tags)
		isAGI := false
		if inst.TagsToAdd["aerolab.type"] == "agi" {
			isAGI = true
		} else {
			// Also check for preserved AGI-specific tags (both AWS tags and GCP labels)
			agiTags := []string{
				"aerolab7agiav", "agiLabel", "agilabel0", "aginodim", "agiinstance",
				"agiSrcLocal", "agiSrcSftp", "agiSrcS3", "agiDomain",
				// GCP lowercase variants
				"agisrclocal", "agisrcsftp", "agisrcs3", "agidomain",
			}
			for _, tag := range agiTags {
				if _, ok := inst.TagsToAdd[tag]; ok {
					isAGI = true
					break
				}
			}
		}

		// Check for AGI Monitor (client type "agimonitor" or agimUrl/agimZone tags)
		isAGIMonitor := false
		if inst.TagsToAdd["aerolab.client.type"] == "agimonitor" ||
			inst.TagsToAdd["aerolab-client-type"] == "agimonitor" {
			isAGIMonitor = true
		} else {
			// Check for monitor-specific tags
			monitorTags := []string{"agimUrl", "agimZone", "agimurl", "agimzone"}
			for _, tag := range monitorTags {
				if _, ok := inst.TagsToAdd[tag]; ok {
					isAGIMonitor = true
					break
				}
			}
		}

		if isAGI {
			agiCount++
			if c.Verbose {
				system.Logger.Info("  AGI instance: %s (%s) in %s", inst.Name, inst.InstanceID, inst.Zone)
			}
		}
		if isAGIMonitor {
			agiMonitorCount++
			agiMonitorNames = append(agiMonitorNames, inst.ClusterName)
			if c.Verbose {
				system.Logger.Info("  AGI Monitor instance: %s (%s) in %s", inst.Name, inst.InstanceID, inst.Zone)
			}
		}
	}
	return agiCount, agiMonitorCount, agiMonitorNames
}

// zoneToRegion converts a zone to a region based on the backend type
func (c *InventoryMigrateCmd) zoneToRegion(zone string, backendType backends.BackendType) string {
	if zone == "" {
		return ""
	}

	switch backendType {
	case backends.BackendTypeAWS:
		// AWS zones are like "us-east-1a", region is "us-east-1"
		// Check if last character is a letter (zone suffix)
		if len(zone) > 0 {
			lastChar := zone[len(zone)-1]
			if lastChar >= 'a' && lastChar <= 'z' {
				return zone[:len(zone)-1]
			}
		}
		return zone
	case backends.BackendTypeGCP:
		// GCP zones are like "us-central1-a", region is "us-central1"
		// Find last dash and remove suffix
		if idx := strings.LastIndex(zone, "-"); idx != -1 {
			return zone[:idx]
		}
		return zone
	default:
		return zone
	}
}
