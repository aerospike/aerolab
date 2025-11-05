# Log Management Commands

Commands for viewing and downloading logs from cluster nodes.

## Commands Overview

- `logs show` - Show logs from nodes
- `logs get` - Download logs from nodes

## Logs Show

Display logs from cluster nodes.

### Basic Usage

```bash
aerolab logs show -n mydc
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list (comma-separated or ranges) |
| `-j, --json` | Output in JSON format |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Show logs from all nodes:**
```bash
aerolab logs show -n mydc
```

**Show logs from specific node:**
```bash
aerolab logs show -n mydc -l 1
```

**Show logs from multiple nodes:**
```bash
aerolab logs show -n mydc -l 1-3
```

**Show logs in JSON format:**
```bash
aerolab logs show -n mydc -j
```

**Show logs from all nodes in JSON:**
```bash
aerolab logs show -n mydc -j
```

### Output

The `logs show` command displays:
- Recent log entries
- Log file locations
- Timestamp information
- Log levels and messages

## Logs Get

Download logs from cluster nodes to local directory.

### Basic Usage

```bash
aerolab logs get -n mydc -d ./logs/
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list |
| `-d, --directory` | Destination directory (required) |
| `-j, --json` | Download in JSON format |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Download logs from all nodes:**
```bash
aerolab logs get -n mydc -d ./logs/
```

**Download logs from specific node:**
```bash
aerolab logs get -n mydc -l 1 -d ./logs/
```

**Download logs from multiple nodes:**
```bash
aerolab logs get -n mydc -l 1-3 -d ./logs/
```

**Download logs in JSON format:**
```bash
aerolab logs get -n mydc -j -d ./logs/
```

### Output Structure

Logs are downloaded with the following structure:
```
./logs/
  ├── mydc-1/
  │   ├── aerospike.log
  │   └── ...
  ├── mydc-2/
  │   ├── aerospike.log
  │   └── ...
  └── ...
```

Each node's logs are saved in a separate directory named `{cluster-name}-{node-number}`.

## Common Workflows

### View Recent Logs

```bash
# Show logs from all nodes
aerolab logs show -n mydc

# Show logs from specific node
aerolab logs show -n mydc -l 1
```

### Download Logs for Analysis

```bash
# Download logs from all nodes
aerolab logs get -n mydc -d ./logs/

# Download logs from specific node
aerolab logs get -n mydc -l 1 -d ./logs/

# Download logs in JSON format
aerolab logs get -n mydc -j -d ./logs/
```

### Troubleshooting

```bash
# 1. Show logs to identify issue
aerolab logs show -n mydc

# 2. Download logs for detailed analysis
aerolab logs get -n mydc -d ./logs/

# 3. Analyze logs locally
cat ./logs/mydc-1/aerospike.log | grep ERROR
```

### Monitor Logs

```bash
# Continuously monitor logs
watch -n 5 'aerolab logs show -n mydc'
```

### Export Logs for Support

```bash
# Download all logs
aerolab logs get -n mydc -d ./logs-export/

# Create archive
tar -czf logs-export.tar.gz logs-export/
```

## Tips

1. **JSON format**: Use `-j` for structured log output that's easier to parse
2. **Node filtering**: Use `-l` to download logs from specific nodes
3. **Directory structure**: Logs are organized by cluster name and node number
4. **Analysis**: Download logs for detailed analysis or troubleshooting
5. **Real-time viewing**: Use `logs show` for quick log viewing
6. **Export**: Download logs before cluster destruction for troubleshooting

