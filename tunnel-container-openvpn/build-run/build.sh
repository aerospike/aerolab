TREPO="openvpn-base"
uname -p |grep arm >/dev/null 2>&1 && TREPO="openvpn-base-arm"
uname -m |grep arm >/dev/null 2>&1 && TREPO="openvpn-base-arm"
sed "s/TREPO/bestmethod\/${TREPO}/g" Dockerfile.template > Dockerfile
docker build -t openvpn:1 .
