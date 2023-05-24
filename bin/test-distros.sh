if [ "$1" = "" ]
then
    echo "Usage: $0 backend"
    echo "  backends: aws|docker|gcp"
    exit 1
fi

comm="aerolab-next"
project="aerolab-test-project-1"
region="ca-central-1"

$comm config backend -t $1 -o ${project} -r ${region}
if [ "$1" = "aws" ]
then
    $comm config aws create-security-groups
    $comm config aws lock-security-groups
elif [ "$1" = "gcp" ]
then
    $comm config gcp create-firewall-rules
    $comm config gcp lock-firewall-rules
fi

$comm cluster destroy -f
$comm client destroy -f -n client
$comm template destroy -d all -i all -v all
$comm template vacuum

set -e
$comm cluster create -v 6.3.0.3 -d ubuntu -i 22.04 --instance-type t3a.medium --instance e2-medium
$comm cluster grow -v 6.3.0.3 -d ubuntu -i 20.04 --instance-type t3a.medium --instance e2-medium
$comm cluster grow -v 6.3.0.3 -d debian -i 10 --instance-type t3a.medium --instance e2-medium
$comm cluster grow -v 6.3.0.3 -d debian -i 11 --instance-type t3a.medium --instance e2-medium
$comm cluster grow -v 6.3.0.3 -d centos -i 7 --instance-type t3a.medium --instance e2-medium
$comm cluster grow -v 6.3.0.3 -d centos -i 8 --instance-type t3a.medium --instance e2-medium
$comm cluster grow -v 6.3.0.3 -d centos -i 9 --instance-type t3a.medium --instance e2-medium
$comm cluster grow -v 6.3.0.3 -d amazon -i 2 --instance-type t3a.medium --instance e2-medium
set +e

$comm cluster destroy -f
$comm template destroy -d all -i all -v all
$comm template vacuum

set -e
$comm client create base -d ubuntu -i 22.04 --instance-type t3a.medium --instance e2-medium
$comm client grow base -d ubuntu -i 20.04 --instance-type t3a.medium --instance e2-medium
$comm client grow base -d debian -i 10 --instance-type t3a.medium --instance e2-medium
$comm client grow base -d debian -i 11 --instance-type t3a.medium --instance e2-medium
$comm client grow base -d centos -i 7 --instance-type t3a.medium --instance e2-medium
$comm client grow base -d centos -i 8 --instance-type t3a.medium --instance e2-medium
$comm client grow base -d centos -i 9 --instance-type t3a.medium --instance e2-medium
$comm client grow base -d amazon -i 2 --instance-type t3a.medium --instance e2-medium
set +e

$comm client destroy -f -n client
