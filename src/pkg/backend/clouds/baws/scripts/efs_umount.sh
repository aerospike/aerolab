#!/bin/bash

## usage: ./efs_umount.sh FSID1 [FSID2 [FSID3 [...]]]

# remove filesystems from /etc/fstab
fstab=$(cat /etc/fstab)
cp /etc/fstab /etc/fstab.bak
for fsid in "${@}"; do
    fstab=$(printf "%s" "$fstab" |egrep -v "^${fsid}:/")
done
printf "%s\n" "$fstab" > /etc/fstab

# unmount filesystems
ISERR=0
for fsid in "${@}"; do
    while read fs; do
        umount "${fs}"
        if [ $? -ne 0 ]; then
            sleep 1
            umount -f "${fs}"
            [ $? -ne 0 ] && ISERR=1
        fi
    done < <(mount |grep tmpfs |egrep "^${fsid}" |awk '{print $3}')
done
if [ $ISERR -ne 0 ]; then
    echo
    echo "ERROR: Some filesystems failed to unmount, reboot to force"
    exit 1
fi
exit 0
