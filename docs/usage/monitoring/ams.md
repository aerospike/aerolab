[Docs home](../../../README.md)

# Deploy the Aerospike Monitoring Stack

## Deploy the Aerospike Monitoring Stack and Prometheus exporter

The [Aerospike Monitoring Stack](/monitorstack) (AMS)
provides a monitoring dashboard and alerting through Prometheus, Grafana and the
Aerospike Prometheus exporter.

### Create a Cluster
In this example you'll create two Aerospike Database clusters, each with three nodes.

```
aerolab cluster create -n dc1 -c 3
aerolab cluster create -n dc2 -c 3
```

### Install the Prometheus exporter

Add an Aerospike Prometheus exporter agent for each node of both clusters.
```
aerolab cluster add exporter -n dc1,dc2
```

#### Add a custom config for the exporter

You can add a [custom](/monitorstack/configure/configure-exporter) `ape.toml`,
for example when using authentication or TLS to talk to the Aerospike cluster, like so:

```
aerolab cluster add exporter -n dc1,dc2 -o /path/to/ape.toml
```

###### Upgrading the exporter

You can execute the `cluster add exporter` command on the same cluster multiple times.
This will result in the exporter being upgraded to the latest version with a new `ape.toml` exporter configuration file.

### Add an AMS client

#### Install the AMS client - AWS, Docker on Linux

```
aerolab client create ams -n ams -s dc1,dc2
```

#### Install the AMS client - Docker Desktop

```
aerolab client create ams -n ams -s dc1,dc2 -e 3000:3000
```

### Update the AMS client's list of node IPs to monitor in a given cluster

You can reconfigure the same AMS client to monitor an updated list of node IPs in a cluster. This command avoids recreating the AMS client machine.

```
aerolab client configure ams -n ams -s dc1,dc2
```

### Access the monitoring dashboard

```
aerolab client list
```

You can access the AMS client's monitoring dashboard using its IP address and port 3000 in your browser.

For AWS, Docker-Laptop and Docker on Linux, that is: `http://<IP>:3000`

For Docker Desktop, use: `http://127.0.0.1:3000`

The default username and password are: `admin/admin`
