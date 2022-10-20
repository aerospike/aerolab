#!/bin/bash
mkdir -p /usr/local/bin/
BIN=aerolab-macos-amd64
uname -p |grep arm && BIN=aerolab-macos-arm64 || echo "amd"
uname -m |grep arm && BIN=aerolab-macos-arm64 || echo "amd"
chmod 755 /Library/aerolab/*
rm -f /usr/local/bin/aerolab || echo "first_install"
ln -s /Library/aerolab/${BIN} /usr/local/bin/aerolab

