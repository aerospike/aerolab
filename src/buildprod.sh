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
go get -u "golang.org/x/crypto/ssh"
go get -u "golang.org/x/crypto/ssh/terminal"

fi


if [ $(basename $(pwd)) != "src" ]; then
cd src || exit 1
fi
MPATH=$(pwd)
env GOOS=linux GOARCH=amd64 go build -gcflags=-trimpath=$MPATH -asmflags=-trimpath=$MPATH -ldflags="-s -w" -o ../bin/aerolab-linux
env GOOS=darwin GOARCH=amd64 go build -gcflags=-trimpath=$MPATH -asmflags=-trimpath=$MPATH -ldflags="-s -w" -o ../bin/aerolab-osx
cd ../bin || exit 1
upx aerolab-linux
upx aerolab-osx
cp aerolab-osx aerolab-osx-aio || exit 1
echo -n ">>>>aerolab-osx-aio>>>>" >> aerolab-osx-aio
cat aerolab-linux >> aerolab-osx-aio || exit 1
echo -n "<<<<aerolab-osx-aio" >> aerolab-osx-aio
