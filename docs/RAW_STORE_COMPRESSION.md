# Compression and raw storage using Aerolab

## Just command dump below

Lines with `%` are on host, with `$` are in aerolab docker container.

```
% aerolab make-cluster -c 1 --privileged
% aerolab node-attach

$ dd if=/dev/zero of=/store.raw bs=1M count=1024
$ losetup -f /store.raw
$ losetup --raw |grep store.raw |cut -d " " -f 1
/dev/loop0
$ vi /etc/aerospike/aerospike.conf
...
namespace bar {
        replication-factor 2
        memory-size 4G
        default-ttl 0
        storage-engine device {
                device /dev/loop0
                data-in-memory false
                write-block-size 8M
                compression zstd
                compression-level 9
        }
}
...

$ service aerospike restart
$ asadm -e "show config like compress"
$ aql
INSERT INTO bar.setA (PK, a) VALUES ('xyz1', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setB (PK, a) VALUES ('xyz2', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setA (PK, a) VALUES ('xyz3', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setB (PK, a) VALUES ('xyz4', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setA (PK, a) VALUES ('xyz5', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setB (PK, a) VALUES ('xyz6', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setA (PK, a) VALUES ('xyz7', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setB (PK, a) VALUES ('xyz8', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setA (PK, a) VALUES ('xyz9', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setB (PK, a) VALUES ('xyz10', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setA (PK, a) VALUES ('xyz11', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
INSERT INTO bar.setB (PK, a) VALUES ('xyz12', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')

$ asinfo -l -v namespace/bar | grep compression
device_compression_ratio=0.625
record_proto_compression_ratio=1.000
scan_proto_compression_ratio=1.000
query_proto_compression_ratio=1.000
storage-engine.compression=zstd
storage-engine.compression-level=9

$ losetup --raw |grep store |grep raw |cut -d " " -f 1 |while read line; do losetup -d $line; done
$ exit

% aerolab cluster-destroy -f
```
