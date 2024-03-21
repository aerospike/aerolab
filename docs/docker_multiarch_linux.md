# Multiarch on docker/podman on linux (not desktop versions)

## Enable

First install `qemu-user-static` package from your repository. For example, on debian/ubuntu:

```bash
apt update && apt -y install qemu-user-static
```

Next, enable emulation for `binfmt` on docker or podman:
```bash
sudo curl -o /root/qemu-binfmt-conf.sh https://raw.githubusercontent.com/qemu/qemu/master/scripts/qemu-binfmt-conf.sh
sudo find /proc/sys/fs/binfmt_misc -type f -name 'qemu-*' -exec sh -c 'echo -1 > {}' \;
sudo bash /root/qemu-binfmt-conf.sh --qemu-suffix "-static" --qemu-path /usr/bin -p yes
```

NOTE that the last 2 commands in the enablement section will have to be repeated on reboot to re-enable emulation.

NOTE that some distros may have the `binfmt` configuration tool as a package. Check with your OS documentation.

## Usage

Simply switch your arch in aerolab with:

```bash
aerolab config backend -t docker -a arm64
...
aerolab config backend -t docker -a amd64
...
```
