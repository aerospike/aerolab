# Multizone benchmark demo

This script set requires aerolab version 5.4.0 or higher.

## Usage
1. git clone this repository
2. modify `configure.sh` to your needs
3. optionally, modify `asbench` parameters to your needs in `12_run_asbench.sh`
4. perform any required namespace modifications in `template.conf` - do not modify the storage backend, aerolab will auto-configure this to NVMe devices it finds on the instances in AWS
5. ensure AWS configuration prerequisites are met (one-time setup)
   1. install `awscli`
   2. run `aws configure`
6. run the scripts to have the stack created for you (see below for script list)

## Scripts and configuration files

name | type | description
--- | --- | ---
ape.toml | exporter config | premade configuration file for aerospike prometheus exporter
astools.conf | astools config | premade configuration file for aerospike tools package (for asbench)
template.conf | aerospike config | template aerospike configuration file
configure.sh | configuration script | script with basic configuration - edit this to tune the deployment
00_create_lock_security_groups.sh | script | run this to pre-create and lock AeroLab managed security groups to your source IP
01_setup_server_ams.sh | script | run this first - sets up aerospike servers and AMS monitoring stack
02_setup_clients.sh | script | run this second - sets up aerospike clients (tools) with asbench and monitoring
10_asadm.sh | script | run to attach to asadm on a running server node
11_grafana_url.sh | script | run this any time after the above were executed - to get grafana URL to access the graphs
12_run_asbench.sh | script | run this to start asbench data insert or read/update load
13_kill_asbench.sh | script | run this to kill asbench on all client machines
14_stat_asbench.sh | script | run this to check if asbench is running
21_stop_last_rack.sh | script | run to simulate last rack going away by shutting down the nodes in that rack
22_start_last_rack.sh | script | run to bring the last rack back up
91_destroy_clients.sh | script | cleanup process - destroy client instances
92_destroy_server_ams.sh | script | cleanup process - destroy aerospike servers and AMS

## Manually running aerolab commands against this demo:

```
$ source configure.sh
$ aerolab cluster list
$ aerolab client list
...
```

## AIO setup, run delete

```
./00_create_lock_security_groups.sh && ./01_setup_server_ams.sh && ./02_setup_clients.sh && ./10_asadm.sh -e info && ./11_grafana_url.sh && ./12_run_asbench.sh i && sleep 5 && ./14_stat_asbench.sh
./12_run_asbench.sh ru && sleep 5 && ./14_stat_asbench.sh && sleep 120 && ./21_stop_last_rack.sh && sleep 300 && ./22_start_last_rack.sh
./91_destroy_clients.sh && ./92_destroy_server_ams.sh
```