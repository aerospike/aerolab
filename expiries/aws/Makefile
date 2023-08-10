.NOTPARALLEL:

.PHONY: build
build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap -tags lambda.norpc .
	zip myFunction.zip bootstrap
	rm -f bootstrap
	cp myFunction.zip ../../src/
