echo "Initializing"
ver=$(cat ../../VERSION.md)
# auth with apple
[ -f ~/xcode-secrets.sh ] && source ~/xcode-secrets.sh

if [ "${APPLE_ID}" = "" ]
then
  printf "AppleID: "
  read APPLE_ID || exit 1
fi

if [ "${APPLE_ID_PASSWORD}" = "" ]
then
  printf "Password: "
  read -s APPLE_ID_PASSWORD ||exit 1
  echo
fi

[ "${APPLE_ID_PASSWORD}" = "" ] && echo "Empty password, exiting" && exit 1
[ "${APPLE_ID}" = "" ] && echo "Empty username, exiting" && exit 1

# cleanup
rm -f packages/aerolab/aerolab-*
rm -rf packages/build
rm -f notarize_result
rm -f notarization_progress
cp ../aerolab-macos-amd64 packages/aerolab/
cp ../aerolab-macos-amd64 packages/aerolab/

function signer() {
    echo "cleanup done"
    ls
    echo "========="

    BIN=packages/aerolab/${1}
    cp ../${1} packages/aerolab/
    echo "FILE: ${1}"
    echo "DEST: ${BIN}"

    ##echo "Press ENTER to sign"
    ##read

    # codesign and test
    echo "Codesigning and verifying"
    codesign --verbose --deep --timestamp --force --options runtime --sign "Developer ID Application: Aerospike, Inc. (22224RFU67)" ${BIN} && \
    codesign --verbose --verify ${BIN} || exit 1
}

signer aerolab-macos-amd64
signer aerolab-macos-arm64

rm -rf ~/Desktop/packages
cp -a packages ~/Desktop/.
pushd ~/Desktop/packages
sed -i.bak "s/AEROLABVERSIONHERE/${ver}/g" AeroLab.pkgproj
/usr/local/bin/packagesbuild --project AeroLab.pkgproj
cd build

##echo "Press ENTER to sign pkg file"
##read 
productsign --timestamp --sign "Developer ID Installer: Aerospike, Inc. (22224RFU67)" AeroLab.pkg aerolab-macos.pkg || exit 1

##echo "Press ENTER to notarize"
##read 

# notarize
echo "Notarizing"
xcrun -v altool --notarize-app --primary-bundle-id "aerolab" --username "$APPLE_ID" --password "$APPLE_ID_PASSWORD" --file aerolab-macos.pkg --output-format xml | tee notarize_result
[ $? -ne 0 ] && exit 1

# get notarize request UUID
req="$(cat notarize_result | grep -A1 "RequestUUID" | sed -n 's/\s*<string>\([^<]*\)<\/string>/\1/p' | xargs)"
echo "Request: $req"

# wait for notarization to succeed
echo "Wait for $req"
while true; do
  printf "."
  sleep 10
  xcrun altool --notarization-info "$req" -u "$APPLE_ID" -p "$APPLE_ID_PASSWORD" > notarization_progress 2>&1
  if grep -q "Status: success" notarization_progress; then
    echo ""
    cat notarization_progress
    echo "Notarization succeeded"
    break
  elif grep -q "Status: in progress" notarization_progress; then
    continue
  else
    cat notarization_progress
    echo "Notarization failed"
    exit 1
  fi
done

popd
cp ~/Desktop/packages/build/aerolab-macos.pkg ../final/aerolab-macos.pkg
echo "Done"
