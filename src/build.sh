MPATH=$(pwd)

# b64
b64switch=""
if [[ "$OSTYPE" != "darwin"* ]]; then b64switch=(-w 0); fi

# cleanup embeddables
rm -f embed_darwin.go
rm -f embed_linux.go

# linux empty skel
cat <<EOF > embed_linux.go
package main

var nLinuxBinaryX64 = ""

var nLinuxBinaryArm64 = ""
EOF

# build linux versions without embedding
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-linux-amd64-wip || exit 1
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-linux-arm64-wip || exit 1
upx aerolab-linux-amd64-wip || exit 1
upx aerolab-linux-arm64-wip || exit 1

# embed arm into amd linux
cat <<EOF > embed_linux.go
package main

var nLinuxBinaryX64 = ""

var nLinuxBinaryArm64 = "$(cat aerolab-linux-arm64-wip |base64 ${b64switch[@]} |sed 's/\n//g')"
EOF
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-linux-amd64 || exit 1

# embed amd into arm linux
cat <<EOF > embed_linux.go
package main

var nLinuxBinaryX64 = "$(cat aerolab-linux-amd64-wip |base64 ${b64switch[@]} |sed 's/\n//g')"

var nLinuxBinaryArm64 = ""
EOF
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-linux-arm64 || exit 1

# reset empty skel
cat <<EOF > embed_linux.go
package main

var nLinuxBinaryX64 = ""

var nLinuxBinaryArm64 = ""
EOF

# build darwin embedding
cat <<EOF > embed_darwin.go
package main

var nLinuxBinaryX64 = "$(cat aerolab-linux-amd64-wip |base64 ${b64switch[@]} |sed 's/\n//g')"

var nLinuxBinaryArm64 = "$(cat aerolab-linux-arm64-wip |base64 ${b64switch[@]} |sed 's/\n//g')"
EOF

# build macos
#env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=11.3" CGO_LDFLAGS="-mmacosx-version-min=11.3" go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-macos-amd64 || exit 1
#env GOOS=darwin GOARCH=arm64 CGO_CFLAGS="-mmacosx-version-min=11.3" CGO_LDFLAGS="-mmacosx-version-min=11.3" go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-macos-arm64 || exit 1
env GOOS=darwin GOARCH=amd64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-macos-amd64 || exit 1
env GOOS=darwin GOARCH=arm64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o aerolab-macos-arm64 || exit 1

# cleanup
cp embed_linux.go embed_darwin.go
rm -f aerolab-linux-amd64-wip
rm -f aerolab-linux-arm64-wip
