# Launch a Graph client VM with AeroLab

AeroLab supports installing Graph client to Aerospike. This is the Aerospike Gremlin server (TinkerPop). The `gremlin console` is not included but can be easily started on the graph servers or locally in docker.

## Basic usage

### Install Graph with Aerolab

Once Aerospike Cluster is installed and configured, deploy Graph servers as follows, editing the cluster name and namespace to your needs.

```bash
aerolab client create graph -n graph --count 1 --cluster-name mydc --namespace test
```

### Other options

The other notable options are:
```
-e, --extra=                     extra properties to add; can be specified multiple times; ex: -e 'aerospike.client.timeout=2000'
    --ram-mb=                    manually specify amount of RAM MiB to use
```

If using a non-standard graph docker image, the following parameters can be utilized:
```
--graph-image=               graph is installed using docker images; docker image to use for graph installation (default: aerospike/aerospike-graph-service)
--docker-user=               login to docker registry for graph installation
--docker-pass=               login to docker registry for graph installation
--docker-url=                login to docker registry for graph installation
```

### Connecting

To connect to Graph:
1. Get the Private IP of the Graph machine using `aerolab client list`
2. If using AWS/GCP, run `aerolab attach client -n graph` to attach the graph instance shell first. On docker, skip this step.
3. Run the gremlin console using a docker container: `docker run -it --rm tinkerpop/gremlin-console`

See [this page](https://aerospike.com/docs/graph/getting-started/basic-usage) for Aerospike-Graph usage instructions.

### See help for all options

```bash
aerolab client create graph help
```

## Full Example on GCP

```bash
# setup - backend, cluster, graph client, AMS
aerolab config backend -t gcp -o myproject
aerolab cluster create -n fire -v 6.4.0.11 -c 2 --instance e2-standard-2 --zone us-central1-a --firewall=bob
aerolab cluster add exporter -n fire
aerolab client create graph -n fly -m test -C fire --instance e2-standard-2 --zone us-central1-a --firewall=bob
aerolab client create ams -n eyes --clusters=fire --clients=fly --instance e2-standard-2 --zone us-central1-a --firewall=bob

# attach to shell
aerolab attach client -n fly

# attach to gremlin console
aerolab attach client -n fly -- docker run -it --rm tinkerpop/gremlin-console

# visit the webui
aerolab webui

# example create a dedicated gremlin-console client
aerolab client create base -n mygremlin
aerolab client attach -n mygremlin -- curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
aerolab client attach -n mygremlin -- bash /tmp/get-docker.sh
aerolab client attach -n mygremlin -- docker run -it --rm tinkerpop/gremlin-console

# destroy everything
aerolab cluster destroy -f -n fire; aerolab client destroy -f -n fly; aerolab client destroy -f -n eyes
```
