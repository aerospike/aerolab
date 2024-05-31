# absolute path to a valid features.conf file
FEATURES_FILE="/Users/rglonek/aerolab/features.conf"

# if backend is 'docker', total nodes is NODES_PER_AZ*count(AWS_AVAILABILITY_ZONES), total clients is CLIENTS_PER_AZ*count(AWS_AVAILABILITY_ZONES)
BACKEND="docker"

# features
ENABLE_STRONG_CONSISTENCY=0
ENABLE_SECURITY=0

# aerospike version
VER="7.0.0.7"

# names
CLUSTER_NAME="robert"
AMS_NAME="glonek"
CLIENT_NAME="hammertime"

# region and list of AWS AZs to deploy in; also defines number of aerospike racks
AWS_REGION="us-east-1"
AWS_AVAILABILITY_ZONES=(us-east-1a us-east-1b us-east-1c)

# region and list of GCP AZs to deploy in; also defines number of aerospike racks
GCP_PROJECT="aerolab-test-project-2"
GCP_AVAILABILITY_ZONES=(us-central1-a us-central1-c)

# number of server nodes and client machines per AZ (per rack)
NODES_PER_AZ=2
CLIENTS_PER_AZ=2

# aws instances - cluster instance requires type with NVMe disks
CLUSTER_AWS_INSTANCE="r5ad.4xlarge"
AMS_AWS_INSTANCE="t3a.large"
CLIENT_AWS_INSTANCE="t3a.large"

# gcp instances - cluster instance requires type with local-ssd disks
CLUSTER_GCP_INSTANCE="c2d-standard-4"
AMS_GCP_INSTANCE="e2-standard-4"
CLIENT_GCP_INSTANCE="e2-standard-4"

# namespace name
NAMESPACE="test"

# gcp/aws size of the root volume
ROOT_VOL=40

# number of GCP local-ssd
GCP_LOCAL_SSDS=2

# partitions to create per NVMe if on AWS/GCP, split as percentages
AWS_GCP_PARTITIONS=25,25,25,25

# number of asbench per client instance/container - insert and read/update load
asbench_per_instance_insert=2
asbench_per_instance_load=2

# asbench details
asbench_start_key=0
asbench_end_key=1000000
asbench_threads=16
asbench_bin_name="testbin"
asbench_ru_runtime=86400
asbench_object="I1"
asbench_ru_throughput=1000
asbench_ru_percent=80 # 80 means 80 percent reads, 20 percent writes
asbench_socket_timeout=200
asbench_total_timeout=1000
asbench_retries=2
asbench_read_policy="allowReplica"

# name of config file that will be created for aerolab
export AEROLAB_CONFIG_FILE="multizone.aerolab.conf"

# setup the configs, do not modify this
. ./configure_set.sh
setsys
