# Data Management Commands

Data management commands enable you to insert and delete test data in Aerospike clusters for testing and benchmarking purposes.

## Commands Overview

- `data insert` - Insert test data into an Aerospike cluster
- `data delete` - Delete data that was inserted via AeroLab

## Data Insert

Insert test data into an Aerospike cluster with flexible configuration for keys, bins, and content patterns.

### Basic Usage

```bash
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 10000 -b static:mybin -c unique:data_
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Cluster name | `mydc` |
| `-l, --node` | Node to run insert command on | `1` |
| `-g, --seed-node` | Seed node IP:PORT | `127.0.0.1:3000` |
| `-m, --namespace` | Namespace name | `test` |
| `-s, --set` | Set name or `random:SIZE` | `myset` |
| `-p, --pk-prefix` | Prefix for primary keys | (empty) |
| `-a, --pk-start-number` | Starting primary key number | `1` |
| `-z, --pk-end-number` | Ending primary key number | `1000` |
| `-b, --bin` | Bin name pattern | `static:mybin` |
| `-c, --bin-contents` | Bin content pattern | `unique:bin_` |
| `-f, --read-after-write` | Read (get) after each write | `false` |
| `-T, --ttl` | TTL for records (-1=default, 0=never expire) | `-1` |
| `-N, --to-nodes` | Insert to specific node IDs (comma-separated) | (all) |
| `-C, --to-partitions` | Insert to X number of partitions at most | `0` |
| `-L, --to-partition-list` | Insert to specific partition numbers | (empty) |
| `-E, --exists-action` | Exists policy action | (empty) |
| `-d, --run-direct` | Run directly from current machine | `false` |
| `-u, --multi-thread` | Number of threads for processing | `0` |
| `-v, --version` | Aerospike client library version | `8` |

### Authentication Options

| Option | Description |
|--------|-------------|
| `-U, --username` | Aerospike username |
| `-P, --password` | Aerospike password |
| `-Q, --auth-external` | Use external authentication |

### TLS Options

| Option | Description |
|--------|-------------|
| `-y, --tls-ca-cert` | TLS CA certificate path |
| `-w, --tls-client-cert` | TLS client certificate path |
| `-i, --tls-server-name` | TLS server name |

### Pattern Syntax

#### Set Patterns

**static name:**
- `myset` - Use fixed set name

**random:**
- `random:10` - Generate random 10-character set name

#### Bin Name Patterns

**static:**
- `static:mybin` - Fixed bin name "mybin"

**unique:**
- `unique:bin_` - Sequential names: bin_1, bin_2, bin_3, ...

**random:**
- `random:8` - Random 8-character bin names

#### Bin Content Patterns

**static:**
- `static:hello` - Fixed content "hello" for all records

**unique:**
- `unique:data_` - Sequential: data_1, data_2, data_3, ...

**random:**
- `random:100` - Random 100-character strings

### Exists Actions

Control behavior when records already exist:

- `CREATE_ONLY` - Only create new records, fail if exists
- `REPLACE_ONLY` - Only replace existing records, fail if doesn't exist
- `REPLACE` - Replace if exists, create if doesn't exist (default write)
- `UPDATE_ONLY` - Only update existing records, fail if doesn't exist
- `UPDATE` - Update if exists, create if doesn't exist

### Advanced Targeting

**Insert to specific nodes:**
```bash
# Insert records whose master is node 1 or 2
aerolab data insert -n mycluster -N 1,2 -a 1 -z 10000
```

**Insert to limited partitions:**
```bash
# Insert to at most 100 partitions
aerolab data insert -n mycluster -C 100 -a 1 -z 10000
```

**Insert to specific partitions:**
```bash
# Insert to partitions 0, 100, 200
aerolab data insert -n mycluster -L 0,100,200 -a 1 -z 10000
```

### Examples

**Simple insert - 10,000 records:**
```bash
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 10000 -b static:mybin -c unique:value_
```

**Insert with random data:**
```bash
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 10000 -b random:10 -c random:100
```

**Insert with prefix and TTL:**
```bash
aerolab data insert -n mycluster -m test -s myset \
  -p user_ -a 1 -z 10000 -T 3600 \
  -b static:data -c unique:value_
```

**Insert to specific node:**
```bash
aerolab data insert -n mycluster -N 1 \
  -a 1 -z 10000 -b static:mybin -c static:value
```

**Multi-threaded insert:**
```bash
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 1000000 -u 10 \
  -b static:mybin -c random:1000
```

**Insert with authentication:**
```bash
aerolab data insert -n secure-cluster \
  -U admin -P password \
  -m test -s myset -a 1 -z 10000
```

**Insert with TLS:**
```bash
aerolab data insert -n secure-cluster \
  -y /path/to/ca.pem \
  -w /path/to/client.pem \
  -i aerospike-cluster \
  -m test -s myset -a 1 -z 10000
```

**Insert with read verification:**
```bash
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 10000 -f -b static:mybin -c unique:data_
```

**Replace existing records:**
```bash
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 10000 -E REPLACE \
  -b static:mybin -c static:newvalue
```

### How It Works

**Remote Execution (default):**
1. **Installs AeroLab** on target node if not present
2. **Generates insert script** with specified parameters
3. **Uploads script** to node via SFTP
4. **Executes remotely** using Aerospike Go client
5. **Returns results** to local machine

**Direct Execution (-d flag):**
1. **Connects directly** from current machine
2. **Uses local Aerospike client** library
3. **Inserts data** with specified parameters
4. **Reports progress** and completion

### Performance Considerations

- **Multi-threading (-u)**: Use for large datasets (>100K records)
- **Batch size**: Automatically optimized based on record count
- **Node selection (-l)**: Choose node with lowest load
- **Network**: Remote execution avoids network bottlenecks
- **TTL**: Setting TTL=0 improves write performance slightly

### Record Key Format

Primary keys are generated as:
```
<pk-prefix><pk-number>
```

Examples:
- No prefix, numbers 1-1000: `1`, `2`, `3`, ..., `1000`
- Prefix "user_", numbers 1-100: `user_1`, `user_2`, ..., `user_100`

---

## Data Delete

Delete records that were inserted via AeroLab's data insert command.

### Basic Usage

```bash
aerolab data delete -n mycluster -m test -s myset -a 1 -z 10000
```

### Common Options

Same options as `data insert` for targeting records:

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Cluster name | `mydc` |
| `-l, --node` | Node to run delete command on | `1` |
| `-g, --seed-node` | Seed node IP:PORT | `127.0.0.1:3000` |
| `-m, --namespace` | Namespace name | `test` |
| `-s, --set` | Set name | `myset` |
| `-p, --pk-prefix` | Primary key prefix (must match insert) | (empty) |
| `-a, --pk-start-number` | Starting primary key number | `1` |
| `-z, --pk-end-number` | Ending primary key number | `1000` |
| `-d, --run-direct` | Run directly from current machine | `false` |
| `-u, --multi-thread` | Number of threads | `0` |

### Examples

**Delete matching insert:**
```bash
# Original insert
aerolab data insert -n mycluster -m test -s myset \
  -p user_ -a 1 -z 10000

# Delete same records
aerolab data delete -n mycluster -m test -s myset \
  -p user_ -a 1 -z 10000
```

**Multi-threaded delete:**
```bash
aerolab data delete -n mycluster -m test -s myset \
  -a 1 -z 1000000 -u 10
```

**Delete with authentication:**
```bash
aerolab data delete -n secure-cluster \
  -U admin -P password \
  -m test -s myset -a 1 -z 10000
```

### How It Works

1. **Connects to cluster** using specified credentials
2. **Generates delete keys** based on prefix and range
3. **Deletes records** in parallel if multi-threading enabled
4. **Reports results** including success/failure counts

### Notes

- Must match the **exact parameters** used in insert (prefix, range)
- Does not delete bins or modify records, only deletes entire records
- Use multi-threading for large datasets
- Deletion is idempotent (safe to run multiple times)

---

## Complete Examples

### Testing Workload

```bash
# 1. Create cluster
aerolab cluster create -n testcluster -c 3 -d ubuntu -i 24.04 -v '8.*'

# 2. Insert test data
aerolab data insert -n testcluster -m test -s testset \
  -a 1 -z 100000 -u 5 \
  -b static:data -c random:1000

# 3. Run benchmarks
aerolab attach shell -n testcluster -l 1
asbench -h 127.0.0.1 -n test -s testset -o R:100

# 4. Clean up data
aerolab data delete -n testcluster -m test -s testset \
  -a 1 -z 100000 -u 5
```

### XDR Testing

```bash
# 1. Create XDR clusters
aerolab xdr create-clusters -n source -N dest \
  -c 3 -C 3 -d ubuntu -i 24.04 -v '8.*'

# 2. Insert data on source
aerolab data insert -n source -m test -s xdrtest \
  -a 1 -z 50000 -b static:data -c unique:value_

# 3. Wait for XDR to replicate (check via asadm)

# 4. Verify on destination
aerolab attach shell -n dest -l 1
asadm -e "show statistics namespace test like records"

# 5. Clean up
aerolab data delete -n source -m test -s xdrtest -a 1 -z 50000
aerolab data delete -n dest -m test -s xdrtest -a 1 -z 50000
```

### Partition-Specific Testing

```bash
# Insert to specific partitions
aerolab data insert -n mycluster -L 0,100,200,300 \
  -a 1 -z 10000 -b static:mybin -c unique:data_

# Insert to master on specific nodes
aerolab data insert -n mycluster -N 1,2 \
  -a 1 -z 10000 -b static:mybin -c unique:data_
```

### Large Dataset with Monitoring

```bash
# 1. Create cluster with AMS
aerolab cluster create -n bigdata -c 5 -d ubuntu -i 24.04 -v '8.*'
aerolab cluster add exporter -n bigdata
aerolab client create ams -n ams -d ubuntu -i 24.04 -s bigdata

# 2. Insert large dataset
aerolab data insert -n bigdata -m test -s largeset \
  -a 1 -z 10000000 -u 20 \
  -b random:10 -c random:5000

# 3. Monitor in Grafana (check AMS)
# Open http://<ams-ip>:3000

# 4. Cleanup
aerolab data delete -n bigdata -m test -s largeset \
  -a 1 -z 10000000 -u 20
```

### TLS Cluster Data Insert

```bash
# 1. Create cluster with TLS
aerolab cluster create -n secure -c 3 -d ubuntu -i 24.04 -v '8.*'
aerolab tls generate -n secure -t tls1 -c cacert

# 2. Restart with TLS
aerolab aerospike restart -n secure

# 3. Create tools client and copy certs
aerolab client create tools -n tools -d ubuntu -i 24.04
aerolab tls copy -s secure -l 1 -d tools -t tls1

# 4. Insert data with TLS
aerolab data insert -n secure \
  -y /etc/aerospike/CA/cacert.pem \
  -w /etc/aerospike/tls1/tls1.pem \
  -i secure \
  -m test -s secureset -a 1 -z 10000
```

## Best Practices

1. **Start Small**: Test with small datasets (1K-10K) before scaling up
2. **Use Multi-Threading**: Enable for datasets >100K records
3. **Monitor Performance**: Use AMS to track insert rates
4. **Match Delete Parameters**: Keep track of insert parameters for cleanup
5. **Set Appropriate TTL**: Use TTL to auto-expire test data
6. **Unique Sets**: Use unique set names for different test scenarios
7. **Partition Testing**: Use partition targeting for specific test cases

## Performance Tips

- **Local vs Remote**: Remote execution usually faster for large datasets
- **Multi-threading**: Linear scaling up to ~20 threads
- **Bin Size**: Smaller bin content = faster inserts
- **Read-after-write**: Doubles execution time (use only for verification)
- **Network**: Run from node in same network/region as cluster

## Troubleshooting

### Slow Insert Performance

```bash
# Check cluster load
aerolab attach shell -n mycluster -l 1
asadm -e "show statistics like write"

# Increase threads
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 100000 -u 20

# Run from client in same region
aerolab data insert -n mycluster -l 1 -m test -s myset \
  -a 1 -z 100000
```

### Authentication Failures

```bash
# Verify credentials
aerolab attach shell -n mycluster -l 1
asinfo -v "users"

# Use correct username/password
aerolab data insert -n mycluster \
  -U admin -P correctpassword \
  -m test -s myset -a 1 -z 1000
```

### TLS Connection Issues

```bash
# Verify TLS configuration
aerolab attach shell -n mycluster -l 1
asadm -e "show config tls"

# Check certificate paths
ls -la /etc/aerospike/CA/
ls -la /etc/aerospike/tls1/

# Use correct TLS parameters
aerolab data insert -n mycluster \
  -y /etc/aerospike/CA/cacert.pem \
  -i correct-tls-name \
  -m test -s myset -a 1 -z 1000
```

### Out of Memory

```bash
# Reduce record size
aerolab data insert -n mycluster -m test -s myset \
  -a 1 -z 1000000 -c random:100  # Instead of random:10000

# Increase namespace memory
aerolab conf aerospike memory-size 8G -n mycluster

# Check current usage
aerolab attach shell -n mycluster -l 1
asadm -e "show statistics namespace like memory"
```

## Limitations

- **Pattern Support**: Limited to static, unique, and random patterns
- **Data Types**: Currently supports string bin values
- **Complex Keys**: No support for complex/compound keys
- **Bin Count**: Single bin per record (can be extended)
- **Lists/Maps**: No direct support for complex data types

## See Also

- [Cluster Management](cluster.md) - Creating and managing clusters
- [XDR Commands](xdr.md) - XDR replication testing
- [Client Commands](client.md) - Tools and monitoring clients
- [Attach Commands](attach.md) - Accessing cluster nodes
- [Configuration](conf.md) - Cluster configuration

