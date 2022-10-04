# cleanup
rm -f aerolab-*
rm -rf final
rm -f notarize/*.zip
rm -f notarize/aerolab*

# compile
cd ../src && bash ./build.sh || exit 1
cd ../bin
mv ../src/aerolab-* .

# prepare for packaging
mkdir -p final

# amd64
mv aerolab-linux-amd64 aerolab
zip final/aerolab-linux-amd64.zip aerolab
rm -f aerolab

# arm64
mv aerolab-linux-arm64 aerolab
zip final/aerolab-linux-arm64.zip aerolab
rm -f aerolab

echo "Ready to notarize mac binaries"
