./sign-pkg-notarize.sh || exit 1
sleep 60
./sign-zip-notarize.sh || exit 1
cd ../final
ls |while read line; do shasum -a 256 $line > $line.sha256; done
