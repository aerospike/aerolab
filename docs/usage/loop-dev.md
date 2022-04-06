# Deploying clusters on docker with device storage engine and limiting storage for xdr digestlog with a filesystem

## The aim

This article descibes how to:

* configure a cluster to use device instead of file while working on docker
* configure a cluster where xdr digestlog is on a separate filesystem while working on docker

We achieve this by deploying the following setup:

* cluster 'source' being xdr source for namespace bar, with namespace being file-backed and xdr on a separate mounted filesystem (filesystem-in-file)
* cluster 'destination' being the xdr destination for namespace bar, with storage engine 'device', using loopback device mounts in provileged mode

## Deploy

### Deploy both clusters

Deploy clusters, naming them `source` and `destination`, with 2 nodes in each, using the `bar-file-store.conf` template. Do not start aerospike automatically, and enter privileged mode.

```bash
$ aerolab make-cluster -n source -c 2 -s n -o templates/bar-file-store.conf --privileged -v 4.9.0.32
May 20 13:08:29+0000 AERO-LAB[60638]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible
May 20 13:08:29+0000 AERO-LAB[60638]: INFO     Checking if version template already exists
May 20 13:08:29+0000 AERO-LAB[60638]: INFO     Checking aerospike version
May 20 13:08:41+0000 AERO-LAB[60638]: INFO     Starting deployment
May 20 13:08:43+0000 AERO-LAB[60638]: INFO     Done
```

```bash
$ aerolab make-cluster -n destination -c 2 -s n -o templates/bar-file-store.conf --privileged -v 4.9.0.32
May 20 13:09:13+0000 AERO-LAB[60662]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible
May 20 13:09:13+0000 AERO-LAB[60662]: INFO     Checking if version template already exists
May 20 13:09:13+0000 AERO-LAB[60662]: INFO     Checking aerospike version
May 20 13:09:25+0000 AERO-LAB[60662]: INFO     Starting deployment
May 20 13:09:27+0000 AERO-LAB[60662]: INFO     Done
```

## Configure destination cluster

### Create raw empty files to use as storage 1024MB (1GB)

```bash
aerolab node-attach -n destination -l all -- /bin/bash -c 'dd if=/dev/zero of=/store$NODE.raw bs=1M count=1024'
```

### Loop-mount the files as devices

NOTE: loopback interfaces are global and shared by all containers, as they belong to the docker host. Therefore naming must be unique.

Note that aerolab exposes environment variable `$NODE` to the shell when running the `node-attach` command. We can make use of that to create unique names.

```bash
aerolab node-attach -n destination -l all -- /bin/bash -c 'losetup -f /store$NODE.raw'
```

### Perform changes in the aerospike.conf file using sed one-liners

Change the file to device backing,  noting the /dev/loopX device created by losetup - need to find the one that is for this container and use it.

```bash
aerolab node-attach -n destination -l all -- /bin/bash -c 'sed -i "s~file /opt/aerospike/data/bar.dat~device $(losetup --raw |grep store$NODE.raw |cut -d " " -f 1)~g" /etc/aerospike/aerospike.conf'
```

Remove `filesize 1G`

```bash
aerolab node-attach -n destination -l all -- /bin/bash -c 'sed -i "s~filesize 1G~~g" /etc/aerospike/aerospike.conf'
```

Change `data-in-memory` from `true` to `false`

```bash
aerolab node-attach -n destination -l all -- /bin/bash -c 'sed -i "s~data-in-memory true~data-in-memory false~g" /etc/aerospike/aerospike.conf'
```

### Start aerospike on the destination and check logs on node 1

```bash
aerolab start-aerospike -n destination

aerolab node-attach -n destination -- cat /var/log/aerospike.log
```

## Connect source cluster to destination on namespace bar

```bash
$ aerolab xdr-connect -s source -d destination -m bar
May 20 14:07:14+0000 AERO-LAB[63831]: INFO     XdrConnect running
May 20 14:07:16+0000 AERO-LAB[63831]: INFO     Done, now restart the source cluster for changes to take effect.
```

## Configure raw file with a filesystem and mount to use in xdr

### Create file 100MB large

```bash
aerolab node-attach -n source -l all -- dd if=/dev/zero of=/xdr.raw bs=1M count=100MB
```

### Create filesystem in file

```bash
aerolab node-attach -n source -l all -- mkfs.ext4 /xdr.raw
```

### Mount

Mount `/xdr/raw` as `/opt/aerospike/xdr` directory

```bash
aerolab node-attach -n source -l all -- mount /xdr.raw /opt/aerospike/xdr
```

### Start aerospike and cat logs of node 1

```bash
aerolab start-aerospike -n source

aerolab node-attach -n source -- cat /var/log/aerospike.log
```

## Cleanup - ESSENTIAL

### Because loopback is essentially a kernel-level device, stop aerospike and cleanup loopback

NOTE: if you forget this step, stopping docker and starting it will clear loopback interfaces on it's own anyways

```bash
aerolab stop-aerospike -n destination

aerolab node-attach -n destination -- /bin/bash -c 'losetup --raw |grep store |grep raw |cut -d " " -f 1 |while read line; do losetup -d $line; done'
```

### Destroy containers

```bash
aerolab cluster-destroy -f -n source
aerolab cluster-destroy -f -n destination
```

## Caveats

* Because loopback is essentially set on kernel level of the host, stopping and starting docker will remove loopback devices. This setup is only good for testing. If you stop docker host, it's easier to redo this and manually re-setup loopback devices
* The loopback devices are visible to any privileged container and any privileged container can access them. E.g. running `losetup -a` on source cluster nodes would show those loop devices from destination nodes
