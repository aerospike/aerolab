# Network Simulation Commands

Network simulation commands enable you to test cluster behavior under various network conditions including packet loss, latency, and blocked connections.

## Commands Overview

- `net block` - Block network ports between clusters
- `net unblock` - Unblock previously blocked ports
- `net list` - List blocked ports and active rules
- `net loss-delay` - Simulate packet loss, latency, bandwidth limits, and corruption

## Net Block

Block network communication on specific ports between source and destination clusters.

### Basic Usage

```bash
aerolab net block -s source-cluster -d dest-cluster -p 3000
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-s, --source` | Source cluster name | `mydc` |
| `-l, --source-node-list` | Source nodes, comma separated. Empty=ALL | (all) |
| `-d, --destination` | Destination cluster name | `mydc-xdr` |
| `-i, --destination-node-list` | Destination nodes, comma separated. Empty=ALL | (all) |
| `-t, --type` | Block type (reject\|drop) | `reject` |
| `-p, --ports` | Ports to block, comma separated | `3000` |
| `-b, --block-on` | Where to block (input\|output) | `input` |
| `-M, --statistic-mode` | Partial loss mode (random\|nth\|"") | (empty) |
| `-P, --probability` | Probability for random mode (0.0-1.0) | `0.5` |
| `-E, --every` | Match every Nth packet for nth mode | `2` |

### Block Types

**reject:**
- Sends ICMP port unreachable response
- Client receives immediate "Connection refused"
- Fast failure detection

**drop:**
- Silently drops packets
- Client waits for timeout
- Simulates network black hole

### Block Location

**input** (default):
- Blocks on destination nodes
- Blocks incoming traffic on specified ports
- Simulates destination being unreachable

**output:**
- Blocks on source nodes
- Blocks outgoing traffic to destination
- Simulates source being unable to reach destination

### Statistic Modes

**No statistic mode (default):**
- Blocks 100% of matching packets

**random:**
- Random packet loss/blocking
- Use `-P` to set probability (0.0 to 1.0)
- Example: `-P 0.3` = 30% packet loss

**nth:**
- Deterministic packet loss
- Use `-E` to set pattern (e.g., every 2nd packet)
- Example: `-E 3` = drop every 3rd packet (33% loss)

### Examples

**Block Aerospike service port:**
```bash
aerolab net block -s cluster1 -d cluster2 -p 3000
```

**Block with 50% random packet loss:**
```bash
aerolab net block -s cluster1 -d cluster2 -p 3000 \
  -M random -P 0.5
```

**Block every 3rd packet (33% loss):**
```bash
aerolab net block -s cluster1 -d cluster2 -p 3000 \
  -M nth -E 3
```

**Block XDR ports between specific nodes:**
```bash
aerolab net block -s source -l 1,2 -d dest -i 1,2 \
  -p 3000,8081
```

**Block on output (from source):**
```bash
aerolab net block -s cluster1 -d cluster2 -p 3000 -b output
```

**Use drop instead of reject:**
```bash
aerolab net block -s cluster1 -d cluster2 -p 3000 -t drop
```

### How It Works

1. **Identifies nodes** in source and destination clusters
2. **Gets IP addresses** of all involved nodes
3. **Applies iptables rules** on appropriate nodes (input/output)
4. **Configures statistic module** if partial packet loss is requested

### iptables Rules Created

**Standard block (input, reject):**
```bash
iptables -I INPUT -s <dest-ip> -p tcp --dport 3000 -j REJECT
```

**With random packet loss (50%):**
```bash
iptables -I INPUT -s <dest-ip> -p tcp --dport 3000 \
  -m statistic --mode random --probability 0.5 -j REJECT
```

**With nth packet loss (every 2nd):**
```bash
iptables -I INPUT -s <dest-ip> -p tcp --dport 3000 \
  -m statistic --mode nth --every 2 --packet 0 -j REJECT
```

---

## Net Unblock

Remove previously created network blocks.

### Basic Usage

```bash
aerolab net unblock -s source-cluster -d dest-cluster -p 3000
```

### Options

Same as `net block` - must match the original block command parameters.

### Examples

```bash
# Unblock exact rule
aerolab net unblock -s cluster1 -d cluster2 -p 3000

# Unblock with statistic mode
aerolab net unblock -s cluster1 -d cluster2 -p 3000 -M random -P 0.5

# Unblock from specific nodes
aerolab net unblock -s source -l 1,2 -d dest -p 3000
```

**Note:** Parameters must match the original `net block` command to remove the correct iptables rules.

---

## Net List

List all active network rules and blocks.

### Basic Usage

```bash
aerolab net list -s cluster-name
```

### Options

| Option | Description |
|--------|-------------|
| `-s, --source` | Cluster name to list rules for |
| `-l, --source-node-list` | Specific nodes (default: all) |

### Example Output

```
Cluster: mycluster
Node 1:
  Chain INPUT:
    - Block from 10.0.1.10 port 3000 (REJECT)
    - Block from 10.0.1.11 port 3000 (REJECT) [random 50%]
  
  Chain OUTPUT:
    - Block to 10.0.2.10 port 8081 (DROP)

Node 2:
  Chain INPUT:
    - Block from 10.0.1.10 port 3000 (REJECT)
```

---

## Net Loss-Delay

Simulate advanced network conditions including latency, packet loss, bandwidth limits, and corruption using Traffic Control (tc).

### Basic Usage

```bash
aerolab net loss-delay -s source -d dest -a set -D 100 -L 10
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-s, --source` | Source cluster name | `mydc` |
| `-l, --source-node-list` | Source nodes. Empty=ALL | (all) |
| `-d, --destination` | Destination cluster name | `mydc-xdr` |
| `-i, --destination-node-list` | Destination nodes. Empty=ALL | (all) |
| `-a, --action` | Action: set\|del\|reset\|show | `show` |
| `-D, --latency-ms` | Latency in milliseconds | (none) |
| `-L, --loss-pct` | Packet loss percentage | (none) |
| `-E, --rate-bytes` | Link speed rate in bytes/sec | (none) |
| `-O, --corrupt-pct` | Packet corruption percentage | (none) |
| `-o, --on-destination` | Apply rules on destination nodes | `false` |
| `-p, --dst-port` | Apply to specific destination port | (all) |
| `-P, --src-port` | Apply to specific source port | (all) |
| `-v, --verbose` | Verbose output | `false` |
| `-t, --threads` | Number of parallel threads | `10` |

### Actions

**show** (default):
- Display current traffic control rules
- Shows active impairments

**set:**
- Add or update network impairments
- Combines multiple impairments if specified

**del:**
- Remove specific network impairments
- Must match original parameters

**reset:**
- Remove ALL traffic control rules
- Does not require destination cluster
- Clears all impairments

### Network Impairments

**Latency (-D, --latency-ms):**
```bash
# Add 100ms latency
aerolab net loss-delay -s cluster1 -d cluster2 -a set -D 100
```

**Packet Loss (-L, --loss-pct):**
```bash
# Add 10% packet loss
aerolab net loss-delay -s cluster1 -d cluster2 -a set -L 10
```

**Bandwidth Limit (-E, --rate-bytes):**
```bash
# Limit to 1MB/s
aerolab net loss-delay -s cluster1 -d cluster2 -a set -E 1048576
```

**Packet Corruption (-O, --corrupt-pct):**
```bash
# Corrupt 5% of packets
aerolab net loss-delay -s cluster1 -d cluster2 -a set -O 5
```

### Combined Impairments

```bash
# Latency + packet loss + bandwidth limit
aerolab net loss-delay -s cluster1 -d cluster2 -a set \
  -D 50 -L 5 -E 524288
```

### Port-Specific Rules

```bash
# Apply only to XDR port 8081
aerolab net loss-delay -s source -d dest -a set \
  -D 100 -p 8081

# Apply only to Aerospike service port
aerolab net loss-delay -s cluster1 -d cluster2 -a set \
  -L 10 -p 3000
```

### Examples

**Simulate WAN latency:**
```bash
# 200ms latency between regions
aerolab net loss-delay -s us-east -d eu-west -a set -D 200
```

**Simulate poor network:**
```bash
# High latency + packet loss
aerolab net loss-delay -s cluster1 -d cluster2 -a set \
  -D 500 -L 20
```

**Simulate bandwidth constraint:**
```bash
# Limit to 10MB/s
aerolab net loss-delay -s cluster1 -d cluster2 -a set \
  -E 10485760
```

**Test XDR resilience:**
```bash
# Add latency and loss to XDR
aerolab net loss-delay -s source -d dest -a set \
  -D 100 -L 5 -p 8081
```

**View current rules:**
```bash
aerolab net loss-delay -s cluster1 -a show
```

**Remove specific impairment:**
```bash
aerolab net loss-delay -s cluster1 -d cluster2 -a del
```

**Reset all rules:**
```bash
aerolab net loss-delay -s cluster1 -a reset
```

### How It Works

1. **Installs easytc** (Easy Traffic Control) if not present
2. **Configures tc qdisc** (queuing discipline) on network interfaces
3. **Applies netem** (Network Emulator) rules
4. **Filters traffic** by destination IP and optionally ports

### Under the Hood

Uses Linux Traffic Control (`tc`) with Network Emulator (`netem`):

```bash
# Example: 100ms latency + 10% loss
tc qdisc add dev eth0 root handle 1: prio
tc qdisc add dev eth0 parent 1:3 handle 30: netem delay 100ms loss 10%
tc filter add dev eth0 parent 1:0 protocol ip pref 3 u32 \
  match ip dst 10.0.1.10/32 flowid 1:3
```

---

## Use Cases

### Testing Split-Brain Scenarios

```bash
# Block heartbeat port between nodes
aerolab net block -s cluster1 -l 1,2 -d cluster1 -i 3,4,5 -p 3002

# Observe cluster behavior
aerolab attach shell -n cluster1 -l 1
asadm -e "info"

# Restore
aerolab net unblock -s cluster1 -l 1,2 -d cluster1 -i 3,4,5 -p 3002
```

### Testing XDR Under Network Issues

```bash
# Create XDR clusters
aerolab xdr create-clusters -n source -N dest -c 3 -C 3 \
  -d ubuntu -i 24.04 -v '8.*'

# Add monitoring
aerolab cluster add exporter -n source,dest
aerolab client create ams -n ams -d ubuntu -i 24.04 -s source,dest

# Simulate network issues
aerolab net loss-delay -s source -d dest -a set -D 200 -L 10 -p 8081

# Monitor XDR lag in Grafana
# Check metrics: aerospike_namespace_xdr_ship_lag_ms

# Restore network
aerolab net loss-delay -s source -d dest -a reset
```

### Testing Client Timeouts

```bash
# Block service port
aerolab net block -s cluster1 -d cluster-tools -p 3000

# Run benchmark from tools client (will fail/timeout)
aerolab attach shell -n cluster-tools -l 1
asbench -h <cluster-ip>

# Restore
aerolab net unblock -s cluster1 -d cluster-tools -p 3000
```

### Simulating Intermittent Connectivity

```bash
# 30% random packet loss
aerolab net block -s cluster1 -d cluster2 -p 3000 -M random -P 0.3

# Or use loss-delay
aerolab net loss-delay -s cluster1 -d cluster2 -a set -L 30
```

### Testing Geo-Distribution

```bash
# Simulate cross-region latency
aerolab net loss-delay -s us-east -d eu-west -a set -D 100
aerolab net loss-delay -s us-east -d ap-south -a set -D 200
aerolab net loss-delay -s eu-west -d ap-south -a set -D 150
```

## Best Practices

1. **Start Simple**: Test with block/unblock before using loss-delay
2. **Monitor Impact**: Use AMS/Grafana to observe cluster behavior
3. **Realistic Values**:
   - LAN latency: 1-10ms
   - Cross-datacenter: 50-200ms
   - Packet loss: 1-5% (poor network), 10%+ (severe)
4. **Clean Up**: Always reset/unblock after testing
5. **Document**: Keep track of which rules are active
6. **Port-Specific**: Apply rules to specific ports when possible

## Troubleshooting

### Rules Not Working

```bash
# Check if rules are applied
aerolab net list -s cluster1

# Verify iptables directly
aerolab attach shell -n cluster1 -l 1
iptables -L -n -v

# Check tc rules
tc qdisc show
```

### Cannot Remove Rules

```bash
# Use reset to clear everything
aerolab net loss-delay -s cluster1 -a reset

# Or manually clean iptables
aerolab attach shell -n cluster1 -l 1
iptables -F INPUT
iptables -F OUTPUT
```

### Traffic Still Flowing

```bash
# Verify source and destination IPs are correct
aerolab cluster list
aerolab net list -s cluster1

# Check if using correct port
netstat -tulpn | grep aerospike
```

### easytc Not Found

```bash
# easytc is automatically installed
# If issues persist, check installation
aerolab attach shell -n cluster1 -l 1
which easytc
```

## Performance Impact

- **iptables rules**: Minimal overhead
- **tc/netem**: Low overhead for most scenarios
- **Multiple rules**: Each rule adds small processing cost
- **High packet rates**: May need tuning for very high throughput

## Limitations

- **Docker backend**: May have limitations with tc/netem
- **Container networking**: Some advanced features may not work
- **Requires root**: All network manipulation requires root access
- **Persistence**: Rules do not persist across reboots/restarts

## See Also

- [Cluster Management](cluster.md) - Managing clusters
- [XDR Commands](xdr.md) - XDR configuration and testing
- [Client Commands](client.md) - AMS monitoring setup
- [Attach Commands](attach.md) - Accessing nodes for verification

