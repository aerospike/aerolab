# Inventory Management Commands

Commands for viewing and managing all Aerolab resources.

## Commands Overview

- `inventory list` - List all resources
- `inventory ansible` - Export inventory in Ansible format
- `inventory genders` - Export inventory in genders format
- `inventory hostfile` - Export inventory in /etc/hosts format
- `inventory instance-types` - List available instance types (AWS/GCP)
- `inventory delete-project-resources` - Delete all project resources
- `inventory refresh-disk-cache` - Refresh inventory disk cache
- `inventory migrate` - Migrate v7 resources to v8 format (AWS/GCP only)

## Inventory List

List all clusters, instances, and images.

### Basic Usage

```bash
aerolab inventory list
```

### Options

| Option | Description |
|--------|-------------|
| `-o, --output` | Output format: `table`, `tsv`, or `json` |

### Examples

**List all resources:**
```bash
aerolab inventory list
```

**List in TSV format:**
```bash
aerolab inventory list -o tsv
```

**List in JSON format:**
```bash
aerolab inventory list -o json
```

### Output

The `inventory list` command shows:
- All clusters with node information
- All instances
- All images
- Resource states (running, stopped, etc.)
- Resource details (instance types, IPs, etc.)

## Inventory Ansible

Export inventory in Ansible format.

### Basic Usage

```bash
aerolab inventory ansible
```

### Output

Generates Ansible inventory file format that can be used with Ansible playbooks.

### Examples

**Export to file:**
```bash
aerolab inventory ansible > ansible-inventory.ini
```

**Use with Ansible:**
```bash
aerolab inventory ansible > ansible-inventory.ini
ansible -i ansible-inventory.ini all -m ping
```

## Inventory Genders

Export inventory in genders format.

### Basic Usage

```bash
aerolab inventory genders
```

### Output

Generates genders configuration file format.

### Examples

**Export to file:**
```bash
aerolab inventory genders > genders.conf
```

## Inventory Hostfile

Export inventory in /etc/hosts format.

### Basic Usage

```bash
aerolab inventory hostfile
```

### Output

Generates /etc/hosts format with cluster names and IP addresses.

### Examples

**Export to file:**
```bash
aerolab inventory hostfile > hosts-file
```

**Append to /etc/hosts:**
```bash
aerolab inventory hostfile | sudo tee -a /etc/hosts
```

## Inventory Instance-Types

List available instance types (AWS/GCP only).

### Basic Usage

```bash
aerolab inventory instance-types
```

### Options

| Option | Description |
|--------|-------------|
| `-o, --output` | Output format: `table`, `tsv`, or `json` |

### Examples

**List instance types:**
```bash
aerolab inventory instance-types
```

**List in JSON format:**
```bash
aerolab inventory instance-types -o json
```

**List in TSV format:**
```bash
aerolab inventory instance-types -o tsv
```

### Output

Shows available instance types with:
- Instance type names
- CPU information
- Memory information
- Pricing (if available)

## Inventory Delete-Project-Resources

Delete all resources in the current project.

### Basic Usage

```bash
aerolab inventory delete-project-resources -f
```

### Options

| Option | Description |
|--------|-------------|
| `-f, --force` | Force deletion without confirmation |
| `--expiry` | Also delete expiry automation resources |

### Examples

**Delete all resources:**
```bash
aerolab inventory delete-project-resources -f
```

**Delete all resources including expiry:**
```bash
aerolab inventory delete-project-resources -f --expiry
```

**Warning**: This deletes ALL clusters, instances, and images in the current project. Use with caution.

## Inventory Refresh-Disk-Cache

Refresh the inventory disk cache.

### Basic Usage

```bash
aerolab inventory refresh-disk-cache
```

### Examples

**Refresh cache:**
```bash
aerolab inventory refresh-disk-cache
```

**Note**: This forces a refresh of the cached inventory. Useful if inventory cache is enabled and resources have changed.

## Inventory Migrate

Migrate v7 AeroLab resources to v8 format. This command updates cloud resource tags and copies SSH keys for existing resources created with AeroLab v7.

**Note:** This command is only supported for AWS and GCP backends. Docker is NOT supported.

### Basic Usage

```bash
aerolab inventory migrate
```

### Options

| Option | Description |
|--------|-------------|
| `--dry-run` | Show what would be migrated without making changes |
| `-y, --yes` | Skip confirmation prompt and proceed with migration |
| `--force` | Force re-migration of already migrated resources |
| `-v, --verbose` | Show detailed migration information |
| `--ssh-key-path` | Path to old v7 SSH keys directory (default: `~/aerolab-keys/`) |

### Examples

**Preview migration (dry-run):**
```bash
aerolab inventory migrate --dry-run
```

**Preview with verbose output:**
```bash
aerolab inventory migrate --dry-run --verbose
```

**Migrate without confirmation:**
```bash
aerolab inventory migrate --yes
```

**Re-migrate already migrated resources:**
```bash
aerolab inventory migrate --force
```

**Specify custom SSH key path:**
```bash
aerolab inventory migrate --ssh-key-path /custom/path/to/keys
```

### What Gets Migrated

The migration process updates the following resources:

**Instances (servers and clients):**
- Updates resource tags to v8 format
- Adds new tags: `AEROLAB_VERSION`, `AEROLAB_PROJECT`, `AEROLAB_CLUSTER_UUID`, `AEROLAB_V7_MIGRATED`
- Maps old tags to new format (e.g., `Aerolab4ClusterName` → `AEROLAB_CLUSTER_NAME`)
- Sets `aerolab.type` tag (`aerospike` for servers, client type for clients)
- Preserves old tags for backward compatibility

**Volumes (EFS/EBS in AWS, Persistent Disks in GCP):**
- Updates volume tags to v8 format
- Adds migration marker tag
- Preserves AGI-specific labels

**Images (AMIs in AWS, Custom Images in GCP):**
- Parses image names to extract OS, version, and architecture
- Adds appropriate tags for v8 compatibility

**SSH Keys:**
- Copies old keys from `~/aerolab-keys/` to `~/.config/aerolab/{project}/ssh-keys/{backend}/old/`
- Keys are **copied**, not moved (original location remains intact)
- Shared keys (`~/.aerolab/sshkey`) take precedence over per-cluster keys

### Backend-Specific Behavior

**AWS:**
- Tags are added alongside existing tags (old tags are never removed)
- SSH key format: `aerolab-{clusterName}_{region}`
- Keys are region-scoped (same cluster in different regions = different keys)

**GCP:**
- Labels are added; old labels removed only if 64-label limit would be exceeded
- SSH key format: `aerolab-gcp-{clusterName}`
- Keys are cluster-scoped (NOT region-specific)
- Both private key and `.pub` file are copied

**Docker:**
- NOT SUPPORTED - returns error immediately
- Docker containers don't need migration (local labels are different from cloud tags)

### Output

**Dry-run output shows:**
- Number of instances, volumes, images found
- Tags that would be added to each resource
- SSH keys that would be copied
- Any warnings (e.g., missing SSH keys)

**Migration output shows:**
- Progress during migration
- Success/failure count for each resource type
- Details of migrated resources
- Any errors or warnings encountered

### Safety Features

- **Idempotent:** Safe to run multiple times; already-migrated resources are skipped (unless `--force`)
- **Non-destructive:** Old tags are preserved on AWS; SSH keys are copied, not moved
- **Dry-run:** Always preview changes before applying
- **Confirmation:** Prompts for confirmation before making changes (unless `--yes`)

### Common Issues

**"cluster not found" errors:**
- Ensure resources exist in the configured region
- Check backend configuration: `aerolab config backend`

**Missing SSH keys:**
- Check that keys exist at the expected path
- Use `--ssh-key-path` to specify a custom location
- Missing keys are warnings, not errors (migration continues)

**GCP label limit:**
- GCP has a 64-label limit per resource
- Migration may remove old labels if limit would be exceeded
- New v8 labels take priority

## Common Workflows

### View All Resources

```bash
# List all resources
aerolab inventory list

# List in JSON format
aerolab inventory list -o json

# List in TSV format
aerolab inventory list -o tsv
```

### Export Inventory for Automation

```bash
# Export for Ansible
aerolab inventory ansible > ansible-inventory.ini

# Export for hosts file
aerolab inventory hostfile > hosts-file

# Export for genders
aerolab inventory genders > genders.conf
```

### Check Available Instance Types

```bash
# List instance types
aerolab inventory instance-types

# List in JSON format
aerolab inventory instance-types -o json
```

### Clean Up All Resources

```bash
# Delete all resources
aerolab inventory delete-project-resources -f

# Delete all resources including expiry
aerolab inventory delete-project-resources -f --expiry
```

### Refresh Cache

```bash
# Refresh inventory cache
aerolab inventory refresh-disk-cache
```

### Migrate v7 Resources

```bash
# Preview what would be migrated
aerolab inventory migrate --dry-run --verbose

# Migrate AWS resources
aerolab config backend -t aws -r us-east-1
aerolab inventory migrate

# Migrate GCP resources
aerolab config backend -t gcp -r us-central1 -o my-project-id
aerolab inventory migrate

# Migrate with custom SSH key path
aerolab inventory migrate --ssh-key-path ~/my-old-keys/
```

## Tips

1. **Export formats**: Use different export formats for integration with other tools
2. **Instance types**: Check available instance types before creating clusters
3. **Cleanup**: Use `delete-project-resources` carefully as it deletes everything
4. **Cache**: Refresh cache if resources have changed externally
5. **JSON format**: Use JSON format for programmatic access to inventory data
6. **Migration**: Always use `--dry-run` before migrating to preview changes
7. **Multi-backend migration**: Run migration separately for each cloud backend (AWS, GCP)

