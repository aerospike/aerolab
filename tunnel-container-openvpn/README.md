# Tunnel Container

## Description

This container will auto-build and then auto-run every time you start docker

The container allows for the host MacOS/Windows to access all containers directly using the 172.17.0.0/24 IP addresses. This makes it possible to run for example a client application on your MacOS and connect to the cluster that is running in the containers (deployed in docker using aerolab).

## Initial Container Deployment

1. Ensure no other containers are running (i.e. all are stopped)
2. Run the following commands from the terminal / bash shell:

```bash
git clone https://github.com/citrusleaf/aerolab
cd aerolab/tunnel-container-openvpn/build-run
chmod 755 *sh && ./RUNME.sh
```

## Initial host system setup

### MacOS

1. Download Tunnelblick from [here](https://tunnelblick.net/), install and start it
2. From the task-bar shortcut, click on "VPN Details"
3. In Finder/Explorer, navigate to the `aerolab/tunnel-container-openvpn/build-run/keys` directory
4. Drag-drop the client.conf from finder/explorer to the "Configurations" left-pane of the Tunnelblick window
5. Choose either "Only Me" or "All Users", not really important
6. Close the Tunnelblick window

### Windows
1. Download OpenVPN Connect from [here](https://openvpn.net/client-connect-vpn-for-windows/), install and start it
2. Click the Plus button
3. Click "Import from file"
4. Rename `aerolab/tunnel-container-openvpn/build-run/keys/client.conf` to `aerolab/tunnel-container-openvpn/build-run/keys/client.ovpn`
5. Drag-drop `aerolab/tunnel-container-openvpn/build-run/keys/client.ovpn` into the window and click "Add"
6. Save and Close the window

## Usage following installation

Once docker is started on Windows/MacOS, click on the `OpenVPN Connect` or `Tunnelblick` icon in the taskbar, and click `Connect`

NOTE: on first run you *may* get 2 warnings, one about DNS not changing and one about IPs not changing. This is normal as we are not tunneling anything apart from/to Docker traffic. Click on `Do not warn ...` on both warning windows and click `OK`.

## Technical bits

All this does is install openvpn server (with all the bells and whistles of configuration), generate ca/server/client certificates and export the certificates to the host machine. The server configuration has a route to force Docker IP range of `172.17.0.0/16` to go through this VPN tunnel. Tunnelblick and OpenVPN Connect are GUIs for openvpn. Essentially you are just connecting to the openvpn server in a container from the openvpn client on your host machine, allowing a `172.17.0.0/16` route to traverse through.
