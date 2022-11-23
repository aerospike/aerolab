# Multizone benchmark demo

This script set requires aerolab version 4.3.7 or higher if using x86_64 instances only; or 4.4.0 or higher if intending to use arm instances.

## Process in a nutshell
  * setup your AWS account
  * adjust configure.sh
  * run the setup scripts in the order provided below
  * follow the steps printed by the last setup script to configure grafana for asbench
  * run gimme_url.sh to get the URLs for pretty graphs
  * run the run_asbench (adjust if you want to adjust it)
  * run stop_ and start_ scripts to your needs to replicate nodes being taken offline
  * destroy everything by running the 3 destroy scripts

## Note on nvme usage
 * edit template_nvme.conf to your needs
 * edit configure.sh and change instance type to one that uses nvme disks
   * note each instance type may have different nvme disks mapped, check manually by deployinga one-node cluster with aerolab and ls /dev
 * in configure.sh also uncomment and edit the PROVISION= line to list the disks that you want provisioned
   * note this must match disk names what's in template_nvme as partitions cause otherwise it won't work

## AWS requirements - one time stup
 * install awscli
 * run: `aws configure`
 * `us-west-2` is the only supported region for this demo, switch to it or adjust all scripts
   * security group for servers and clients:
     * open ports from all sources: 22, 443, 5000-10000
     * save group, then edit group and add rule that allows all communications from itself (from sg-...) - all ports
   * security group for ams and grafana:
     * open ports from all sources: 22, 443, 3000
     * all ports from server sg-xxx group
   * open server sg, and add a rule:
     * all ports from ams sg-xxx group
   * take note of both security group IDs and 4 subnet IDs for the 4 AZs for your VPC (all SG and Subnets live under a single VPC)
     * move on to editing `configure.sh`

## Files provided here

### ape.toml

ready template for exporter (using username/password)

### astools.conf

ready template for cluster - using username/password

### template.conf

ready aerospike.conf template

### template_nvme.conf

this will be deployed instead of template.conf if configure.sh PROVISION variable is set to a list of disks

### asbench.json

grafana json file for the asbench dashboard

### configure.sh

configuration - adjust this to your aws account needs if not using sales aws account

### **** BELOW SCRIPTS ARE TO BE RAN TO EXECUTE ACTIONS; SETUP SCRIPTS MUST BE RAN IN THE ORDER GIVEN HERE ****

### setup_server_ams.sh

script which sets up asd and ams and exporter

### setup_clients.sh

script which sets up the client machines

### setup_pretty.sh

script which sets up the grafana instance for asbench monitoring

### gimme_url.sh

script that prints all the grafana URLs

### run_asbench.sh

run asbench on all clients

### stat_asbench.sh

see if asbench is running on all clients

### logs_asbench.sh

tail logs of all clients asbench

### kill_asbench.sh

kill all asbench

### stop_last_rack.sh

stops aerospike in last rack

### start_last_rack.sh

starts aerospike in last rack

### destroy_pretty.sh

destroy the grafana stack for asbench logs

### destroy_clients.sh

destroy client machines

### destroy_server_ams.sh

destroy server and ams machines

## Manually running stuff afterwards:

```
$ source configure.sh
$ aerolab cluster list
$ aerolab client list
...
```
