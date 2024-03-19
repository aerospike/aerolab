[Docs home](../../../README.md)

# Deploy an all-flash namespace


### Configuring All Flash with AeroLab

#### Prepare a template for the All Flash namespace

The following example is a snippet of the configuration template you need. Pay attention to the
`index-type` sub-section. Your template must be a complete aerospike.conf. You can generate a sample
configuration file with `aerolab conf generate`.

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

#### Deploy AeroLab with the All Flash template

Deploying AeroLab with your All Flash template requires a privileged container. In this example
you will be deploying a cluster with three nodes, using mesh:

```bash
aerolab cluster create -n someName -o template.conf -c 3 --privileged
```

That's it, the cluster will be created, with the primary index of the namespace stored on `/mnt`.
Note that in this example no mount points or file systems are created. We are simply using a
pre-existing `/mnt` directory on the root filesystem, which works well for testing purposes.
