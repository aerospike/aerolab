#!/bin/bash
which easytc >/dev/null 2>&1
if [ $? -ne 0 ]; then
    UN=$(uname -m)
    arch=amd64
    [ "$UN" = "aarch64" ] && arch=arm64
    [ "$UN" = "arm64" ] && arch=arm64
    set -e
    wget -q -O /tmp/easytc.tgz https://github.com/rglonek/easytc/releases/latest/download/easytc.${arch}.tgz
    cd /tmp
    tar -zxf easytc.tgz
    mv easytc /usr/local/bin/
fi
set -e
