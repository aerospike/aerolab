# Docker - Multiarch

AeroLab supports forcing a specific architecture on docker/podman(desktop). This allows running `arm64` images on `amd64` and vice-versa.

In order for this to work, the docker-host must have a `qemu-user-static` dependent library installed. This library performs a live translation of syscalls between incompatible architectures.

Docker Desktop already has this automatically installed in newest versions. No prerequisite actions are reququired for this; scroll down to `AeroLab Usage` section.

## Installing prerequisites

### Updating

Whether using podman/docker/podman-desktop/docker-desktop, please first install all the relevant software updates to ensure the latest stable is installed.

### Podman desktop
```bash
podman machine ssh -- rpm-ostree install qemu-user-static
podman machine ssh -- sed -i 's/SELINUX=enforcing/SELINUX=permissive/g' /etc/selinux/config
podman machine ssh -- systemctl reboot
podman machine ssh -- podman run --rm --privileged multiarch/qemu-user-static --reset -p yes
```

### Docker-Desktop

Docker Desktop automatically enables the required `qemu-user-static` and `binfmt` extensions. No actions are required.

### Docker/Podman on Linux

First install `qemu-user-static` package from your repository. For example, on debian/ubuntu:

```bash
apt update && apt -y install qemu-user-static
```

Next, enable emulation for `binfmt` on docker or podman:
* docker - `docker run --rm --privileged multiarch/qemu-user-static --reset -p yes`
* podman - `podman run --rm --privileged multiarch/qemu-user-static --reset -p yes`

## Enabling multiarch support on reboot

NOTE: Do not execute this step on docker desktop. Docker Desktop already has the required extensions auto-enabled.

After system reboot, or restart of podman-desktop, multiarch support must be enabled again, as follows:

* docker - `docker run --rm --privileged multiarch/qemu-user-static --reset -p yes`
* podman - `podman run --rm --privileged multiarch/qemu-user-static --reset -p yes`

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
