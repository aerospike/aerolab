# Image Management Commands

Image management commands provide low-level control over system images. Images are base snapshots without Aerospike installed (unlike templates which include Aerospike).

## Commands Overview

- `images list` - List available images
- `images create` - Create a new custom image
- `images destroy` - Delete an image
- `images vacuum` - Remove dangling unfinished images

## Images vs Templates

| Feature | Images | Templates |
|---------|--------|-----------|
| Aerospike | ❌ Not installed | ✅ Installed |
| Use Case | Base OS snapshots | Aerospike deployments |
| Creation Time | Faster | Slower (includes Aerospike setup) |
| Size | Smaller | Larger |
| Primary Use | Custom base images | Cluster creation |

## Images List

List all available system images.

### Basic Usage

```bash
# List all images
aerolab images list

# JSON output
aerolab images list -o json

# TSV output
aerolab images list -o tsv
```

### What's Shown

- Image ID/Name
- Distribution
- Architecture
- Creation date
- Size
- Status

## Images Create

Create a custom system image from an existing instance.

### Basic Usage

```bash
aerolab images create -n myimage --cluster-name mycluster --node 1
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `-n, --name` | Name for the new image | Yes |
| `--cluster-name` | Source cluster name | Yes |
| `--node` | Source node number | Yes |
| `--description` | Image description | No |

### Example Workflow

```bash
# 1. Create an instance cluster
aerolab instances create -n mybase -c 1 -d ubuntu -i 24.04

# 2. Customize the instance
aerolab instances attach -n mybase -l 1 -- apt-get update
aerolab instances attach -n mybase -l 1 -- apt-get install -y htop vim

# 3. Create an image from the customized instance
aerolab images create -n custom-ubuntu-24.04 \
  --cluster-name mybase --node 1 \
  --description "Ubuntu 24.04 with custom tools"

# 4. Clean up the instance
aerolab instances destroy -n mybase --force

# 5. Use the custom image for new instances
aerolab instances create -n newcluster --image custom-ubuntu-24.04 -c 3
```

## Images Destroy

Delete a custom image.

### Basic Usage

```bash
aerolab images destroy --name myimage --force
```

### Options

| Option | Description |
|--------|-------------|
| `--name` | Image name to destroy | Required |
| `--force` | Force deletion without confirmation | Required |

### Important Notes

- Cannot delete system/base images
- Can only delete custom images you created
- Deletion is permanent and cannot be undone
- Will fail if instances are currently using the image

## Images Vacuum

Remove dangling or incomplete images from failed creation attempts.

### Basic Usage

```bash
aerolab images vacuum
```

### When to Use

Run `images vacuum` when:
- Image creation fails
- Partial images remain after errors
- Cleanup is needed before creating new images
- Storage space needs to be freed

### What It Cleans

- Incomplete image creations
- Orphaned image data
- Failed image conversion attempts
- Temporary image files

## Use Cases

### Custom Base Images

Create standardized base images with your organization's tools and configurations:

```bash
# Create base instance
aerolab instances create -n baseline -c 1 -d ubuntu -i 24.04

# Install standard tools
aerolab instances attach -n baseline -l 1 -- bash -c '
  apt-get update
  apt-get install -y htop vim tmux jq
  # Custom configurations
  echo "set -o vi" >> /etc/bash.bashrc
'

# Create reusable image
aerolab images create -n company-baseline-ubuntu-24.04 \
  --cluster-name baseline --node 1

# Cleanup
aerolab instances destroy -n baseline --force
```

### Testing Environments

Create snapshots of configured test environments:

```bash
# Setup test environment
aerolab instances create -n testenv -c 1 -d ubuntu -i 24.04
aerolab files upload test-data.tar.gz /opt/
aerolab instances attach -- tar -xzf /opt/test-data.tar.gz -C /opt/

# Create image
aerolab images create -n test-env-configured \
  --cluster-name testenv --node 1

# Quickly spin up identical test instances
aerolab instances create --image test-env-configured -c 5
```

### Development Environments

Snapshot development setups with tools pre-installed:

```bash
# Setup dev environment
aerolab instances create -n devbox -c 1 -d ubuntu -i 24.04
aerolab instances attach -- bash -c '
  # Install development tools
  apt-get update
  apt-get install -y build-essential git python3 python3-pip
  pip3 install ipython jupyter
'

# Create dev image
aerolab images create -n dev-environment \
  --cluster-name devbox --node 1 \
  --description "Development environment with build tools"
```

## Backend-Specific Behavior

### Docker Backend
- Images stored as Docker images
- Use `docker images` to see all images
- Shares storage with Docker

### AWS Backend
- Images stored as AMIs (Amazon Machine Images)
- Must be in same region as instances
- Associated EBS snapshots created automatically
- Cost: ~$0.05 per GB-month

### GCP Backend
- Images stored as custom images in GCP
- Global availability across zones
- Cost: ~$0.05 per GB-month

## Best Practices

1. **Use descriptive names**
   ```bash
   aerolab images create -n ubuntu-24.04-python-dev ...
   ```

2. **Add descriptions**
   ```bash
   aerolab images create --description "Ubuntu 24.04 with Python 3.12 and ML libraries" ...
   ```

3. **Clean up unused images**
   ```bash
   aerolab images list
   aerolab images destroy --name old-image --force
   ```

4. **Vacuum after failures**
   ```bash
   aerolab images vacuum
   ```

5. **Document image contents**
   - Keep track of what's installed
   - Use consistent naming conventions
   - Tag with version numbers

## Limitations

- **Cannot modify existing images** - Create new ones instead
- **Backend-specific** - Images cannot be moved between backends
- **Region-specific (AWS)** - AMIs are per-region
- **Architecture-specific** - amd64 images won't work on arm64

## Troubleshooting

**Image creation fails:**
1. Ensure source instance is stopped
2. Check available storage quota
3. Verify permissions
4. Run `aerolab images vacuum`

**Cannot delete image:**
- Check if instances are using it
- Destroy dependent instances first
- Use `--force` flag

**Image not appearing:**
- Wait for creation to complete (can take 5-10 minutes)
- Check backend console (AWS/GCP)
- Run `aerolab images list` to refresh

## See Also

- [Templates](templates.md) - Aerospike-ready images
- [Instances](instances.md) - Create instances from images
- [Cluster Management](cluster.md) - Create clusters

