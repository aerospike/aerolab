# install docker
set -e
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh
set +e
systemctl enable --now docker || exit 0
