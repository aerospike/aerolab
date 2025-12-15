# TLS Certificate Management Commands

TLS commands enable you to generate and distribute TLS certificates for securing Aerospike clusters.

## Commands Overview

- `tls generate` - Generate TLS certificates and optionally upload to cluster nodes
- `tls copy` - Copy certificates from one cluster/node to another

## TLS Generate

Generate TLS certificates for an Aerospike cluster with optional automatic upload and configuration.

### Basic Usage

```bash
aerolab tls generate -n mycluster -t tls1 -c cacert
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Cluster names, comma separated | `mydc` |
| `-l, --nodes` | Nodes list, comma separated. Empty=ALL | (all nodes) |
| `-t, --tls-name` | Common Name (TLS name) for certificates | `tls1` |
| `-c, --ca-name` | Name of the CA certificate file | `cacert` |
| `-b, --cert-bits` | Bits size for CA and certificates | `2048` |
| `-e, --ca-expiry-days` | Number of days the CA certificate is valid | `3650` |
| `-E, --cert-expiry-days` | Number of days certificates are valid | `365` |
| `-u, --no-upload` | Generate locally but don't upload to nodes | `false` |
| `-m, --no-mesh` | Don't configure mesh-seed-address-port for TLS | `false` |
| `-W, --work-dir` | Working directory for generation and downloads | `.` |
| `--parallel-threads` | Number of parallel threads | `10` |

### Generated Files

The command creates the following structure in the working directory:

```
CA/
├── cacert.pem          # CA certificate
├── cacert.key          # CA private key
└── cacert-<hash>.srl   # Serial number file

<tlsname>/
├── tls1.pem            # Certificate
└── tls1.key            # Private key
```

### Examples

**Generate certificates for entire cluster:**
```bash
aerolab tls generate -n mycluster -t tls1 -c cacert
```

**Generate for specific nodes:**
```bash
aerolab tls generate -n mycluster -l 1,2,3 -t tls1 -c cacert
```

**Generate for multiple clusters:**
```bash
aerolab tls generate -n cluster1,cluster2,cluster3 -t tls1 -c cacert
```

**Generate with custom bit size and expiry:**
```bash
aerolab tls generate -n mycluster -t tls1 -c cacert \
  -b 4096 -e 7300 -E 730
```

**Generate locally without uploading:**
```bash
aerolab tls generate -n mycluster -t tls1 -c cacert -u
```

**Generate in custom directory:**
```bash
aerolab tls generate -n mycluster -t tls1 -c cacert -W /path/to/certs
```

### How It Works

1. **Checks for existing CA** in `CA/` directory
   - If exists: Reuses existing CA
   - If not exists: Creates new CA certificate and key

2. **Generates certificates** for each node
   - Creates certificate signed by CA
   - Generates private key
   - Stores in `<tlsname>/` directory

3. **Uploads to nodes** (unless `-u` is set)
   - Uploads CA certificate to `/etc/aerospike/CA/`
   - Uploads node certificate to `/etc/aerospike/<tlsname>/`
   - Sets proper permissions

4. **Configures Aerospike** (unless `-m` is set)
   - Modifies `/etc/aerospike/aerospike.conf`
   - Adds TLS configuration stanza
   - Configures mesh heartbeat with TLS

5. **Logs configuration** for manual review

### Configuration Added

The command adds TLS configuration to `aerospike.conf`:

```
network {
    service {
        tls-name tls1
        tls-port 4333
        tls-authenticate-client any
    }
    
    heartbeat {
        tls-name tls1
        tls-port 3012
    }
    
    fabric {
        tls-name tls1
        tls-port 3011
    }
}

tls tls1 {
    ca-file /etc/aerospike/CA/cacert.pem
    cert-file /etc/aerospike/tls1/tls1.pem
    key-file /etc/aerospike/tls1/tls1.key
}
```

### Certificate Reuse

- **Existing CA**: If `CA/` directory exists, the CA is reused for new certificates
- **Multiple Clusters**: Use same CA for multiple clusters to allow cross-cluster communication
- **Rotation**: Delete `CA/` directory to generate new CA

### Notes

- Certificates are node-specific with unique serial numbers
- CA certificate is shared across all nodes
- After generation, restart Aerospike for TLS to take effect
- Use `-u` flag for generating certificates without cluster access
- Generated files remain in working directory for backup/distribution

---

## TLS Copy

Copy TLS certificates from one node/cluster to another cluster's nodes.

### Basic Usage

```bash
aerolab tls copy -s source-cluster -l 1 -d dest-cluster -t tls1
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-s, --source` | Source cluster name | `mydc` |
| `-l, --source-node` | Source node to copy certificates from | `1` |
| `-d, --destination` | Destination cluster name | `client` |
| `-a, --destination-nodes` | Destination nodes, comma separated. Empty=ALL | (all nodes) |
| `-t, --tls-name` | TLS name (directory name for certificates) | `tls1` |
| `--parallel-threads` | Number of parallel threads | `10` |

### Examples

**Copy certificates to entire destination cluster:**
```bash
aerolab tls copy -s prod-cluster -l 1 -d test-cluster -t tls1
```

**Copy to specific destination nodes:**
```bash
aerolab tls copy -s prod -l 1 -d test -a 1,2,3 -t tls1
```

**Copy from cluster to client machines:**
```bash
aerolab tls copy -s aerospike-cluster -l 1 -d ams-client -t tls1
```

### How It Works

1. **Downloads certificates** from source node:
   - CA certificate from `/etc/aerospike/CA/<ca-name>.pem`
   - Node certificate from `/etc/aerospike/<tls-name>/<tls-name>.pem`
   - Private key from `/etc/aerospike/<tls-name>/<tls-name>.key`

2. **Uploads to destination nodes**:
   - Creates directories if needed
   - Uploads all certificate files
   - Sets proper permissions

3. **Processes in parallel** for multiple destination nodes

### Use Cases

- **Client Access**: Copy cluster certificates to client machines
- **Testing**: Copy production certificates to test environment
- **Monitoring**: Copy certificates to AMS/monitoring clients
- **Development**: Share certificates across development clusters

### Notes

- Does not modify Aerospike configuration on destination
- Preserves directory structure from source
- Source and destination clusters must be running
- CA certificate is also copied for complete TLS setup

## Best Practices

1. **CA Management**:
   - Keep CA directory backed up
   - Use same CA across related clusters
   - Rotate CA certificates periodically

2. **Certificate Generation**:
   - Use appropriate bit sizes (2048 minimum, 4096 for high security)
   - Set expiry appropriately (365 days for certs, 3650 for CA)
   - Generate in a secure working directory

3. **Distribution**:
   - Generate once, distribute to all nodes
   - Use `tls copy` for clients and monitoring tools
   - Keep local copies for disaster recovery

4. **After Generation**:
   ```bash
   # Restart Aerospike to activate TLS
   aerolab aerospike restart -n mycluster
   
   # Verify TLS is active
   aerolab attach shell -n mycluster -l 1
   asadm -e "show config network"
   ```

5. **Security**:
   - Store CA private key securely
   - Limit access to certificate directories
   - Use strong passphrases in production

## Troubleshooting

### TLS Not Working After Generation

```bash
# Check if certificates exist on nodes
aerolab attach shell -n mycluster -l 1
ls -la /etc/aerospike/CA/
ls -la /etc/aerospike/tls1/

# Verify configuration
asadm -e "show config tls"

# Restart Aerospike
aerolab aerospike restart -n mycluster
```

### Certificate Permission Issues

```bash
# Fix permissions on nodes
aerolab attach shell -n mycluster
chmod 644 /etc/aerospike/CA/*.pem
chmod 644 /etc/aerospike/tls1/*.pem
chmod 600 /etc/aerospike/tls1/*.key
```

### Client Connection Issues

```bash
# Copy certificates to client
aerolab tls copy -s cluster -l 1 -d client -t tls1

# Test connection with asadm
aerolab attach shell -n client -l 1
asadm -h <cluster-ip> --tls-enable \
  --tls-cafile=/etc/aerospike/CA/cacert.pem \
  --tls-name=tls1
```

### Regenerate Certificates

```bash
# Delete existing certificates
rm -rf CA/ tls1/

# Generate new ones
aerolab tls generate -n mycluster -t tls1 -c cacert
```

## Integration Examples

### TLS with XDR

```bash
# Generate TLS for both clusters
aerolab tls generate -n source,dest -t tls1 -c cacert

# Configure XDR (after restarting with TLS)
aerolab xdr connect -S source -D dest -M test
```

### TLS with Monitoring

```bash
# Generate TLS for cluster
aerolab tls generate -n prod-cluster -t tls1 -c cacert

# Copy to AMS client
aerolab tls copy -s prod-cluster -l 1 -d ams -t tls1

# Configure exporter with TLS
aerolab cluster add exporter -n prod-cluster --tls-enable
```

### TLS with Tools

```bash
# Generate and upload TLS
aerolab tls generate -n mycluster -t tls1 -c cacert

# Copy to tools client
aerolab tls copy -s mycluster -l 1 -d tools-client -t tls1

# Use asbench with TLS
aerolab attach shell -n tools-client -l 1
asbench -h <cluster-ip>:4333 --tls-enable \
  --tls-cafile=/etc/aerospike/CA/cacert.pem \
  --tls-name=tls1
```

## See Also

- [Cluster Management](cluster.md) - Managing clusters
- [XDR Commands](xdr.md) - XDR with TLS
- [Client Commands](client.md) - Client machine management
- [Attach Commands](attach.md) - Accessing nodes for verification

