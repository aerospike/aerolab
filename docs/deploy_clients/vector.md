# Launch a Vector (Proximus) client with AeroLab

AeroLab supports installing Vector (Proximus) client to Aerospike.

## Version

AeroLab supports vector version `0.3.1` by default. Version can be overriden using the `--version=...` parameter. Note that due to the current state of Vector development, only the default version has been tested to work with AeroLab.

## Basic usage

### Generate an example aerospike configuration file

Vector has specific 2-or-more-namespaces requirement from Aerospike servers. An example can be generated as follows:

```bash
aerolab conf generate
```

Tick the `vector` checkbox and optionally the `on-disk` checkbox for the vector namespace. Press `CTRL+X` to save as `aerospike.conf`.

### Create an aerospike cluster

In this example, create `2` nodes, specifying `GCP` details. Use the generated `aerospike.conf`.

```bash
aerolab cluster create -n vectordb -c 2  -o aerospike.conf --zone us-central1-a --instance e2-standard-4
```

### Create a vector client machine

```bash
aerolab client create vector -n vector -C vectordb --confirm --zone us-central1-a --instance e2-standard-4
```

### Other options

Vector-specific command-line parameters are:
```
-C, --cluster-name=        cluster name to seed from (default: mydc)
    --seed=                specify a seed IP:PORT instead of providing a ClusterName; if this parameter is provided, ClusterName is ignored
    --listen=              specify a listen IP:PORT for the service (default: 0.0.0.0:5555)
    --no-touch-listen      set this to prevent aerolab from touching the service: configuration part
    --no-touch-seed        set this to prevent aerolab from configuring the aerospike seed ip and port
    --no-touch-advertised  set this to prevent aerolab from configuring the advertised listeners
    --version=             vector version to install; only 0.3.1 is officially supported by aerolab (0.3.1-1 for rpm) (default: 0.3.1)
    --custom-conf=         provide a custom aerospike-proximus.yml to ship
    --no-start             if set, service will not be started after installation
-f, --featurefile=         Features file to install; if not provided, the features.conf from the seed cluster will be taken
    --metans=              configure the metadata namespace name (default: proximus-meta)
    --confirm              set this parameter to confirm any warning questions without being asked to press ENTER to continue
```

### Usage

See [this link](https://github.com/aerospike/aerospike-proximus-client-python) for official aerospike proximus python client.

See [this link](https://github.com/aerospike/proximus-examples) for proximus usage examples.
