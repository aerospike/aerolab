# Configuration Commands

Configuration commands allow you to configure Aerolab backends, defaults, and backend-specific settings.

## Commands Overview

- `config backend` - Configure backend (Docker, AWS, GCP)
- `config defaults` - Set default configuration values
- `config env-vars` - Show environment variables
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

```bash
aerolab config backend -t gcp -r us-central1 -o project-id
```

**GCP Options:**
- `-o, --project` - GCP project ID (required)
- `-m, --gcp-auth-method` - Auth method: `any`, `login`, or `service-account`
- `-b, --gcp-no-browser` - Don't open browser for authentication
- `-i, --gcp-client-id` - Custom OAuth client ID
- `-s, --gcp-client-secret` - Custom OAuth client secret

**Examples:**
```bash
# Basic GCP configuration (will open browser for auth)
aerolab config backend -t gcp -r us-central1 -o my-project-id

# With inventory cache
aerolab config backend -t gcp -r us-central1 -o my-project-id --inventory-cache

# Service account authentication
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
aerolab config backend -t gcp -r us-central1 -o my-project-id \
  --gcp-auth-method service-account

# No browser
aerolab config backend -t gcp -r us-central1 -o my-project-id \
  --gcp-no-browser

# Multiple regions
aerolab config backend -t gcp -r us-central1,us-east1 -o my-project-id
```

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

