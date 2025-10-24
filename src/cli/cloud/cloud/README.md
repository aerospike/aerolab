# Aerospike Cloud CLI

A command-line interface for managing Aerospike Cloud resources. This CLI provides access to all features available in the Aerospike Cloud Public API v2.

## Installation

```bash
go build -o aerospike-cloud-cli
```

## Authentication

The CLI uses OAuth2 client credentials flow for authentication. Set the following environment variables:

```bash
export AEROSPIKE_CLOUD_KEY="your-api-key"
export AEROSPIKE_CLOUD_SECRET="your-api-secret"
```

The CLI will automatically obtain an access token using these credentials when making API calls.

## Usage

### Cloud Provider Operations

#### List Cloud Providers
```bash
# List all cloud providers
./aerospike-cloud-cli cloud-provider list

# List with filters (JSON format)
./aerospike-cloud-cli cloud-provider list --filter '{"cloudProvider":"aws","region":"us-west-2"}'
```

#### Get Instance Type Specifications
```bash
./aerospike-cloud-cli cloud-provider get-specs --cloud-provider aws --instance-type t3.medium
```

### Organization Operations

#### Get Organization Information
```bash
./aerospike-cloud-cli organization get
```

### API Key Operations

#### List API Keys
```bash
./aerospike-cloud-cli api-keys list
```

#### Create API Key
```bash
./aerospike-cloud-cli api-keys create --name "my-dev-key"
```

#### Delete API Key
```bash
./aerospike-cloud-cli api-keys delete --client-id "client-id-here"
```

### Secret Operations

#### List Secrets
```bash
./aerospike-cloud-cli secrets list
```

#### Create Secret
```bash
./aerospike-cloud-cli secrets create --name "my-secret" --description "My secret description" --value "secret-value"
```

#### Delete Secret
```bash
./aerospike-cloud-cli secrets delete --secret-id "secret-id-here"
```

### Database Operations

#### List Databases
```bash
# List all databases
./aerospike-cloud-cli databases list

# List databases excluding specific statuses
./aerospike-cloud-cli databases list --status-ne "deleting,failed"
```

#### Create Database
```bash
# Basic database creation
./aerospike-cloud-cli databases create --name "my-database" --cloud-provider aws --region us-west-2 --instance-type t3.medium --cluster-size 2 --data-storage memory

# Advanced database creation with all parameters
./aerospike-cloud-cli databases create \
  --name "production-database" \
  --data-plane-version "0.0.1" \
  --cloud-provider aws \
  --instance-type i4i.large \
  --region us-east-1 \
  --availability-zone-count 3 \
  --zone-ids "use1-az1,use1-az2,use1-az3" \
  --cidr-block "10.128.0.0/19" \
  --cluster-size 3 \
  --data-storage local-disk \
  --data-resiliency network-storage \
  --cluster-name "prod-cluster" \
  --auto-pin numa \
  --batch-index-threads 4 \
  --enable-health-check \
  --enable-hist-info \
  --info-max-ms 1000 \
  --info-threads 2
```

#### Get Database Details
```bash
./aerospike-cloud-cli databases get --database-id "database-id-here"
```

#### Update Database
```bash
# Basic update - change name only
./aerospike-cloud-cli databases update --database-id "database-id-here" --name "new-database-name"

# Advanced update - change infrastructure and aerospike settings
./aerospike-cloud-cli databases update \
  --database-id "database-id-here" \
  --name "updated-database-name" \
  --cloud-provider aws \
  --instance-type i4i.xlarge \
  --region us-west-2 \
  --availability-zone-count 2 \
  --cluster-size 4 \
  --data-storage network-storage \
  --data-resiliency network-storage \
  --data-plane-version "0.0.2"
```

#### Delete Database
```bash
./aerospike-cloud-cli databases delete --database-id "database-id-here"
```

#### Get Database Metrics
```bash
./aerospike-cloud-cli databases metrics --database-id "database-id-here"
```

### Database Credentials Operations

#### List Database Credentials
```bash
./aerospike-cloud-cli databases credentials list --database-id "database-id-here"
```

#### Create Database Credentials
```bash
./aerospike-cloud-cli databases credentials create --database-id "database-id-here" --username "myuser" --password "mypassword" --privileges "read-write"
```

#### Delete Database Credentials
```bash
./aerospike-cloud-cli databases credentials delete --database-id "database-id-here" --credentials-id "credentials-id-here"
```

### VPC Peering Operations

#### List VPC Peerings
```bash
./aerospike-cloud-cli databases vpc-peering list --database-id "database-id-here"
```

#### Create VPC Peering
```bash
./aerospike-cloud-cli databases vpc-peering create \
  --database-id "database-id-here" \
  --vpc-id "vpc-12345678" \
  --cidr-block "10.0.0.0/16" \
  --account-id "123456789012" \
  --region "us-east-1" \
  --is-secure-connection
```

#### Delete VPC Peering
```bash
./aerospike-cloud-cli databases vpc-peering delete --database-id "database-id-here" --vpc-id "vpc-12345678"
```

### Topology Operations

#### List Topologies
```bash
./aerospike-cloud-cli topologies list
```

#### Create Topology
```bash
./aerospike-cloud-cli topologies create --name "my-topology"
```

#### Get Topology Details
```bash
./aerospike-cloud-cli topologies get --topology-id "topology-id-here"
```

#### Delete Topology
```bash
./aerospike-cloud-cli topologies delete --topology-id "topology-id-here"
```

### Web Interface

#### Start Interactive Web UI
```bash
# Start web UI on default port 8080
./aerospike-cloud-cli webui

# Start web UI on custom port and host
./aerospike-cloud-cli webui --port 8081 --host 0.0.0.0
```

The web UI provides an interactive interface where you can:
- Browse all available commands and parameters
- Fill out forms for complex commands with many parameters
- Execute commands and view results in a user-friendly format
- Access all CLI functionality through a modern web interface

Once started, open your browser to `http://localhost:8080` (or your specified host:port) to access the interface.

## Command Aliases

The CLI supports convenient aliases for common commands:

- `cloud-provider` → `cp`
- `organization` → `org`
- `api-keys` → `keys`
- `secrets` → `secret`
- `databases` → `db`
- `topologies` → `top`
- `credentials` → `creds`
- `vpc-peering` → `vpc`
- `list` → `ls`

## Examples

### Complete Database Management Workflow

```bash
# 1. List available cloud providers and instance types
./aerospike-cloud-cli cp list
./aerospike-cloud-cli cp get-specs --cloud-provider aws --instance-type t3.medium

# 2. Create a new database
./aerospike-cloud-cli db create --name "production-db" --cloud-provider aws --region us-west-2 --instance-type t3.medium

# 3. Get database details
./aerospike-cloud-cli db get --database-id "db-12345"

# 4. Create database credentials
./aerospike-cloud-cli db creds create --database-id "db-12345" --username "app-user" --password "secure-password" --privileges "read-write"

# 5. Set up VPC peering
./aerospike-cloud-cli db vpc create --database-id "db-12345" --vpc-id "vpc-12345678"

# 6. Monitor database metrics
./aerospike-cloud-cli db metrics --database-id "db-12345"

# 7. Update database name
./aerospike-cloud-cli db update --database-id "db-12345" --name "updated-db-name"
```

### API Key Management

```bash
# List existing API keys
./aerospike-cloud-cli keys list

# Create a new API key for development
./aerospike-cloud-cli keys create --name "dev-api-key"

# Delete an old API key
./aerospike-cloud-cli keys delete --client-id "old-client-id"
```

## Error Handling

The CLI provides detailed error messages for common issues:

- **Authentication errors**: Check that `AEROSPIKE_CLOUD_KEY` and `AEROSPIKE_CLOUD_SECRET` are set correctly
- **API errors**: The CLI displays the full API error response for debugging
- **Validation errors**: Required parameters are validated before making API calls

## Output Format

All commands output JSON by default for easy parsing and integration with other tools. The output is pretty-printed for readability.

## Parameter Reference

### Database Create Parameters

#### Basic Parameters
- `--name` (required): Database name
- `--data-plane-version`: Data plane version (default: latest)

#### Infrastructure Parameters
- `--cloud-provider` (required): Cloud provider (aws, gcp)
- `--instance-type` (required): Instance type (e.g., t3.medium, i4i.large)
- `--region` (required): AWS/GCP region (e.g., us-east-1, us-west-2)
- `--availability-zone-count`: Number of availability zones (1-3, default: 2)
- `--zone-ids`: Specific availability zone IDs (comma-separated)
- `--cidr-block`: IPv4 CIDR block for database VPC (e.g., 10.128.0.0/19)

#### Aerospike Cloud Parameters
- `--cluster-size` (required): Number of nodes in cluster
- `--data-storage` (required): Data storage type (memory, local-disk, network-storage)
- `--data-resiliency`: Data resiliency (local-disk, network-storage)

#### Aerospike Server Parameters
- `--advertise-ipv6`: Advertise IPv6
- `--auto-pin`: Auto pin mode (none, cpu, numa, adq)
- `--batch-index-threads`: Batch index threads (1-256)
- `--batch-max-buffers-per-queue`: Batch max buffers per queue
- `--batch-max-requests`: Batch max requests
- `--batch-max-unused-buffers`: Batch max unused buffers
- `--cluster-name`: Cluster name
- `--debug-allocations`: Debug allocations
- `--disable-udf-execution`: Disable UDF execution
- `--enable-benchmarks-fabric`: Enable benchmarks fabric
- `--enable-health-check`: Enable health check
- `--enable-hist-info`: Enable hist info
- `--enforce-best-practices`: Enforce best practices
- `--feature-key-file`: Feature key file
- `--feature-key-files`: Feature key files (comma-separated)
- `--group`: Group
- `--indent-allocations`: Indent allocations
- `--info-max-ms`: Info max ms (500-10000)
- `--info-threads`: Info threads

### VPC Peering Create Parameters
- `--database-id` (required): Database ID
- `--vpc-id` (required): VPC ID
- `--cidr-block` (required): CIDR block
- `--account-id` (required): Account ID
- `--region` (required): Region
- `--is-secure-connection` (required): Is secure connection

## Help

Use the `--help` flag on any command to see detailed usage information:

```bash
./aerospike-cloud-cli --help
./aerospike-cloud-cli databases --help
./aerospike-cloud-cli databases create --help
```
