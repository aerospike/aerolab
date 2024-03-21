# Podman Desktop Multiarch

## Enable

Execute the below to enable multiarch support:
```bash
podman machine ssh -- sudo rpm-ostree install qemu-user-static
podman machine ssh -- sudo sed -i 's/SELINUX=enforcing/SELINUX=permissive/g' /etc/selinux/config
podman machine ssh -- sudo systemctl reboot
```

Once podman desktop restarts, everything should work. Just start using aerolab with arch choice:
```bash
aerolab config backend -t docker -a arm64
...
aerolab config backend -t docker -a amd64
...
```

## Troubleshooting

Some older versions of Podman Desktop do not enable by default. In such cases, manualy enablement may be required:

```bash
podman machine ssh
sudo curl -o /root/qemu-binfmt-conf.sh https://raw.githubusercontent.com/qemu/qemu/master/scripts/qemu-binfmt-conf.sh
sudo find /proc/sys/fs/binfmt_misc -type f -name 'qemu-*' -exec sh -c 'echo -1 > {}' \;
sudo bash /root/qemu-binfmt-conf.sh --qemu-suffix "-static" --qemu-path /usr/bin -p yes
exit
```

In such cases, this will have to be repeated every time podman desktop is restarted to re-enable emulation.
