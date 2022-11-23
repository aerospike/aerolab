. configure.sh
aerolab client destroy -f -n ${AMS_NAME}
aerolab cluster destroy -f -n ${NAME}
echo "Don't forget to run both destroy_clients.sh AND destroy_pretty.sh"
