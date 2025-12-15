# Client Management Commands

Client commands enable you to create and manage client machines for various purposes including monitoring, development, testing, and administration.

## Commands Overview

- `client create` - Create new client machines (none, base, tools, ams, vscode, graph, eksctl)
- `client grow` - Add machines to existing client groups
- `client configure` - Configure client machines (ams, firewall, tools)
- `client list` - List all client machine groups
- `client start` - Start client machines
- `client stop` - Stop client machines
- `client destroy` - Destroy client machines
- `client attach` - Attach to a client machine (shorthand for `attach client`)
- `client share` - Share client access via SSH public key

## Client Types

### None
Vanilla OS image with no modifications. Useful for custom setups.

```bash
aerolab client create none -n myclient -d ubuntu -i 24.04
```

### Base
Simple base image with basic tools installed.

```bash
aerolab client create base -n myclient -d ubuntu -i 24.04
```

### Tools
Aerospike tools (asbench, asadm, asinfo, asloglatency) pre-installed.

```bash
aerolab client create tools -n tools -d ubuntu -i 24.04
```

### AMS
Aerospike Monitoring Stack with Prometheus, Grafana, and Loki.

```bash
aerolab client create ams -n ams -d ubuntu -i 24.04 \
  -s mycluster -S graph-client
```

**AMS Options:**
- `--grafana-version` - Grafana version (default: `latest`)
- `--prometheus-version` - Prometheus version (default: `latest`)
| `-s, --clusters` | Clusters to monitor (comma-separated) |
- `-S, --clients` - Graph clients to monitor (comma-separated)
- `--dashboards` - Custom dashboards YAML file
- `--debug-dashboards` - Enable debug output for dashboard installation

### VSCode
VSCode Server for browser-based development.

```bash
aerolab client create vscode -n ide -d ubuntu -i 24.04
```

### Graph
Graph database client for Aerospike Graph.

```bash
aerolab client create graph -n graph -d ubuntu -i 24.04 \
  -s mycluster
```

### EksCtl
Client machine with eksctl pre-configured for Kubernetes Aerospike deployments.

```bash
aerolab client create eksctl -n k8s-admin -d ubuntu -i 24.04
```

---

## Client Create

Create new client machines.

### Basic Usage

```bash
aerolab client create <type> -n <name> -d <distro> -i <version> [options]
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Client group name | `client` |
| `-c, --count` | Number of client machines | `1` |
| `-d, --distro` | Distribution (ubuntu, centos, rocky, debian, amazon) | Required |
| `-i, --distro-version` | Distribution version | Required |
| `--type-override` | Override auto-detected client type | (empty) |
| `-P, --parallel-threads` | Number of parallel threads | `10` |

### Docker Backend Options

```bash
aerolab client create tools -n tools -d ubuntu -i 24.04 \
  -c 2 --docker-expose 9100:9100
```

| Option | Description |
|--------|-------------|
| `--docker-expose` | Expose ports (format: `host:container` or `+host:container` for cumulative) |

### AWS Backend Options

```bash
aerolab client create tools -n tools -d ubuntu -i 24.04 \
  -I t3a.medium --aws-disk type=gp3,size=20 --aws-expire=4h
```

| Option | Description |
|--------|-------------|
| `-I, --instance-type` | Instance type (e.g., `t3a.medium`) |
| `--aws-disk` | Disk spec: `type={gp2\|gp3},size={GB}[,count=N]` |
| `--aws-expire` | Expiry time (e.g., `4h`, `30m`) |
| `-U, --subnet-id` | Subnet ID or availability zone |
| `-L, --public-ip` | Enable public IP |
| `--secgroup-name` | Security group names |
| `--tags` | Custom tags (format: `key=value`) |

### GCP Backend Options

```bash
aerolab client create tools -n tools -d ubuntu -i 24.04 \
  --instance e2-medium --gcp-disk type=pd-ssd,size=20
```

| Option | Description |
|--------|-------------|
| `--instance` | Instance type (e.g., `e2-medium`) |
| `--zone` | Zone name (e.g., `us-central1-a`) |
| `--gcp-disk` | Disk spec: `type=pd-ssd[,size={GB}][,count=N]` |
| `--gcp-expire` | Expiry time |
| `--external-ip` | Enable public IP |
| `--firewall` | Firewall rule names |
| `--label` | Custom labels |
| `--tag` | Network tags |

---

## Client Grow

Add more machines to an existing client group.

### Basic Usage

```bash
aerolab client grow <type> -n <name> -c <count>
```

### Examples

```bash
# Add 2 more tools clients
aerolab client grow tools -n tools -c 2

# Add 1 more AMS client
aerolab client grow ams -n ams -c 1 -s mycluster
```

---

## Client Configure

Configure or reconfigure client machines.

### Configure AMS

Reconfigure AMS to monitor different clusters or clients.

```bash
aerolab client configure ams -n ams -s cluster1,cluster2 -S graph1
```

**Options:**
- `-n, --group-name` - Client group name (default: `client`)
- `-l, --machines` - Specific machines, comma separated (default: all)
- `-s, --clusters` - Clusters to monitor (comma-separated)
- `-S, --clients` - Graph clients to monitor (comma-separated)

**Example:**
```bash
# Add monitoring for new cluster
aerolab client configure ams -n ams -s prod-us,prod-eu

# Reconfigure specific AMS machines
aerolab client configure ams -n ams -l 1,2 -s mycluster
```

### Configure Firewall

Assign firewall rules to client machines.

```bash
aerolab client configure firewall -n myclient -f firewall-name
```

**Options:**
- `-n, --group-name` - Client group name (default: `client`)
- `-l, --machines` - Specific machines (default: all)
- `-f, --firewall` - Firewall name to assign (required)

**Example:**
```bash
# Assign firewall to all clients
aerolab client configure firewall -n tools -f allow-outbound

# Assign to specific machines
aerolab client configure firewall -n tools -l 1,2,3 -f restricted
```

### Configure Tools

Configure tools clients to send logs to AMS (Loki).

```bash
aerolab client configure tools -n tools -m ams
```

**Options:**
- `-n, --group-name` - Client group name (default: `client`)
- `-l, --machines` - Specific machines (default: all)
- `-m, --ams` - AMS client machine name (default: `ams`)
- `-t, --threads` - Number of parallel threads (default: `10`)

**What it does:**
- Installs Promtail (log aggregator)
- Configures Promtail to scrape asbench logs
- Sends logs to Loki on AMS client
- Creates systemd service for Promtail
- Enables autostart on boot

**Example:**
```bash
# Configure all tools clients
aerolab client configure tools -n tools -m ams

# Configure specific machines
aerolab client configure tools -n tools -l 1,2 -m my-ams
```

---

## Client List

List all client machine groups.

### Basic Usage

```bash
aerolab client list
```

### Example Output

```
Client Groups:
  Name: ams
    Type: ams
    Count: 1
    State: running
    IPs: 10.0.1.100
  
  Name: tools
    Type: tools
    Count: 3
    State: running
    IPs: 10.0.1.101, 10.0.1.102, 10.0.1.103
```

---

## Client Start/Stop

Start or stop client machines.

### Basic Usage

```bash
# Start clients
aerolab client start -n tools

# Stop clients
aerolab client stop -n tools
```

**Options:**
- `-n, --group-name` - Client group name (default: `client`)
- `-l, --machines` - Specific machines (default: all)

---

## Client Destroy

Destroy client machines.

### Basic Usage

```bash
aerolab client destroy -n tools
```

**Options:**
- `-n, --group-name` - Client group name (default: `client`)
- `-l, --machines` - Specific machines (default: all)
- `-f, --force` - Force destruction without confirmation

**Examples:**
```bash
# Destroy entire client group
aerolab client destroy -n tools -f

# Destroy specific machines
aerolab client destroy -n tools -l 1,2 -f

# Destroy multiple groups
aerolab client destroy -n tools,ams,vscode -f
```

---

## Client Share

Share client access with other users via SSH public key.

### Basic Usage

```bash
aerolab client share -n tools -k ~/.ssh/id_rsa.pub
```

**Options:**
- `-n, --group-name` - Client group name
- `-l, --machines` - Specific machines (default: all)
- `-k, --key-file` - SSH public key file path

**Example:**
```bash
# Share access to all machines
aerolab client share -n ams -k ~/.ssh/team_key.pub

# Share with specific machines
aerolab client share -n tools -l 1,2 -k ~/.ssh/developer.pub
```

---

## Complete Examples

### AMS Monitoring Setup

```bash
# 1. Create Aerospike cluster
aerolab cluster create -n prod -c 3 -d ubuntu -i 24.04 -v '8.*'

# 2. Add exporter to cluster
aerolab cluster add exporter -n prod

# 3. Create AMS client
aerolab client create ams -n ams -d ubuntu -i 24.04 -s prod

# 4. Create tools client for benchmarking
aerolab client create tools -n tools -d ubuntu -i 24.04

# 5. Configure tools to send logs to AMS
aerolab client configure tools -n tools -m ams

# 6. Access Grafana (check client list for IP)
aerolab client list
# Open browser to http://<ams-ip>:3000
# Username: admin, Password: admin
```

### Development Environment

```bash
# Create VSCode IDE client
aerolab client create vscode -n ide -d ubuntu -i 24.04

# Create tools client
aerolab client create tools -n dev-tools -d ubuntu -i 24.04

# Create graph client
aerolab client create graph -n graph-dev -d ubuntu -i 24.04 -s mycluster

# Share access with team
aerolab client share -n ide,dev-tools,graph-dev -k ~/.ssh/team.pub
```

### Multi-Region XDR with Monitoring

```bash
# Create clusters
aerolab xdr create-clusters -n us-east -N eu-west,ap-south \
  -c 3 -C 3 -d ubuntu -i 24.04 -v '8.*'

# Add exporters
aerolab cluster add exporter -n us-east,eu-west,ap-south

# Create AMS for monitoring all regions
aerolab client create ams -n global-ams -d ubuntu -i 24.04 \
  -s us-east,eu-west,ap-south

# Access monitoring
aerolab client list
```

### EKS/Kubernetes Setup

```bash
# Create eksctl client
aerolab client create eksctl -n k8s-admin -d ubuntu -i 24.04

# Attach to client
aerolab client attach -n k8s-admin -l 1

# Inside client: deploy Aerospike Kubernetes Operator
eksctl create cluster --name=aerospike-k8s --region=us-east-1
kubectl apply -f https://operatorhub.io/install/aerospike-kubernetes-operator.yaml
```

## Best Practices

1. **Naming**: Use descriptive names for client groups (e.g., `prod-ams`, `dev-tools`)
2. **Monitoring**: Always create AMS client for production clusters
3. **Tools Integration**: Configure tools clients to send logs to AMS
4. **Access Control**: Use `client share` to manage team access
5. **Resource Cleanup**: Destroy unused clients to save resources
6. **Expiry**: Set appropriate expiry times for cloud-based clients

## Troubleshooting

### AMS Not Showing Metrics

```bash
# Check if exporter is installed on cluster
aerolab cluster add exporter -n mycluster

# Verify Prometheus targets
aerolab attach shell -n ams -l 1
curl http://localhost:9090/api/v1/targets

# Reconfigure AMS
aerolab client configure ams -n ams -s mycluster
```

### Tools Client Logs Not in Loki

```bash
# Check Promtail status
aerolab attach shell -n tools -l 1
systemctl status promtail

# Reconfigure tools
aerolab client configure tools -n tools -m ams

# Check Promtail logs
journalctl -u promtail -f
```

### Cannot Access Grafana

```bash
# Check if port is exposed (Docker)
aerolab client list

# Verify Grafana is running
aerolab attach shell -n ams -l 1
systemctl status grafana-server

# Restart Grafana
systemctl restart grafana-server
```

## See Also

- [Cluster Management](cluster.md) - Managing Aerospike clusters
- [Attach Commands](attach.md) - Accessing client machines
- [Files Commands](files.md) - File operations
- [XDR Commands](xdr.md) - XDR configuration
- [TLS Commands](tls.md) - TLS certificates

