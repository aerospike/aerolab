#!/bin/bash
# supported distros; centos-stream distros corresponding to the rockylinux are also supported
# amazon linux is also supported
distros=(rockylinux:9 rockylinux:8 ubuntu:24.04 ubuntu:22.04 ubuntu:20.04 debian:12 debian:11)

docker stop -t 1 testefs >/dev/null 2>&1 && sleep 2
for distro in "${distros[@]}"; do
  echo "=-=-=-=-=-=-=-=-=-= $distro =-=-=-=-=-=-=-=-=-="
  docker run -itd --rm --name testefs $distro bash ||exit 1
  docker cp efs_install.sh testefs:/tmp/ ||exit 1
  echo "run installer"
  docker exec -it testefs bash /tmp/efs_install.sh > "${distro}"_1.log 2>&1 ||exit 1
  echo "run installer again"
  docker exec -it testefs bash /tmp/efs_install.sh > "${distro}"_2.log 2>&1 ||exit 1
  docker stop -t 1 testefs && sleep 2
done
