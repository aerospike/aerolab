#!/bin/bash

## adds a filesystem to fstab and mounts it (if 'mount -a' fails it sleeps 10 seconds and retries, up to a maximum specified ATTEMPTS)
## usage: ./efs_mount.sh ATTEMPTS IP FSID REGION DEST_PATH [on [PROFILENAME]]
## params:
##  ATTEMPTS - integer specifying how many time to attempt `mount -a` before giving up if it doesn't succeed; 0 means do not run the command
##  IP - mount target IP address
##  FSID - filesystem ID
##  REGION - AWS region (e.g., us-east-1)
##  DEST_PATH - mount path
##  on - if specified, 'iam' mount parameter will be enabled
##  PROFILENAME - if specified, the 'awsprofile=...' parameter will be enabled; required 'on' to be specified

MAX=$1
ip=$2
fsid=$3
region=$4
dest=$5
iam=""
iamprofile=""
if [ "$6" = "on" ]; then
    iam=",iam"
    if [ "$7" != "" ]; then
        iamprofile=",awsprofile=$7"
    fi
fi

if [ "$ip" = "" ]; then
    echo "Invalid usage, IP not specified"
    exit 1
fi

if [ "$fsid" = "" ]; then
    echo "Invalid usage, fsid not specified"
    exit 1
fi

if [ "$region" = "" ]; then
    echo "Invalid usage, region not specified"
    exit 1
fi

if [ "$dest" = "" ]; then
    echo "Invalid usage, destination path not specified"
    exit 1
fi

# Add IP to /etc/hosts mapping fsId and fsId.efs.REGION.amazonaws.com
if ! grep -q "^${ip} ${fsid}$" /etc/hosts; then
    printf "%s %s\n" "$ip" "$fsid" >> /etc/hosts
fi
if ! grep -q "^${ip} ${fsid}.efs.${region}.amazonaws.com$" /etc/hosts; then
    printf "%s %s.efs.%s.amazonaws.com\n" "$ip" "$fsid" "$region" >> /etc/hosts
fi

mkdir -p $dest
printf "\n%s:/ %s efs _netdev,noresvport,tls%s%s 0 0\n" $fsid $dest $iam $iamprofile >> /etc/fstab

ATTEMPT=0
RET=$MAX # if MAX attempts is 0, it will not enter below loop; otherwise, we are guaranteed to execute at least once
while [ $RET -ne 0 ]; do
    ATTEMPT=$(($ATTEMPT + 1))
    mount -a
    RET=$?
    if [ $RET -ne 0 ]; then
        if [ $ATTEMPT -eq $MAX ]; then
            echo "Failed max attempts, exiting"
            exit 1
        fi
        resolvectl flush-caches || systemd-resolve --flush-caches
        sleep 10
    fi
done
systemctl daemon-reload
exit 0
