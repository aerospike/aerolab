# Cluster Commands

## Create a 2-node cluster

### Newest version of aerospike

```bash
aerolab cluster create -c 2 -n mycluster
```

### Pick a version

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

## List clusters, templates and IPs

```bash
aerolab cluster list
```

## Add 2 more nodes to the existing cluster

```bash
aerolab cluster grow -c 2 -n mycluster
```

## Fix mesh heartbeat configuration in aerospike cluster

```bash
aerolab conf fix-mesh -n mycluster
```

## Destroy the cluster

```bash
aerolab cluster destroy -n mycluster -f
```

## Create a 2-node cluster, shipping own arospike.conf and with a custom version

```bash
aerolab cluster create -c 2 -n mycluster -v 4.9.0.32 -o my-aerospike-conf-template.conf
```

## Stop node 2

```bash
aerolab cluster stop -n mycluster -l 2
```

## Destroy node 2

```bash
aerolab cluster destroy -n mycluster -l 2
```

## Destroy template image

```bash
aerolab cluster list
aerolab template destroy -v 4.9.0.32 -d all -i all
```

## Destroy all aerolab template images

```bash
aerolab template destroy -v all -d all -i all
```
