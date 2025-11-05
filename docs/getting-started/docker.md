# Getting Started with Docker Backend

The Docker backend is the quickest way to get started with Aerolab. It requires Docker, Docker Desktop, Podman, or Podman Desktop to be installed and running.

## Prerequisites

Install one of the following container runtimes:

- **Docker** - [Install Docker](https://docs.docker.com/get-docker/)
- **Docker Desktop** - [Install Docker Desktop](https://www.docker.com/products/docker-desktop/)
- **Podman** - [Install Podman](https://podman.io/getting-started/installation)
- **Podman Desktop** - [Install Podman Desktop](https://podman.io/getting-started/installation)

Verify installation:

```bash
# For Docker
docker --version

# For Podman
podman --version
```

## Initial Setup

### 1. Configure Docker Backend

Configure Aerolab to use the Docker backend:

```bash
aerolab config backend -t docker
```

This will:
- Set Docker as the default backend
- Prepare the local environment for cluster management

### Optional: Enable Inventory Cache

If you're not sharing Docker resources with other users, you can enable inventory caching for faster operations:

```bash
aerolab config backend -t docker --inventory-cache
```

### Optional: Set Docker Architecture

If you need to force a specific architecture (amd64 or arm64):

```bash
aerolab config backend -t docker --docker-arch amd64
```

### 2. Verify Configuration

Check your backend configuration:

```bash
aerolab config backend
```

You should see:
```
Config.Backend.Type = docker
Config.Backend.SshKeyPath = ${HOME}/.config/aerolab
```

### 3. Clean Up Existing Resources (Optional)

If you have existing Aerolab resources, clean them up:

```bash
aerolab inventory delete-project-resources -f
```

## Creating Your First Cluster

### Basic Cluster Creation

Create a simple 2-node cluster:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*'
```

This command:
- Creates 2 nodes (`-c 2`)
- Uses Ubuntu 24.04 (`-d ubuntu -i 24.04`)
- Installs Aerospike version 8.x (`-v '8.*'`)
- Default cluster name is `mydc`
- Auto-starts Aerospike after creation

### Custom Cluster Name

Specify a custom cluster name:

```bash
aerolab cluster create -n mycluster -c 2 -d ubuntu -i 24.04 -v '8.*'
```

### Create Without Auto-Start

If you want to manually start Aerospike later:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' --start n
```

### List Available Versions

List available Aerospike versions:

```bash
aerolab installer list-versions
```

### List Your Clusters

Check what clusters exist:

```bash
aerolab cluster list
```

### View All Resources

View all clusters, instances, and images:

```bash
aerolab inventory list
```

## Starting and Stopping Clusters

### Start Cluster

Start all nodes in the cluster:

```bash
aerolab cluster start
```

Or start specific nodes:

```bash
aerolab cluster start -n mydc -l 1-2
```

### Stop Cluster

Stop all nodes:

```bash
aerolab cluster stop
```

Or stop specific nodes:

```bash
aerolab cluster stop -n mydc -l 1-2
```

## Managing Aerospike Service

### Start Aerospike

Start the Aerospike service on all nodes:

```bash
aerolab aerospike start
```

### Stop Aerospike

Stop the Aerospike service:

```bash
aerolab aerospike stop
```

### Restart Aerospike

Restart the Aerospike service:

```bash
aerolab aerospike restart
```

### Check Status

Check Aerospike service status:

```bash
aerolab aerospike status
```

### Wait for Cluster Stability

Wait for the cluster to become stable:

```bash
aerolab aerospike is-stable -w
```

With timeout (30 seconds):

```bash
aerolab aerospike is-stable -w -o 30
```

## Connecting to Nodes

### Shell Access

Connect to a node via shell:

```bash
aerolab attach shell -n mydc -l 1
```

Or run a command:

```bash
aerolab attach shell -n mydc -l 1 -- ls /tmp
```

### Aerospike Tools

Use Aerospike command-line tools:

```bash
# AQL (Aerospike Query Language)
aerolab attach aql -n mydc -- -c "show namespaces"

# asinfo
aerolab attach asinfo -n mydc -- -v "cluster-stable"

# asadm (Aerospike Admin)
aerolab attach asadm -n mydc -- -e info
```

## File Operations

### Upload Files

Upload a file to nodes:

```bash
aerolab files upload -n mydc local-file.txt /tmp/remote-file.txt
```

### Download Files

Download files from nodes:

```bash
aerolab files download -n mydc /tmp/remote-file.txt ./local-dir/
```

### Sync Files

Sync files across nodes in a cluster:

```bash
aerolab files sync -n mydc -l 1 /tmp/file.txt
```

This syncs the file from node 1 to all other nodes.

## Configuration Management

### View Configuration

View the current Aerospike configuration:

```bash
aerolab attach shell -n mydc -- cat /etc/aerospike/aerospike.conf
```

### Adjust Configuration Parameters

Modify configuration parameters:

```bash
aerolab conf adjust set network.heartbeat.interval 250
```

### Fix Mesh Configuration

Automatically fix mesh configuration:

```bash
aerolab conf fix-mesh
```

### Set Rack IDs

Assign rack IDs to nodes:

```bash
aerolab conf rackid -l 1-2 -i 1
aerolab conf rackid -l 3-4 -i 2
```

## Cleanup

### Destroy a Cluster

Destroy a cluster and all its resources:

```bash
aerolab cluster destroy -n mydc --force
```

### Clean Up All Resources

Remove all Aerolab resources:

```bash
aerolab inventory delete-project-resources -f
```

### Clean Up Docker Networks

Clean up unused Docker networks:

```bash
aerolab config docker prune-networks
```

## Common Workflows

### Complete Workflow Example

```bash
# 1. Configure backend
aerolab config backend -t docker

# 2. Create cluster
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*'

# 3. Wait for cluster stability
aerolab aerospike is-stable -w

# 4. Check status
aerolab aerospike status

# 5. Configure rack IDs
aerolab conf rackid -l 1 -i 1
aerolab conf rackid -l 2 -i 2
aerolab conf rackid -l 3 -i 3

# 6. Restart to apply changes
aerolab aerospike restart -n mydc

# 7. Wait for stability again
aerolab aerospike is-stable -w

# 8. Use the cluster
aerolab attach aql -n mydc -- -c "show namespaces"

# 9. Clean up when done
aerolab cluster destroy -n mydc --force
```

## Troubleshooting

### Docker Not Running

If you see connection errors, ensure Docker is running:

```bash
docker ps
```

### Permission Issues

If you see permission denied errors, add your user to the docker group:

```bash
sudo usermod -aG docker $USER
```

Then log out and log back in.

### Network Issues

If nodes can't communicate, check Docker networks:

```bash
docker network ls
aerolab config docker list-networks
```

### Clean Up Failed Resources

If cluster creation fails, clean up:

```bash
aerolab inventory delete-project-resources -f
```

## Next Steps

- Explore [cluster management commands](commands/cluster.md)
- Learn about [Aerospike daemon controls](commands/aerospike.md)
- Check out [configuration management](commands/conf.md)
- See [advanced features](commands/) for more options

