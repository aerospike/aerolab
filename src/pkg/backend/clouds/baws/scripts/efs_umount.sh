#!/bin/bash

## usage: ./efs_umount.sh FSID1 [FSID2 [FSID3 [...]]]

# remove filesystems from /etc/fstab
fstab=$(cat /etc/fstab)
cp /etc/fstab /etc/fstab.bak
for fsid in "${@}"; do
    fstab=$(printf "%s" "$fstab" |grep -E -v "^${fsid}:/")
done
printf "%s\n" "$fstab" > /etc/fstab

# remove entries from /etc/hosts (both fsId and fsId.efs.REGION.amazonaws.com)
hosts=$(cat /etc/hosts)
cp /etc/hosts /etc/hosts.bak
for fsid in "${@}"; do
    # Remove entries matching: IP fsId or IP fsId.efs.*.amazonaws.com
    hosts=$(printf "%s" "$hosts" |grep -E -v " ${fsid}$| ${fsid}\\.efs\\..*\\.amazonaws\\.com$")
done
printf "%s\n" "$hosts" > /etc/hosts

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
    done < <(grep -E "^${fsid}:" /etc/fstab.bak |awk '{print $2}')
done
if [ $ISERR -ne 0 ]; then
    echo
    echo "ERROR: Some filesystems failed to unmount, reboot to force"
    exit 1
fi
exit 0
