# AGI Configuration Reference

This document provides detailed configuration options for all AGI commands.

## Table of Contents

- [agi create](#agi-create)
- [agi list](#agi-list)
- [agi start](#agi-start)
- [agi stop](#agi-stop)
- [agi destroy](#agi-destroy)
- [agi delete](#agi-delete)
- [agi status](#agi-status)
- [agi details](#agi-details)
- [agi attach](#agi-attach)
- [agi add-auth-token](#agi-add-auth-token)
- [agi change-label](#agi-change-label)
- [agi run-ingest](#agi-run-ingest)
- [agi share](#agi-share)
- [agi template](#agi-template)
- [agi monitor](#agi-monitor)

---

## agi create

Create a new AGI instance for log analysis.

### Basic Usage

```bash
aerolab agi create [options]
```

### Instance Naming

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name (use `~auto~` for auto-generated) | `agi` |
| `--agi-label` | Friendly display label | |

### Source Options

At least one source must be specified.

#### Local Source

| Option | Description |
|--------|-------------|
| `--source-local` | Path to local directory containing logs |
| `--bind-files-dir` | Docker only: Bind mount host directory to `/opt/agi/files` (rw) for AGI output |

**Docker Bind Mount (Docker backend only):**

Use the `bind:` prefix to mount the log directory instead of copying:

```bash
aerolab agi create --source-local bind:/path/to/logs
```

This bind-mounts the host directory as read-only at `/opt/agi/files/input` inside the container. Benefits:
- **No copy overhead** - logs are accessed directly
- **Instant startup** - no upload time regardless of size
- **Disk efficient** - no duplication of log data
- **Read-only** - source logs are protected

**Persistent Output with Bind Mount:**

Use `--bind-files-dir` to persist AGI output files on your host:

```bash
aerolab agi create \
  --bind-files-dir /host/path/to/agi-output \
  --source-local bind:/path/to/logs
```

This creates:
- `/host/path/to/agi-output` → `/opt/agi/files` (read-write)
- `/path/to/logs` → `/opt/agi/files/input` (read-only)

#### SFTP Source

| Option | Description | Default |
|--------|-------------|---------|
| `--source-sftp-enable` | Enable SFTP source | |
| `--source-sftp-host` | SFTP hostname | |
| `--source-sftp-port` | SFTP port | `22` |
| `--source-sftp-user` | SFTP username | |
| `--source-sftp-pass` | SFTP password (supports `ENV::VAR_NAME`) | |
| `--source-sftp-key` | Path to SSH private key | |
| `--source-sftp-path` | Remote path to download from | |
| `--source-sftp-regex` | Regex pattern to filter files | |
| `--source-sftp-threads` | Concurrent download threads | `1` |
| `--source-sftp-skipcheck` | Skip SFTP accessibility check | |

#### S3 Source

| Option | Description | Default |
|--------|-------------|---------|
| `--source-s3-enable` | Enable S3 source | |
| `--source-s3-bucket` | S3 bucket name | |
| `--source-s3-region` | AWS region | |
| `--source-s3-path` | Path prefix in bucket | |
| `--source-s3-key-id` | AWS access key ID (supports `ENV::VAR_NAME`) | |
| `--source-s3-secret-key` | AWS secret key (supports `ENV::VAR_NAME`) | |
| `--source-s3-regex` | Regex pattern to filter files | |
| `--source-s3-threads` | Concurrent download threads | `4` |
| `--source-s3-endpoint` | Custom S3 endpoint URL | |
| `--source-s3-skipcheck` | Skip S3 accessibility check | |

#### Cluster Source

| Option | Description |
|--------|-------------|
| `--source-cluster` | Aerospike cluster name to collect logs from |

### Memory and Storage

| Option | Description | Default |
|--------|-------------|---------|
| `--no-dim` | Disable data-in-memory (less RAM, slower queries) | |
| `--no-dim-filesize` | File size in GB when using `--no-dim` | auto |

### SSL/TLS Options

| Option | Description | Default |
|--------|-------------|---------|
| `--proxy-ssl-disable` | Disable TLS on proxy | |
| `--proxy-ssl-cert` | Custom SSL certificate file | self-signed |
| `--proxy-ssl-key` | Custom SSL private key file | self-signed |

### Proxy Timeouts

| Option | Description | Default |
|--------|-------------|---------|
| `--proxy-max-inactive` | Max inactivity before shutdown | `1h` |
| `--proxy-max-uptime` | Max uptime before shutdown | `24h` |

### Time Range Filtering

| Option | Description |
|--------|-------------|
| `--ingest-timeranges-enable` | Enable time range filtering |
| `--ingest-timeranges-from` | Start time (RFC3339: `2006-01-02T15:04:05Z07:00`) |
| `--ingest-timeranges-to` | End time (RFC3339: `2006-01-02T15:04:05Z07:00`) |

### Ingest Options

| Option | Description | Default |
|--------|-------------|---------|
| `--ingest-custom-source-name` | Custom source name for Grafana | |
| `--ingest-patterns-file` | Custom patterns YAML file | |
| `--ingest-log-level` | Log level (1=CRITICAL...6=DETAIL) | `4` |
| `--ingest-cpu-profiling` | Enable CPU profiling | |

### Plugin Options

| Option | Description | Default |
|--------|-------------|---------|
| `--plugin-log-level` | Plugin log level | `4` |
| `--plugin-cpu-profiling` | Enable CPU profiling | |

### Notifications

| Option | Description |
|--------|-------------|
| `--notify-slack-token` | Slack token (supports `ENV::VAR_NAME`) |
| `--notify-slack-channel` | Slack channel |

### Monitor Integration

| Option | Description |
|--------|-------------|
| `--monitor-url` | AGI Monitor URL for auto-scaling |
| `--monitor-ignore-cert` | Ignore invalid monitor SSL certificate |

### Version Options

| Option | Description | Default |
|--------|-------------|---------|
| `-v, --aerospike-version` | Aerospike server version | `latest` |
| `--grafana-version` | Grafana version | `11.2.6` |
| `-d, --distro` | Linux distribution | `ubuntu` |
| `--distro-version` | Distribution version | `latest` |

**Note:** Architecture is automatically detected based on the backend:
- **Docker**: Uses the host system's architecture
- **AWS/GCP**: Inferred from the selected instance type

### Other Options

| Option | Description | Default |
|--------|-------------|---------|
| `-t, --timeout` | Creation timeout in minutes | `30` |
| `--no-vacuum` | Don't cleanup on failure | |
| `--no-config-override` | Don't override config on restart with volume | |
| `--owner` | Owner tag value | |
| `--aerolab-binary` | Path to custom aerolab binary (for unofficial builds) | |

**Note on Unofficial Builds:**

When running an unofficial aerolab build and no existing AGI template is found:
- If on Linux with matching architecture, aerolab automatically uses itself
- Otherwise, `--aerolab-binary` must be specified with a Linux binary for the target architecture

### AWS-Specific Options

| Option | Description | Default |
|--------|-------------|---------|
| `--aws-instance-type, -I` | Instance type (min 12GB RAM) | auto-select |
| `--aws-instance-arch-arm` | Prefer ARM instance types | |
| `--aws-ebs, -E` | EBS volume size in GB | `40` |
| `--aws-secgroup-id, -S` | Security group IDs (comma-separated) | |
| `--aws-subnet-id, -U` | Subnet ID or availability zone | |
| `--aws-tags` | Custom tags (key=value) | |
| `--aws-with-efs` | Use EFS for persistent storage | |
| `--aws-efs-name` | EFS volume name | `{AGI_NAME}` |
| `--aws-efs-path` | EFS mount path | `/` |
| `--aws-efs-multizone` | Enable multi-AZ EFS | |
| `--aws-efs-expire` | EFS expiry after last use | `96h` |
| `--aws-terminate-on-poweroff` | Terminate on poweroff | |
| `--aws-spot-instance` | Request spot instance | |
| `--aws-spot-fallback` | Fall back to on-demand | |
| `--aws-expire` | Instance expiry time | `30h` |
| `--aws-route53-zoneid` | Route53 zone ID | |
| `--aws-route53-domain` | Route53 domain name | |
| `--aws-disable-public-ip` | Disable public IP | |

### GCP-Specific Options

| Option | Description | Default |
|--------|-------------|---------|
| `--gcp-instance` | Instance type | `c2d-highmem-4` |
| `--gcp-disk` | Disk configuration (type:sizeGB) | `pd-ssd:40` |
| `--gcp-zone` | GCP zone | |
| `--gcp-tag` | Network tags | |
| `--gcp-label` | Labels (key=value) | |
| `--gcp-spot-instance` | Request spot instance | |
| `--gcp-expire` | Instance expiry time | `30h` |
| `--gcp-with-vol` | Use persistent volume | |
| `--gcp-vol-name` | Volume name | `{AGI_NAME}` |
| `--gcp-vol-expire` | Volume expiry | `96h` |
| `--gcp-terminate-on-poweroff` | Terminate on poweroff | |

### Docker-Specific Options

| Option | Description |
|--------|-------------|
| `--docker-expose-ports, -e` | Port forwarding (HOST_PORT:CONTAINER_PORT); auto-allocated if not specified |
| `--docker-cpu-limit, -l` | CPU limit (e.g., 1, 0.5) |
| `--docker-ram-limit, -r` | RAM limit (e.g., 8g) |
| `--docker-swap-limit, -w` | Total memory limit (RAM+swap) |
| `--docker-privileged, -B` | Run in privileged mode |
| `--docker-network` | Docker network name |
| `--bind-files-dir` | Bind mount host directory to `/opt/agi/files` for persistent output |

**Port Allocation:**

When `--docker-expose-ports` is not specified, AGI automatically allocates ports:
- HTTPS (default): Starting at 9443, incrementing if in use (9444, 9445, ...)
- HTTP (with `--proxy-ssl-disable`): Starting at 9080, incrementing if in use

This allows running multiple AGI instances simultaneously on Docker.

### Examples

**Basic local source:**
```bash
aerolab agi create --source-local /var/log/aerospike
```

**S3 source with custom name:**
```bash
aerolab agi create -n prod-logs \
  --agi-label "Production Logs Analysis" \
  --source-s3-enable \
  --source-s3-bucket my-logs \
  --source-s3-path prod/ \
  --source-s3-region us-east-1 \
  --aws-expire=24h
```

**SFTP source with time filtering:**
```bash
aerolab agi create -n filtered-logs \
  --source-sftp-enable \
  --source-sftp-host sftp.example.com \
  --source-sftp-user myuser \
  --source-sftp-key ~/.ssh/id_rsa \
  --source-sftp-path /logs \
  --ingest-timeranges-enable \
  --ingest-timeranges-from "2024-01-01T00:00:00Z" \
  --ingest-timeranges-to "2024-01-31T23:59:59Z"
```

**Cluster source with EFS persistence:**
```bash
aerolab agi create -n cluster-analysis \
  --source-cluster mydc \
  --aws-with-efs \
  --aws-efs-expire=168h \
  --aws-expire=24h
```

---

## agi list

List AGI instances.

### Basic Usage

```bash
aerolab agi list [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Filter by name | all |
| `-o, --output` | Output format (table, json, csv, tsv) | `table` |
| `--owner` | Filter by owner | |
| `--pager` | Use pager for output | |

### Output Fields

| Field | Description |
|-------|-------------|
| Name | AGI instance name |
| Label | Friendly display label |
| State | Instance state (running/stopped) |
| AccessURL | URL to access the AGI web interface |
| Expires | Expiry time (cloud backends) |

**JSON Output:**

The JSON output (`-o json`) includes a `details` field with comprehensive instance information, similar to `cluster list` output.

### Examples

```bash
# List all AGI instances
aerolab agi list

# JSON output with full details
aerolab agi list -o json

# Filter by name
aerolab agi list -n myagi
```

---

## agi start

Start an AGI instance.

### Basic Usage

```bash
aerolab agi start [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name | `agi` |
| `--no-wait` | Don't wait for startup | |
| `--wait-timeout` | Wait timeout | `5m` |
| `--dry-run` | Show actions without executing | |

### Examples

```bash
# Start default AGI
aerolab agi start

# Start specific AGI
aerolab agi start -n myagi

# Start without waiting
aerolab agi start -n myagi --no-wait
```

---

## agi stop

Stop an AGI instance.

### Basic Usage

```bash
aerolab agi stop [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name | `agi` |
| `--force` | Force stop without graceful shutdown | |
| `--no-wait` | Don't wait for stop | |
| `--wait-timeout` | Wait timeout | `5m` |
| `--dry-run` | Show actions without executing | |

### Examples

```bash
# Stop gracefully
aerolab agi stop -n myagi

# Force stop
aerolab agi stop -n myagi --force
```

---

## agi destroy

Destroy an AGI instance (preserves volume if using EFS/GCP vol).

### Basic Usage

```bash
aerolab agi destroy [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name(s) (comma-separated) | `agi` |
| `--force` | Skip confirmation | |
| `--no-wait` | Don't wait for destruction | |
| `--dry-run` | Show actions without executing | |

### Examples

```bash
# Destroy with confirmation
aerolab agi destroy -n myagi

# Force destroy
aerolab agi destroy -n myagi --force

# Destroy multiple
aerolab agi destroy -n agi1,agi2,agi3 --force
```

---

## agi delete

Destroy an AGI instance AND delete associated volume.

### Basic Usage

```bash
aerolab agi delete [options]
```

### Options

Same as `agi destroy`, but also deletes volumes.

### Examples

```bash
# Delete instance and volume
aerolab agi delete -n myagi --force
```

---

## agi status

Show AGI service status.

### Basic Usage

```bash
aerolab agi status [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name | `agi` |
| `-o, --output` | Output format (table, json, text) | `table` |
| `--pager` | Use pager for output | |

### Output Includes

- Service status (aerospike, grafana, plugin, ingest, proxy, etc.)
- System resources (memory, disk)
- Ingest step status

### Examples

```bash
aerolab agi status -n myagi
aerolab agi status -n myagi -o json
```

---

## agi details

Show detailed ingest progress.

### Basic Usage

```bash
aerolab agi details [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name | `agi` |
| `-o, --output` | Output format (table, json) | `table` |
| `--watch` | Watch mode for real-time updates | |
| `--watch-interval` | Watch refresh interval | `5s` |

### Output Includes

- Ingest steps progress (init, download, unpack, preprocess, process)
- File counts per stage
- Download/processing times
- Error details

### Examples

```bash
# View details
aerolab agi details -n myagi

# Watch progress
aerolab agi details -n myagi --watch
```

---

## agi attach

Attach to AGI shell.

### Basic Usage

```bash
aerolab agi attach [options] [-- command]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name | `agi` |
| `--detach` | Run command in background | |

### Examples

```bash
# Interactive shell
aerolab agi attach -n myagi

# Run command
aerolab agi attach -n myagi -- ls /opt/agi

# Check logs
aerolab agi attach -n myagi -- tail -f /var/log/agi-ingest.log
```

---

## agi add-auth-token

Manage authentication tokens.

### Basic Usage

```bash
aerolab agi add-auth-token [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name | `agi` |
| `--token` | Token value (or read from stdin) | auto-generated |
| `--token-name` | Token file name | timestamp |
| `--size` | Generated token size | `64` |
| `--list` | List all tokens | |
| `--remove` | Remove a token | |
| `--url` | Generate access URL with token | |

### Examples

```bash
# Generate and add token, show URL
aerolab agi add-auth-token -n myagi --url

# Add custom token
aerolab agi add-auth-token -n myagi --token "your-secure-token"

# List tokens
aerolab agi add-auth-token -n myagi --list

# Remove token
aerolab agi add-auth-token -n myagi --remove token-name
```

---

## agi change-label

Change the friendly label of an AGI instance.

### Basic Usage

```bash
aerolab agi change-label [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | AGI instance name | `agi` |
| `--label` | New label | required |

### Examples

```bash
aerolab agi change-label -n myagi --label "Production Log Analysis"
```

---

## agi run-ingest

Retrigger log ingestion with new configuration.

### Basic Usage

```bash
aerolab agi run-ingest [options]
```

### Options

Accepts most source options from `agi create`:
- `--source-local`
- `--source-sftp-*`
- `--source-s3-*`
- `--source-cluster`
- `--ingest-timeranges-*`
- `--ingest-patterns-file`
- `--force` - Skip confirmation

### Examples

```bash
# Re-run with new local source
aerolab agi run-ingest -n myagi --source-local /path/to/new/logs

# Update time range
aerolab agi run-ingest -n myagi \
  --ingest-timeranges-enable \
  --ingest-timeranges-from "2024-02-01T00:00:00Z" \
  --ingest-timeranges-to "2024-02-29T23:59:59Z"
```

---

## agi share

Share AGI access via SSH public key.

### Basic Usage

```bash
aerolab agi share [options]
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | AGI instance name |
| `--pubkey` | Path to public key file |
| `--key` | Public key content |
| `--list` | List authorized keys |
| `--remove` | Remove a key |

### Examples

```bash
# Share access
aerolab agi share -n myagi --pubkey ~/.ssh/id_rsa.pub

# List keys
aerolab agi share -n myagi --list
```

---

## agi template

Manage AGI templates.

### Subcommands

#### agi template create

Create an AGI template.

```bash
aerolab agi template create [options]
```

| Option | Description | Default |
|--------|-------------|---------|
| `-v, --aerospike-version` | Aerospike version | `latest` |
| `--grafana-version` | Grafana version | `11.2.6` |
| `-d, --distro` | Linux distribution | `ubuntu` |
| `--distro-version` | Distribution version | `latest` |
| `-a, --arch` | Architecture (amd64/arm64) | `amd64` |
| `-t, --timeout` | Timeout in minutes | `20` |
| `--no-vacuum` | Don't cleanup on failure | |
| `--dry-run` | Validate only | |
| `--owner` | Owner tag | |
| `-b, --aerolab-binary` | Path to custom aerolab binary (required for unofficial builds) | |

**Note on Unofficial Builds:**

When running an unofficial aerolab build:
- If on Linux with matching architecture, aerolab uses itself automatically
- Otherwise, `--aerolab-binary` must be specified with a Linux binary for the target architecture

#### agi template list

List AGI templates.

```bash
aerolab agi template list [options]
```

| Option | Description | Default |
|--------|-------------|---------|
| `-o, --output` | Output format | `table` |

#### agi template destroy

Destroy an AGI template.

```bash
aerolab agi template destroy [options]
```

| Option | Description |
|--------|-------------|
| `--name` | Template name/ID |
| `--force` | Skip confirmation |

#### agi template vacuum

Clean up unused templates.

```bash
aerolab agi template vacuum [options]
```

---

## agi monitor

Monitor system for auto-scaling (AWS/GCP only).

### Monitor Architecture

**Important:** Each AGI monitor instance operates within a single AWS region or GCP project.

If you have AGI instances across multiple regions or projects, you need to deploy a separate monitor for each:

```bash
# Deploy monitor for us-east-1
aerolab config backend -t aws -r us-east-1
aerolab agi monitor create --name agimonitor-east

# Deploy monitor for eu-west-1
aerolab config backend -t aws -r eu-west-1
aerolab agi monitor create --name agimonitor-west
```

Each monitor will:
- Only manage AGI instances in its region/project
- Handle auto-scaling and spot instance rotation for local instances
- Send notifications for events in its scope

### agi monitor create

Deploy an AGI monitor instance.

```bash
aerolab agi monitor create [options]
```

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Monitor instance name | `agimonitor` |
| `--owner` | Owner tag | |

**AWS Options:**
| Option | Description |
|--------|-------------|
| `--aws-instance` | Instance type |
| `--aws-secgroup-id` | Security group ID |
| `--aws-secgroup-name` | Security group name |
| `--aws-subnet-id` | Subnet ID |
| `--aws-role` | IAM role name |
| `--aws-route53-zoneid` | Route53 zone ID |
| `--aws-route53-fqdn` | Route53 FQDN |

**GCP Options:**
| Option | Description |
|--------|-------------|
| `--gcp-instance` | Instance type |
| `--gcp-zone` | Zone |
| `--gcp-firewall` | Firewall name |
| `--gcp-role` | Service account role |

### agi monitor listen

Start the monitor listener (usually run via systemd).

```bash
aerolab agi monitor listen [options]
```

| Option | Description | Default |
|--------|-------------|---------|
| `--address` | Listen address | `:443` |
| `--no-tls` | Disable TLS | |
| `--autocert` | Enable Let's Encrypt | |
| `--autocert-email` | Let's Encrypt email | |
| `--cert-file` | TLS certificate file | |
| `--key-file` | TLS key file | |
| `--gcp-disk-thres-pct` | Disk threshold % | `80` |
| `--gcp-disk-grow-gb` | Disk growth GB | `20` |
| `--ram-thres-used-pct` | RAM used threshold % | `90` |
| `--ram-thres-minfree-gb` | Min free RAM GB | `2` |
| `--sizing-disable` | Disable auto-sizing | |
| `--sizing-max-ram-gb` | Max RAM GB | |
| `--sizing-max-disk-gb` | Max disk GB | |
| `--capacity-disable` | Disable spot rotation | |
| `--notify-url` | Notification webhook URL | |
| `--notify-slack-token` | Slack token | |
| `--notify-slack-channel` | Slack channel | |

---

## Environment Variables

AGI supports reading sensitive values from environment variables using the `ENV::` prefix:

```bash
# Set environment variable
export MY_PASSWORD="secret123"

# Use in command
aerolab agi create --source-sftp-pass ENV::MY_PASSWORD ...
```

Supported fields:
- `--source-sftp-pass`
- `--source-s3-key-id`
- `--source-s3-secret-key`
- `--notify-slack-token`

