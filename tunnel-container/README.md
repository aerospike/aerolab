# Tunnel Container

## Description

This container will auto-build and then auto-run every time you start docker on mac or windows (or anywhere)

The container allows for your host MAC/WINDOWS to access all containers directly using the 172.17.0.0/24 IP addresses. This makes it possible to run for example a client application on your MAC and connect to the cluster that is running in the containers (deployed in docker using aerolab).

## Smplicity

Once you deploy, and configure your MAC (should be about 5 minutes work,  out of which 3 minutes is waiting on installation) you NEVER need to do anything else. Everything auto-starts as it should and you can seemlessly always access your containers directly from your MAC/WINDOWS. Below is single one-time configuration instruction. Enjoy :)

## Deployment

1. Ensure you have no other containers running (i.e. all are stopped)
2. Download this repo
3. cd tunnel-container
4. chmod 755 *sh && ./RUNME.sh

## Allowing your MAC to use the tunnel

1. Download ProxyCap from [here](http://www.proxycap.com/download.html) and install it (requires system reboot at end of installation)
2. From the task-bar shortcut, go to ProxyCap->Configuration
3. Click on "New" under "Proxies" tab and configure as follows:
   1. Display Name: docker
   2. Type: SSH
   3. Hostname: 127.0.0.1
   4. Port: 2222
   5. Username: root
   6. Password: root
4. Click OK
5. Click on "New" under "Rules" tab and configure as follows:
   1. Redirect through proxy
   2. proxy: "docker"
   3. "All Programs"
   4. "TCP"
   5. Rule Name: docker
   6. Port Range: "Not Restricted"
   7. Destination IP Range: Specify: 172.17.0.0 mask 24
   8. Destination Hostname: Not restricted
6. Click OK

And that's it, from now on, every time you start docker on mac/windows, this special container will auto-start (you never need to run RUNME.sh or any such thing again).

ProxyCap auto-starts too, so you never need to change anything from that point. All you have to do is start docker on MAC/WINDOWS and enjoy being able to access your containers directly. Smooth, right?

## A bit of technical RTFM if you are really interested

All this does is install openssh server, and configure it allowing SSH tunnelling (which in turn creates a SOCKS5 proxy). This allows us to tunnel connections though that container. That container can see all others, which is simple. Now all that happens underneath is that ProxyCap proxies all connections for specific subnets (docker ones) to that ssh container, allowing you to access interrnal docker IPs from your host directly.
