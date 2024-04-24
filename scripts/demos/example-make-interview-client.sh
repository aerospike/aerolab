# settings
EXPIRE=5h
BACKEND=aws # aws/gcp
AWS_REGION=eu-west-3
GCP_PROJECT=aerolab-test-project-1
GCP_REGION=europe-west2-a
SCRIPT=./interview.sh
NAME=interview

# create firewall and client
function create() {
	if [ ! -f $SCRIPT ]
	then
		echo "ERROR: script $SCRIPT not found"
		return 1
	fi
	if [ "$BACKEND" == "aws" ]
	then
		aerolab config backend -t aws -r $AWS_REGION || exit 1
		aerolab config aws list-security-groups |grep interview
		[ $? -ne 0 ] && aerolab config aws create-security-groups -n interview -p 80 -p 443 --no-defaults
		aerolab config aws lock-security-groups -n interview -i 0.0.0.0/0 -p 22 -p 80 -p 443 --no-defaults || exit 1
	else
	    aerolab config backend -t gcp -o $GCP_PROJECT || exit 1
		aerolab config gcp list-firewall-rules |grep interview
		[ $? -ne 0 ] && aerolab config gcp create-firewall-rules -n interview -p 22 -p 80 -p 443 --no-defaults
		aerolab config gcp lock-firewall-rules -n interview -i 0.0.0.0/0 -p 22 -p 80 -p 443 --no-defaults || exit 1
	fi

	set -e
	aerolab client create base -n $NAME --instance-type=t3a.xlarge --secgroup-name=interview --aws-expire=$EXPIRE --instance=e2-standard-2 --zone=$GCP_REGION --firewall=interview --gcp-expire=$EXPIRE
	aerolab files upload -n $NAME -c $SCRIPT /tmp/setup.sh
	aerolab client attach -n $NAME -- bash /tmp/setup.sh
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

# enable tmux/screen sharing
function enabletmux() {
	aerolab client attach -n $NAME -- bash /root/enable-screen.sh
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
elif [ "$1" == "enable" ]
then
	enabletmux
elif [ "$1" == "create-enable" ]
then
	create || exit 1
	enabletmux
elif [ "$1" == "showip" ]
then
	aerolab client list --ip |egrep '^client=interview' |egrep -o 'ext_ip=[^ ]+' |sed 's/ext_ip=/ssh root@/g'
else
	echo "Usage: $0 create|enable|start|stop|destroy"
	echo " * create:        create inverview machine"
	echo " * enable:        enable screen/tmux (this will work only once)"
	echo " * create-enable: shortcut to auto-run create followed by enable in a single command"
	echo " * showip:        show the IP of the interview machine"
	echo " * start:         start a stopped inverview machine"
	echo " * stop:          stop a running inverview machine"
	echo " * destroy:       remove the inverview machine"
	echo
	echo "Before running, edit the settings parameters in this script"
	exit 1
fi
