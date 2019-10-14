#Check aerolab is in your path
AEROLAB_AVAILABLE=`which aerolab`
if [ -z "${AEROLAB_AVAILABLE}" ]
then
	echo "aerolab not found on your system. Make sure aerolab binary is in your path before proceeding"
	echo "See https://github.com/citrusleaf/opstools/tree/master/Aero-Lab_Quick_Cluster_Spinup for more details"
	echo
	exit 1
fi

CONFIG_FILE=standard-aerospike.conf
CLUSTER_NAME=standard
CREDENTIALS=credentials.conf
FEATURE_KEY=../features.conf
VERSION=4.6.0.4

aerolab make-cluster --config=$CREDENTIALS -n $CLUSTER_NAME -m mesh -c 3 -o `pwd`/$CONFIG_FILE -f $FEATURE_KEY -v $VERSION
