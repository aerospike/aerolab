docker stop openssh
docker rm openssh
docker rmi openssh:1
./build.sh && ./run.sh
