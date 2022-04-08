basedir=$(basename $(pwd))
name_host="goclient"
version="v1.1"

container_name=""

function replaceconf() {
  if [ "$(uname -s)" != "Darwin" ]
  then
    return
  fi
  if [ ! -f ~/.docker/config.json.backup ]
  then
    mv ~/.docker/config.json ~/.docker/config.json.backup || return
  fi
  cat <<'EOF' >~/.docker/config.json
{
  "credsStore" : "osxkeychain",
  "auths" : {

  }
}
EOF
}

function iscreated() {
  docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${basedir}_${name_host}_1 2>/dev/null 1>/dev/null
  if [ $? -ne 0 ]
  then
    docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${basedir}-${name_host}-1 2>/dev/null 1>/dev/null
    if [ $? -ne 0 ]
    then
      return 1
    fi
  fi
  return 0
}

function restoreconf() {
  mv ~/.docker/config.json.backup ~/.docker/config.json
}

function start() {
  docker-compose start && getip
}

function stop() {
  docker-compose stop
}

function destroy() {
  iscreated
  if [ $? -ne 0 ]
  then
    echo "E: Doesn't exist"
    return
  fi
  docker-compose down
}

function run() {
  iscreated
  if [ $? -eq 0 ]
  then
    echo "E: Already exists"
    return
  fi
  docker-compose up -d --build && getip
  echo "Final Configuration and Init"
  docker cp ../aerolab-ldap/certs ${container_name}:/root/
  liburl="github.com/aerospike/aerospike-client-go"
  docker exec -it ${container_name} /bin/bash -c "source /root/.bashrc; cd /root/go/src/v4/aerospike-basic; go mod init; go get ${liburl}"
  docker exec -it ${container_name} /bin/bash -c "source /root/.bashrc; cd /root/go/src/v4/aerospike-auth; go mod init; go get ${liburl}"
  docker exec -it ${container_name} /bin/bash -c "source /root/.bashrc; cd /root/go/src/v4/aerospike-tls; go mod init; go get ${liburl}"
  docker exec -it ${container_name} /bin/bash -c "source /root/.bashrc; cd /root/go/src/v5/aerospike-basic; go mod init; go get ${liburl}/v5"
  docker exec -it ${container_name} /bin/bash -c "source /root/.bashrc; cd /root/go/src/v5/aerospike-auth; go mod init; go get ${liburl}/v5"
  docker exec -it ${container_name} /bin/bash -c "source /root/.bashrc; cd /root/go/src/v5/aerospike-tls; go mod init; go get ${liburl}/v5"
}

function getip() {
  container_name=${basedir}_${name_host}_1
  clientip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${basedir}_${name_host}_1 2>/dev/null)
  if [ $? -ne 0 ]
  then
    container_name="${basedir}-${name_host}-1"
    clientip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${basedir}-${name_host}-1 2>/dev/null)
    if [ $? -ne 0 ]
    then
        echo "CLIENT IP  not found"
        container_name=""
        return
    fi
  fi
  echo ""
  echo "CLIENT IP: ${clientip} (${container_name})"
  echo ""
}

function checkdeps() {
  docker version 1>/dev/null 2>&1
  RET=$?
  if [ ${RET} -ne 0 ]
  then
    echo "E: docker command not available, or docker not running"
    return ${RET}
  fi
  docker-compose version 1>/dev/null 2>&1
  RET=$?
  if [ ${RET} -ne 0 ]
  then
    echo "E: docker-compose command not available"
    return ${RET}
  fi
}

function attach() {
  getip
  if [ "${container_name}" = "" ]
  then
    echo "ERROR: container not found"
    return
  fi
  docker exec -it ${container_name} /bin/bash -c "cd /root/go && /bin/bash"
}

replaceconf
if [ "${1}" = "start" ]
then
  checkdeps && start
elif [ "${1}" = "stop" ]
then
  checkdeps && stop
elif [ "${1}" = "destroy" ]
then
  checkdeps && destroy
elif [ "${1}" = "run" ]
then
  checkdeps && run
elif [ "${1}" = "get" ]
then
  checkdeps && getip
elif [ "${1}" = "attach" ]
then
  checkdeps && attach
elif [ "${1}" = "version" ]
then
  echo "${version}"
else
  echo ""
  echo "Usage: ${0} start|stop|destroy|run|get"
  echo ""
  echo "  run     - create and start Client stack"
  echo "  start   - start an existing, stopped, Client stack"
  echo "  stop    - stop a running Client stack, without destroying it"
  echo "  get     - get the IPs of Client stack"
  echo "  attach  - attach to the client container"
  echo "  destroy - stop and destroy the Client stack"
  echo "  version - print version number of this script and stack"
  echo ""
fi
restoreconf
