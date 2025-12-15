# XDR Management Commands

XDR (Cross-Datacenter Replication) commands enable you to configure and manage replication between Aerospike clusters.

## Commands Overview

- `xdr connect` - Configure XDR between existing clusters
- `xdr create-clusters` - Create source and destination clusters with XDR pre-configured

## XDR Connect

Configure XDR replication from a source cluster to one or more destination clusters.

### Basic Usage

```bash
aerolab xdr connect -S mycluster -D destcluster1,destcluster2 -M test
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-S, --source` | Source cluster name | `mydc` |
| `-D, --destinations` | Destination cluster names, comma separated | `destdc` |
| `-c, --connector` | Set to indicate destination is a client connector, not a cluster | `false` |
| `-V, --xdr-version` | Aerospike XDR configuration version (4\|5\|auto) | `auto` |
| `-T, --restart-source` | Restart source nodes after connecting (y/n) | `y` |
| `-M, --namespaces` | Comma-separated list of namespaces to replicate | `test` |
| `-P, --destination-port` | Custom destination port for XDR connection | (empty) |
| `-p, --parallel-threads` | Number of parallel threads | `10` |

### XDR Version Support

**XDR Version 5** (Aerospike Server 5.0+):
- Uses `dc` (datacenter) stanza
- Uses `node-address-port` configuration
- Namespace configuration within DC stanza
- Supports connector mode

**XDR Version 4** (Aerospike Server 4.x):
- Uses `datacenter` stanza
- Uses `dc-node-address-port` configuration  
- Namespace-level XDR configuration
- **Does not support connector mode**

**Auto Detection**:
- Automatically detects server version from cluster tags
- Version 3.x and 4.x → XDR v4
- Version 5.x and above → XDR v5

### Examples

**Connect two clusters via XDR:**
```bash
aerolab xdr connect -S source-cluster -D dest-cluster -M test,bar
```

**Connect to multiple destination clusters:**
```bash
aerolab xdr connect -S source -D dest1,dest2,dest3 -M test
```

**Connect to connector client:**
```bash
aerolab xdr connect -S source -D connector-client -c -M test
```

**Force XDR version 5:**
```bash
aerolab xdr connect -S source -D dest -V 5 -M test
```

**Connect without restarting source:**
```bash
aerolab xdr connect -S source -D dest -T n -M test
```

### How It Works

1. **Creates XDR directory** on source nodes: `/opt/aerospike/xdr`
2. **Reads aerospike.conf** from each source node
3. **Adds XDR stanza** if not present
4. **Configures datacenters** with destination IPs and ports
5. **Updates namespace configuration** (XDR v4 only)
6. **Writes modified config** back to nodes
7. **Restarts Aerospike** on source nodes (if `-T y`)

### Configuration Details

The command modifies `/etc/aerospike/aerospike.conf` on source nodes to add:

**XDR v5 Configuration:**
```
xdr {
    dc destcluster {
        node-address-port 10.0.1.10 3000
        node-address-port 10.0.1.11 3000
        namespace test {
        }
    }
}
```

**XDR v4 Configuration:**
```
xdr {
    enable-xdr true
    xdr-digestlog-path /opt/aerospike/xdr/digestlog 1G
    datacenter destcluster {
        dc-node-address-port 10.0.1.10 3000
        dc-node-address-port 10.0.1.11 3000
    }
}

namespace test {
    enable-xdr true
    xdr-remote-datacenter destcluster
}
```

### Notes

- Source cluster nodes must be running
- Destination cluster nodes must be running
- For connectors, only Aerospike 5.0+ is supported
- The command preserves existing XDR configuration and adds new datacenters
- Restart is required for changes to take effect (use `-T y`)

---

## XDR Create Clusters

Create a source cluster and destination clusters with XDR already configured.

### Basic Usage

```bash
aerolab xdr create-clusters -n source -N dest1,dest2 -c 3 -C 3 \
  -d ubuntu -i 24.04 -v '8.*'
```

### Common Options

Inherits all options from `cluster create` plus:

| Option | Description | Default |
|--------|-------------|---------|
| `-N, --destinations` | Comma-separated list of destination cluster names | `destdc` |
| `-C, --destination-count` | Number of nodes per destination cluster | `1` |
| `-V, --xdr-version` | XDR configuration version (4\|5\|auto) | `auto` |
| `-T, --restart-source` | Restart source nodes after connecting (y/n) | `y` |
| `-M, --namespaces` | Comma-separated list of namespaces to replicate | `test` |
| `-P, --destination-port` | Custom destination port for XDR connection | (empty) |

### Examples

**Create source and two destination clusters:**
```bash
aerolab xdr create-clusters -n prod-us -N prod-eu,prod-asia \
  -c 3 -C 2 -d ubuntu -i 24.04 -v '8.*' -M test,userdata
```

**Create with AWS backend:**
```bash
aerolab xdr create-clusters -n source -N dest1,dest2 \
  -c 3 -C 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.large --aws-disk type=gp3,size=20 --aws-expire=8h
```

**Create with GCP backend:**
```bash
aerolab xdr create-clusters -n source -N dest1,dest2 \
  -c 3 -C 2 -d ubuntu -i 24.04 -v '8.*' \
  --instance e2-standard-4 --gcp-disk type=pd-ssd,size=20
```

### How It Works

1. **Creates source cluster** with specified node count
2. **Creates each destination cluster** with destination node count
3. **Configures XDR** from source to all destinations
4. **Restarts source cluster** to activate XDR (if `-T y`)

### Notes

- All clusters use the same distribution and Aerospike version
- Destination clusters use `-C` for node count instead of `-c`
- Source cluster uses `-c` for node count
- XDR is automatically configured after all clusters are created
- Useful for quickly setting up XDR test environments

## Best Practices

1. **Version Compatibility**: Ensure source and destination run compatible Aerospike versions
2. **Network Connectivity**: Verify network connectivity between clusters before configuring XDR
3. **Namespace Consistency**: Ensure namespaces exist on both source and destination
4. **Monitoring**: Use `aerolab client create ams` to monitor XDR metrics
5. **Testing**: Use `aerolab xdr create-clusters` for quick test environment setup
6. **Production**: Use `aerolab xdr connect` for existing production clusters

## Troubleshooting

### XDR Not Replicating

```bash
# Check XDR configuration on source
aerolab attach shell -n source -l 1
asadm -e "show config xdr"

# Check XDR statistics
asadm -e "show statistics xdr"

# Verify network connectivity
aerolab attach shell -n source -l 1
telnet <dest-ip> 3000
```

### Configuration Issues

```bash
# View aerospike.conf
aerolab files download -n source -l 1 -s /etc/aerospike/aerospike.conf

# Check XDR logs
aerolab logs aerospike -n source -l 1 -f | grep -i xdr
```

### Restart Required

If XDR was configured with `-T n`, manually restart:

```bash
aerolab aerospike restart -n source
```

## See Also

- [Cluster Management](cluster.md) - Creating and managing clusters
- [Network Commands](net.md) - Testing network connectivity
- [Attach Commands](attach.md) - Accessing cluster nodes
- [Logs Commands](logs.md) - Viewing logs

