function aws_auth() {
	[ ! -d ~/.aws ] && mkdir ~/.aws
	printf "[default]\noutput = text\nregion = %s\n" ${AWS_REGION} > ~/.aws/config
	printf "[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n" ${KEYID} ${SECRETKEY} > ~/.aws/credentials
}

function aws_region() {
	[ ! -d ~/.aws ] && mkdir ~/.aws
	printf "[default]\noutput = text\nregion = %s\n" ${AWS_REGION} > ~/.aws/config
}

function install_packages() {
	apt update
	DEBIAN_FRONTEND=noninteractive apt -y install curl vim openssh-client zip git jq less wget tmux
	DEBIAN_FRONTEND=noninteractive apt install -y --no-install-recommends tzdata
}

function keygen() {
	ssh-keygen -f ~/.ssh/id_rsa -N ''
}

# cause Colton created it
function get_coltons_helper() {
	old=$(pwd)
	if [ ! -d /root/deploy-olm-ako ]
	then
		cd /root && git clone -b eksctl https://github.com/colton-aerospike/deploy-olm-ako
	else
		cd /root/deploy-olm-ako && git pull
	fi
	cd ${old}
}

function install_eksctl() {
	ARCH=amd64
	[[ $(uname -m) =~ arm ]] && ARCH=arm64
	[[ $(uname -p) =~ arm ]] && ARCH=arm64
	PLATFORM=$(uname -s)_$ARCH
	curl -sLO "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_$PLATFORM.tar.gz"
	curl -sL "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_checksums.txt" | grep $PLATFORM | sha256sum --check
	tar -xzf eksctl_$PLATFORM.tar.gz -C /tmp && rm eksctl_$PLATFORM.tar.gz
	mv /tmp/eksctl /usr/local/bin
	chmod 755 /usr/local/bin/eksctl
}

function install_awscli() {
	ARCH=x86_64
	[[ $(uname -m) =~ arm ]] && ARCH=aarch64
	[[ $(uname -p) =~ arm ]] && ARCH=aarch64
	curl "https://awscli.amazonaws.com/awscli-exe-linux-${ARCH}.zip" -o "awscliv2.zip"
	unzip awscliv2.zip
	if [ -f /usr/local/bin/aws ]
	then
		./aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli --update
	else
		./aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli
	fi
	rm -f awscliv2.zip
	rm -rf aws
}

function install_kubectl() {
	ARCH=amd64
	[[ $(uname -m) =~ arm ]] && ARCH=arm64
	[[ $(uname -p) =~ arm ]] && ARCH=arm64
	curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${ARCH}/kubectl"
	mv kubectl /usr/local/bin/
	chmod 755 /usr/local/bin/kubectl
}

function install_helm() {
	curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 || return 0
	chmod 700 get_helm.sh || return 0
	./get_helm.sh || return 0
}

function initial_bootstrap() {
	ln -s /usr/local/bin/aerolab /usr/local/bin/eksexpiry
	aerolab config backend -t aws -r ${AWS_REGION}
	set +e
	aerolab config aws expiry-install || echo "WARNING: EXPIRY system could not be installed; EKS clusters may not expire"
	set -e
	aerolab client create eksctl --install-yamls -r ${AWS_REGION}
}

function enable_tmux() {
cat <<'EOF' >> /root/.bashrc
if command -v tmux &> /dev/null && [ -n "$PS1" ] && [[ ! "$TERM" =~ screen ]] && [[ ! "$TERM" =~ tmux ]] && [ -z "$TMUX" ] && [ $# -eq 0 ]; then
  exec tmux new-session -A -s eksctl
fi
EOF
}

usage() { echo "Usage: $0 [-k <AWS_KEY_ID>] -s [<AWS_SECRET_KEY>] [-r <AWS_DEFAULT_REGION>]" 1>&2; exit 1; }

KEYID=""
SECRETKEY=""
AWS_REGION=""
SWREGION=0
while getopts ":n:k:s:r:" o; do
    case "${o}" in
        k)
            KEYID=${OPTARG}
            ;;
        s)
            SECRETKEY=${OPTARG}
            ;;
        r)
        	AWS_REGION=${OPTARG}
        	;;
		n)
        	AWS_REGION=${OPTARG}
			SWREGION=1
        	;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND-1))

set -e

if [ ${SWREGION} -gt 0 ]
then
	echo "Switching region and installing expiry system"
	aws_region
	aerolab config backend -t aws -r ${AWS_REGION}
	aerolab config aws expiry-install
	exit 0
fi

cwd=$(pwd)
cd /tmp
rm -rf /tmp/eks-installer
mkdir eks-installer
cd eks-installer
if [ ! -z "${KEYID}" ] && [ ! -z "${SECRETKEY}" ]; then
	echo "Installing AWS Auth..."
	aws_auth
elif [ ! -z "${AWS_REGION}" ]; then
	echo "Skipping AWS Auth installation; Installing region settings only..."
	aws_region
else
	echo "Skipping AWS Auth/Region reconfiguration"
fi
echo "Installing dependencies"
install_packages
if [ ! -f ~/.ssh/id_rsa ]
then
	echo "Generating ssh ID..."
	keygen
else
	echo "SSH ID found, not recreating"
fi
echo "Installing eksctl..."
install_eksctl
echo "Installing awscli..."
install_awscli
echo "Installing kubectl..."
install_kubectl
echo "Getting deplok-olm-ako script (thanks Colton!)..."
get_coltons_helper
echo "Installing helm (optional)..."
set +e
install_helm
set -e
if [ ! -f /etc/aerolab-eks-bootstrapped ]
then
	echo "Initial bootstrap of aerolab-eks system..."
	initial_bootstrap
	touch /etc/aerolab-eks-bootstrapped
else
	echo "File '/etc/aerolab-eks-bootstrapped' exists, skipping initial install commands"
fi
echo "Enable tmux"
enable_tmux
echo "Cleanup"
cd ${cwd}
rm -rf /tmp/eks-installer
echo "Done"
