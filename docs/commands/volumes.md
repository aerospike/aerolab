# Volume Management Commands

Volume management commands allow you to create, attach, and manage persistent storage volumes for your instances. Supports AWS EFS (shared) and GCP persistent disks (attached/shared).

## Commands Overview

- `volumes create` - Create a volume
- `volumes list` - List volumes
- `volumes attach` - Attach/mount a volume to instances
- `volumes grow` - Grow a volume (GCP only)
- `volumes detach` - Detach a volume from instances (GCP only)
- `volumes add-tags` - Add tags to volumes
- `volumes remove-tags` - Remove tags from volumes
- `volumes delete` - Delete a volume

## Volume Types

| Type | AWS | GCP | Use Case |
|------|-----|-----|----------|
| **Attached** | ❌ | ✅ | Additional persistent disks per instance |
| **Shared** | ✅ (EFS) | ✅ | Shared storage across multiple instances |

## Volumes Create

Create a new volume.

### Basic Usage

```bash
# GCP attached volume
aerolab volumes create --name myvolume --volume-type attached \
  --gcp.size 10 --gcp.zone us-central1-a --gcp.disk-type pd-ssd

# AWS shared volume (EFS)
aerolab volumes create --name myvolume --volume-type shared \
  --aws.placement us-east-1
```

### Options

| Option | Description |
|--------|-------------|
| `--name` | Volume name (unique identifier) | Required |
| `--volume-type` | Type: `attached` or `shared` | Required |

**GCP-Specific Options:**
| Option | Description | Default |
|--------|-------------|---------|
| `--gcp.size` | Size in GB | Required for attached volumes |
| `--gcp.zone` | Zone (e.g., us-central1-a) | Required |
| `--gcp.disk-type` | Disk type (pd-standard, pd-ssd, pd-balanced) | `pd-standard` |
| `--gcp.expire` | Expiry time (e.g., 8h, 24h) | None |

**AWS-Specific Options:**
| Option | Description | Default |
|--------|-------------|---------|
| `--aws.placement` | Region or AZ | Required |
| `--aws.disk-type` | For shared: `shared` (EFS) | `shared` |
| `--aws.expire` | Expiry time | None |

### Examples

**GCP attached volume with expiry:**
```bash
aerolab volumes create --name data-vol --volume-type attached \
  --gcp.size 100 \
  --gcp.zone us-central1-a \
  --gcp.disk-type pd-ssd \
  --gcp.expire=24h
```

**AWS shared volume (EFS):**
```bash
aerolab volumes create --name shared-data --volume-type shared \
  --aws.placement us-east-1 \
  --aws.disk-type shared \
  --aws.expire=8h
```

## Volumes List

List all volumes.

### Basic Usage

```bash
# List all volumes
aerolab volumes list

# JSON output
aerolab volumes list -o json

# Filter by name
aerolab volumes list --filter.name myvolume
```

### Output Information

- Volume name
- Type (attached/shared)
- Size
- Status (available, in-use)
- Attached instances
- Zone/region
- Creation date
- Expiry time (if set)

## Volumes Attach

Attach/mount a volume to instances.

### Basic Usage

```bash
# Attach volume to a cluster
aerolab volumes attach --filter.name myvolume \
  --instance.cluster-name mycluster

# Attach shared volume with mount point
aerolab volumes attach --filter.name shared-data \
  --instance.cluster-name mycluster \
  --shared-target=/mnt/shared
```

### Options

| Option | Description |
|--------|-------------|
| `--filter.name` | Volume name to attach | Required |
| `--instance.cluster-name` | Target cluster name | Required |
| `--instance.node` | Specific node number (optional) |
| `--shared-target` | Mount point for shared volumes | Required for shared |

### Examples

**Attach to specific node:**
```bash
aerolab volumes attach --filter.name data-vol \
  --instance.cluster-name mycluster \
  --instance.node 1
```

**Attach shared volume to all nodes:**
```bash
aerolab volumes attach --filter.name efs-volume \
  --instance.cluster-name mycluster \
  --shared-target=/mnt/efs
```

### What Happens

1. **GCP Attached Volumes:**
   - Attaches disk to instance
   - Formats filesystem (if needed)
   - Mounts at `/mnt/<volume-name>`
   - Adds to `/etc/fstab` for persistence

2. **AWS Shared Volumes (EFS):**
   - Installs NFS client
   - Mounts EFS at specified target
   - Adds to `/etc/fstab`
   - Available across all nodes

## Volumes Grow

Grow a volume to a larger size (GCP only).

### Basic Usage

```bash
aerolab volumes grow --filter.name myvolume --new-size-gb 200
```

### Options

| Option | Description |
|--------|-------------|
| `--filter.name` | Volume name | Required |
| `--new-size-gb` | New size in GB | Required |

### Important Notes

- **Cannot shrink** - Only increase size
- **Must be larger** than current size
- **If attached** - Filesystem is automatically resized
- **If detached** - Manual resize needed on next attach
- **AWS EFS** - Does not support resize (auto-grows)

### Example

```bash
# Check current size
aerolab volumes list --filter.name data-vol

# Grow from 100GB to 200GB
aerolab volumes grow --filter.name data-vol --new-size-gb 200

# Filesystem automatically resized if attached
```

## Volumes Detach

Detach a volume from instances (GCP only).

### Basic Usage

```bash
aerolab volumes detach --filter.name myvolume \
  --instance.cluster-name mycluster
```

### Options

| Option | Description |
|--------|-------------|
| `--filter.name` | Volume name | Required |
| `--instance.cluster-name` | Cluster to detach from | Required |
| `--instance.node` | Specific node (optional) |

### What Happens

1. Unmounts the volume
2. Removes from `/etc/fstab`
3. Detaches from instance
4. Volume remains available for reattach

**Note:** AWS EFS volumes must be unmounted manually (detach not supported for shared volumes).

## Tag Management

Add or remove tags/labels from volumes.

### Basic Usage

```bash
# Add tags
aerolab volumes add-tags --filter.name myvolume \
  --tag env=production --tag team=data

# Remove tags
aerolab volumes remove-tags --filter.name myvolume \
  --tag team
```

### Common Use Cases

- Environment tagging (dev/staging/prod)
- Cost allocation
- Ownership tracking
- Project organization

## Volumes Delete

Delete a volume.

### Basic Usage

```bash
aerolab volumes delete --filter.name myvolume --force
```

### Options

| Option | Description |
|--------|-------------|
| `--filter.name` | Volume name to delete | Required |
| `--force` | Force deletion without confirmation | Required |

### Important Notes

- Volume must be detached first (GCP)
- For AWS EFS, unmount from all instances first
- Deletion is permanent and irreversible
- All data on the volume is lost

## Complete Workflow Examples

### GCP Attached Volume Workflow

```bash
# 1. Create cluster
aerolab cluster create -n apt -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 --gcp-expire=8h

# 2. Create attached volume
aerolab volumes create --name data-vol --volume-type attached \
  --gcp.size 100 --gcp.zone us-central1-a \
  --gcp.disk-type pd-ssd --gcp.expire=8h

# 3. Attach to cluster
aerolab volumes attach --filter.name data-vol \
  --instance.cluster-name apt

# 4. Use the volume
aerolab attach shell -n apt -l 1 -- df -h
aerolab attach shell -n apt -l 1 -- ls -la /mnt/data-vol

# 5. Grow the volume
aerolab volumes grow --filter.name data-vol --new-size-gb 200

# 6. Add tags
aerolab volumes add-tags --filter.name data-vol \
  --tag project=analytics --tag env=test

# 7. Detach volume
aerolab volumes detach --filter.name data-vol \
  --instance.cluster-name apt

# 8. Clean up
aerolab volumes delete --filter.name data-vol --force
aerolab cluster destroy -n apt --force
```

### AWS Shared Volume (EFS) Workflow

```bash
# 1. Create cluster
aerolab cluster create -n mycluster -c 3 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge --aws-expire=8h

# 2. Create shared volume (EFS)
aerolab volumes create --name shared-data --volume-type shared \
  --aws.placement us-east-1 --aws.disk-type shared \
  --aws.expire=8h

# 3. Attach to all nodes
aerolab volumes attach --filter.name shared-data \
  --instance.cluster-name mycluster \
  --shared-target=/mnt/shared

# 4. Test shared access
aerolab attach shell -n mycluster -l 1 -- \
  "echo 'test from node 1' > /mnt/shared/test.txt"

aerolab attach shell -n mycluster -l 2 -- \
  cat /mnt/shared/test.txt

# 5. Add tags
aerolab volumes add-tags --filter.name shared-data \
  --tag purpose=shared-storage

# 6. Clean up (detach from all nodes first)
aerolab attach shell -n mycluster -l all -- umount /mnt/shared
aerolab volumes delete --filter.name shared-data --force
aerolab cluster destroy -n mycluster --force
```

## Best Practices

1. **Always set expiry in cloud environments**
   ```bash
   --gcp.expire=24h --aws.expire=24h
   ```

2. **Use descriptive names**
   ```bash
   --name prod-database-backup-volume
   ```

3. **Tag volumes for organization**
   ```bash
   --tag env=production --tag owner=team-name
   ```

4. **Detach before deletion (GCP)**
   ```bash
   aerolab volumes detach ...
   aerolab volumes delete ...
   ```

5. **Match volume zone with instances (GCP)**
   - Volumes can only attach to instances in the same zone

6. **Plan capacity growth**
   - Start with reasonable size
   - Use `volumes grow` as needed
   - Cannot shrink volumes

## Cost Considerations

### GCP
- **pd-standard**: ~$0.04 per GB-month
- **pd-ssd**: ~$0.17 per GB-month
- **pd-balanced**: ~$0.10 per GB-month

### AWS
- **EFS**: ~$0.30 per GB-month (Standard)
- **EFS**: ~$0.043 per GB-month (Infrequent Access)

## Troubleshooting

**Attach fails:**
- Verify zone matches (GCP)
- Check volume is available (not in-use)
- Ensure permissions are correct
- Check instance is running

**Cannot detach:**
- Ensure volume is not in use
- No processes accessing the mount point
- Unmount manually if needed

**Grow fails:**
- Must specify size larger than current
- GCP only feature
- AWS EFS grows automatically

**Delete fails:**
- Detach volume first (GCP)
- Unmount from all instances (AWS EFS)
- Use `--force` flag

## See Also

- [Cluster Management](cluster.md) - Create clusters with volumes
- [Instances](instances.md) - Instance management
- [Configuration](config.md) - Backend configuration

