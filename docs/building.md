# Building AeroLab manually

### Build Prerequisites:

* golang build environment
* `make` package for handling Makefiles
* (optional) `upx` package for shrinking resulting binaries

### Packaging Prerequisites:

* `dpkg` package providing `dpkg-deb` command
* `rpmbuild` package providing `rpmbuild` command
* `zip` package providing `zip` command

### Clone

#### Master branch:
```
git clone https://github.com/aerospike/aerolab.git
```

#### Another branch
```
git clone -b v6.2.0 https://github.com/aerospike/aerolab.git
```

### Get build help

```
cd aerolab/src
make
```

### Make usage

```
SHORTHANDS:
        make build install                         - build and install on current system
        make build-linux pkg-linux                 - build and package all linux releases

BUILD COMMANDS:
        build              - A version for the current system
        buildall           - All versions for all supported systems
        build-linux-amd64  - Linux on x86_64
        build-linux-arm64  - Linux on aarch64
        build-linux        - Linux on x86_64 and aarach64
        build-darwin-amd64 - MacOS on x86_64
        build-darwin-arm64 - MacOS on M1/M2 style aarach64
        build-darwin       - MacOS on x86_64 and aarch64

INSTALL COMMANDS:
        install            - Install a previously built aerolab on the current system

CLEAN COMMANDS:
        clean              - Remove remainders of a build and reset source modified during build

PACKAGING COMMANDS:
        pkg-linux          - Package all linux packages - zip, rpm and deb
        pkg-zip            - Package linux zip
        pkg-rpm            - Package linux rpm
        pkg-deb            - Package linux deb
        pkg-zip-amd64      - Package linux zip for amd64 only
        pkg-rpm-amd64      - Package linux rpm for amd64 only
        pkg-deb-amd64      - Package linux deb for amd64 only
        pkg-zip-arm64      - Package linux zip for arm64 only
        pkg-rpm-arm64      - Package linux rpm for arm64 only
        pkg-deb-arm64      - Package linux deb for arm64 only

OUTPUTS: ../bin/ and ../bin/packages/
```
