cd ../src && bash ./build.sh || exit 1
cd ../bin
./test.sh |tee test-result.log
