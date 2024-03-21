# Docker - Multiarch

AeroLab supports forcing a specific architecture on docker/podman(desktop). This allows running `arm64` images on `amd64` and vice-versa.

In order for this to work, the docker-host must have a `qemu-user-static` dependent library installed. This library performs a live translation of syscalls between incompatible architectures.

Docker Desktop already has this automatically installed in newest versions. No prerequisite actions are reququired for this; scroll down to `AeroLab Usage` section.

## Installing prerequisites

### Updating

Whether using podman/docker/podman-desktop/docker-desktop, please first install all the relevant software updates to ensure the latest stable is installed.

### Enabling

* [Docker Desktop](docker_multiarch_desktop.md)
* [Docker/Podman on linux](docker_multiarch_linux.md)
* [Podman Desktop](docker_multiarch_podman.md)

## AeroLab Usage

By default, `aerolab` will use whichever architecture is native to docker. To force `aerolab` to use a specific architecture, specify it during the backend setup. Once an architecture has been selected, proceed as normal.

Note: this can be switched back and forth as required without any issues.

### Force arm64 builds

```bash
aerolab config backend -t docker -a arm64
```

### Force amd64 builds

```bash
aerolab config backend -t docker -a amd64
```

### Reset to use native builds

```bash
aerolab config backend -t docker -a unset
```
