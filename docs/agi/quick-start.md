# AGI Quick Start Guide

This guide will help you get started with AGI (Aerospike Grafana Integration) for log analysis and visualization.

## Prerequisites

- **Aerolab** - Install Aerolab from the [releases page](https://github.com/aerospike/aerolab/releases)
- **Backend configured** - Docker, AWS, or GCP backend must be configured

## Choose Your Backend

### Docker (Fastest Start)

Best for quick local analysis and testing.

```bash
# Configure Docker backend
aerolab config backend -t docker
```

### AWS

Best for larger log sets and production use.

```bash
# Configure AWS backend
aerolab config backend -t aws -r us-east-1
```

### GCP

Best for Google Cloud users.

```bash
# Authenticate first
gcloud auth application-default login

# Configure GCP backend
aerolab config backend -t gcp -r us-central1 -o your-project-id
```

---

## Quick Start: Local Logs

The fastest way to analyze logs is using local files.

### Step 1: Create AGI Instance

```bash
aerolab agi create --source-local /path/to/your/logs
```

This will:
1. Create an AGI template (first time only, ~10-15 minutes)
2. Create an AGI instance (~1-2 minutes)
3. Upload your log files
4. Start log ingestion

#### Docker: Using Bind Mount (Recommended for Large Log Sets)

When using Docker, you can bind-mount your log directory instead of copying files. This is faster and saves disk space:

```bash
aerolab agi create --source-local bind:/path/to/your/logs
```

With `bind:` prefix:
- **No file copying** - logs are accessed directly from your host
- **Instant startup** - no upload delay regardless of log size
- **Live updates** - new logs appear immediately (re-run ingest to process)
- **Read-only mount** - your source logs are protected from modification

This is ideal for:
- Very large log directories
- Iterative analysis during debugging
- Shared log storage

#### Docker: Persistent Output with Bind Mount

To persist AGI output files (processed logs, collect info, etc.) on your host:

```bash
aerolab agi create \
  --bind-files-dir /host/path/to/agi-output \
  --source-local bind:/path/to/your/logs
```

This creates two bind mounts:
- `/host/path/to/agi-output` → `/opt/agi/files` (read-write, for AGI output)
- `/path/to/your/logs` → `/opt/agi/files/input` (read-only, for input logs)

### Step 2: Access AGI

The access URL is displayed after creation. For Docker, ports are dynamically allocated starting at 9443 (HTTPS) or 9080 (HTTP):
```
Access URL: https://127.0.0.1:9443
```

If you have multiple AGI instances, each gets the next available port (9444, 9445, etc.).

For cloud backends, it will show the instance IP:
```
Access URL: https://1.2.3.4
```

### Step 3: Check Ingest Status

```bash
# Quick status
aerolab agi status -n agi

# Detailed progress
aerolab agi details -n agi
```

### Step 4: View Logs in Grafana

1. Open the access URL in your browser
2. Accept the self-signed certificate warning (if HTTPS)
3. Navigate to the dashboards to explore your logs

---

## Quick Start: S3 Logs

For logs stored in Amazon S3:

```bash
aerolab agi create -n s3-logs \
  --source-s3-enable \
  --source-s3-bucket my-log-bucket \
  --source-s3-path logs/2024/ \
  --source-s3-region us-east-1 \
  --source-s3-key-id ENV::AWS_ACCESS_KEY_ID \
  --source-s3-secret-key ENV::AWS_SECRET_ACCESS_KEY \
  --aws-expire=8h
```

**Note:** Use `ENV::VAR_NAME` syntax to read credentials from environment variables securely.

---

## Quick Start: SFTP Logs

For logs on an SFTP server:

```bash
# Set password in environment
export SFTP_PASSWORD="your-password"

# Create AGI with SFTP source
aerolab agi create -n sftp-logs \
  --source-sftp-enable \
  --source-sftp-host sftp.example.com \
  --source-sftp-port 22 \
  --source-sftp-user myuser \
  --source-sftp-pass ENV::SFTP_PASSWORD \
  --source-sftp-path /var/log/aerospike \
  --aws-expire=8h
```

With SSH key authentication:
```bash
aerolab agi create -n sftp-logs \
  --source-sftp-enable \
  --source-sftp-host sftp.example.com \
  --source-sftp-user myuser \
  --source-sftp-key ~/.ssh/id_rsa \
  --source-sftp-path /var/log/aerospike \
  --aws-expire=8h
```

---

## Quick Start: Cluster Logs

Collect logs directly from a running Aerospike cluster:

```bash
# First, create a cluster
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*' -n mydc

# Run some operations...

# Then create AGI from cluster logs
aerolab agi create -n cluster-analysis --source-cluster mydc
```

---

## Common Operations

### List AGI Instances and Volumes

```bash
aerolab agi list
```

Output includes two sections:

**AGI Instances:**
- Name and label
- State (running/stopped)
- Access URL
- Expiry time

**AGI Volumes** (AWS/GCP only):
- Name and label
- Volume type and size
- State and attached instance
- Expiry time

This makes it easy to see both running instances and available volumes that can be started.

### Check Status

```bash
# Service status
aerolab agi status -n agi

# Ingest progress details
aerolab agi details -n agi

# Watch progress in real-time
aerolab agi details -n agi --watch
```

### Stop and Start

```bash
# Stop AGI (preserves data)
aerolab agi stop -n agi

# Start AGI
aerolab agi start -n agi

# Start AGI from an existing volume (if instance was destroyed but volume preserved)
aerolab agi start -n myagi
```

### Open AGI in Browser

Quickly open AGI in your default browser with an authentication token:

```bash
aerolab agi open -n agi
```

This generates a new authentication token and opens the AGI URL in your browser automatically.

### Attach to Shell

```bash
# Interactive shell
aerolab agi attach -n agi

# Run a command
aerolab agi attach -n agi -- ls /opt/agi/files
```

### Destroy AGI

```bash
# Destroy instance (preserves volume if using EFS/GCP vol)
aerolab agi destroy -n agi --force

# Destroy instance AND delete volume (permanent)
aerolab agi delete -n agi --force
```

---

## Adding Authentication

By default, AGI has no authentication (open access). For production use:

### Token Authentication

```bash
# Generate and add a token
aerolab agi add-auth-token -n agi --url

# Output shows access URL with token
# https://1.2.3.4/?token=abc123...
```

### List/Remove Tokens

```bash
# List all tokens
aerolab agi add-auth-token -n agi --list

# Remove a token
aerolab agi add-auth-token -n agi --remove token-name
```

---

## Persistence with Volumes

For persistent storage that survives instance restarts:

### AWS with EFS

```bash
aerolab agi create -n persistent-agi \
  --source-local /path/to/logs \
  --aws-with-efs \
  --aws-efs-expire=96h \
  --aws-expire=8h
```

### GCP with Persistent Volume

```bash
aerolab agi create -n persistent-agi \
  --source-local /path/to/logs \
  --gcp-with-vol \
  --gcp-vol-expire=96h \
  --gcp-expire=8h
```

### Using Existing Volumes

When you have a persistent volume (EFS or GCP Persistent Disk), you can:

1. **Destroy the instance** to save costs while preserving data:
   ```bash
   aerolab agi destroy -n myagi --force
   ```

2. **Check available volumes**:
   ```bash
   aerolab agi list  # Shows both instances and volumes
   ```

3. **Start a new instance from the existing volume**:
   ```bash
   aerolab agi start -n myagi
   ```

This workflow is cost-effective for long-term log storage - only pay for compute when actively analyzing logs.

---

## Re-running Ingestion

To process new logs or update time ranges:

```bash
# Re-run ingest with new source
aerolab agi run-ingest -n agi --source-local /path/to/new/logs

# Or update time range filter
aerolab agi run-ingest -n agi \
  --ingest-timeranges-enable \
  --ingest-timeranges-from "2024-01-01T00:00:00Z" \
  --ingest-timeranges-to "2024-01-31T23:59:59Z"
```

---

## Template Management

AGI templates are created automatically, but you can manage them manually:

```bash
# List templates
aerolab agi template list

# Create template manually
aerolab agi template create -v '8.*' -d ubuntu

# Clean up unused templates
aerolab agi template vacuum
```

---

## Best Practices

### Memory Considerations

- **With DIM (default)**: Faster queries, requires more RAM
- **Without DIM**: Slower queries, uses less RAM

```bash
# Use less memory (slower)
aerolab agi create --no-dim --source-local /path/to/logs
```

### Log Size Guidelines

| Log Size | Recommended Instance | Backend |
|----------|---------------------|---------|
| < 1 GB   | t3a.medium          | Any     |
| 1-5 GB   | t3a.large           | Any     |
| 5-20 GB  | t3a.xlarge          | Cloud   |
| > 20 GB  | t3a.2xlarge         | Cloud   |

### Security

1. **Always use authentication** in production:
   ```bash
   aerolab agi add-auth-token -n agi
   ```

2. **Use custom SSL certificates** for proper HTTPS:
   ```bash
   aerolab agi create \
     --proxy-ssl-cert /path/to/cert.pem \
     --proxy-ssl-key /path/to/key.pem \
     --source-local /path/to/logs
   ```

3. **Use ENV:: variables** for secrets:
   ```bash
   export S3_SECRET="your-secret"
   aerolab agi create --source-s3-secret-key ENV::S3_SECRET ...
   ```

---

## Next Steps

- See [Configuration Reference](configuration.md) for all available options
- Check [Troubleshooting Guide](troubleshooting.md) if you encounter issues
- Explore Grafana dashboards for log analysis

