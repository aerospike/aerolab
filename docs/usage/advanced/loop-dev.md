[Docs home](../../../README.md)

# Simulate raw block storage


## Use a loop device to simulate raw block storage

When deploying Aerospike Database on Docker with AeroLab, you can simulate `device` block storage using a loop device.

In this example you will deploy the following setup:

* An Aerospike EE 4.9 cluster called 'source' being an XDR source for a namespace _bar_. The namespace has its data on file storage and XDR on a separately mounted file system.
* An Aerospike EE 4.9 cluster called 'destination' being the XDR destination for the namespace _bar_, with its data on a `device` storage engine. This device storage uses a loop device mounted in privileged mode.

### Deploy the clusters

Deploy clusters, naming them `source` and `destination`, with two nodes in each, using the `bar-file-store.conf` template. Do not start Aerospike automatically, and enter privileged mode.

```bash
$ aerolab cluster create -n source -c 2 -s n -o templates/bar-file-store.conf --privileged -v 4.9.0.32
```

```bash
$ aerolab cluster create -n destination -c 2 -s n -o templates/bar-file-store.conf --privileged -v 4.9.0.32
```

### Configure the destination cluster

#### Create raw empty files to use as storage 1024MB (1GB)

```bash
aerolab attach shell -n destination -l all -- /bin/bash -c 'dd if=/dev/zero of=/store$NODE.raw bs=1M count=1024'
```

#### Loop-mount the files as devices

**Note:** loop device interfaces are global and shared by all containers, as they belong to the Docker host. Therefore they must be given unique names.
Since AeroLab exposes the environment variable `$NODE` to the shell when running the `attach shell` command, you can make use of it to create unique names.

```bash
aerolab attach shell -n destination -l all -- /bin/bash -c 'losetup -f /store$NODE.raw'
```

#### Perform changes in the aerospike.conf file using sed

Change the configuration from `file` to `device` storage,  noting the /dev/loopX device created by `losetup` - you need to find the one that is for this container and use it.

```bash
aerolab attach shell -n destination -l all -- /bin/bash -c 'sed -i "s~file /opt/aerospike/data/bar.dat~device $(losetup --raw |grep store$NODE.raw |cut -d " " -f 1)~g" /etc/aerospike/aerospike.conf'
```

Remove `filesize 1G`

```bash
aerolab attach shell -n destination -l all -- /bin/bash -c 'sed -i "s~filesize 1G~~g" /etc/aerospike/aerospike.conf'
```

Change `data-in-memory` from `true` to `false`

```bash
aerolab attach shell -n destination -l all -- /bin/bash -c 'sed -i "s~data-in-memory true~data-in-memory false~g" /etc/aerospike/aerospike.conf'
```

#### Start Aerospike on the destination cluster and check the logs on node 1

```bash
aerolab aerospike start -n destination

aerolab attach shell -n destination -- cat /var/log/aerospike.log
```

### Connect the source cluster to destination cluster on namespace bar

```bash
$ aerolab xdr connect -s source -d destination -m bar
```

### Configure raw file with a file system, and mount it for use with XDR

#### Create a 100MB file

```bash
aerolab attach shell -n source -l all -- dd if=/dev/zero of=/xdr.raw bs=1M count=100MB
```

#### Create a filesystem in the file

```bash
aerolab attach shell -n source -l all -- mkfs.ext4 /xdr.raw
```

#### Mount

Mount `/xdr/raw` as `/opt/aerospike/xdr` directory

```bash
aerolab attach shell -n source -l all -- mount /xdr.raw /opt/aerospike/xdr
```

#### Start the source Aerospike cluster, and print the logs of node 1

```bash
aerolab aerospike start -n source

aerolab attach shell -n source -- cat /var/log/aerospike.log
```

### ESSENTIAL cleanup

Because loop devices are essentially kernel-level devices, you must stop the Aerospike cluster and clean up the loop device.

**Note:** if you forget this step, stopping Docker and starting it will clear the loop device interfaces on its own anyway.

```bash
aerolab aerospike stop -n destination

aerolab attach shell -n destination -- /bin/bash -c 'losetup --raw |grep store |grep raw |cut -d " " -f 1 |while read line; do losetup -d $line; done'
```

#### Destroy the Docker containers

```bash
aerolab cluster destroy -f -n source
aerolab cluster destroy -f -n destination
```

### Caveats

* Because a loop device is essentially set on the kernel level of the host, stopping and starting Docker will remove loop devices. This setup is only good for testing. If you stop the Docker host, it's easier to redo this manually, including setting up loop devices from scratch.
* The loop devices are visible to any privileged container, and any privileged container can access them. E.g. running `losetup -a` on the source cluster nodes would show those loop devices from destination nodes
