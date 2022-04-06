
# basic parameters
basedir=$(basename $(pwd))
name_ldap="ldap1"
name_admin="ldapadmin"
version="v1.3"

# fix docker credential config
function replaceconf() {
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

# check if the stack is already created
function iscreated() {
  docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${basedir}_${name_ldap}_1 2>/dev/null 1>/dev/null
  if [ $? -ne 0 ]
  then
    docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${basedir}-${name_ldap}-1 2>/dev/null 1>/dev/null
    if [ $? -ne 0 ]
    then
      return 1
    fi
  fi
  return 0
}

# restore docker config to previous setup
function restoreconf() {
  mv ~/.docker/config.json.backup ~/.docker/config.json
}

# start containers
function start() {
  docker-compose start && printip && checkrunning
}

#stop containers
function stop() {
  docker-compose stop
}

# destroy containers
function destroy() {
  iscreated
  if [ $? -ne 0 ]
  then
    echo "E: Doesn't exist"
    return
  fi
  docker-compose down
}

# run docker-compose and execute relevant supporting functions
function run() {
  iscreated
  if [ $? -eq 0 ]
  then
    echo "E: Already exists"
    return
  fi
  docker-compose up -d && printip && checkrunning && patchldapadmin
}

# add ldap1 to ldapadmin /etc/hosts file
function patchldapadmin() {
  ldapip=$(getip ${basedir} ${name_ldap})
  if [ "${ldapip}" = "" ]
  then
    echo "LDAP IP  not found"
    return
  fi
  ldapadmin=$(getcontainername ${basedir} ${name_admin})
  if [ "${ldapadmin}" = "" ]
  then
    echo "WARNING: You do not appear to have an LDAP admin server; cannot fix /etc/hosts in ldapadmin"
    return
  fi
  docker exec -it "${ldapadmin}" /bin/bash -c "echo '${ldapip} ${name_ldap}' >> /etc/hosts"
}

# get ip of container given basedir and name of container service
function getip() {
  nip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}_${2}_1 2>/dev/null)
  if [ $? -ne 0 ]
  then
    nip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}-${2}-1 2>/dev/null)
    if [ $? -ne 0 ]
    then
      return
    fi
  fi
  echo ${nip}
}

# print help information for started services
function printip() {
  ldapip=$(getip ${basedir} ${name_ldap})
  if [ "${ldapip}" = "" ]
  then
    echo "LDAP IP  not found"
    return
  fi
  ldapadmin=$(getip ${basedir} ${name_admin})
  if [ "${ldapip}" = "" ]
  then
    echo "LDAPADMIN IP  not found"
    return
  fi
  echo ""
  echo "LDAP IP: ${ldapip}"
  echo "LDAP Admin:"
  echo "  * KubeLab and Direct URL: http://${ldapadmin}"
  echo "  * Username:               cn=admin,dc=aerospike,dc=com"
  echo "  * Password:               admin"
  echo "  * notes:"
  echo "    * if using minikube, you can also use alternative connection URL"
  echo '      * run this to get the URL: echo "http://$(minikube ip):8099"'
  echo "    * if using docker desktop, you can use the below alternative connection URL"
  echo "      * URL: http://127.0.0.1:8099"
  echo ""
  echo "To get useful LDAP commands help, run: ${0} help"
  echo ""
}

# get name of container given basedir and name of container service
function getcontainername() {
  name=""
  docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}_${2}_1 1>/dev/null 2>&1
  if [ $? -eq 0 ]
  then
    name="${1}_${2}_1"
  fi
  docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}-${2}-1 1>/dev/null 2>&1
  if [ $? -eq 0 ]
  then
    name="${1}-${2}-1"
  fi
  echo ${name}
}

# print ldap help of commands
function helpldap() {
  name=$(getcontainername ${basedir} ${name_ldap})
  echo ""
  if [ "${name}" = "" ]
  then
    echo "WARNING: You do not appear to have an LDAP server"
  else
    echo "Connect to ${name_ldap} ldap server shell:"
    echo "  \$ docker exec -it ${name} /bin/bash"
    echo "Get logs for the LDAP server:"
    echo "  \$ docker logs ${name}"
  fi
  echo ""
  echo "Run basic ldapsearch:"
  echo "  \$ ldapsearch -x -H ldap://${name_ldap}/ -D cn=admin,dc=aerospike,dc=com -w admin -b dc=aerospike,dc=com uid=badwan uidNumber dn"
  echo "Run basic ldapsearch with TLS:"
  echo "  \$ ldapsearch -x -H ldaps://${name_ldap}/ -D cn=admin,dc=aerospike,dc=com -w admin -b dc=aerospike,dc=com uid=badwan uidNumber dn"
  echo "Run ldapsearch with TLS and debug mode for connection debugging:"
  echo "  \$ ldapsearch -d 1 -x -ZZ -H ldaps://${name_ldap}/ -D cn=admin,dc=aerospike,dc=com -w admin -b dc=aerospike,dc=com uid=badwan uidNumber dn"
  echo ""
}

# wait for ldap to be fully up
function checkrunning() {
  echo "LDAP is up and available (non-TLS)"
  echo "Waiting for LDAPS to start"
  RET=1
  name=$(getcontainername ${basedir} ${name_ldap})
  while [ $RET -ne 0 ]
  do
    sleep 1
    docker logs ${name} 2>&1 |grep 'slapd starting'
    RET=$?
  done
  echo "Done, LDAP initialised"
}

# check if we have docker and compose
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

### main command logic
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
  checkdeps && printip
elif [ "${1}" = "help" ]
then
  checkdeps && helpldap
elif [ "${1}" = "version" ]
then
  echo "${version}"
else
  echo ""
  echo "Usage: ${0} start|stop|destroy|run|get"
  echo ""
  echo "  run     - create and start LDAP stack"
  echo "  start   - start an existing, stopped, LDAP stack"
  echo "  stop    - stop a running LDAP stack, without destroying it"
  echo "  get     - get the IPs of LDAP stack"
  echo "  help    - get a list of useful commands for cli ldapsearch"
  echo "  destroy - stop and destroy the LDAP stack"
  echo "  version - print version number of this script and stack"
  echo ""
fi
restoreconf
