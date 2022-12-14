sudo ls
# dependencies
echo "step 1"
which dpkg-deb; [ $? -ne 0 ] && brew install dpkg
which rpmbuild; [ $? -ne 0 ] && brew install rpm
which upx; [ $? -ne 0 ] && brew install upx

# version
echo "step 2"
ver=$(cat ../VERSION.md)

# cleanup
echo "step 3"
rm -rf aerolab-rpm-centos
rm -f aerolab-*
rm -rf final/aerolab-*
rm -f notarize/*.zip notarize/*.pkg
rm -f notarize/aerolab*

# compile
echo "step 4"
cd ../src && bash ./build.sh || exit 1
cd ../bin
mv ../src/aerolab-* .
mkdir -p final

# amd64
echo "step 5"
mv aerolab-linux-amd64 aerolab
zip final/aerolab-linux-amd64.zip aerolab
rm -rf deb
mkdir -p deb/DEBIAN
mkdir -p deb/usr/bin
cat <<EOF > deb/DEBIAN/control
Website: www.aerospike.com
Maintainer: Aerospike <support@aerospike.com>
Name: AeroLab
Package: aerolab
Section: aerospike
Version: ${ver}
Architecture: amd64
Description: Tool for deploying non-prod Aerospike server clusters on docker or in AWS
EOF
mv aerolab deb/usr/bin/
mv deb aerolab-linux-amd64
sudo dpkg-deb -b aerolab-linux-amd64
mv aerolab-linux-amd64.deb final/
cp -a aerolabrpm aerolab-rpm-centos
sed -i.bak "s/VERSIONHERE/${ver}/g" aerolab-rpm-centos/aerolab.spec
cp aerolab-linux-amd64/usr/bin/aerolab aerolab-rpm-centos/usr/bin/aerolab
rm -rf aerolab-linux-amd64
rpmbuild --target=x86_64-redhat-linux --buildroot $(pwd)/aerolab-rpm-centos -bb aerolab-rpm-centos/aerolab.spec
mv aerolab-linux-x86_64.rpm final/

# arm64
echo "step 6"
mv aerolab-linux-arm64 aerolab
zip final/aerolab-linux-arm64.zip aerolab
rm -rf deb
mkdir -p deb/DEBIAN
mkdir -p deb/usr/bin
cat <<EOF > deb/DEBIAN/control
Website: www.aerospike.com
Maintainer: Aerospike <support@aerospike.com>
Name: AeroLab
Package: aerolab
Section: aerospike
Version: ${ver}
Architecture: arm64
Description: Tool for deploying non-prod Aerospike server clusters on docker or in AWS
EOF
mv aerolab deb/usr/bin/
mv deb aerolab-linux-arm64
sudo dpkg-deb -b aerolab-linux-arm64
mv aerolab-linux-arm64.deb final/
cp -a aerolabrpm aerolab-rpm-centos
sed -i.bak "s/VERSIONHERE/${ver}/g" aerolab-rpm-centos/aerolab.spec
cp aerolab-linux-arm64/usr/bin/aerolab aerolab-rpm-centos/usr/bin/aerolab
rm -rf aerolab-linux-arm64
rpmbuild --target=arm64-redhat-linux --buildroot $(pwd)/aerolab-rpm-centos -bb aerolab-rpm-centos/aerolab.spec
mv aerolab-linux-arm64.rpm final/

echo "Ready to sign, package and notarize mac binaries"
