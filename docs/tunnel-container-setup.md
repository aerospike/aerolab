# Tunnel Container Setup

## Description

This container will auto-build and then auto-run every time you start Docker.

The container allows the host macOS/Windows system to access all containers directly
using the `172.17.0.0/24` IP addresses. This makes it possible to run a client application
on your macOS/Windows system and connect to the cluster that is running in the containers
(deployed in Docker using AeroLab).

## Initial container deployment

1. Use the `docker ps` command to check that no other Docker containers are running.
2. Run the following commands from the terminal command line:

```bash
git clone https://github.com/aerospike/aerolab
cd aerolab/tunnel-container-openvpn/build-run
chmod 755 *sh && ./RUNME.sh
```

## Initial host system setup

### macOS

1. Download [Tunnelblick](https://tunnelblick.net/), install and start it.
2. From the taskbar menu, click on `VPN Details`.
3. In the Finder, navigate to the `aerolab/tunnel-container-openvpn/build-run/keys` directory.
4. Drag and drop the file `client.conf` to the `Configurations` pane of the Tunnelblick window.
5. Choose either `Only Me` or `All Users`.
6. Close the Tunnelblick window.

### Windows
1. Download [OpenVPN Connect](https://openvpn.net/client-connect-vpn-for-windows/), install and start it.
2. Click the Plus button.
3. Click `Import from file`.
4. Rename `aerolab/tunnel-container-openvpn/build-run/keys/client.conf` to `aerolab/tunnel-container-openvpn/build-run/keys/client.ovpn`.
5. Drag and drop the file `aerolab/tunnel-container-openvpn/build-run/keys/client.ovpn` into the OpenVPN window and click `Add`.
6. Save and close the window.

## Usage following installation

Once Docker is started on macOS/Windows, click on the `OpenVPN Connect` or `Tunnelblick` icon
in the taskbar, and click `Connect`.

NOTE: on first run you may get two warnings, one about DNS not changing and one about IPs
not changing. This is normal, as we are not tunneling anything apart from/to Docker traffic.
Click on `Do not warn ...` on both warning windows and click `OK`.

## Technical details

This procedure installs OpenVPN Server (with all the bells and whistles of
configuration), generates CA/server/client certificates and exports the certificates to
the host machine. The server configuration has a route to force the Docker IP address range of
`172.17.0.0/16` to go through this VPN tunnel. Tunnelblick and OpenVPN Connect are GUIs
for OpenVPN, allowing you to connect to the OpenVPN server in a container from the OpenVPN
client on your host machine and allowing a `172.17.0.0/16` route to traverse through.
