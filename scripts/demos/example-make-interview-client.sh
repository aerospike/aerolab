# settings
EXPIRE=5h
BACKEND=aws # aws/gcp
AWS_REGION=eu-west-3
GCP_PROJECT=aerolab-test-project-1
GCP_REGION=europe-west2-a
SCRIPT=./interview.sh
NAME=interview

### NOTE - ensure the script runs just once, not on every start/stop
### script should start with:
###   [ -f /opt/installed ] && exit 0
### script should end with:
###   touch /opt/installed

# create firewall and client
function create() {
	if [ "$BACKEND" == "aws" ]
	then
		aerolab config backend -t aws -r $AWS_REGION || exit 1
		aerolab config aws list-security-groups |grep interview
		[ $? -ne 0 ] && aerolab config aws create-security-groups -n interview -p 22 -p 80 -p 443 --no-defaults
		aerolab config aws lock-security-groups -n interview -i 0.0.0.0/0 -p 22 -p 80 -p 443 --no-defaults || exit 1
	else
	    aerolab config backend -t gcp -o $GCP_PROJECT || exit 1
		aerolab config gcp list-firewall-rules |grep interview
		[ $? -ne 0 ] && aerolab config gcp create-firewall-rules -n interview -p 22 -p 80 -p 443 --no-defaults
		aerolab config gcp lock-firewall-rules -n interview -i 0.0.0.0/0 -p 22 -p 80 -p 443 --no-defaults || exit 1
	fi

	set -e
	aerolab client create base -n $NAME -X $SCRIPT --instance-type=t3a.xlarge --secgroup-name=interview --aws-expire=$EXPIRE --instance=e2-standard-2 --zone=$GCP_REGION --firewall=interview --gcp-expire=$EXPIRE
}

# start client
function start() {
	aerolab client start -n $NAME
}

# stop client
function stop() {
	aerolab client stop -n $NAME
}

# destroy client
function destroy() {
	aerolab client destroy -f -n $NAME
}

if [ "$1" == "create" ]
then
	create
elif [ "$1" == "start" ]
then
	start
elif [ "$1" == "stop" ]
then
	stop
elif [ "$1" == "destroy" ]
then
	destroy
else
	echo "Usage: $0 create|start|stop|destroy"
	echo "Before running, edit the settings parameters in this script"
	exit 1
fi
