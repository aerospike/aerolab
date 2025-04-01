# Launch a Vector client with AeroLab

AeroLab supports installing Vector client to Aerospike.

## Basic usage

### Generate a vector config file for aerospike

Run the below command, scroll down to the "Vector namespace" section, select it, and the "on-disk storage" under it. Then press CTRL+X to save the file.

```bash
aerolab conf generate -f vector.conf
```

### Create an aerospike cluster

In this example, create `2` nodes, specifying `GCP` details.

```bash
aerolab cluster create -n vectordb -c 2 --zone us-central1-a --instance e2-standard-4 -o vector.conf
```

### Create a vector client machine

```bash
aerolab client create vector -n vector -C vectordb --confirm --zone us-central1-a --instance e2-standard-4
```

### Other options

The following vector-specific command-line parameters apply to your vector cluster:
```
-C, --cluster-name=        aerospike cluster name to seed from (default: mydc)
    --seed=                specify an aerospike cluster seed IP:PORT instead of providing a ClusterName; if this parameter is provided, ClusterName is ignored
    --listen=              specify a listen IP:PORT for the service (default: 0.0.0.0:5555)
    --no-touch-listen      set this to prevent aerolab from touching the service: configuration part
    --no-touch-seed        set this to prevent aerolab from configuring the aerospike seed ip and port
    --no-touch-advertised  set this to prevent aerolab from configuring the advertised listeners
    --custom-conf=         provide a custom aerospike-vector-search.yml to ship
    --no-start             if set, service will not be started after installation
-f, --featurefile=         Features file to install; if not provided, the features.conf from the seed aerospike cluster will be taken
    --metans=              configure the metadata namespace name (default: test)
    --confirm              set this parameter to confirm any warning questions without being asked to press ENTER to continue
```

### Usage

The vector client is best paired with a set of [examples](https://github.com/aerospike/aerospike-vector) you can utilize.
These are already cloned into `/root/aerospike-vector/` for your convenience.

## Full example

The below example is for Docker. For GCP/AWS add the appropriate `--zone=`, `--instance=`, `--instance-type=` to all the cluster/client `create` commands. See `aerolab cluster create help` and `aerolab client create help` for more details.

### Installation:

```bash
# generate a config file
aerolab conf generate -f vector.conf

# create aerospike cluster
aerolab cluster create -n vectordb -f vector.conf

# add exporter to aerospike cluster for monitoring
aerolab cluster add exporter -n vectordb 

# create vector client and configure it to use the aerospike cluster
aerolab client create vector -n vector -C vectordb --confirm

# create AMS monitoring stack, configure to monitor cluster and client, exposing prometheus port in docker
aerolab client create ams -n ams --clusters=vectordb --vector=vector -e 9090:9090
```

### Destroy:

```bash
aerolab cluster destroy -f -n vectordb
aerolab client destroy -f -n ams,vector
```
