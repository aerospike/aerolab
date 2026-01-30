# AGI (Aerospike Grafana Integration)

AGI (Aerospike Grafana Integration) is a powerful log analysis and visualization system that allows you to ingest, process, and visualize Aerospike server logs through a Grafana-based dashboard interface.

## Overview

AGI provides:
- **Log Ingestion** - Import logs from local files, SFTP, S3, or directly from running clusters
- **Log Processing** - Parse and store log data in an embedded Aerospike database for fast querying
- **Visualization** - Pre-built Grafana dashboards for analyzing log patterns, errors, and performance
- **Collect Info Analysis** - Process and visualize Aerospike collect info bundles
- **Web Access** - Browser-based access to dashboards, terminal, and file browser
- **Auto-scaling** - Optional monitor system for automatic instance sizing (AWS/GCP only)

## Architecture

AGI uses a **template-based approach** for fast deployment:

```
AGI Template (pre-built with all software) → AGI Instance → Minimal Config → AGI Ready
```

This architecture provides:
- **Fast Instance Creation** - 1-2 minutes vs 5-10 minutes with traditional approaches
- **Consistent Environments** - Templates are tested once and reused
- **Simplified Maintenance** - Version-controlled templates

### Components

Each AGI instance includes:
- **Aerospike Database** - Stores parsed log data for fast querying
- **Grafana** - Visualization platform with pre-configured dashboards
- **AGI Plugin** - Custom Grafana datasource for querying log data
- **AGI Ingest** - Log parsing and ingestion pipeline
- **AGI Proxy** - Web proxy with authentication and activity monitoring
- **ttyd** - Web-based terminal for shell access
- **Filebrowser** - Web-based file manager

## Supported Backends

| Backend | Full Support | Volume Persistence | Auto-scaling |
|---------|--------------|-------------------|--------------|
| AWS     | ✅           | EFS               | ✅           |
| GCP     | ✅           | Persistent Disk   | ✅           |
| Docker  | ✅           | Volumes/Bind Mounts | ❌         |

## Documentation

- **[Quick Start Guide](quick-start.md)** - Get started with AGI in minutes
- **[Configuration Reference](configuration.md)** - Detailed command options and configuration
- **[Troubleshooting Guide](troubleshooting.md)** - Common issues and solutions

## Quick Example

### Docker (Fastest Start)

```bash
# Configure Docker backend
aerolab config backend -t docker

# Create AGI with local logs
aerolab agi create --source-local /path/to/logs

# List AGI instances
aerolab agi list

# Access URL will be displayed after creation
```

### AWS

```bash
# Configure AWS backend
aerolab config backend -t aws -r us-east-1

# Create AGI with S3 source
aerolab agi create -n myagi \
  --source-s3-enable \
  --source-s3-bucket my-log-bucket \
  --source-s3-path logs/ \
  --source-s3-region us-east-1 \
  --aws-expire=8h

# List AGI instances
aerolab agi list
```

### GCP

```bash
# Configure GCP backend
aerolab config backend -t gcp -r us-central1 -o my-project

# Create AGI with SFTP source
aerolab agi create -n myagi \
  --source-sftp-enable \
  --source-sftp-host sftp.example.com \
  --source-sftp-user myuser \
  --source-sftp-pass ENV::SFTP_PASSWORD \
  --source-sftp-path /logs \
  --gcp-expire=8h

# List AGI instances
aerolab agi list
```

## Commands Overview

### Instance Management

| Command | Description |
|---------|-------------|
| `agi create` | Create a new AGI instance |
| `agi list` | List AGI instances and volumes |
| `agi start` | Start an AGI instance (or create instance from existing volume) |
| `agi stop` | Stop an AGI instance |
| `agi destroy` | Destroy an AGI instance (preserves volume) |
| `agi delete` | Destroy instance and delete volume |
| `agi status` | Show AGI service status |
| `agi details` | Show ingest progress details |
| `agi open` | Open AGI in browser with authentication token |

### Access and Configuration

| Command | Description |
|---------|-------------|
| `agi attach` | Attach to AGI shell |
| `agi add-auth-token` | Manage authentication tokens |
| `agi change-label` | Change instance label |
| `agi run-ingest` | Retrigger log ingestion |
| `agi share` | Share access via SSH key |

### Template Management

| Command | Description |
|---------|-------------|
| `agi template create` | Create AGI template |
| `agi template list` | List AGI templates |
| `agi template destroy` | Destroy AGI template |
| `agi template vacuum` | Clean up unused templates |

### Monitor System (AWS/GCP only)

| Command | Description |
|---------|-------------|
| `agi monitor create` | Deploy AGI monitor instance |
| `agi monitor listen` | Start monitor listener |

## Data Sources

AGI supports multiple data sources:

### Local Files
Upload logs directly from your local machine:
```bash
aerolab agi create --source-local /path/to/logs
```

### SFTP
Download logs from an SFTP server:
```bash
aerolab agi create \
  --source-sftp-enable \
  --source-sftp-host sftp.example.com \
  --source-sftp-user myuser \
  --source-sftp-pass ENV::SFTP_PASS \
  --source-sftp-path /logs
```

### S3
Download logs from an S3 bucket:
```bash
aerolab agi create \
  --source-s3-enable \
  --source-s3-bucket my-bucket \
  --source-s3-path logs/ \
  --source-s3-region us-east-1 \
  --source-s3-key-id ENV::AWS_KEY \
  --source-s3-secret-key ENV::AWS_SECRET
```

### Running Cluster
Collect logs from a running Aerospike cluster:
```bash
aerolab agi create --source-cluster mydc
```

## Authentication

AGI supports multiple authentication modes:

- **None** - No authentication (default for Docker)
- **Token** - URL-based token authentication
- **Basic Auth** - Username/password authentication

For production use, always configure authentication:
```bash
# Add authentication token
aerolab agi add-auth-token -n myagi

# Or generate with specific size
aerolab agi add-auth-token -n myagi --size 128
```

## Persistence

### AWS EFS
```bash
aerolab agi create -n myagi --aws-with-efs --aws-efs-expire=96h ...
```

### GCP Persistent Volume
```bash
aerolab agi create -n myagi --gcp-with-vol --gcp-vol-expire=96h ...
```

### Starting an Instance from an Existing Volume

When using persistent volumes (EFS on AWS, Persistent Disk on GCP), you can destroy an instance to save costs while preserving all ingested data. Later, you can start a new instance using the existing volume:

```bash
# Check existing instances and volumes
aerolab agi list

# If a volume exists for 'myagi', start an instance for it
aerolab agi start -n myagi
```

The `agi list` command shows both running instances AND available volumes, making it easy to see which volumes can be started.

### Docker Volumes and Bind Mounts

Docker AGI supports flexible storage options:

**Bind mount for input logs (read-only):**
```bash
aerolab agi create --source-local bind:/path/to/logs
```

**Bind mount for both input and output:**
```bash
aerolab agi create \
  --bind-files-dir /host/path/to/agi-output \
  --source-local bind:/path/to/logs
```

This configuration:
- Mounts `/host/path/to/agi-output` to `/opt/agi/files` (read-write) for AGI output
- Mounts `/path/to/logs` to `/opt/agi/files/input` (read-only) for input logs

## Getting Help

- Use `aerolab agi --help` for command overview
- Use `aerolab agi <command> --help` for detailed command help
- See the [Troubleshooting Guide](troubleshooting.md) for common issues

