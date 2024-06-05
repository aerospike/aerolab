CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o test-web-server.linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o test-web-server.linux-arm64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o test-web-server.macos-amd64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o test-web-server.macos-arm64 .
