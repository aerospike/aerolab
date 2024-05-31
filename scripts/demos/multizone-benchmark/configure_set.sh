function setsys() {
AEROLAB_MAJOR=$(aerolab version |awk -F'.' '{print $1}' |sed 's/v//g')
AEROLAB_MINOR=$(aerolab version |awk -F'.' '{print $2}')
if [ ${AEROLAB_MAJOR} -lt 5 ]
then
  echo "Minimum aerolab version is 5.4.0"
  exit 1
fi
if [ ${AEROLAB_MAJOR} -eq 5 ]
then
  if [ ${AEROLAB_MINOR} -lt 4 ]
  then
    echo "Minimum aerolab version is 5.4.0"
    exit 1
  fi
fi
rm -f ${AEROLAB_CONFIG_FILE}
[ "${BACKEND}" = "docker" ] && aerolab config backend -t docker
[ "${BACKEND}" = "aws" ] && aerolab config backend -t aws -r ${AWS_REGION}
[ "${BACKEND}" = "gcp" ] && aerolab config backend -t gcp -o ${GCP_PROJECT}
aerolab config defaults -k '*FeaturesFilePath' -v ${FEATURES_FILE} || exit 1
}