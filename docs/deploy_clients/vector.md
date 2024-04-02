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

The following vector-specific command-line parameters apply to your proximus cluster:
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

The vector client comes pre-installed with a set of [examples](https://github.com/aerospike/proximus-examples) you can utilize.
These are located in the /opt/ director and be invoked with the following steps.

Or develop your own application using the aerospike [vector python client](https://github.com/aerospike/aerospike-proximus-client-python).

### Example image search

Install the prism example image search application:

```bash
aerolab client attach -n vector -- /bin/bash /opt/prism-example.sh
```

Once installed, exit the application by using CTRL+C. Then upload images:

```bash
aerolab files upload -c -n vector ~/Pictures /opt/proximus-examples/prism-image-search/prism/static/images/data/
```

Start the web server again:

```bash
aerolab client attach -n vector -- /bin/bash /opt/prism-example.sh
```

### Accessing the webserver

On docker, access `http://127.0.0.1:8998`.

On cloud desployments, run `aerolab client list` and then access `http://EXTERNAL_IP:8080`.

### Running the prism example in tbe background

```bash
$ aerolab client attach -n vector
echo "nohup /bin/bash /opt/prism-example.sh >> /var/log/prism.log 2>&1 &" > /opt/autoload/15-prism
chmod 755 /opt/autoload/15-prism
exit
$ aerolab client attach -n vector --detach -- /bin/bash /opt/autoload/15-prism
```

As the script is in `/opt/autoload`, prism will also be auto-started whenever aerolab starts the client machine.
