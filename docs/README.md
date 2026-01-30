# Aerolab CLI Documentation

Aerolab is a comprehensive command-line tool for creating, managing, and operating Aerospike clusters across multiple cloud backends (Docker, AWS, and GCP). This documentation provides detailed usage instructions for all Aerolab CLI commands.

## Installation

Aerolab is distributed as pre-built binaries for Linux, macOS, and Windows. Download the latest release from the [GitHub Releases page](https://github.com/aerospike/aerolab/releases).

### Quick Install

1. Download the appropriate binary for your operating system
2. Extract and place the binary in your PATH (e.g., `/usr/local/bin` or `~/bin`)
3. Make it executable:
   ```bash
   chmod +x aerolab
   ```
4. Verify installation:
   ```bash
   aerolab version
   ```

## Getting Started

Choose your backend and follow the appropriate getting started guide:

- **[Docker Backend](getting-started/docker.md)** - Quick start with Docker, Docker Desktop, Podman, or Podman Desktop
- **[AWS Backend](getting-started/aws.md)** - Set up with AWS credentials and EC2
- **[GCP Backend](getting-started/gcp.md)** - Set up with Google Cloud Platform (Application Default Credentials)

### Upgrading from v7

If you're upgrading from AeroLab v7.x, see the **[Migration Guide](migration-guide.md)** for instructions on migrating your configuration and cloud resources.

## Command Categories

### Core Commands

- **[Cluster Management](commands/cluster.md)** - Create, manage, and operate clusters
  - Create, grow, apply, list, start, stop, destroy clusters
  - Add features (exporter, aerolab, firewall, public IP)
  - Partition management for disk configuration

- **[Aerospike Daemon Controls](commands/aerospike.md)** - Control Aerospike service
  - Start, stop, restart, status
  - Upgrade, cold-start, stability checks

- **[Configuration Management](commands/config.md)** - Configure Aerolab and backends
  - Backend configuration (Docker, AWS, GCP)
  - Default settings and environment variables
  - Backend-specific settings (security groups, firewalls, expiry)

- **[Inventory Management](commands/inventory.md)** - View and manage all resources
  - List all clusters, instances, and images
  - Export formats (Ansible, genders, hostfile)
  - Delete project resources

- **[Instance Management](commands/instances.md)** - Direct instance operations
  - Create, grow, apply, list instances
  - Start, stop, restart instances
  - Tag management and firewall assignment

- **[Client Management](commands/client.md)** - Create and manage client machines
  - Create client machines (none, base, tools, ams, vscode, graph, eksctl)
  - Configure clients (AMS, firewall, tools)
  - Start, stop, destroy, share clients

### Operational Commands

- **[Attach Commands](commands/attach.md)** - Connect to nodes and run commands
  - Shell access
  - Aerospike tools (aql, asinfo, asadm)
  - Client tools and AGI

- **[Configuration File Management](commands/conf.md)** - Manage Aerospike configuration files
  - Rack ID assignment
  - Strong consistency configuration
  - Mesh configuration fixes
  - Parameter adjustments
  - Namespace memory configuration

- **[File Operations](commands/files.md)** - Transfer files to/from instances
  - Upload, download, sync files
  - Edit files remotely

- **[Log Management](commands/logs.md)** - View and download logs
  - Show logs
  - Download logs to local directory

- **[Data Management](commands/data.md)** - Insert and delete test data
  - Insert test data with flexible patterns
  - Delete test data
  - Multi-threaded operations

- **[Network Simulation](commands/net.md)** - Test network conditions
  - Block/unblock network ports
  - Simulate packet loss, latency, bandwidth limits
  - Test split-brain and network failures

### Resource Management

- **[Templates](commands/templates.md)** - Manage Aerospike server templates
  - Create, list, destroy templates
  - Template vacuuming

- **[Images](commands/images.md)** - Manage system images
  - Create, list, delete images
  - Image vacuuming

- **[Volumes](commands/volumes.md)** - Volume management (AWS EFS/GCP volumes)
  - Create attached and shared volumes
  - Attach, detach, grow volumes
  - Tag management

- **[Installer](commands/installer.md)** - Download Aerospike installers
  - List available versions
  - Download installer packages

### Advanced Features

- **[XDR Management](commands/xdr.md)** - Cross-datacenter replication
  - Connect clusters via XDR
  - Create XDR-connected clusters
  - Configure XDR v4 and v5

- **[TLS Certificate Management](commands/tls.md)** - Secure clusters with TLS
  - Generate TLS certificates
  - Copy certificates between clusters
  - CA and certificate management

- **[Roster Management](commands/roster.md)** - Strong consistency roster operations
  - Apply rosters
  - Show roster status
  - Roster cheat sheet

- **[Aerospike Cloud](cloud/README.md)** - Manage Aerospike Cloud clusters
  - [Cluster Management](cloud/clusters.md) - Create, update, delete clusters
  - [Credentials Management](cloud/credentials.md) - Manage cluster credentials
  - [Secrets Management](cloud/secrets.md) - Manage secrets
  - [VPC Peering](cloud/vpc-peering.md) - Peer VPCs with clusters

### Log Analysis and Visualization

- **[AGI (Aerospike Grafana Integration)](agi/README.md)** - Log analysis and visualization
  - [Quick Start Guide](agi/quick-start.md) - Get started with AGI
  - [Configuration Reference](agi/configuration.md) - Detailed command options
  - [Troubleshooting Guide](agi/troubleshooting.md) - Common issues and solutions
  - Features:
    - Ingest logs from local files, SFTP, S3, or clusters
    - Pre-built Grafana dashboards for log analysis
    - Collect info bundle analysis
    - Token-based authentication
    - Auto-scaling with AGI Monitor (AWS/GCP)

## Reference

- **[Environment Variables](reference/environment-variables.md)** - All available environment variables

## Quick Start Example

### Docker Backend

```bash
# Configure Docker backend
aerolab config backend -t docker

# Create a 2-node cluster
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*'

# Check cluster status
aerolab cluster list

# Start Aerospike
aerolab aerospike start

# Check if cluster is stable
aerolab aerospike is-stable -w

# View cluster status
aerolab aerospike status
```

### AWS Backend

```bash
# Configure AWS backend (requires AWS credentials in ~/.aws/credentials)
aerolab config backend -t aws -r us-east-1

# Create a 2-node cluster with expiry
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h

# Start cluster
aerolab cluster start

# Start Aerospike
aerolab aerospike start
```

### GCP Backend

```bash
# Authenticate with GCP first (required)
gcloud auth application-default login

# Configure GCP backend
aerolab config backend -t gcp -r us-central1 -o your-project-id

# Create a 2-node cluster with expiry
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 \
  --gcp-disk type=pd-ssd,size=20 \
  --gcp-expire=8h

# Start cluster
aerolab cluster start

# Start Aerospike
aerolab aerospike start
```

## Common Workflows

### Creating a Production Cluster

1. Configure backend
2. Create cluster with appropriate instance types and disks
3. Configure partitions (if using local storage)
4. Start cluster and Aerospike
5. Wait for cluster stability
6. Apply configuration changes
7. Verify cluster operation

### Setting Up Monitoring (AMS)

1. Create Aerospike cluster(s)
2. Add Prometheus exporter to clusters
3. Create AMS client with Grafana and Prometheus
4. Configure AMS to monitor clusters
5. Access Grafana dashboard for metrics

### Configuring XDR Between Clusters

1. Create source and destination clusters
2. Use `xdr connect` to configure replication
3. Verify XDR status with asadm
4. Monitor XDR lag in Grafana

### Securing Clusters with TLS

1. Generate TLS certificates for cluster
2. Restart Aerospike to enable TLS
3. Copy certificates to client machines
4. Connect clients using TLS parameters

### Testing Network Resilience

1. Create test clusters
2. Use `net loss-delay` to simulate latency/packet loss
3. Monitor cluster behavior
4. Use `net block` to test split-brain scenarios
5. Reset network conditions after testing

### Managing Multiple Clusters

- Use cluster names to manage multiple clusters
- Filter operations by cluster name and node numbers
- Use inventory commands to see all resources

### Cleanup and Expiry

- Set expiry times on resources to auto-cleanup
- Use `inventory delete-project-resources` to clean up all resources
- Configure expiry automation for AWS/GCP backends

## Getting Help

- Use `aerolab <command> --help` for command-specific help
- Use `aerolab <command> help` for detailed help
- Check the [command documentation](commands/) for detailed examples
