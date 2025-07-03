Buildroot: ./
Name: aerolab
Version: VERSIONHERE
Release: 2
Summary: Tool for deploying non-prod Aerospike server clusters on Docker, AWS, or GCP
License: see github.com/aerospike/aerolab
Group: aerospike

%define _rpmdir ./
%define _rpmfilename %%{NAME}-linux-%%{ARCH}.rpm
%define _unpackaged_files_terminate_build 0
%define _binaries_in_noarch_packages_terminate_build 0

%description


Tool for deploying non-prod Aerospike server clusters on Docker, AWS, or GCP

%files
"/usr/bin/aerolab"
