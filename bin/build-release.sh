# dependencies
which dpkg-deb; [ $? -ne 0 ] && brew install dpkg
which rpmbuild; [ $? -ne 0 ] && brew install rpm
which upx; [ $? -ne 0 ] && brew install upx

# cleanup
rm -f aerolab-*
rm -rf final/aerolab-*
rm -f notarize/*.zip notarize/*.pkg
rm -f notarize/aerolab*

# compile
cd ../src && bash ./build.sh || exit 1
cd ../bin
mv ../src/aerolab-* .

# amd64
mv aerolab-linux-amd64 aerolab
zip final/aerolab-linux-amd64.zip aerolab
rm -rf deb
mkdir -p deb/DEBIAN
mkdir -p deb/usr/bin
cat <<'EOF' > deb/DEBIAN/control
Website: www.aerospike.com
Maintainer: Aerospike <support@aerospike.com>
Name: AeroLab
Package: aerolab
Section: aerospike
Version: 4.3.2
Architecture: amd64
Description: Tool for deploying non-prod Aerospike server clusters on docker or in AWS
EOF
mv aerolab deb/usr/bin/
mv deb aerolab-linux-amd64
sudo dpkg-deb -b aerolab-linux-amd64
rm -rf aerolab-linux-amd64
mv aerolab-linux-amd64.deb final/
cd alien
sudo ./alien.pl --to-rpm ../final/aerolab-linux-amd64.deb
mv aerolab-* ../final/
cd ..

# arm64
mv aerolab-linux-arm64 aerolab
zip final/aerolab-linux-arm64.zip aerolab
rm -rf deb
mkdir -p deb/DEBIAN
mkdir -p deb/usr/bin
cat <<'EOF' > deb/DEBIAN/control
Website: www.aerospike.com
Maintainer: Aerospike <support@aerospike.com>
Name: AeroLab
Package: aerolab
Section: aerospike
Version: 4.3.2
Architecture: arm64
Description: Tool for deploying non-prod Aerospike server clusters on docker or in AWS
EOF
mv aerolab deb/usr/bin/
mv deb aerolab-linux-arm64
sudo dpkg-deb -b aerolab-linux-arm64
rm -rf aerolab-linux-arm64
mv aerolab-linux-arm64.deb final/
cd alien
sudo ./alien.pl --to-rpm ../final/aerolab-linux-arm64.deb
mv aerolab-* ../final/
cd ..

echo "Ready to sign, package and notarize mac binaries"
