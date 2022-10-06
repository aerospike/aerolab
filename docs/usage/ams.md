# Deploying AMS and the Prometheus Exporter

## Cluster side

### Create 2 clusters, 3 nodes each

```
aerolab cluster create -n dc1 -c 3
aerolab cluster create -n dc2 -c 3
```

### Install exporter

```
aerolab cluster add exporter -n dc1,dc2
```

#### Custom config

If a custom `ape.toml` is required (for example when using authentication or TLS on aerospike server), this can also be specified, like so:

```
aerolab cluster add exporter -n dc1,dc2 -o /path/to/ape.toml
```

#### Upgrading

The `cluster add exporter` command can be executed on the same clusters multiple times and will result in an upgraded expoter to the latest version and a new `ape.toml`

## AMS client

### Install AMS client

```
aerolab client create ams -n ams -s dc1,dc2
```

### Reconfigure (update) AMS client with a new list of IPs for given clusters to monitor

It is possible to reconfigure the same AMS client to add/remove cluster nodes and/or whole clusters without having to recreate the AMS client machine.

```
aerolab client configure ams -n ams -s dc1,dc2
```

## Access

```
aerolab client list
```

Access the client using it's IP and port 3000 in your browser, that is: `http://IP:3000`

Default username and password are: admin/admin
