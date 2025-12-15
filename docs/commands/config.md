# Configuration Commands

Configuration commands allow you to configure Aerolab backends, defaults, and backend-specific settings.

## Commands Overview

- `config backend` - Configure backend (Docker, AWS, GCP)
- `config defaults` - Set default configuration values
- `config env-vars` - Show environment variables
- `config migrate` - Migrate configuration from v7 to v8
- `config docker` - Docker-specific configuration
- `config aws` - AWS-specific configuration
- `config gcp` - GCP-specific configuration

## Config Backend

Configure the backend to use (Docker, AWS, or GCP).

### Basic Usage

```bash
aerolab config backend -t docker
```

### Options

| Option | Description |
|--------|-------------|
| `-t, --type` | Backend type: `docker`, `aws`, or `gcp` |
| `-r, --region` | Regions (comma-separated for multiple) |
| `-c, --inventory-cache` | Enable inventory cache |
| `-p, --key-path` | Custom SSH key path |
| `--check-access` | Check access to backend |

### Docker Backend

```bash
aerolab config backend -t docker
```

**Docker Options:**
- `-a, --docker-arch` - Force architecture (`amd64` or `arm64`)
- `-d, --temp-dir` - Custom temporary directory (useful for WSL2)

**Examples:**
```bash
# Basic Docker configuration
aerolab config backend -t docker

# With inventory cache
aerolab config backend -t docker --inventory-cache

# Force architecture
aerolab config backend -t docker --docker-arch amd64
```

### AWS Backend

```bash
aerolab config backend -t aws -r us-east-1
```

**AWS Options:**
- `-P, --aws-profile` - AWS profile name
- `--aws-nopublic-ip` - Don't request public IPs

**Examples:**
```bash
# Basic AWS configuration
aerolab config backend -t aws -r us-east-1

# With profile
aerolab config backend -t aws -r us-east-1 -P myprofile

# With inventory cache
aerolab config backend -t aws -r us-east-1 --inventory-cache

# Multiple regions
aerolab config backend -t aws -r us-east-1,us-west-2

# With EKS cluster name
aerolab config backend -t aws -r us-east-1 -P eks

# No public IPs
aerolab config backend -t aws -r us-east-1 --aws-nopublic-ip
```

### GCP Backend

**Prerequisites:** Before configuring the GCP backend, authenticate using Application Default Credentials:

```bash
gcloud auth application-default login
```

```bash
aerolab config backend -t gcp -r us-central1 -o project-id
```

**GCP Options:**
- `-o, --project` - GCP project ID (required)

**Examples:**
```bash
# Authenticate first (required)
gcloud auth application-default login

# Basic GCP configuration
aerolab config backend -t gcp -r us-central1 -o my-project-id

# With inventory cache
aerolab config backend -t gcp -r us-central1 -o my-project-id --inventory-cache

# Multiple regions
aerolab config backend -t gcp -r us-central1,us-east1 -o my-project-id
```

**Note:** Aerolab uses Application Default Credentials for authentication. Ensure you've run `gcloud auth application-default login` before configuring the backend. If you encounter authentication errors, see the [troubleshooting section](getting-started/gcp.md#authentication-issues) in the GCP getting started guide.

### Verify Configuration

```bash
aerolab config backend
```

Shows current backend configuration.

### Check Access

```bash
aerolab config backend -t aws --check-access
```

Verifies you have access to the configured backend.

## Config Defaults

Set default configuration values that apply to all commands.

### Basic Usage

```bash
aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features
```

### Options

| Option | Description |
|--------|-------------|
| `-k, --key` | Configuration key (supports wildcards) |
| `-v, --value` | Configuration value |

### Examples

**Set features file path:**
```bash
aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features
```

**Get default value:**
```bash
aerolab config defaults -k '*.FeaturesFilePath'
```

**List all defaults:**
```bash
aerolab config defaults
```

## Config Env-Vars

Show environment variables that affect Aerolab.

### Basic Usage

```bash
aerolab config env-vars
```

Lists all environment variables and their current values.

## Config Migrate

Migrate AeroLab configuration from v7 to v8. This command copies configuration files from the old v7 directory to the new v8 directory structure.

### Basic Usage

```bash
aerolab config migrate
```

### Options

| Option | Description |
|--------|-------------|
| `-o, --old-dir` | Old AeroLab directory to migrate from (default: `~/.aerolab`) |
| `-n, --new-dir` | New AeroLab directory to migrate to (default: `~/.config/aerolab`) |
| `-i, --migrate-inventory` | Also migrate cloud resource inventory tags |
| `-f, --force` | Skip confirmation prompts |

### What Gets Migrated

**Configuration files:**
- `conf` - Main configuration file
- `conf.ts` - Timestamp configuration file

**Automatic fixes:**
- Docker backend region is cleared if it was incorrectly set (Docker doesn't use regions)

**Optional inventory migration:**
- When `-i` flag is used, calls `inventory migrate` for the current backend
- This updates cloud resource tags to v8 format

### Examples

**Basic migration with default paths:**
```bash
aerolab config migrate
```

**Migration with custom paths:**
```bash
aerolab config migrate -o /path/to/old/.aerolab -n /path/to/new/.config/aerolab
```

**Migration without prompts:**
```bash
aerolab config migrate -f
```

**Migration with inventory:**
```bash
aerolab config migrate -i
```

**Complete migration (config + inventory, no prompts):**
```bash
aerolab config migrate -f -i
```

### Directory Structure

| Version | Default Path |
|---------|--------------|
| AeroLab 7.x | `~/.aerolab` |
| AeroLab 8.x | `~/.config/aerolab` |

The v8 directory can be overridden using the `AEROLAB_HOME` environment variable.

### Interactive Mode

When running interactively (without `-f` flag), the command will prompt:

```
Do you want to migrate the inventory to the new AeroLab directory? (y/n):
```

**Note:** Inventory migration is only available for AWS and GCP backends. Docker inventory cannot be migrated (and doesn't need to be).

### Safety Features

- **Idempotent:** Safe to run multiple times
- **Non-destructive:** Original files in old directory are preserved
- **Validation:** Checks backend type before inventory migration

### Common Workflows

**Single backend (AWS or GCP):**
```bash
# Migrate config and inventory together
aerolab config migrate -i
```

**Multiple backends:**
```bash
# 1. Migrate config only
aerolab config migrate -f

# 2. Migrate AWS inventory
aerolab config backend -t aws -r us-east-1
aerolab inventory migrate

# 3. Migrate GCP inventory
aerolab config backend -t gcp -r us-central1 -o my-project
aerolab inventory migrate
```

**Docker users:**
```bash
# Docker only needs config migration
aerolab config migrate -f
```

## Config Docker

Docker-specific configuration commands.

### List Networks

List Docker networks:

```bash
aerolab config docker list-networks
```

### Prune Networks

Clean up unused Docker networks:

```bash
aerolab config docker prune-networks
```

## Config AWS

AWS-specific configuration commands.

### List Subnets

List available subnets:

```bash
aerolab config aws list-subnets
```

### List Security Groups

List security groups:

```bash
aerolab config aws list-security-groups
```

### Create Security Groups

Create a security group:

```bash
aerolab config aws create-security-groups -n my-sg -p 3000-3005
```

**Options:**
- `-n, --name` - Security group name
- `-p, --ports` - Ports to allow (comma-separated or ranges)

**Examples:**
```bash
# Single port
aerolab config aws create-security-groups -n my-sg -p 3000

# Multiple ports
aerolab config aws create-security-groups -n my-sg -p 3000-3005

# Port range
aerolab config aws create-security-groups -n my-sg -p 3000-3010
```

### Lock Security Groups

Lock a security group to prevent deletion:

```bash
aerolab config aws lock-security-groups -n my-sg
```

### Delete Security Groups

Delete a security group:

```bash
aerolab config aws delete-security-groups -n my-sg
```

### Expiry Management

Install, configure, and manage automated resource expiry.

#### Install Expiry

Install expiry automation (Lambda function):

```bash
aerolab config aws expiry-install
```

#### List Expiry Rules

List current expiry rules:

```bash
aerolab config aws expiry-list
```

#### Set Expiry Frequency

Set how often expiry runs:

```bash
aerolab config aws expiry-run-frequency -f 20
```

This sets expiry to run every 20 minutes.

#### Remove Expiry

Remove expiry automation:

```bash
aerolab config aws expiry-remove
```

## Config GCP

GCP-specific configuration commands.

### List Firewall Rules

List firewall rules:

```bash
aerolab config gcp list-firewall-rules
```

### Create Firewall Rules

Create a firewall rule:

```bash
aerolab config gcp create-firewall-rules -n my-fw -p 3000-3005
```

**Options:**
- `-n, --name` - Firewall rule name
- `-p, --ports` - Ports to allow

**Examples:**
```bash
# Single port
aerolab config gcp create-firewall-rules -n my-fw -p 3000

# Multiple ports
aerolab config gcp create-firewall-rules -n my-fw -p 3000-3005
```

### Lock Firewall Rules

Lock a firewall rule to prevent deletion:

```bash
aerolab config gcp lock-firewall-rules -n my-fw
```

### Delete Firewall Rules

Delete a firewall rule:

```bash
aerolab config gcp delete-firewall-rules -n my-fw
```

### Expiry Management

Similar to AWS expiry management.

#### Install Expiry

```bash
aerolab config gcp expiry-install
```

#### List Expiry Rules

```bash
aerolab config gcp expiry-list
```

#### Set Expiry Frequency

```bash
aerolab config gcp expiry-run-frequency -f 20
```

#### Remove Expiry

```bash
aerolab config gcp expiry-remove
```

## Common Workflows

### Migrating from v7 to v8

```bash
# Option 1: Migrate everything at once (single backend)
aerolab config migrate -i

# Option 2: Migrate config first, then inventory per-backend
aerolab config migrate -f
aerolab config backend -t aws -r us-east-1
aerolab inventory migrate
aerolab config backend -t gcp -r us-central1 -o my-project
aerolab inventory migrate

# Option 3: Docker users (no inventory migration needed)
aerolab config migrate -f
```

### Initial Setup

```bash
# 1. Configure backend
aerolab config backend -t docker

# 2. Set defaults
aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features

# 3. Verify configuration
aerolab config backend
```

### AWS Setup with Expiry

```bash
# 1. Configure AWS backend
aerolab config backend -t aws -r us-east-1

# 2. Create security group
aerolab config aws create-security-groups -n aerolab-sg -p 3000-3005

# 3. Install expiry
aerolab config aws expiry-install

# 4. Configure expiry frequency
aerolab config aws expiry-run-frequency -f 20
```

### GCP Setup with Expiry

```bash
# 1. Configure GCP backend
aerolab config backend -t gcp -r us-central1 -o my-project-id

# 2. Create firewall rule
aerolab config gcp create-firewall-rules -n aerolab-fw -p 3000-3005

# 3. Install expiry
aerolab config gcp expiry-install

# 4. Configure expiry frequency
aerolab config gcp expiry-run-frequency -f 20
```

## Tips

1. **Inventory Cache**: Only enable if you're the sole user of the backend account/project
2. **Multiple Regions**: Configure multiple regions for better resource management
3. **Expiry Automation**: Use expiry automation to automatically clean up resources
4. **Security Groups/Firewalls**: Create these before creating clusters for easier management
5. **Defaults**: Set defaults for commonly used values to save time
6. **Migration**: When upgrading from v7, use `config migrate` first, then `inventory migrate` for each backend

