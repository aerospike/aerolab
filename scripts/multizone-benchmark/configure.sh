# don't touch these
AWS_REGION=""
PROVISION=""

# basics
NAME="demo-cluster"         # cluster name
AMS_NAME="demo-ams"         # ams client name
CLIENT_NAME="demo-clients"  # name of client machine group
PRETTY_NAME="demo-cgraphs"  # name of the asbench grafana machine
#AWS_REGION="us-west-2"     # uncomment to switch to using AWS instead of docker
AWS_INSTANCE="t3a.medium"   # example with NVMe disks: c5ad.4xlarge; can use ARM instances here
AWS_CLIENT_INSTANCE="t3a.medium" # for this script, x86_64 only here
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

# security groups - adjust to your IDs
us_west_2=sg-0aad8946ddc28141c
us_west_2_open=sg-02d0e461917ff8d5e

# subnets - adjust to your IDs
us_west_2a=subnet-0a92aefc2053beed0
us_west_2b=subnet-0eab0b37ce92a3bab
us_west_2c=subnet-08f71713aa089cb44
us_west_2d=subnet-0e153f691c1313a49

### below parts should not be touched ###

# aerolab config file
export AEROLAB_CONFIG_FILE="aerolab.conf"
rm -f ${AEROLAB_CONFIG_FILE}

# setup backend
[ "${AWS_REGION}" = "" ] && aerolab config backend -t docker || aerolab config backend -t aws -r ${AWS_REGION}
aerolab config defaults -k '*FeaturesFilePath' -v ${FEATURES} || exit 1
