# don't touch these
AWS_REGION=""
PROVISION=""

# basics
NAME="demo-cluster"         # cluster name
AMS_NAME="demo-ams"         # ams client name; contains asbench graphs too
CLIENT_NAME="demo-clients"  # name of client machine group
#AWS_REGION="us-west-2"     # uncomment to switch to using AWS instead of docker
AWS_AZ_1="ca-central-1a"
AWS_AZ_2="ca-central-1b"
AWS_AZ_3="ca-central-1c"
AWS_INSTANCE="t3a.medium"   # example with NVMe disks: c5ad.4xlarge; can use ARM instances here
AWS_AMS_INSTANCE="t3a.medium"    # instance for AMS to use
AWS_CLIENT_INSTANCE="t3a.medium" # instance to use for client asbench machines
VER="6.1.0.4"
FEATURES="/Users/rglonek/aerolab/templates/features.conf"
NAMESPACE="bar"
CLIENTS=2

# AWS only switch! If using docker backend, ensure this is commented out.
# nvme - if set, will provision the disks to create 4x 20% sized partitions
# ONLY USE THIS WHEN USING NVME INSTANCES - ADJUST TO YOUR INSTANCE TYPE DISK NAMES
# IF THIS IS SET, template_nvme.conf WILL BE SHIPPED INSTEAD, ADJUST THAT TO YOUR NEEDS TOO
# below example works with c5ad.4xlarge
#PROVISION="/dev/nvme1n1 /dev/nvme2n1" # list disks to provision, space separated

### below parts should not be touched ###

# aerolab config file
export AEROLAB_CONFIG_FILE="aerolab.conf"
rm -f ${AEROLAB_CONFIG_FILE}

# setup backend
[ "${AWS_REGION}" = "" ] && aerolab config backend -t docker || aerolab config backend -t aws -r ${AWS_REGION}
aerolab config defaults -k '*FeaturesFilePath' -v ${FEATURES} || exit 1
