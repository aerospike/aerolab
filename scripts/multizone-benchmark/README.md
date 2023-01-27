# Multizone benchmark demo

This script set requires aerolab version 4.6.0 or higher.

## Usage
1. git clone this repository
2. modify `configure.sh` to your needs
3. modify `asbench` parameters to your needs in `12_run_asbench.sh`
4. perform any required namespace modifications in `template.conf` or `template_nvme.conf` (see below note on NVME usage)
5. ensure AWS configuration prerequisites are met (one-time setup)
   1. install `awscli`
   2. run `aws configure`
6. run the scripts to have the stack created for you (see below for script list)

## Note on nvme usage
 * edit template_nvme.conf to your needs
 * edit configure.sh and change instance type to one that uses nvme disks
   * note each instance type may have different nvme disks mapped, check manually by deployinga one-node cluster with aerolab and ls /dev
 * in configure.sh also uncomment and edit the PROVISION= line to list the disks that you want provisioned
   * note this must match disk names what's in template_nvme as partitions cause otherwise it won't work

## Scripts and configuration files

name | type | description
--- | --- | ---
ape.toml | exporter config | premade configuration file for aerospike prometheus exporter
astools.conf | astools config | premade configuration file for aerospike tools package (for asbench)
template.conf | aerospike config | template aerospike configuration file - non-nvme instances or docker
template_nvme.conf | aerospike config | template aerospike configuration file - nvme aws instances
configure.sh | configuration script | script with basic configuration - edit this to tune the deployment
01_setup_server_ams.sh | script | run this first - sets up aerospike servers and AMS monitoring stack
02_setup_clients.sh | script | run this second - sets up aerospike clients (tools) with asbench and monitoring
11_grafana_url.sh | script | run this any time after the above were executed - to get grafana URL to access the graphs
12_run_asbench.sh | script | run this to start asbench data insert or read/update load
13_kill_asbench.sh | script | run this to kill asbench on all client machines
14_stat_asbench.sh | script | run this to check if asbench is running
19_asbench_logs.sh | script | run this to tail the logs for all asbench machines
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
