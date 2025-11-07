# Getting Started with GCP Backend

The GCP backend allows you to create and manage Aerospike clusters on Google Cloud Platform (GCP) Compute Engine.

## Prerequisites

### GCP Account Setup

1. **GCP Account** - You need an active Google Cloud Platform account
2. **GCP Project** - Create or select a GCP project
3. **Google Cloud CLI** - Install and configure the [Google Cloud CLI](https://cloud.google.com/sdk/docs/install)
4. **Authentication** - Aerolab uses Application Default Credentials (ADC) via the gcloud CLI

### Authentication Setup

Aerolab requires service account authentication using Application Default Credentials. You must authenticate using the gcloud CLI before using Aerolab:

```bash
gcloud auth application-default login
```

This command will:
- Open your browser for authentication
- Store credentials locally for use by Aerolab
- Allow Aerolab to authenticate with GCP services

**Note**: If you see an authentication error, ensure you've run `gcloud auth application-default login` first. See the [troubleshooting section](#authentication-issues) for more details.

## Initial Setup

### 1. Authenticate with GCP

Before configuring Aerolab, authenticate with Google Cloud using Application Default Credentials:

```bash
gcloud auth application-default login
```

This will open your browser for authentication. After completing the authentication flow, your credentials will be stored locally and used by Aerolab.

**Note**: You may need to specify a project when authenticating:
```bash
gcloud auth application-default login --project=your-project-id
```

### 2. Configure GCP Backend

Configure Aerolab to use the GCP backend:

```bash
aerolab config backend -t gcp -r us-central1 -o your-project-id
```

This will:
- Set GCP as the default backend
- Set the default region to `us-central1`
- Set the project ID to `your-project-id`
- Use Application Default Credentials for authentication

**Note**: Replace `your-project-id` with your actual GCP project ID.

### Optional: Enable Inventory Cache

If you're not sharing the GCP project with other users, enable inventory caching:

```bash
aerolab config backend -t gcp -r us-central1 -o your-project-id --inventory-cache
```

**Note**: Only use inventory cache if you're the sole user of the GCP project, as it caches resource state locally.

### Optional: Specify Multiple Regions

You can specify multiple regions:

```bash
aerolab config backend -t gcp -r us-central1,us-east1,us-west1 -o your-project-id
```

### 2. Verify Configuration

Check your backend configuration:

```bash
aerolab config backend
```

You should see:
```
Config.Backend.Type = gcp
Config.Backend.Project = your-project-id
Config.Backend.Region = us-central1
Config.Backend.GCPAuthMethod = service-account
Config.Backend.SshKeyPath = ${HOME}/.config/aerolab
```

### 3. Check Access

Verify you have access to GCP:

```bash
aerolab config backend -t gcp --check-access
```

### 4. Clean Up Existing Resources (Optional)

If you have existing Aerolab resources:

```bash
aerolab inventory delete-project-resources -f
```

Or with expiry:

```bash
aerolab inventory delete-project-resources -f --expiry
```

## GCP-Specific Configuration

### List Firewall Rules

View existing firewall rules:

```bash
aerolab config gcp list-firewall-rules
```

### Create Firewall Rules

Create a firewall rule for Aerospike:

```bash
aerolab config gcp create-firewall-rules -n aerolab-fw -p 3000-3005
```

This creates a firewall rule allowing ports 3000-3005.

### Lock Firewall Rules

Lock a firewall rule to prevent deletion:

```bash
aerolab config gcp lock-firewall-rules -n aerolab-fw
```

### Delete Firewall Rules

Delete a firewall rule:

```bash
aerolab config gcp delete-firewall-rules -n aerolab-fw
```

## Creating Your First Cluster

### Basic Cluster Creation

Create a simple 2-node cluster:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

This command:
- Creates 2 nodes (`-c 2`)
- Uses Ubuntu 24.04 (`-d ubuntu -i 24.04`)
- Installs Aerospike version 8.x (`-v '8.*'`)
- Uses `e2-standard-4` instance type (`--instance e2-standard-4`)
- Creates 20GB PD-SSD root disk (`--gcp-disk type=pd-ssd,size=20`)
- Sets expiry to 8 hours (`--gcp-expire=8h`)

### Specify Zone

Specify a zone for deployment:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --zone us-central1-a \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

### Multiple Disks

Add multiple disks:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-disk type=pd-ssd,size=100,count=3 \
  --gcp-expire=8h
```

This creates:
- One 20GB PD-SSD root disk
- Three 100GB PD-SSD data disks

### Custom Disk Types

Use different disk types:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-disk type=hyperdisk-balanced,size=100,iops=3060,throughput=155,count=2 \
  --gcp-expire=8h
```

### Custom Firewall Rules

Add custom firewall rules:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --firewall aerolab-fw \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

### Public IP

Enable public IP access:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --external-ip \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

### Spot Instances

Use spot instances for cost savings:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --gcp-spot-instance \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

### Custom Labels

Add custom labels:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --label Environment=Development \
  --label Team=Platform \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

### Custom Tags

Add custom network tags:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --tag aerolab \
  --tag production \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

## Resource Expiry Management

### Install Expiry Automation

Install automated resource expiry:

```bash
aerolab config gcp expiry-install
```

This installs a Cloud Function that automatically cleans up expired resources.

### List Expiry Rules

View expiry rules:

```bash
aerolab config gcp expiry-list
```

### Set Expiry Run Frequency

Configure how often expiry runs:

```bash
aerolab config gcp expiry-run-frequency -f 20
```

This sets expiry to run every 20 minutes.

### Remove Expiry

Remove expiry automation:

```bash
aerolab config gcp expiry-remove
```

## Starting and Stopping Clusters

### Start Cluster

Start all nodes in the cluster:

```bash
aerolab cluster start
```

### Stop Cluster

Stop all nodes:

```bash
aerolab cluster stop
```

**Note**: Stopping instances in GCP doesn't delete them, but you'll still be charged for persistent disks. Use expiry or destroy to completely remove resources.

## Managing Aerospike Service

### Start Aerospike

```bash
aerolab aerospike start
```

### Stop Aerospike

```bash
aerolab aerospike stop
```

### Restart Aerospike

```bash
aerolab aerospike restart
```

### Check Status

```bash
aerolab aerospike status
```

### Wait for Cluster Stability

```bash
aerolab aerospike is-stable -w
```

## Connecting to Nodes

### Shell Access

```bash
aerolab attach shell -n mydc -l 1
```

### Aerospike Tools

```bash
# AQL
aerolab attach aql -n mydc -- -c "show namespaces"

# asinfo
aerolab attach asinfo -n mydc -- -v "cluster-stable"

# asadm
aerolab attach asadm -n mydc -- -e info
```

## File Operations

### Upload Files

```bash
aerolab files upload -n mydc local-file.txt /tmp/remote-file.txt
```

### Download Files

```bash
aerolab files download -n mydc /tmp/remote-file.txt ./local-dir/
```

### Sync Files

```bash
aerolab files sync -n mydc -l 1 /tmp/file.txt
```

## GCP-Specific Features

### Add Public IP Later

Add public IP to existing cluster:

```bash
aerolab cluster add public-ip -n mydc
```

### Add Firewall Rules

Add firewall rules to cluster:

```bash
aerolab cluster add firewall -n mydc -f aerolab-fw
```

### Volume Mounting

Mount additional volumes:

```bash
# Create cluster with volume mount
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --gcp-vol-create \
  --gcp-vol-mount myvolume:/mnt/data \
  --gcp-vol-size 100 \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h
```

## Cleanup

### Destroy a Cluster

```bash
aerolab cluster destroy -n mydc --force
```

### Clean Up All Resources

```bash
aerolab inventory delete-project-resources -f --expiry
```

## Common Workflows

### Complete Workflow Example

```bash
# 1. Authenticate with GCP
gcloud auth application-default login

# 2. Configure backend
aerolab config backend -t gcp -r us-central1 -o your-project-id

# 3. Create firewall rule
aerolab config gcp create-firewall-rules -n aerolab-fw -p 3000-3005

# 4. Create cluster
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-disk type=pd-ssd,size=100,count=3 \
  --firewall aerolab-fw \
  --gcp-expire=8h

# 5. Start cluster
aerolab cluster start

# 6. Start Aerospike
aerolab aerospike start

# 7. Wait for stability
aerolab aerospike is-stable -w

# 8. Check status
aerolab aerospike status

# 9. Use the cluster
aerolab attach aql -n mydc -- -c "show namespaces"

# 10. Clean up
aerolab cluster destroy -n mydc --force
```

## Troubleshooting

### Authentication Issues

If you see authentication errors:

1. **Application Default Credentials not found**: Ensure you've run `gcloud auth application-default login`:
   ```bash
   gcloud auth application-default login
   ```
   If you see the error "could not authenticate using application credentials", follow the instructions at: https://docs.cloud.google.com/docs/authentication/set-up-adc-local-dev-environment

2. **Check authentication status**: Verify your gcloud authentication:
   ```bash
   gcloud auth list
   gcloud config get-value project
   ```

3. **Re-authenticate if needed**: If credentials have expired, run:
   ```bash
   gcloud auth application-default login
   ```

4. **Check permissions**: Ensure your user account has Compute Engine permissions in the GCP project

### Project Issues

If resources aren't found, verify the project ID:

```bash
aerolab config backend
```

### Region/Zone Issues

Check available zones in your region:

```bash
gcloud compute zones list
```

### Instance Type Availability

Check available instance types in your region:

```bash
aerolab inventory instance-types
```

### Network Issues

If nodes can't communicate:

1. Check firewall rules
2. Verify VPC configuration
3. Check network tags

### Quota Issues

If you hit quota limits:

1. Check quotas in GCP Console
2. Request quota increases if needed
3. Use smaller instance types or fewer nodes

## Next Steps

- Explore [cluster management commands](commands/cluster.md)
- Learn about [Aerospike daemon controls](commands/aerospike.md)
- Check out [GCP-specific volume management](commands/volumes.md)
- See [advanced features](commands/) for more options

