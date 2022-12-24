# Cluster Commands

AeroLab `cluster` commands deal with provisioning and destroying Aerospike
machines, whether they're on Docker containers or AWS VMs.

## Create a two node Aerospike cluster

### Use the newest version of Aerospike Database

```bash
aerolab cluster create -c 2 -n mycluster
```

### Pick a specific version of Aerospike

```bash
aerolab cluster create -c 2 -n mycluster -v 4.9.0.32
```

## Stop a cluster

```bash
aerolab cluster stop -n mycluster
```

## Start an existing cluster

```bash
aerolab cluster start -n mycluster
```

## List clusters, templates and IP addresses

```bash
aerolab cluster list
```

## Add two more nodes to an existing cluster

```bash
aerolab cluster grow -c 2 -n mycluster
```

## Fix the mesh heartbeat configuration in an Aerospike cluster

```bash
aerolab conf fix-mesh -n mycluster
```

## Destroy the cluster

```bash
aerolab cluster destroy -n mycluster -f
```

## Create a two node cluster with a custom version, and passing in an aerospike.conf file

```bash
aerolab cluster create -c 2 -n mycluster -v 4.9.0.32 -o my-aerospike-conf-template.conf
```

## Stop cluster node 2

```bash
aerolab cluster stop -n mycluster -l 2
```

## Destroy cluster node 2

```bash
aerolab cluster destroy -n mycluster -l 2
```

## Destroy a template image

```bash
aerolab cluster list
aerolab template destroy -v 4.9.0.32 -d all -i all
```

## Destroy all AeroLab template images

```bash
aerolab template destroy -v all -d all -i all
```
