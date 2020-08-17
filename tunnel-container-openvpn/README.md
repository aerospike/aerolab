# Tunnel Container

## Description

This container will auto-build and then auto-run every time you start docker on mac or windows (or anywhere)

The container allows for your host MAC/WINDOWS to access all containers directly using the 172.17.0.0/24 IP addresses. This makes it possible to run for example a client application on your MAC and connect to the cluster that is running in the containers (deployed in docker using aerolab).

## Simplicity

Once you deploy, and configure your MAC (should be about 5 minutes work,  out of which 3 minutes is waiting on installation) you NEVER need to do anything else apart from clicking "Connect" in Tunnelblick whenever you start docker-on-mac. Everything else auto-starts as it should and you can seemlessly always access your containers directly from your MAC/WINDOWS. Below is single one-time configuration instruction. Enjoy :)

## Deployment

1. Ensure you have no other containers running (i.e. all are stopped)
2. Download this repo
3. cd tunnel-container-openvpn/build-run
4. chmod 755 *sh && ./RUNME.sh

## Allowing your MAC to use the tunnel

1. Download Tunnelblick from [here](https://tunnelblick.net/) and install it (if you VPN to MV office, you should already have it)
2. From the task-bar shortcut, click on "VPN Details"
3. In Finder/Explorer, navigate to the `tunnel-container-openvpn/build-run/keys` directory
4. Drag-drop the client.conf from finder/explorer to the "Configurations" left-pane of the Tunnelblick window
5. Choose either "Only Me" or "All Users", not really important
6. Close the Tunnelblick window

## Allowing your Windows to use the tunnel
1. Download OpenVPN Connect from [here](https://openvpn.net/client-connect-vpn-for-windows/) (if you VPN to MV office, you should already have it)
2. Click the Plus button
3. Click "Import from file"
4. Rename `tunnel-container-openvpn/build-run/keys/client.conf` to `tunnel-container-openvpn/build-run/keys/client.ovpn`
5. Drag-drop `tunnel-container-openvpn/build-run/keys/client.ovpn` into the window and click "Add"
6. Connect VPN

And that's it, from now on, every time you start docker on mac/windows, this special container will auto-start (you never need to run RUNME.sh or any such thing again).

## Usage following installation

Now, once you start docker on mac/windows, all you have to do is click on the Tunnelblick icon and click on "Connect client".

NOTE: on first run you *may* get 2 warnings, one about DNS and one about IPs not changing. This is normal as we are not tunneling anything apart from Docker traffic. Click on "Do not warn ..." on both warning windows and click "OK".

## A bit of technical RTFM if you are really interested

All this does is install openvpn server (with all the bells and whistles of configuration), generate ca/server/client certificates and export the certificates to your machine. The server configuration has a route to force Docker IP range to go through this VPN tunnel (and nothing else). Tunnelblick is a GUI for openvpn, so really you are just connecting to openvpn server in a container from openvpn client on your host machine, allowing a 172.17.0.0/16 route to travers through.
