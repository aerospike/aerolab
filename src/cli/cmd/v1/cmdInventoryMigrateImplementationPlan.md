# Inventory Migrate Implementation Plan

## Overview

The `inventory migrate` command migrates AeroLab v7 resources (instances, volumes, images) to the v8 tagging format. This allows existing v7 resources to be managed by AeroLab v8 without recreating them.

## Scope

### Supported Backends
- **AWS** - Full support
- **GCP** - Full support  
- **Docker** - **NOT SUPPORTED** (returns error immediately)

### Resources to Migrate
1. **Instances** (servers and clients)
2. **Volumes** (EFS/EBS in AWS, Persistent Disks in GCP)
3. **Images** (AMIs in AWS, Custom Images in GCP)

---

## Phase 1: Tag Translation Reference

### 1.1 AWS Instance Tag Translation

**Server Instances (UsedBy=aerolab4):**

| Old v7 Tag | New v8 Tag | Notes |
|------------|------------|-------|
| `UsedBy` = "aerolab4" | Keep as-is | Preserved for backward compatibility |
| `Name` | Keep as-is | Instance naming unchanged |
| `Aerolab4ClusterName` | `AEROLAB_CLUSTER_NAME` | Direct mapping |
| `Aerolab4NodeNumber` | `AEROLAB_NODE_NO` | Direct mapping |
| `Aerolab4OperatingSystem` | `AEROLAB_OS_NAME` | Direct mapping |
| `Aerolab4OperatingSystemVersion` | `AEROLAB_OS_VERSION` | Direct mapping |
| `Aerolab4AerospikeVersion` | `aerolab.soft.version` | Software version |
| `Arch` | `v7-arch` | Preserve architecture tag |
| `aerolab4expires` | `AEROLAB_EXPIRES` | Direct mapping |
| `owner` | `AEROLAB_OWNER` | Direct mapping |
| `Aerolab4CostPerHour` | `AEROLAB_COST_PPH` | Direct mapping |
| `Aerolab4CostSoFar` | `AEROLAB_COST_SO_FAR` | Direct mapping |
| `Aerolab4CostStartTime` | `AEROLAB_LAST_START_TIME` | Direct mapping |
| N/A | `aerolab.type` | Set to "aerospike" for servers |
| N/A | `AEROLAB_PROJECT` | Set to configured project |
| N/A | `AEROLAB_VERSION` | Set to current aerolab version |
| N/A | `AEROLAB_CLUSTER_UUID` | Generate new UUID per cluster+region |
| N/A | `AEROLAB_V7_MIGRATED` | Set to "true" (marker) |

**Client Instances (UsedBy=aerolab4client):**

| Old v7 Tag | New v8 Tag | Notes |
|------------|------------|-------|
| `UsedBy` = "aerolab4client" | Keep as-is | Preserved for backward compatibility |
| `Aerolab4clientClusterName` | `AEROLAB_CLUSTER_NAME` | Direct mapping |
| `Aerolab4clientNodeNumber` | `AEROLAB_NODE_NO` | Direct mapping |
| `Aerolab4clientOperatingSystem` | `AEROLAB_OS_NAME` | Direct mapping |
| `Aerolab4clientOperatingSystemVersion` | `AEROLAB_OS_VERSION` | Direct mapping |
| `Aerolab4clientAerospikeVersion` | `aerolab.soft.version` | Software version (if present) |
| `Aerolab4clientType` | `aerolab.type` | Client type (e.g., "tools", "graph", "ams", "vscode") |
| `owner` | `AEROLAB_OWNER` | Direct mapping (if present) |
| `aerolab4expires` | `AEROLAB_EXPIRES` | Direct mapping (if present) |

**Tags to Preserve/Migrate:**
- `telemetry` → `aerolab.telemetry` (migrated to new v8 telemetry tag)
- `aerolab7spot` → `v7-spot`
- `Arch` → `v7-arch`

**AGI-Specific Instance Tags (preserve with v7- prefix):**

| Old v7 Tag | New v8 Tag | Notes |
|------------|------------|-------|
| `aerolab7agiav` | `v7-agiav` | AGI Aerospike version |
| `aerolab4features` | `v7-features` | Feature flags bitmask |
| `aerolab4ssl` | `v7-ssl` | SSL enabled flag |
| `agiLabel` | `v7-agilabel` | AGI label string |
| `agiinstance` | `v7-agiinstance` | Instance type used |
| `aginodim` | `v7-aginodim` | NoDIM mode flag |
| `termonpow` | `v7-termonpow` | Terminate on poweroff |
| `isspot` | `v7-isspot` | Spot instance flag |
| `agiSrcLocal` | `v7-agisrclocal` | Local source string |
| `agiSrcSftp` | `v7-agisrcsftp` | SFTP source string |
| `agiSrcS3` | `v7-agisrcs3` | S3 source string |
| `agiDomain` | `v7-agidomain` | AGI Route53 domain |

**Important Type/Version Mapping Notes:**
- **Server instances**: Always set `aerolab.type` = "aerospike"
- **Client instances**: Set `aerolab.type` from `Aerolab4clientType` (e.g., "tools", "graph", "ams", "vscode")
- **Software version**: Map `Aerolab4AerospikeVersion` or `Aerolab4clientAerospikeVersion` to `aerolab.soft.version`
- The `aerolab.type` tag is used by the v8 backend to compute `AccessURL` for web-accessible clients

### 1.2 GCP Instance Label Translation

GCP uses `encodeToLabels()` which creates:
1. Base32-encoded JSON blob in `aerolab-metadata-{N}` labels
2. Native labels for common fields (for filtering)

**Server Instances (used_by=aerolab4):**

| Old v7 Label | Maps to v8 Metadata Key | Notes |
|--------------|------------------------|-------|
| `used_by` = "aerolab4" | Keep as-is | Preserved for backward compatibility |
| `aerolab4cluster_name` | `aerolab-cn` | Cluster name |
| `aerolab4node_number` | `aerolab-nn` | Node number |
| `aerolab4operating_system` | `aerolab-os` | OS name |
| `aerolab4operating_system_version` | `aerolab-ov` | OS version |
| `aerolab4aerospike_version` | `aerolab.soft.version` | Software version |
| `aerolab4expires` | `aerolab-e` | Expiry timestamp |
| `owner` | `aerolab-o` | Owner |
| `aerolab_cost_ph` | `aerolab-cp` | Cost per hour |
| `aerolab_cost_sofar` | `aerolab-cs` | Cost so far |
| `aerolab_cost_starttime` | `aerolab-st` | Start time |
| `arch` | `v7-arch` | Preserve architecture |
| N/A | `aerolab.type` | Set to "aerospike" |
| `telemetry` | `aerolab.telemetry` | Migrated to new v8 telemetry tag |
| `isspot` | preserve as `v7-isspot` | Preserve with prefix |

**Client Instances (used_by=aerolab4client):**

| Old v7 Label | Maps to v8 Metadata Key | Notes |
|--------------|------------------------|-------|
| `used_by` = "aerolab4client" | Keep as-is | Preserved for backward compatibility |
| `aerolab4client_name` | `aerolab-cn` | Cluster name |
| `aerolab4client_node_number` | `aerolab-nn` | Node number |
| `aerolab4client_operating_system` | `aerolab-os` | OS name |
| `aerolab4client_operating_system_version` | `aerolab-ov` | OS version |
| `aerolab4client_aerospike_version` | `aerolab.soft.version` | Software version (if present) |
| `aerolab4client_type` | `aerolab.type` | Client type (tools, graph, ams, vscode, etc.) |
| `owner` | `aerolab-o` | Owner (if present) |
| `aerolab4expires` | `aerolab-e` | Expiry timestamp (if present) |

**New labels added:**
- `usedby` = `aerolab` (native label for filtering)
- `aerolab-v7-migrated` = `true` (native label marker)
- `aerolab-metadata-{N}` = encoded metadata chunks

**GCP Label Constraints:**
- Max 64 labels total per resource
- Max 63 characters per label value
- Only lowercase letters, numbers, underscores, hyphens
- Must start with lowercase letter

**GCP Label Encoding Reference:**
When v7 encoded values for GCP labels:
- **Timestamps (RFC3339)**: Colons (`:`) → underscores (`_`), Plus (`+`) → dashes (`-`), lowercase
  - Example: `2024-01-15T10:30:00+00:00` → `2024-01-15t10_30_00-00_00`
- **Durations**: Periods (`.`) → underscores (`_`), lowercase
  - Example: `24h0m0s` → `24h0m0s`
- **Long values (AGI labels)**: Base32 encoded, no padding, lowercase, chunked to 63 chars
  - Uses labels: `agilabel0`, `agilabel1`, `agilabel2`, etc.
- **Version numbers**: Dots replaced with dashes
  - Example: `7.0.0` → `7-0-0`, `22.04` → `22-04`

**GCP Label Limit Strategy:**
If adding new labels would exceed 64 total:
1. Count current labels + new labels needed
2. If over 64, identify removable old v7 labels (those already translated to new format)
3. Remove old labels that have been migrated to new format
4. Prioritize keeping: new v8 labels > migration marker > v7-prefixed preserved tags

### 1.3 Volume Tag Translation

**AWS EFS/EBS Volumes:**

| Old v7 Tag | New v8 Tag | Notes |
|------------|------------|-------|
| `Name` | `Name` (unchanged) | Volume name preserved |
| `UsedBy` = "aerolab7" | Keep as-is | Preserved for backward compatibility |
| `lastUsed` | `AEROLAB_LAST_START_TIME` | RFC3339 timestamp |
| `expireDuration` | `v7-expireduration` | Preserve with prefix |
| `aerolab7owner` | `AEROLAB_OWNER` | Owner name |
| `agiLabel` | `v7-agilabel` | AGI label string |
| Custom tags | Keep as-is | User-defined tags preserved |
| N/A | `AEROLAB_V7_MIGRATED` | Set to "true" (marker) |
| N/A | `AEROLAB_PROJECT` | Set to configured project |

**GCP Persistent Disk Labels:**

| Old v7 Label | Maps to v8 Label | Notes |
|--------------|------------------|-------|
| `usedby` = "aerolab7" | Keep as-is | Preserved for backward compatibility |
| `lastused` | `aerolab-st` | Last used timestamp (encoded) |
| `expireduration` | `v7-expireduration` | Preserve with prefix |
| `aerolab7owner` | `aerolab-o` | Owner name |
| `agilabel{N}` | `v7-agilabel{N}` | AGI label (base32 encoded chunks) |
| `agiinstance` | `v7-agiinstance` | Instance type |
| `aginodim` | `v7-aginodim` | NoDIM mode flag |
| `termonpow` | `v7-termonpow` | Terminate on poweroff |
| `isspot` | `v7-isspot` | Spot instance flag |
| N/A | `aerolab-v7-migrated` | Set to "true" (marker) |
| N/A | `aerolab-p` | Set to configured project |

**Note on GCP Volume AGI Labels:**
- AGI labels in GCP are base32 encoded and chunked into multiple labels (agilabel0, agilabel1, etc.)
- During migration, preserve all chunks with v7- prefix (v7-agilabel0, v7-agilabel1, etc.)

### 1.4 Image Tag Translation

**AWS AMIs (name pattern: aerolab4-template-{distro}_{version}_{aerospikeVersion}_{arch}):**

Example: `aerolab4-template-ubuntu_22.04_7.0.0_amd`

| Old Tag/Attribute | New v8 Tag | Notes |
|-------------------|------------|-------|
| Image Name | Parse for distro, version, arch | Pattern: `aerolab4-template-{distro}_{osVersion}_{asVersion}_{arch}` |
| N/A | `AEROLAB_OS_NAME` | Parsed from image name (e.g., "ubuntu") |
| N/A | `AEROLAB_OS_VERSION` | Parsed from image name (e.g., "22.04") |
| N/A | `aerolab.soft.version` | Parsed Aerospike version (e.g., "7.0.0") |
| N/A | `v7-arch` | Parsed architecture (e.g., "amd", "arm") |
| N/A | `AEROLAB_PROJECT` | Set to configured project |
| N/A | `AEROLAB_VERSION` | Set to current aerolab version |
| N/A | `AEROLAB_V7_MIGRATED` | Set to "true" (marker) |

**GCP Images (name pattern: aerolab4-template-{distro}-{version}-{aerospikeVersion}-{arch}):**

Example: `aerolab4-template-ubuntu-22-04-7-0-0-amd`

| Old Label/Attribute | New v8 Label | Notes |
|---------------------|--------------|-------|
| Image Name | Parse for distro, version, arch | Pattern: `aerolab4-template-{distro}-{osVersion}-{asVersion}-{arch}` |
| N/A | `aerolab-os` | Parsed from image name (e.g., "ubuntu") |
| N/A | `aerolab-ov` | Parsed from image name (e.g., "22-04" → "22.04") |
| N/A | `aerolab.soft.version` | Parsed Aerospike version (e.g., "7-0-0" → "7.0.0") |
| N/A | `v7-arch` | Parsed architecture (e.g., "amd", "arm") |
| N/A | `aerolab-p` | Set to configured project |
| N/A | `aerolab-v` | Set to current aerolab version |
| N/A | `aerolab-v7-migrated` | Set to "true" (marker) |

**Image Name Parsing Notes:**
- AWS uses underscores as delimiters: `aerolab4-template-ubuntu_22.04_7.0.0_amd`
- GCP uses dashes as delimiters (dots replaced): `aerolab4-template-ubuntu-22-04-7-0-0-amd`
- When parsing GCP names, convert dashes back to dots for version numbers

### 1.5 New Tags Introduced During Migration

The migration introduces these tags that may not have existed in v7:

| Tag Name | Value | Purpose |
|----------|-------|---------|
| `aerolab.type` | "aerospike" for servers, client type for clients | Identifies instance type for access URL computation |
| `aerolab.soft.version` | Aerospike version string (e.g., "7.0.0") | Tracks software version installed |
| `aerolab.telemetry` (AWS) / `aerolab-telemetry` (GCP) | Value from old `telemetry` tag | Migrated telemetry tracking tag |
| `AEROLAB_V7_MIGRATED` (AWS) / `aerolab-v7-migrated` (GCP) | "true" | Marker to identify migrated instances |
| `AEROLAB_CLUSTER_UUID` | UUID string | Generated during migration for cluster grouping |
| `AEROLAB_PROJECT` | Project name from config | Links instance to aerolab project |
| `AEROLAB_VERSION` | Current aerolab version | Records which version performed migration |

**Note on `aerolab.telemetry`:**
- This tag is now used in v8 to track whether telemetry was enabled when the resource was created
- Old v7 `telemetry` tags are migrated to this new format
- For AWS: uses `aerolab.telemetry` (dots allowed in tag names)
- For GCP: uses `aerolab-telemetry` (dots not allowed in label names)

**Note on `aerolab.type`:**
- This tag is already used by v8 to compute `AccessURL` for web-accessible clients
- For **server** instances: always set to "aerospike"
- For **client** instances: set from `Aerolab4clientType` (e.g., "tools", "graph", "ams", "vscode", "trino", "rest-gateway")
- The `computeCloudAccessURL()` function uses this tag to determine the correct port for each client type

**GCP Label Name Translation:**
Since GCP labels must be lowercase with only letters, numbers, underscores, and hyphens:
- These tags are stored in the JSON metadata blob (via `encodeToLabels()`)
- They are decoded back to their original names when reading instances
- The internal encoding handles the dot notation transparently

---

## Phase 2: SSH Key Migration

### 2.1 Key Path Formats

**Old v7 Paths:**
```
AWS: {SshKeyPath}/aerolab-{clusterName}_{region}
     Example: ~/aerolab-keys/aerolab-mydc_us-east-1
     Note: Private key only (no .pub file needed for AWS)

GCP: {SshKeyPath}/aerolab-gcp-{clusterName}
     {SshKeyPath}/aerolab-gcp-{clusterName}.pub
     Example: ~/aerolab-keys/aerolab-gcp-mydc
              ~/aerolab-keys/aerolab-gcp-mydc.pub
     Note: Both private and public keys required for GCP
```

**New v8 Paths:**
```
All backends: {rootDir}/{project}/ssh-keys/{backend}/{project}
              {rootDir}/{project}/ssh-keys/{backend}/{project}.pub
Example: ~/.aerolab/myproject/ssh-keys/aws/myproject
```

**Default v7 SshKeyPath:** `${HOME}/aerolab-keys/`

**Shared Key Override:**
- Location: `${AEROLAB_HOME}/sshkey` (default: `~/.aerolab/sshkey`)
- GCP also requires: `${AEROLAB_HOME}/sshkey.pub`
- AWS shared key name in EC2: `manual-aerolab-agi-shared`
- GCP shared key name: `sshkey`

### 2.2 Migration Strategy

Copy old keys to `{sshKeysDir}/old/` subdirectory:
```
AWS Destination: {rootDir}/{project}/ssh-keys/aws/old/aerolab-{clusterName}_{region}
Example: ~/.aerolab/myproject/ssh-keys/aws/old/aerolab-mydc_us-east-1

GCP Destination: {rootDir}/{project}/ssh-keys/gcp/old/aerolab-gcp-{clusterName}
                 {rootDir}/{project}/ssh-keys/gcp/old/aerolab-gcp-{clusterName}.pub
Example: ~/.aerolab/myproject/ssh-keys/gcp/old/aerolab-gcp-mydc
         ~/.aerolab/myproject/ssh-keys/gcp/old/aerolab-gcp-mydc.pub
```

**SSH Key Migration Flow:**
1. Check for shared key at `${AEROLAB_HOME}/sshkey`:
   - If exists, skip all per-cluster key migration
   - Shared key will be used for all instances (v7 and v8)
   - For GCP, also check `${AEROLAB_HOME}/sshkey.pub` exists
2. For each cluster being migrated:
   - AWS: Determine old key path: `{SshKeyPath}/aerolab-{clusterName}_{region}`
   - GCP: Determine old key path: `{SshKeyPath}/aerolab-gcp-{clusterName}`
   - Check if key exists at old location
   - Copy (not move) to `{sshKeysDir}/old/{key-name}`
   - **AWS**: Copy private key only
   - **GCP**: Copy both private key AND `.pub` file
   - Track migrated keys to avoid duplicate copies (same key shared by nodes)

### 2.2.1 Key Scope Differences

**AWS Key Scope:**
- Keys are scoped per **cluster + region**
- If cluster "mydc" exists in us-east-1 and eu-west-1, two separate keys exist
- Key registered with EC2 Key Pairs service in each region

**GCP Key Scope:**
- Keys are scoped per **cluster only** (NO region component)
- Same key used regardless of which zone instances are in
- Keys NOT registered with GCP (local files + instance metadata only)

### 2.3 Post-Migration Key Resolution

Update `InstancesGetSSHKeyPath` to check for migrated instances:

```go
func (s *b) InstancesGetSSHKeyPath(instances backends.InstanceList) []string {
    out := []string{}
    for _, inst := range instances {
        // Check if this is a migrated v7 instance
        if inst.Tags["AEROLAB_V7_MIGRATED"] == "true" {
            oldKeyPath := s.getOldSSHKeyPath(inst)
            if _, err := os.Stat(oldKeyPath); err == nil {
                out = append(out, oldKeyPath)
                continue
            }
        }
        // Default to project key
        out = append(out, path.Join(s.sshKeysDir, s.project))
    }
    return out
}
```

---

## Phase 3: Discovery Implementation

### 3.1 AWS Discovery

```go
// oldInstance represents a discovered v7 instance for migration
type oldInstance struct {
    InstanceID  string
    Name        string
    ClusterName string
    NodeNo      int
    Zone        string  // region for AWS
    IsClient    bool
    Tags        map[string]string
}

// discoverOldInstances finds v7 instances across all enabled regions
func (s *b) discoverOldInstances(force bool) ([]oldInstance, error) {
    var instances []oldInstance
    
    zones, _ := s.ListEnabledZones()
    for _, zone := range zones {
        cli, err := getEc2Client(s.credentials, &zone)
        if err != nil {
            return nil, err
        }
        
        // AWS EC2 filters for v7 instances
        listFilters := []types.Filter{
            {
                Name:   aws.String("tag:UsedBy"),
                Values: []string{"aerolab4", "aerolab4client"},
            },
            {
                Name:   aws.String("instance-state-name"),
                Values: []string{"pending", "running", "stopping", "stopped"},
            },
        }
        
        paginator := ec2.NewDescribeInstancesPaginator(cli, &ec2.DescribeInstancesInput{
            Filters: listFilters,
        })
        
        for paginator.HasMorePages() {
            out, err := paginator.NextPage(context.TODO())
            if err != nil {
                return nil, err
            }
            for _, res := range out.Reservations {
                for _, inst := range res.Instances {
                    // Post-filter: exclude already migrated (unless --force)
                    // NOTE: AWS EC2 filters don't support "tag does NOT exist"
                    // so we must filter in code after fetching
                    if !force && hasTag(inst.Tags, TAG_V7_MIGRATED) {
                        continue
                    }
                    instances = append(instances, convertToOldInstance(inst, zone))
                }
            }
        }
    }
    return instances, nil
}

// discoverOldVolumes finds v7 EFS and EBS volumes
func (s *b) discoverOldVolumes(force bool) ([]oldVolume, error) {
    // EFS (shared volumes): Filter by tag UsedBy = "aerolab7"
    // EBS (attached volumes): Check for aerolab-related tags
    // Post-filter: exclude if AEROLAB_V7_MIGRATED tag exists (unless force)
}

// discoverOldImages finds v7 template AMIs
func (s *b) discoverOldImages(force bool) ([]oldImage, error) {
    // Filter by name pattern: "aerolab4-template-*"
    // Use ec2.DescribeImages with Filters on name
    // Post-filter: exclude if AEROLAB_V7_MIGRATED tag exists (unless force)
}
```

### 3.2 GCP Discovery

```go
// discoverOldInstances finds v7 instances
func (s *b) discoverOldInstances(force bool) ([]oldInstance, error) {
    // GCP filter string for v7 instances
    // Filter: labels.used_by="aerolab4" OR labels.used_by="aerolab4client"
    filter := `labels.used_by="aerolab4" OR labels.used_by="aerolab4client"`
    
    // Post-filter in code: exclude if labels.usedby = "aerolab" (already v8 format)
    // unless --force is set
}

// discoverOldVolumes finds v7 persistent disks
func (s *b) discoverOldVolumes(force bool) ([]oldVolume, error) {
    // Filter: labels.usedby = "aerolab7"
    // Post-filter: exclude if already has v8 labels (unless force)
}

// discoverOldImages finds v7 template images
func (s *b) discoverOldImages(force bool) ([]oldImage, error) {
    // GCP images filtered by name pattern
    // Use compute.Images.List with filter on name containing "aerolab4-template-"
}

// identifyRemovableLabels determines which old labels can be removed to stay under 64
func (s *b) identifyRemovableLabels(inst oldInstance, newLabels map[string]string) []string {
    // If current + new > 64, identify old v7 labels that have been translated
    // These can safely be removed since data is now in new format
    removable := []string{}
    // Add labels like "aerolab4cluster_name" if we have "aerolab-cn" in new labels
    // Add labels like "used_by" since we now have "usedby"
    return removable
}
```

---

## Phase 4: Backend Implementation

### 4.1 File Structure

Create new files:
- `src/pkg/backend/clouds/baws/migrate.go` - AWS migration
- `src/pkg/backend/clouds/bgcp/migrate.go` - GCP migration
- `src/pkg/backend/backends/migrate.go` - Common types

### 4.2 Migration Types (backends/migrate.go)

```go
package backends

type MigrateV7Input struct {
    Project       string          
    DryRun        bool            // If true, discovery only - no changes made
    Force         bool            // If true, re-migrate already migrated resources
    SSHKeyInfo    *SSHKeyPathInfo 
}

type SSHKeyPathInfo struct {
    KeysDir       string  // Directory containing old key files
    SharedKeyPath string  // Path to shared key if exists (empty if none)
    IsAerolabHome bool    // True if KeysDir was derived from an aerolab home directory
}

type MigrationResult struct {
    DryRun            bool
    InstancesMigrated int
    VolumesMigrated   int
    ImagesMigrated    int
    SSHKeysMigrated   int
    Errors            []error
    
    // Dry-run details: what WOULD be migrated (populated when DryRun=true)
    DryRunInstances   []MigrationInstanceDetail
    DryRunVolumes     []MigrationVolumeDetail
    DryRunImages      []MigrationImageDetail
    DryRunSSHKeys     []MigrationSSHKeyDetail
    
    // Actual migration details: what WAS migrated (populated when DryRun=false)
    MigratedInstances []MigrationInstanceDetail
    MigratedVolumes   []MigrationVolumeDetail
    MigratedImages    []MigrationImageDetail
    MigratedSSHKeys   []MigrationSSHKeyDetail
}

type MigrationInstanceDetail struct {
    InstanceID      string
    Name            string
    ClusterName     string
    NodeNo          int
    Zone            string
    IsClient        bool
    
    // Tags for dry-run (what would be added)
    TagsToAdd       map[string]string
    // Tags actually added (after migration)
    TagsAdded       map[string]string
    // Old tags that get preserved with v7- prefix
    TagsToPrefix    map[string]string
    // GCP only: labels that must be removed due to 64-label limit
    TagsToRemove    []string
    
    // SSH key migration info
    SSHKeyFrom      string
    SSHKeyTo        string
    SSHKeyMigrated  bool
    
    // GCP only: label count tracking
    LabelLimitInfo  string  // e.g., "current=45, adding=12, removing=3, final=54/64"
    
    // Migration result
    MigrationStatus string  // "success", "failed", "skipped"
    MigrationError  string  // Error message if failed
}

type MigrationVolumeDetail struct {
    VolumeID        string
    VolumeType      string  // "efs", "ebs", "pd-ssd"
    Name            string
    Zone            string
    TagsToAdd       map[string]string
    TagsAdded       map[string]string
    MigrationStatus string
    MigrationError  string
}

type MigrationImageDetail struct {
    ImageID         string
    Name            string
    Zone            string
    OSName          string  // Parsed from image name
    OSVersion       string  // Parsed from image name
    Architecture    string  // Parsed from image name
    TagsToAdd       map[string]string
    TagsAdded       map[string]string
    MigrationStatus string
    MigrationError  string
}

type MigrationSSHKeyDetail struct {
    ClusterName string
    Region      string  // AWS only: region part of key name
    FromPath    string  // Source path
    ToPath      string  // Destination path
    Copied      bool    // Whether copy succeeded
    Error       string  // Error message if copy failed
}
```

### 4.3 AWS Migration (baws/migrate.go)

```go
package baws

const TAG_V7_MIGRATED = "AEROLAB_V7_MIGRATED"

// MigrateV7Resources migrates v7 AWS resources to v8 format
func (s *b) MigrateV7Resources(input *backends.MigrateV7Input) (*backends.MigrationResult, error) {
    result := &backends.MigrationResult{DryRun: input.DryRun}
    migratedKeys := make(map[string]bool) // Track keys to avoid duplicates
    
    // Group instances by cluster+region for consistent UUID assignment
    clusterUUIDs := make(map[string]string) // key: "clusterName_region"
    
    // 1. Discover and migrate instances
    oldInstances, err := s.discoverOldInstances(input.Force)
    // ... process instances, generate UUIDs per cluster+region
    
    // 2. Discover and migrate volumes
    oldVolumes, err := s.discoverOldVolumes(input.Force)
    // ... process volumes
    
    // 3. Discover and migrate images
    oldImages, err := s.discoverOldImages(input.Force)
    // ... process images
    
    return result, nil
}

// translateServerTags converts v7 server instance tags to v8 format
func (s *b) translateServerTags(inst oldInstance, project, clusterUUID string) map[string]string {
    tags := map[string]string{
        TAG_CLUSTER_NAME:       inst.Tags["Aerolab4ClusterName"],
        TAG_NODE_NO:            inst.Tags["Aerolab4NodeNumber"],
        TAG_OS_NAME:            inst.Tags["Aerolab4OperatingSystem"],
        TAG_OS_VERSION:         inst.Tags["Aerolab4OperatingSystemVersion"],
        TAG_EXPIRES:            inst.Tags["aerolab4expires"],
        TAG_OWNER:              inst.Tags["owner"],
        TAG_COST_PPH:           inst.Tags["Aerolab4CostPerHour"],
        TAG_COST_SO_FAR:        inst.Tags["Aerolab4CostSoFar"],
        TAG_START_TIME:         inst.Tags["Aerolab4CostStartTime"],
        TAG_AEROLAB_PROJECT:    project,
        TAG_AEROLAB_VERSION:    s.aerolabVersion,
        TAG_CLUSTER_UUID:       clusterUUID,
        TAG_V7_MIGRATED:        "true",
        "aerolab.type":         "aerospike",  // Server type is always "aerospike"
    }
    // Add software version if present
    if v := inst.Tags["Aerolab4AerospikeVersion"]; v != "" {
        tags["aerolab.soft.version"] = v
    }
    // Preserve architecture
    if v := inst.Tags["Arch"]; v != "" {
        tags["v7-arch"] = v
    }
    // Add v7-prefixed tags for other preserved values
    s.addV7PrefixedTags(inst.Tags, tags)
    return tags
}

// translateClientTags converts v7 client instance tags to v8 format
func (s *b) translateClientTags(inst oldInstance, project, clusterUUID string) map[string]string {
    tags := map[string]string{
        TAG_CLUSTER_NAME:       inst.Tags["Aerolab4clientClusterName"],
        TAG_NODE_NO:            inst.Tags["Aerolab4clientNodeNumber"],
        TAG_OS_NAME:            inst.Tags["Aerolab4clientOperatingSystem"],
        TAG_OS_VERSION:         inst.Tags["Aerolab4clientOperatingSystemVersion"],
        TAG_AEROLAB_PROJECT:    project,
        TAG_AEROLAB_VERSION:    s.aerolabVersion,
        TAG_CLUSTER_UUID:       clusterUUID,
        TAG_V7_MIGRATED:        "true",
    }
    // Add owner and expires if present
    if v := inst.Tags["owner"]; v != "" {
        tags[TAG_OWNER] = v
    }
    if v := inst.Tags["aerolab4expires"]; v != "" {
        tags[TAG_EXPIRES] = v
    }
    // Set aerolab.type from client type (e.g., "tools", "graph", "ams", "vscode")
    if v := inst.Tags["Aerolab4clientType"]; v != "" {
        tags["aerolab.type"] = v
    }
    // Add software version if present
    if v := inst.Tags["Aerolab4clientAerospikeVersion"]; v != "" {
        tags["aerolab.soft.version"] = v
    }
    // Add v7-prefixed tags for other preserved values
    s.addV7PrefixedTags(inst.Tags, tags)
    return tags
}

// addV7PrefixedTags adds v7- prefix to preserved tags and migrates telemetry
// This handles telemetry migration, spot flags, and AGI-specific tags
func (s *b) addV7PrefixedTags(oldTags, newTags map[string]string) {
    // Migrate telemetry tag to new v8 format
    if v := oldTags["telemetry"]; v != "" {
        newTags["aerolab.telemetry"] = v
    }
    
    // Map of old tag names to their v7- prefixed versions
    tagsToPrefix := map[string]string{
        "aerolab7spot":    "v7-spot",
        "aerolab7agiav":   "v7-agiav",
        "aerolab4features": "v7-features",
        "aerolab4ssl":     "v7-ssl",
        "agiLabel":        "v7-agilabel",
        "agiinstance":     "v7-agiinstance",
        "aginodim":        "v7-aginodim",
        "termonpow":       "v7-termonpow",
        "isspot":          "v7-isspot",
        "agiSrcLocal":     "v7-agisrclocal",
        "agiSrcSftp":      "v7-agisrcsftp",
        "agiSrcS3":        "v7-agisrcs3",
        "agiDomain":       "v7-agidomain",
    }
    
    for oldKey, newKey := range tagsToPrefix {
        if v := oldTags[oldKey]; v != "" {
            newTags[newKey] = v
        }
    }
}
```

### 4.4 GCP Migration (bgcp/migrate.go)

```go
package bgcp

const LABEL_V7_MIGRATED = "aerolab-v7-migrated"

// MigrateV7Resources migrates v7 GCP resources to v8 format
func (s *b) MigrateV7Resources(input *backends.MigrateV7Input) (*backends.MigrationResult, error) {
    result := &backends.MigrationResult{DryRun: input.DryRun}
    migratedKeys := make(map[string]bool) // Track keys to avoid duplicates
    
    // Group instances by cluster for consistent UUID assignment
    clusterUUIDs := make(map[string]string) // key: "clusterName"
    
    // 1. Discover and migrate instances
    oldInstances, err := s.discoverOldInstances(input.Force)
    // ... process instances, generate UUIDs per cluster
    
    // 2. Discover and migrate volumes
    oldVolumes, err := s.discoverOldVolumes(input.Force)
    // ... process volumes
    
    // 3. Discover and migrate images
    oldImages, err := s.discoverOldImages(input.Force)
    // ... process images
    
    return result, nil
}

// translateServerLabels converts v7 server instance labels to v8 format
func (s *b) translateServerLabels(inst oldInstance, project, clusterUUID string) map[string]string {
    labels := map[string]string{
        "aerolab-cn":          inst.Labels["aerolab4cluster_name"],
        "aerolab-nn":          inst.Labels["aerolab4node_number"],
        "aerolab-os":          inst.Labels["aerolab4operating_system"],
        "aerolab-ov":          inst.Labels["aerolab4operating_system_version"],
        "aerolab-e":           inst.Labels["aerolab4expires"],
        "aerolab-o":           inst.Labels["owner"],
        "aerolab-cp":          inst.Labels["aerolab_cost_ph"],
        "aerolab-cs":          inst.Labels["aerolab_cost_sofar"],
        "aerolab-st":          inst.Labels["aerolab_cost_starttime"],
        "aerolab-p":           project,
        "aerolab-v":           s.aerolabVersion,
        "aerolab-uuid":        clusterUUID,
        LABEL_V7_MIGRATED:     "true",
        "usedby":              "aerolab",  // Native label for v8 filtering
    }
    // Add software version if present
    if v := inst.Labels["aerolab4aerospike_version"]; v != "" {
        labels["aerolab-soft-version"] = v
    }
    // Set type via encodeToLabels metadata
    // "aerolab.type" = "aerospike" will be encoded
    
    // Preserve architecture
    if v := inst.Labels["arch"]; v != "" {
        labels["v7-arch"] = v
    }
    // Add v7-prefixed labels
    s.addV7PrefixedLabels(inst.Labels, labels)
    return labels
}

// translateClientLabels converts v7 client instance labels to v8 format
func (s *b) translateClientLabels(inst oldInstance, project, clusterUUID string) map[string]string {
    labels := map[string]string{
        "aerolab-cn":          inst.Labels["aerolab4client_name"],
        "aerolab-nn":          inst.Labels["aerolab4client_node_number"],
        "aerolab-os":          inst.Labels["aerolab4client_operating_system"],
        "aerolab-ov":          inst.Labels["aerolab4client_operating_system_version"],
        "aerolab-p":           project,
        "aerolab-v":           s.aerolabVersion,
        "aerolab-uuid":        clusterUUID,
        LABEL_V7_MIGRATED:     "true",
        "usedby":              "aerolab",  // Native label for v8 filtering
    }
    // Add owner and expires if present
    if v := inst.Labels["owner"]; v != "" {
        labels["aerolab-o"] = v
    }
    if v := inst.Labels["aerolab4expires"]; v != "" {
        labels["aerolab-e"] = v
    }
    // Client type - will be encoded via encodeToLabels for "aerolab.type"
    // Also keep native label for filtering if present
    if v := inst.Labels["aerolab4client_type"]; v != "" {
        labels["aerolab-client-type"] = v
    }
    // Add software version if present
    if v := inst.Labels["aerolab4client_aerospike_version"]; v != "" {
        labels["aerolab-soft-version"] = v
    }
    // Add v7-prefixed labels
    s.addV7PrefixedLabels(inst.Labels, labels)
    return labels
}

// addV7PrefixedLabels adds v7- prefix to preserved labels and migrates telemetry
func (s *b) addV7PrefixedLabels(oldLabels, newLabels map[string]string) {
    // Migrate telemetry label to new v8 format
    if v := oldLabels["telemetry"]; v != "" {
        newLabels["aerolab-telemetry"] = v  // GCP label format (dots not allowed)
    }
    
    // Map of old label names to their v7- prefixed versions
    labelsToPrefix := map[string]string{
        "isspot":       "v7-isspot",
    }
    
    for oldKey, newKey := range labelsToPrefix {
        if v := oldLabels[oldKey]; v != "" {
            newLabels[newKey] = v
        }
    }
}

// translateVolumeLabels converts v7 volume labels to v8 format
func (s *b) translateVolumeLabels(vol oldVolume, project string) map[string]string {
    labels := map[string]string{
        "aerolab-st":          vol.Labels["lastused"],
        "aerolab-o":           vol.Labels["aerolab7owner"],
        "aerolab-p":           project,
        LABEL_V7_MIGRATED:     "true",
    }
    // Preserve AGI-related labels with v7- prefix
    agiLabelsToPrefix := []string{
        "expireduration", "agiinstance", "aginodim", "termonpow", "isspot",
    }
    for _, key := range agiLabelsToPrefix {
        if v := vol.Labels[key]; v != "" {
            labels["v7-"+key] = v
        }
    }
    // Handle chunked AGI labels (agilabel0, agilabel1, etc.)
    for key, value := range vol.Labels {
        if strings.HasPrefix(key, "agilabel") {
            labels["v7-"+key] = value
        }
    }
    return labels
}
```

**GCP-Specific Considerations:**
- Uses `encodeToLabels()` for storing metadata as base32-encoded JSON
- Native labels kept separately for filtering (e.g., `usedby`, `aerolab-v7-migrated`)
- Must handle 64-label limit by removing old labels after adding new ones
- Label values must be lowercase, max 63 chars, only letters/numbers/underscores/hyphens

---

## Phase 5: CLI Implementation

### 5.1 Command Structure

```go
type InventoryMigrateCmd struct {
    DryRun     bool   `long:"dry-run" description:"Show what would be migrated without making changes"`
    Yes        bool   `short:"y" long:"yes" description:"Skip confirmation prompt"`
    Force      bool   `long:"force" description:"Force re-migration of already migrated resources"`
    Verbose    bool   `short:"v" long:"verbose" description:"Show detailed output"`
    SSHKeyPath string `long:"ssh-key-path" description:"Path to old v7 SSH keys directory (default: ~/aerolab-keys/)"`
    Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}
```

### 5.2 SSH Key Path Resolution

The `--ssh-key-path` flag can point to:

1. **Direct keys directory** (default: `~/aerolab-keys/`)
   - Keys are directly at `{path}/aerolab-mydc_us-east-1`
   
2. **Alternative aerolab home directory** (detected by presence of `keys/` subdir)
   - Keys are at `{path}/keys/aerolab-mydc_us-east-1`
   - Also check for shared key at `{path}/sshkey`

```go
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
```

### 5.3 Execute Flow

```go
func (c *InventoryMigrateCmd) Execute(args []string) error {
    // 1. Validate backend type (reject docker)
    // 2. Resolve SSH key paths
    // 3. If not dry-run, first do discovery pass and show summary
    // 4. If not --yes, prompt for confirmation
    // 5. Run actual migration (or dry-run)
    // 6. Print results
    // 7. Return appropriate exit code
}
```

### 5.4 Output Format

**Dry Run (with --verbose):**
```
=== DRY RUN MODE - No changes will be made ===

Discovered v7 Resources:
  Instances: 5 (3 servers, 2 clients)
  Volumes: 2 (1 EFS, 1 EBS)
  Images: 3
  SSH Keys: 4

Instance Migration Details:
  [1] Instance: i-0abc123def456 (aerolab4-mydc-1) - SERVER
      Cluster: mydc, Node: 1, Region: us-east-1
      Tags to ADD:
        AEROLAB_VERSION = 8.0.0
        AEROLAB_PROJECT = myproject
        AEROLAB_CLUSTER_NAME = mydc
        AEROLAB_NODE_NO = 1
        AEROLAB_OS_NAME = ubuntu
        AEROLAB_OS_VERSION = 22.04
        AEROLAB_CLUSTER_UUID = abc123-def456-...
        AEROLAB_V7_MIGRATED = true
        aerolab.type = aerospike
        aerolab.soft.version = 7.0.0
      Tags MIGRATED to v8 format:
        aerolab.telemetry = abc123
      SSH Key Migration:
        FROM: ~/aerolab-keys/aerolab-mydc_us-east-1
        TO:   ~/.aerolab/myproject/ssh-keys/aws/old/aerolab-mydc_us-east-1

  [2] Instance: i-0xyz789... (aerolab4-mydc-2) - SERVER
      ...

  [3] Instance: i-0client... (aerolab_c-myclient-1) - CLIENT
      Cluster: myclient, Node: 1, Region: us-east-1
      Tags to ADD:
        AEROLAB_VERSION = 8.0.0
        AEROLAB_PROJECT = myproject
        aerolab.type = tools
        ...
      Tags to PRESERVE (with v7- prefix):
        (none - client type migrated to aerolab.type)

Volume Migration Details:
  [1] Volume: fs-0abc123 (myvolume) - EFS
      Region: us-east-1
      Tags to ADD:
        AEROLAB_V7_MIGRATED = true
        ...

Image Migration Details:
  [1] Image: ami-0abc123 (aerolab4-template-ubuntu_22.04_7.0.0_amd)
      Parsed: OS=ubuntu, Version=22.04, Arch=amd
      Tags to ADD:
        AEROLAB_OS_NAME = ubuntu
        AEROLAB_OS_VERSION = 22.04
        ...

SSH Key Migration:
  [1] aerolab-mydc_us-east-1
      FROM: ~/aerolab-keys/aerolab-mydc_us-east-1
      TO:   ~/.aerolab/myproject/ssh-keys/aws/old/aerolab-mydc_us-east-1
  [2] aerolab-myclient_us-east-1
      FROM: ~/aerolab-keys/aerolab-myclient_us-east-1
      TO:   ~/.aerolab/myproject/ssh-keys/aws/old/aerolab-myclient_us-east-1
      WARNING: Source file not found!

Summary:
  Would migrate 5 instances
  Would migrate 2 volumes
  Would migrate 3 images
  Would copy 4 SSH keys (1 not found)

Run without --dry-run to apply these changes.
```

**Actual Migration:**
```
=== MIGRATION IN PROGRESS ===

Migrating instances: [=========>          ] 45% (23/51) - aerolab4-mydc-5

=== MIGRATION COMPLETE ===

Migrated Resources:
  ✓ Instances: 5/5 migrated
  ✓ Volumes: 2/2 migrated
  ✓ Images: 3/3 migrated
  ✓ SSH Keys: 3/4 copied (1 not found)

Instance Details:
  ✓ i-0abc123 (aerolab4-mydc-1) - 12 tags added
  ✓ i-0abc124 (aerolab4-mydc-2) - 12 tags added
  ✓ i-0xyz789 (aerolab_c-myclient-1) - 10 tags added
  ...

Note: Firewall rules are not migrated. Existing rules remain functional.

Warnings:
  - SSH key not found: ~/aerolab-keys/aerolab-oldcluster_us-east-1
    (Instance aerolab4-oldcluster-1 may require manual key setup)
```

### 5.5 Confirmation Prompt

For actual migration (not dry-run), require confirmation unless `--yes`:

```go
if !c.DryRun && !c.Yes {
    // First run discovery to show what will be migrated
    discovery, err := backend.MigrateV7Resources(&backends.MigrateV7Input{
        Project:    project,
        DryRun:     true,  // Discovery only
        Force:      c.Force,
        SSHKeyInfo: sshKeyInfo,
    })
    
    totalResources := len(discovery.DryRunInstances) + 
                      len(discovery.DryRunVolumes) + 
                      len(discovery.DryRunImages)
    
    if totalResources == 0 {
        system.Logger.Info("No v7 resources found to migrate.")
        return nil
    }
    
    // Show summary
    system.Logger.Info("Found v7 resources to migrate:")
    system.Logger.Info("  Instances: %d", len(discovery.DryRunInstances))
    system.Logger.Info("  Volumes: %d", len(discovery.DryRunVolumes))
    system.Logger.Info("  Images: %d", len(discovery.DryRunImages))
    system.Logger.Info("  SSH Keys: %d", len(discovery.DryRunSSHKeys))
    system.Logger.Info("")
    
    // Prompt
    fmt.Print("Proceed with migration? [y/N]: ")
    var response string
    fmt.Scanln(&response)
    if strings.ToLower(strings.TrimSpace(response)) != "y" {
        return fmt.Errorf("migration cancelled by user")
    }
}
```

---

## Phase 6: Post-Migration Backend Updates

### 6.1 Update SSH Key Resolution

Modify `InstancesGetSSHKeyPath()` and `InstancesGetSftpConfig()` in both AWS and GCP backends to check for migrated instances and use old key paths.

### 6.2 Old Key Path Calculation

```go
// AWS: {sshKeysDir}/old/aerolab-{clusterName}_{region}
func (s *b) getOldSSHKeyPath(inst *backends.Instance) string {
    if inst.Tags[TAG_V7_MIGRATED] != "true" {
        return ""
    }
    return path.Join(s.sshKeysDir, "old", 
        fmt.Sprintf("aerolab-%s_%s", inst.ClusterName, inst.ZoneName))
}

// GCP: {sshKeysDir}/old/aerolab-gcp-{clusterName}
func (s *b) getOldSSHKeyPath(inst *backends.Instance) string {
    if inst.Tags[TAG_V7_MIGRATED] != "true" {
        return ""
    }
    return path.Join(s.sshKeysDir, "old",
        fmt.Sprintf("aerolab-gcp-%s", inst.ClusterName))
}
```

---

## Implementation Notes

### Critical Considerations

1. **Idempotency**: Running migration multiple times must be safe
   - Skip resources with `AEROLAB_V7_MIGRATED` tag (unless `--force`)
   - SSH key copy checks destination before copying

2. **AWS Multi-Region**: Same cluster name in different regions = different keys, different UUIDs
   - Group by `{clusterName}_{region}` for UUID assignment

3. **GCP Label Limit**: Max 64 labels
   - If limit exceeded, remove old v7 labels after adding new ones
   - Prioritize: new v8 labels > migration marker > encoded metadata

4. **Partial Failure**: Continue on individual failures
   - Collect all errors and report at end
   - Successfully migrated resources won't be retried

5. **SSH Key Deduplication**: Multiple nodes share same key
   - Track copied keys to avoid redundant operations

6. **Shared Key Handling**: If `${AEROLAB_HOME}/sshkey` exists
   - Skip all per-cluster key migration
   - Backend already handles shared key resolution

7. **Firewalls Not Migrated**: Old firewalls continue working
   - They're VPC-wide, not per-cluster
   - New operations will create v8 firewalls as needed

### Exit Codes
- 0 = Complete success
- 1 = Partial success (some resources failed)
- 2 = Complete failure

### Progress Reporting

For large inventories, show progress during migration:

```go
// Progress callback for CLI display
type ProgressCallback func(current, total int, resourceType, resourceName string)

// Example output:
// Migrating instances: [=========>          ] 45% (23/51) - aerolab4-mydc-5
// Migrating volumes:   [====================] 100% (8/8) - Complete
// Migrating images:    [====>               ] 25% (1/4) - aerolab4-template-ubuntu_22.04
```

---

## Edge Cases to Handle

### Instance Edge Cases
- Instance with partial v7 tags (some tags missing)
- Instance already migrated (idempotency check)
- Mixed v7 and v8 instances in same cluster name
- Cluster with same name exists in multiple regions (AWS - different UUIDs, different keys)
- Instance in terminated state (skip)

### GCP-Specific Edge Cases
- Instance already at 64-label limit
- Instance with long values that exceed 63-char limit when encoded
- Client type label that needs special handling

### SSH Key Edge Cases
- Missing SSH keys for old instances (warn, don't fail)
- Old SSH keys in non-default location (user moved them - use --ssh-key-path)
- Shared key override exists at `${AEROLAB_HOME}/sshkey` (skip per-cluster migration)
- Key already exists in destination `/old/` directory (don't overwrite)
- Same key used by multiple clusters/nodes (deduplicate copies)
- **AWS-specific**: Region is part of key name (`aerolab-mydc_us-east-1`)
- **AWS-specific**: Only private key exists (no .pub file needed)
- **GCP-specific**: Both private and .pub files must exist
- **GCP-specific**: NO region in key name (`aerolab-gcp-mydc`)
- **GCP-specific**: Shared key requires both `${AEROLAB_HOME}/sshkey` AND `sshkey.pub`
- **AWS shared key name in EC2**: `manual-aerolab-agi-shared`

### Volume Edge Cases
- **AWS EFS** volume with multiple mount targets (handle all)
- **AWS EBS** volume currently attached to instance
- Volume with AGI-specific labels (preserve with v7- prefix)
- Volume with `UsedBy` = "aerolab7" (preserve for backward compatibility)
- Volume with custom tags (preserve as-is)
- **GCP-specific**: Chunked AGI labels (agilabel0, agilabel1, etc.) - preserve all chunks
- **GCP-specific**: Encoded timestamps (colons→underscores, plus→dashes)

### Image Edge Cases
- Image name doesn't match expected pattern (skip or warn)
- Image shared across accounts (may not have permission to tag)
- Image with custom tags beyond the standard set (preserve)
- **AWS pattern**: `aerolab4-template-{distro}_{version}_{aerospikeVersion}_{arch}`
- **GCP pattern**: `aerolab4-template-{distro}-{version}-{aerospikeVersion}-{arch}` (dashes replace dots)
- Parsing GCP image names: convert dashes back to dots for version numbers

---

## Risk Mitigation

### Data Safety
- **Never remove old tags on AWS** - Only add new tags alongside existing ones
- **GCP label removal only when necessary** - Remove old labels only if 64-label limit would be exceeded, and only after verifying new labels were successfully applied
- **SSH key preservation** - Copy keys, never move them; original location remains intact

### Rollback Strategy
If migration causes issues:
1. Migrated instances can still be managed by v7 (old tags are preserved)
2. Manual removal of `AEROLAB_V7_MIGRATED` tag would effectively "un-migrate" an instance
3. SSH keys in `/old/` directory can be manually moved back if needed

### Dry-Run Safety
The `--dry-run` flag is **critical for safe preview**:
1. Makes NO changes to any cloud resources
2. Makes NO changes to any local files
3. Shows exact tags/labels that would be added
4. Shows exact SSH keys that would be copied
5. Identifies potential issues (missing keys, label limits)
6. Safe to run multiple times

**Recommended workflow:**
```bash
# Step 1: Preview what would be migrated
aerolab inventory migrate --dry-run --verbose

# Step 2: Review output carefully

# Step 3: Run actual migration only after review
aerolab inventory migrate
```

---

## Files to Create/Modify

### New Files
- `src/pkg/backend/backends/migrate.go` - Migration types
- `src/pkg/backend/clouds/baws/migrate.go` - AWS implementation
- `src/pkg/backend/clouds/bgcp/migrate.go` - GCP implementation

### Modified Files  
- `src/pkg/backend/clouds/baws/tags.go` - Add `TAG_V7_MIGRATED` constant
- `src/pkg/backend/clouds/bgcp/tags.go` - Add migration label constant
- `src/pkg/backend/clouds/baws/instances.go` - Update `InstancesGetSSHKeyPath`
- `src/pkg/backend/clouds/bgcp/instances.go` - Update `InstancesGetSSHKeyPath`
- `src/cli/cmd/v1/cmdInventoryMigrate.go` - CLI implementation
- `src/cli/cmd/v1/cmdInventory.go` - Register migrate subcommand

### Documentation
- `docs/migration-guide.md` - User documentation
- `CHANGELOG/8.0.0.md` - Changelog entry

---

## Implementation Checklist

### Phase 1: Types and Constants
- [ ] Create `backends/migrate.go` with all types (`MigrateV7Input`, `MigrationResult`, detail structs)
- [ ] Add migration tag constants to `baws/tags.go`:
  - `TAG_V7_MIGRATED = "AEROLAB_V7_MIGRATED"`
  - `TAG_SOFT_TYPE = "aerolab.type"` (or use inline string)
  - `TAG_SOFT_VERSION = "aerolab.soft.version"` (or use inline string)
- [ ] Add migration label constants to `bgcp/tags.go`:
  - `LABEL_V7_MIGRATED = "aerolab-v7-migrated"`
  - Add to `encodeToLabels()` native labels if needed for filtering
- [ ] Define `oldInstance`, `oldVolume`, `oldImage` internal types

### Phase 2: AWS Migration
- [ ] Create `baws/migrate.go`
- [ ] Implement `discoverOldInstances()` with EC2 filter for UsedBy tag
- [ ] Implement `discoverOldVolumes()` for both EFS and EBS
- [ ] Implement `discoverOldImages()` for AMIs matching "aerolab4-template-*"
- [ ] Implement `translateServerTags()` - server instance tag mapping
- [ ] Implement `translateClientTags()` - client instance tag mapping
- [ ] Implement `translateVolumeTags()` - volume tag mapping (EFS/EBS)
- [ ] Implement `translateImageTags()` - parse image name and create tags
- [ ] Implement `addV7PrefixedTags()` - handle telemetry, spot, AGI tags
- [ ] Implement `getSSHKeyMigrationPaths()` - calculate source/dest paths (private key only)
- [ ] Implement `migrateSSHKey()` - copy private key to /old/ directory
- [ ] Implement `MigrateV7Resources()` - main migration orchestration
- [ ] Update `InstancesGetSSHKeyPath()` - check for V7_MIGRATED tag and use /old/ path
- [ ] Update `InstancesGetSftpConfig()` - same SSH key resolution update
- [ ] Implement `getOldSSHKeyPath()` helper function
- [ ] Handle shared key check: if `${AEROLAB_HOME}/sshkey` exists, skip per-cluster migration
- [ ] Handle all AGI-specific tags (agiLabel, aerolab7agiav, agiDomain, etc.)

### Phase 3: GCP Migration
- [ ] Create `bgcp/migrate.go`
- [ ] Implement `discoverOldInstances()` with label filter for used_by
- [ ] Implement `discoverOldVolumes()` for persistent disks (usedby=aerolab7)
- [ ] Implement `discoverOldImages()` for images matching "aerolab4-template-*"
- [ ] Implement `translateServerLabels()` using `encodeToLabels()`
- [ ] Implement `translateClientLabels()` using `encodeToLabels()`
- [ ] Implement `translateVolumeLabels()` - handle chunked agilabel{N} labels
- [ ] Implement `translateImageLabels()` - parse image name and create labels
- [ ] Implement `addV7PrefixedLabels()` - handle telemetry, isspot, AGI labels
- [ ] Implement `identifyRemovableLabels()` - handle 64-label limit
- [ ] Implement `mergeLabels()` - combine old and new, remove if needed
- [ ] Implement `getSSHKeyMigrationPaths()` - GCP key format (no region)
- [ ] Implement `migrateSSHKey()` - copy BOTH private AND .pub files
- [ ] Implement `MigrateV7Resources()` - main migration orchestration
- [ ] Update `InstancesGetSSHKeyPath()` - check for V7_MIGRATED and use /old/ path
- [ ] Update `InstancesGetSftpConfig()` - same SSH key resolution update
- [ ] Implement `getOldSSHKeyPath()` helper function
- [ ] Handle shared key check: if `${AEROLAB_HOME}/sshkey` AND `sshkey.pub` exist, skip per-cluster migration

### Phase 4: CLI Implementation
- [ ] Add command flags (`DryRun`, `Yes`, `Force`, `Verbose`, `SSHKeyPath`)
- [ ] Implement `resolveSSHKeyPath()` - detect path type and shared key
- [ ] Implement `printDryRunResults()` - detailed dry-run output
- [ ] Implement `printMigrationResults()` - actual migration output
- [ ] Implement confirmation prompt logic
- [ ] Implement progress reporting for large inventories
- [ ] Register command in `cmdInventory.go`
- [ ] Handle Docker backend rejection gracefully

### Phase 5: Testing
**Unit Tests:**
- [ ] Test AWS tag translation correctness
- [ ] Test GCP label encoding/decoding with migration marker
- [ ] Test SSH key path resolution for migrated vs non-migrated instances
- [ ] Test SSH key deduplication (same key not copied multiple times)
- [ ] Test cluster UUID grouping (same cluster = same UUID)
- [ ] Test `--ssh-key-path` detection (aerolab home vs direct keys dir)

**Integration Tests:**
- [ ] Test AWS migration end-to-end (create v7-style instance, migrate, verify)
- [ ] Test GCP migration end-to-end
- [ ] Test idempotency (run twice, second is no-op)
- [ ] Test partial failure handling (one instance fails, others continue)
- [ ] Test missing SSH keys handling (warn, don't fail)
- [ ] Test shared key scenario (no per-cluster keys copied)
- [ ] Test `--force` flag re-migration
- [ ] Test `--dry-run` makes no changes
- [ ] Test `--yes` skips confirmation
- [ ] Test `--ssh-key-path` pointing to aerolab home dir
- [ ] Test `--ssh-key-path` pointing to direct keys dir

**Edge Case Tests:**
- [ ] Test instance with partial v7 tags
- [ ] Test GCP instance at 64-label limit
- [ ] Test cluster with same name in multiple regions (AWS)
- [ ] Test mixed v7 and v8 instances in same cluster name

### Phase 6: Documentation
- [ ] Update `docs/migration-guide.md` with usage instructions
- [ ] Update `CHANGELOG/8.0.0.md` with new command entry
- [ ] Add example output to documentation
- [ ] Document pre-migration checklist for users

---

## Flexibility Notes

This plan is a guide, not a rigid specification. During implementation:

- **Tag mappings may need adjustment** based on actual data found in v7 resources
- **Error handling may evolve** based on real-world edge cases discovered
- **Output format can be refined** based on usability feedback
- **Additional flags may be needed** (e.g., `--cluster`, `--region` filters)
- **Backend method signatures may change** as implementation progresses

The key constraints that MUST be preserved:
1. Never delete old tags (only add new ones, except GCP label limit handling)
2. Copy SSH keys, never move them
3. Support dry-run mode for safe preview
4. Handle Docker backend rejection gracefully
5. Maintain idempotency
