# Aerolab All-Flash

## Configuring all flash with aerolab - basic usage

### Prepare a template

Configuration template needs to look similar to this (mostly pay attention to the index-type configuration part) - this is just a snippet, the template must have full aerospike.conf - see `templates/` in this repo for examples:

```
namespace bar {
        replication-factor 1
        memory-size 4G
        default-ttl 0

        partition-tree-sprigs 256

        index-type flash {
                mount /mnt
                mounts-size-limit 5G
        }

        storage-engine device {
                file /opt/aerospike/data/bar.dat
                filesize 10G
                data-in-memory false
        }
}
```

### Deploy aerolab with the tamplate

Deploying aerolab with the template for all flash requires a privileged container (deploying 3 nodes in this example, using mesh):

```
aerolab make-cluster -n someName -c 3 --privileged
```

That's it, the cluster will be created and use `index-type flash` on `/mnt`. Note that no mountpoints or filesystems are created. We are simply using a pre-existing `/mnt` directory on the root filesystem. this is fine and works as well for testing purposes.
