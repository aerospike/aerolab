#!/bin/bash
if [ "$1" == "deps" ]
then
go get -u "github.com/aws/aws-sdk-go/aws"
go get -u "github.com/aws/aws-sdk-go/aws/session"
go get -u "github.com/aws/aws-sdk-go/service/ec2"
go get -u "github.com/bestmethod/go-logger"
go get -u "github.com/aerospike/aerospike-client-go"
go get -u "github.com/kballard/go-shellquote"
go get -u "github.com/BurntSushi/toml"
go get -u "github.com/bestmethod/go-logger"
fi


if [ $(basename $(pwd)) != "src" ]; then
cd src || exit 1
fi
MPATH=$(pwd)
env GOOS=linux GOARCH=amd64 go build -gcflags=-trimpath=$MPATH -asmflags=-trimpath=$MPATH -o ../bin/linux/aerolab
env GOOS=darwin GOARCH=amd64 go build -gcflags=-trimpath=$MPATH -asmflags=-trimpath=$MPATH -o ../bin/osx/aerolab
cd ../bin || exit 1
mkdir -p osx-aio
cp osx/aerolab osx-aio/aerolab || exit 1
echo -n ">>>>aerolab-osx-aio>>>>" >> osx-aio/aerolab
cat linux/aerolab >> osx-aio/aerolab
echo -n "<<<<aerolab-osx-aio" >> osx-aio/aerolab
