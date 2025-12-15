set -e
./build.sh all
aerolab version
./aerolab version

if [ "$1" = "aws" ]; then
    aerolab config backend -t aws -r ca-central-1
    ./aerolab config backend -t aws -r ca-central-1
elif [ "$1" = "gcp" ]; then
    aerolab config backend -t gcp -o aerolab-test-project-1
    ./aerolab config backend -t gcp -o aerolab-test-project-1 -r us-central1
else
    echo "Invalid backend: '$1', must be aws or gcp"
    exit 1
fi
aerolab cluster create -n bob -v 8.0.0.5 -c 3 -I t3a.xlarge --instance e2-standard-4 --zone us-central1-a
aerolab client create none -n bobnone -I t3a.large --instance e2-standard-4 --zone us-central1-a
aerolab client create tools -n bobtool -I t3a.large --instance e2-standard-4 --zone us-central1-a
aerolab client create vscode -n bobvs -I t3a.xlarge --instance e2-standard-4 --zone us-central1-a
aerolab volume create -n bobvol --zone us-central1-a
./aerolab inventory migrate --dry-run --verbose --ssh-key-path /Users/rglonek/aerolab-keys
./aerolab inventory migrate --yes --force --verbose --ssh-key-path /Users/rglonek/aerolab-keys
echo "--------------------------------"
echo "Now run:"
echo "./aerolab inventory list"
echo "./aerolab attach shell -n bob"
echo "./aerolab attach client -n bobnone"
echo "./aerolab inventory delete-project-resources -f"
echo "--------------------------------"
