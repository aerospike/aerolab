# cleanup embeddables
rm -f embed_darwin.go
rm -f embed_linux.go

# linux empty skel
cat <<EOF > embed_linux.go
package main

var nLinuxBinaryX64 = []byte{}

var nLinuxBinaryArm64 = []byte{}
EOF

# build linux versions without embedding
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-amd64-wip || exit 1
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-arm64-wip || exit 1
upx aerolab-linux-amd64-wip || exit 1
upx aerolab-linux-arm64-wip || exit 1

# embed arm into amd linux
cat <<EOF > embed_linux.go
package main

import _ "embed"

var nLinuxBinaryX64 []byte

//go:embed aerolab-linux-arm64-wip
var nLinuxBinaryArm64 []byte
EOF
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-amd64 || exit 1

# embed amd into arm linux
cat <<EOF > embed_linux.go
package main

import _ "embed"

//go:embed aerolab-linux-amd64-wip
var nLinuxBinaryX64 []byte

var nLinuxBinaryArm64 []byte
EOF
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-arm64 || exit 1

# reset empty skel
cat <<EOF > embed_linux.go
package main

var nLinuxBinaryX64 []byte

var nLinuxBinaryArm64 []byte
EOF

# build darwin embedding
cat <<EOF > embed_darwin.go
package main

import _ "embed"

//go:embed aerolab-linux-amd64-wip
var nLinuxBinaryX64 []byte

//go:embed aerolab-linux-arm64-wip
var nLinuxBinaryArm64 []byte
EOF

# build macos
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-macos-amd64 || exit 1
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-macos-arm64 || exit 1

# cleanup
cp embed_linux.go embed_darwin.go
rm -f aerolab-linux-amd64-wip
rm -f aerolab-linux-arm64-wip

### note - static linking
###go build -ldflags="-extldflags=-static"
