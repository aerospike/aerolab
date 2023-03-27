TREPO="openvpn-base"
uname -p |grep arm >/dev/null 2>&1 && TREPO="openvpn-base-arm"
uname -m |grep arm >/dev/null 2>&1 && TREPO="openvpn-base-arm"
sed "s/TREPO/bestmethod\/${TREPO}/g" Dockerfile.template > Dockerfile
if [ "$1" = "WINDOZ" ]
then
    # wsl2 uses 172.18. network, so cannot push the whole 172.16 private range
    sed 's/PUSHROUTEHERE/push "route 172.17.0.0 255.255.0.0"/g' server.conf.template > server.conf
else
    sed 's/PUSHROUTEHERE/push "route 172.16.0.0 255.240.0.0"/g' server.conf.template > server.conf
fi
docker build -t openvpn:1 .
