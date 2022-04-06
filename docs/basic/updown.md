# Upload and Download files between host and containers

Aerolab provides a somewhat simplistic way to upload and download a single file. For a more advanced or recursive way, use docker's `cp` option instead.

## Upload file to nodes 1 and 2

```bash
aerolab upload -n dc1 -l 1,2 -i file-to-upload.conf -o /destination/on/nodes/filename.conf
```

## Download file from node 1

```bash
aerolab download -n dc1 -l 1 -i /source/on/node/filename.conf -o filename-to-download-to.conf
```
