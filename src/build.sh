MPATH=$(pwd)
rm -f embed_darwin.go
env GOOS=linux GOARCH=amd64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-linux || exit 1
upx aerolab-linux || exit 1
printf "package main\n\nvar nLinuxBinary=\`" > embed_darwin.go
cat aerolab-linux |base64 |sed 's/\n//g' >>embed_darwin.go
printf "\`" >> embed_darwin.go
env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=11.3" CGO_LDFLAGS="-mmacosx-version-min=11.3" go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-macos || exit 1
cp embed_linux.go embed_darwin.go
