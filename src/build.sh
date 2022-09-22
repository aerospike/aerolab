MPATH=$(pwd)
env GOOS=linux GOARCH=amd64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-linux || exit 1
env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=11.3" CGO_LDFLAGS="-mmacosx-version-min=11.3" go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-macos || exit 1
upx aerolab-linux || exit 1
upx aerolab-macos || exit 1
printf ">>>>aerolab-osx-aio>>>>" >> aerolab-macos || exit 1
cat aerolab-linux >> aerolab-macos || exit 1
printf "<<<<aerolab-osx-aio" >> aerolab-macos || exit 1

