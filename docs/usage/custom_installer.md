# Providing a custom build to aerolab

## Download the custom build and name it according to the below example convention:

```
aerospike-server-enterprise-5.7.0.27-ubuntu20.04.x86_64.tgz
aerospike-server-enterprise-6.2.0.6-amazon2.x86_64.tgz
aerospike-server-enterprise-6.2.0.6-ubuntu20.04.x86_64.tgz
aerospike-server-enterprise-6.2.0.6-centos8.x86_64.tgz
```

In other words, the file needs to be called: `aerospike-server-enterprise-${VERSION}-${OS}${OS_VER}.${ARCH}.tgz`

Version can be any Aerospike version (4 digits separated by dots) with optionally another dot and an rc file designation, for example: `6.4.0.0.rc1`

The operating system has to be one of: `centos`, `amazon`, `ubuntu` and the operating system version must be the version for which this build is made.

The architecture is either `x86_64` or `amd64`.

## Run aerospike

Once the file is downloaded and named accordingly, use `aerolab` as normal, specifying the correct version.

For example, given a file `aerospike-server-enterprise-6.2.0.6.rc1-ubuntu20.04.x86_64.tgz`, install Aerospike as:

```
aerolab cluster create -d ubuntu -i 20.04 -v 6.2.0.6.rc1
```

As the version is already downloaded, aerolab will automatically use it instead of trying to download it.
