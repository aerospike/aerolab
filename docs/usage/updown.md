# Upload and Download Files Between Host and Containers

## Upload a file to Aerospike cluster nodes 1 and 2

```bash
aerolab files upload -n dc1 -l 1,2 file-to-upload.conf /destination/on/nodes/filename.conf
```

## Download a file from cluster node 1

```bash
aerolab files download -n dc1 -l 1 /source/on/node/filename.conf filename-to-download-to.conf
```
