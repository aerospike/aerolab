# Template Management Commands

Templates are pre-configured instance snapshots that significantly speed up cluster creation by caching the Aerospike installation and base configuration.

## Commands Overview

- `template create` - Create a new template
- `template list` - List available templates
- `template destroy` - Destroy a template
- `template vacuum` - Clean up failed or dangling templates

## Why Use Templates?

Templates provide several benefits:

- **Faster Cluster Creation**: Skip Aerospike installation on every node
- **Consistency**: Ensure all nodes use the same base configuration
- **Cost Savings**: Reduce cloud compute time during cluster creation
- **Repeatability**: Create identical environments quickly

## Template Create

Create a template with a specific OS and Aerospike version.

### Basic Usage

```bash
aerolab template create --distro=ubuntu --distro-version=24.04 --aerospike-version='7.*'
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--distro` | Distribution (ubuntu, centos, rocky, debian, amazon) | Required |
| `--distro-version` | Distribution version | Required |
| `--aerospike-version` | Aerospike version (supports wildcards) | Required |
| `--arch` | Architecture (amd64, arm64) | Current system architecture |
| `--owner` | Owner tag for the template | |
| `--no-vacuum` | Don't vacuum failed templates before creating | |

### Examples

**Create Ubuntu 24.04 with latest Aerospike 8:**
```bash
aerolab template create --distro=ubuntu --distro-version=24.04 \
  --aerospike-version='8.*' --arch=amd64
```

**Create with owner tag:**
```bash
aerolab template create --distro=ubuntu --distro-version=24.04 \
  --aerospike-version='7.*' --arch=amd64 --owner=teamname
```

**Create ARM64 template:**
```bash
aerolab template create --distro=ubuntu --distro-version=24.04 \
  --aerospike-version='8.*' --arch=arm64
```

### How Templates Work

1. Creates a temporary instance
2. Installs and configures Aerospike
3. Creates a snapshot/image of the instance
4. Destroys the temporary instance
5. Template is ready for use

## Template List

List all available templates.

### Basic Usage

```bash
# List all templates
aerolab template list

# JSON output
aerolab template list -o json

# TSV output
aerolab template list -o tsv
```

### Output Format

The list shows:
- Template ID/Name
- Distribution and version
- Aerospike version
- Architecture
- Creation date
- Owner (if set)

## Template Destroy

Delete a template to free up space.

### Basic Usage

```bash
aerolab template destroy --distro=ubuntu --distro-version=24.04 \
  --aerospike-version=7.0.0 --arch=amd64 --force
```

### Options

| Option | Description |
|--------|-------------|
| `--distro` | Distribution | Required |
| `--distro-version` | Distribution version | Required |
| `--aerospike-version` | Exact Aerospike version (no wildcards) | Required |
| `--arch` | Architecture | Required |
| `--force` | Force deletion without confirmation | Required |

**Note**: When destroying templates, you must specify the **exact version** (not a wildcard like `'8.*'`). Use `template list` to find the exact version.

### Example

```bash
# First, find the exact version
aerolab template list

# Then destroy with exact version
aerolab template destroy --distro=ubuntu --distro-version=24.04 \
  --aerospike-version=8.1.0.1 --arch=amd64 --force
```

## Template Vacuum

Remove dangling or failed template instances and images.

### Basic Usage

```bash
aerolab template vacuum
```

### When to Use

Run `template vacuum` when:
- Template creation fails
- Dangling template instances remain
- Incomplete template images exist
- You want to clean up before creating new templates

### What It Does

- Identifies incomplete or failed template creations
- Removes orphaned template instances
- Cleans up partial template images
- Frees up resources

## Template Usage in Cluster Commands

When you create a cluster, Aerolab automatically:

1. Checks if a matching template exists
2. Uses the template if available (fast)
3. Creates a new template if needed (slower first time)
4. Reuses the template for subsequent cluster creations

### Example Workflow

```bash
# First time: Creates template (slower)
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*'

# Subsequent times: Uses template (faster)
aerolab cluster create -c 5 -d ubuntu -i 24.04 -v '8.*'
```

## Template Naming Convention

Templates are automatically named using the pattern:
```
aerospike-<distro>-<version>-<aerospike-version>-<arch>
```

Example: `aerospike-ubuntu-24.04-8.1.0.1-amd64`

## Managing Template Storage

### Docker Backend
- Templates stored as Docker images
- Use `docker images` to see all images
- Templates can be large (1-3 GB each)

### AWS Backend
- Templates stored as AMIs (Amazon Machine Images)
- Each AMI has associated EBS snapshots
- Cost: ~$0.05 per GB-month for snapshots

### GCP Backend
- Templates stored as custom images
- Cost: ~$0.05 per GB-month for storage

## Best Practices

1. **Create templates for frequently used configurations**
   - Common OS versions
   - Specific Aerospike versions
   - Different architectures

2. **Clean up unused templates**
   ```bash
   aerolab template list
   aerolab template destroy --distro=... --force
   ```

3. **Use wildcards for cluster creation**
   ```bash
   # Creates/uses template for latest 8.x version
   aerolab cluster create -v '8.*' ...
   ```

4. **Vacuum regularly**
   ```bash
   aerolab template vacuum
   ```

5. **Tag templates with owner in shared environments**
   ```bash
   aerolab template create --owner=myteam ...
   ```

## Troubleshooting

**Template creation fails:**
1. Run `aerolab template vacuum`
2. Check backend connectivity
3. Verify sufficient permissions
4. Check available disk space/quotas

**Template not found:**
- Verify exact version with `aerolab template list`
- Create template manually before cluster creation
- Check architecture matches (amd64 vs arm64)

**Template takes too long:**
- Normal on first creation (5-15 minutes)
- Subsequent uses are fast (< 1 minute per node)

## See Also

- [Cluster Management](cluster.md) - Create clusters using templates
- [Images](images.md) - Lower-level image management
- [Instances](instances.md) - Raw instance management

