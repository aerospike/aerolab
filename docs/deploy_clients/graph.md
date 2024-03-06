# Launch a Graph client VM with AeroLab

AeroLab supports installing Graph client to Aerospike. This is the Aerospike Gremlin server (TinkerPop). The `gremlin console` is not included but can be easily started on the graph servers or locally in docker.

## Install Graph with Aerolab

Once Aerospike Cluster is installed and configured, deploy Graph servers as follows, editing the cluster name and namespace to your needs.

```bash
aerolab client create graph -n graph --count 1 --cluster-name mydc --namespace test
```

## Other options

The other 2 notable options are:
```
-e, --extra=                     extra properties to add; can be specified multiple times; ex: -e 'aerospike.client.timeout=2000'
    --ram-mb=                    manually specify amount of RAM MiB to use; default-docker: 4G; default-cloud: 90pct

```

## Connecting

To connect to Graph:
1. Get the Private IP of the Graph machine using `aerolab client list`
2. If using AWS/GCP, run `aerolab attach client -n graph` to attach the graph instance shell first. On docker, skip this step.
3. Run the gremlin console using a docker container: `docker run -it --rm tinkerpop/gremlin-console`

See [this page](https://aerospike.com/docs/graph/getting-started/basic-usage) for Aerospike-Graph usage instructions.

## See help for all options

```bash
aerolab client create graph help
```
