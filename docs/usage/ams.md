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

## AMS client

### Install AMS client

```
aerolab client create ams -n ams -s dc1,dc2
```

### Reconfigure (update) AMS client with a new list of IPs for given clusters to monitor

```
aerolab client configure ams -n ams -s dc1,dc2
```

## Access

```
aerolab client list
```

Access the client using it's IP and port 3000 in your browser, that is: `http://IP:3000`

Default username and password are: admin/admin
