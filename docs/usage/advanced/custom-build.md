[Docs home](../../../README.md)

# Custom Aerospike build


## Method 1: Use a prebuilt installer tarball

### Download the custom build and name it according to the following example convention:

```
aerospike-server-enterprise-5.7.0.27-ubuntu20.04.x86_64.tgz
aerospike-server-enterprise-6.2.0.6-amazon2.x86_64.tgz
aerospike-server-enterprise-6.2.0.6-ubuntu20.04.x86_64.tgz
aerospike-server-enterprise-6.2.0.6-centos8.x86_64.tgz
```

In other words, the file needs to be called: `aerospike-server-enterprise-${VERSION}-${OS}${OS_VER}.${ARCH}.tgz`

Version can be any Aerospike version (4 digits separated by dots) with optionally another dot and an RC file designation, for example: `6.4.0.0.rc1`

The operating system must be one of the following: `centos`, `amazon`, `ubuntu`. The operating system version must be the version for which this build is made.

The architecture is either `x86_64` or `arm64`.

### Run Aerospike

Use `aerolab` as normal, specifying the correct version.

For example, given a file `aerospike-server-enterprise-6.2.0.6.rc1-ubuntu20.04.x86_64.tgz`, install Aerospike as:

```
aerolab cluster create -d ubuntu -i 20.04 -v 6.2.0.6.rc1
```

The version is already downloaded, so AeroLab will automatically use it instead of trying to download it.

## Method 2: providing a custom binary

### Create an Aerospike cluster (2 example nodes)

```
aerolab cluster create -d ubuntu -i 20.04 -v 6.2.0.6 -c 2 -s n
```

### Upload a custom Aerospike binary

```
aerolab files upload asd /usr/bin/asd
aerolab attach shell -l all -- chmod 755 /usr/bin/asd
```

### Start Aerospike

```
aerolab aerospike start
```

## Method 3: using a custom installer

Example: deb file

### Create an Aerospike cluster (2 example nodes)

```
aerolab cluster create -d ubuntu -i 20.04 -v 6.2.0.6 -c 2 -s n
```

### Upload a custom Aerospike binary

```
aerolab files upload aerospike.deb /tmp/aerospike.deb
aerolab attach shell -l all -- dpkg -i /tmp/aerospike.deb
```

### Start Aerospike

```
aerolab aerospike start
```
