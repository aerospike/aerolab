./sign-pkg-notarize.sh || exit 1
sleep 60
./sign-zip-notarize.sh || exit 1
cd ../packages
rm -f sha256.sum; ls |while read line; do shasum -a 256 $line >> sha256.sum; done