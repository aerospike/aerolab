.NOTPARALLEL:
## available make commands

.PHONY: help
help:
	@printf "\nSHORTHANDS:\n\
	\tmake build install                         - build and install on current system\n\
	\tmake build-linux pkg-linux                 - build and package all linux releases\n\
	\n\
	BUILD COMMANDS:\n\
	\tbuild              - A version for the current system\n\
	\tbuildall           - All versions for all supported systems\n\
	\tbuild-linux-amd64  - Linux on x86_64\n\
	\tbuild-linux-arm64  - Linux on aarch64\n\
	\tbuild-linux        - Linux on x86_64 and aarach64\n\
	\tbuild-darwin-amd64 - MacOS on x86_64\n\
	\tbuild-darwin-arm64 - MacOS on M1/M2 style aarach64\n\
	\tbuild-darwin       - MacOS on x86_64 and aarch64\n\
	\n\
	INSTALL COMMANDS:\n\
	\tinstall            - Install a previously built aerolab on the current system\n\
	\n\
	CLEAN COMMANDS:\n\
	\tclean              - Remove remainders of a build and reset source modified during build\n\
	\n\
	PACKAGING COMMANDS:\n\
	\tpkg-linux          - Package all linux packages - zip, rpm and deb\n\
	\tpkg-zip            - Package linux zip\n\
	\tpkg-rpm            - Package linux rpm\n\
	\tpkg-deb            - Package linux deb\n\
	\tpkg-zip-amd64      - Package linux zip for amd64 only\n\
	\tpkg-rpm-amd64      - Package linux rpm for amd64 only\n\
	\tpkg-deb-amd64      - Package linux deb for amd64 only\n\
	\tpkg-zip-arm64      - Package linux zip for arm64 only\n\
	\tpkg-rpm-arm64      - Package linux rpm for arm64 only\n\
	\tpkg-deb-arm64      - Package linux deb for arm64 only\n\
	\n\
	OUTPUTS: ../bin/ and ../bin/packages/\n\
	"

.PHONY: build
build: run_build

.PHONY: buildall
buildall: clean prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_linux_amd64 compile_linux_arm64 compile_darwin reset2

.PHONY: build-linux-amd64
build-linux-amd64: clean prep compile_linux_wip_arm64 reset1 compile_linux_amd64 reset2

.PHONY: build-linux-arm64
build-linux-arm64: clean prep compile_linux_wip_amd64 reset1 compile_linux_arm64 reset2

.PHONY: build-linux
build-linux: clean prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_linux_amd64 compile_linux_arm64 reset2

.PHONY: build-darwin-amd64
build-darwin-amd64: clean prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_darwin_amd64 reset2

.PHONY: build-darwin-arm64
build-darwin-arm64: clean prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_darwin_arm64 reset2

.PHONY: build-darwin
build-darwin: clean prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_darwin reset2

.PHONY: install
install: run_install

.PHONY: clean
clean:
	rm -f aerolab-linux-amd64-wip
	rm -f aerolab-linux-arm64-wip
	rm -f *.upx
	rm -f aerolab-linux-amd64
	rm -f aerolab-linux-arm64
	rm -f aerolab-macos-amd64
	rm -f aerolab-macos-arm64
	rm -f embed_*.txt
	rm -f ../bin/aerolab-*
	rm -f ../bin/packages/*
	rm -f ../bin/deb
	rm -f ../bin/deb.deb
	rm -f ../bin/aerolab
	printf "package main\n\nvar nLinuxBinaryX64 []byte\n\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	cp embed_linux.go embed_darwin.go

## actual code

OS := $(shell uname -o)
CPU := $(shell uname -m)
ver:=$(shell cat ../VERSION.md)
define _amddebscript
ver=$(cat ../VERSION.md)
cat <<EOF > ../bin/deb/DEBIAN/control
Website: www.aerospike.com
Maintainer: Aerospike <support@aerospike.com>
Name: AeroLab
Package: aerolab
Section: aerospike
Version: ${ver}
Architecture: amd64
Description: Tool for deploying non-prod Aerospike server clusters on docker, GCP or in AWS
EOF
endef
export amddebscript = $(value _amddebscript)
define _armdebscript
ver=$(cat ../VERSION.md)
cat <<EOF > ../bin/deb/DEBIAN/control
Website: www.aerospike.com
Maintainer: Aerospike <support@aerospike.com>
Name: AeroLab
Package: aerolab
Section: aerospike
Version: ${ver}
Architecture: arm64
Description: Tool for deploying non-prod Aerospike server clusters on docker, GCP or in AWS
EOF
endef
export armdebscript = $(value _armdebscript)

.PHONY: run_build
run_build:
ifeq ($(OS), Darwin)
ifeq ($(CPU), x86_64)
	$(MAKE) build-darwin-amd64
else
	$(MAKE) build-darwin-arm64
endif
else
ifeq ($(CPU), x86_64)
	$(MAKE) build-linux-amd64
else
	$(MAKE) build-linux-arm64
endif
endif

.PHONY: run_install
run_install:
ifeq ($(OS), Darwin)
ifeq ($(CPU), x86_64)
	sudo cp ../bin/aerolab-macos-amd64 /usr/local/bin/aerolab
else
	sudo cp ../bin/aerolab-macos-arm64 /usr/local/bin/aerolab
endif
else
ifeq ($(CPU), x86_64)
	sudo cp ../bin/aerolab-linux-amd64 /usr/local/bin/aerolab
else
	sudo cp ../bin/aerolab-linux-arm64 /usr/local/bin/aerolab
endif
endif

.PHONY: reset1
reset1:
	printf "package main\n\nvar nLinuxBinaryX64 []byte\n\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	cp embed_linux.go embed_darwin.go

.PHONY: reset2
reset2:
	rm -f aerolab-linux-amd64-wip
	rm -f aerolab-linux-arm64-wip
	rm -f *.upx
	rm -f embed_*.txt
	printf "package main\n\nvar nLinuxBinaryX64 []byte\n\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	cp embed_linux.go embed_darwin.go

.PHONY: prep
prep:
	go generate

.PHONY: compile_linux_wip_amd64
compile_linux_wip_amd64:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-amd64-wip
ifeq (, $(shell which upx))
$(warning "No upx in $(PATH), binaries will not be shrunk")
else
	upx aerolab-linux-amd64-wip
endif

.PHONY: compile_linux_wip_arm64
compile_linux_wip_arm64:
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-arm64-wip
ifeq (, $(shell which upx))
$(warning "No upx in $(PATH), binaries will not be shrunk")
else
	upx aerolab-linux-arm64-wip
endif

.PHONY: compile_linux_amd64
compile_linux_amd64:
	printf "package main\n\nimport _ \"embed\"\n\nvar nLinuxBinaryX64 []byte\n\n//go:embed aerolab-linux-arm64-wip\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-amd64
	mv aerolab-linux-amd64 ../bin/

.PHONY: compile_linux_arm64
compile_linux_arm64:
	printf "package main\n\nimport _ \"embed\"\n\n//go:embed aerolab-linux-amd64-wip\nvar nLinuxBinaryX64 []byte\n\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-arm64
	mv aerolab-linux-arm64 ../bin/

.PHONY: compile_darwin
compile_darwin:
	printf "package main\n\nimport _ \"embed\"\n\n//go:embed aerolab-linux-amd64-wip\nvar nLinuxBinaryX64 []byte\n\n//go:embed aerolab-linux-arm64-wip\nvar nLinuxBinaryArm64 []byte" > embed_darwin.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-macos-amd64
	env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-macos-arm64
	mv aerolab-macos-amd64 ../bin/
	mv aerolab-macos-arm64 ../bin/

.PHONY: compile_darwin_amd64
compile_darwin_amd64:
	printf "package main\n\nimport _ \"embed\"\n\n//go:embed aerolab-linux-amd64-wip\nvar nLinuxBinaryX64 []byte\n\n//go:embed aerolab-linux-arm64-wip\nvar nLinuxBinaryArm64 []byte" > embed_darwin.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-macos-amd64
	mv aerolab-macos-amd64 ../bin/

.PHONY: compile_darwin_arm64
compile_darwin_arm64:
	printf "package main\n\nimport _ \"embed\"\n\n//go:embed aerolab-linux-amd64-wip\nvar nLinuxBinaryX64 []byte\n\n//go:embed aerolab-linux-arm64-wip\nvar nLinuxBinaryArm64 []byte" > embed_darwin.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-macos-arm64
	mv aerolab-macos-arm64 ../bin/

.PHONY: official
official: clean prep
	printf "" > embed_tail.txt

.PHONY: build-official
build-official: official compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_linux_amd64 compile_linux_arm64 compile_darwin reset2

RET := $(shell echo)

.PHONY: pkg-deb-amd64
pkg-deb-amd64:
ifeq (, $(shell which dpkg-deb))
$(error "No dpkg-deb in $(PATH), install dpkg package")
endif
	cp ../bin/aerolab-linux-amd64 ../bin/aerolab
	rm -rf ../bin/deb
	mkdir -p ../bin/deb/DEBIAN
	mkdir -p ../bin/deb/usr/bin
	@ eval "$$amddebscript"
	mv ../bin/aerolab ../bin/deb/usr/bin/
	sudo dpkg-deb -Zxz -b ../bin/deb
	rm -f ../bin/packages/aerolab-linux-amd64-${ver}.deb
	mv ../bin/deb.deb ../bin/packages/aerolab-linux-amd64-${ver}.deb
	rm -rf ../bin/deb

.PHONY: pkg-deb-arm64
pkg-deb-arm64:
ifeq (, $(shell which dpkg-deb))
$(error "No dpkg-deb in $(PATH), install dpkg package")
endif
	cp ../bin/aerolab-linux-arm64 ../bin/aerolab
	rm -rf ../bin/deb
	mkdir -p ../bin/deb/DEBIAN
	mkdir -p ../bin/deb/usr/bin
	@ eval "$$armdebscript"
	mv ../bin/aerolab ../bin/deb/usr/bin/
	sudo dpkg-deb -Zxz -b ../bin/deb
	rm -f ../bin/packages/aerolab-linux-arm64-${ver}.deb
	mv ../bin/deb.deb ../bin/packages/aerolab-linux-arm64-${ver}.deb
	rm -rf ../bin/deb

.PHONY: pkg-deb
pkg-deb: pkg-deb-amd64 pkg-deb-arm64

.PHONY: pkg-zip-amd64
pkg-zip-amd64:
ifeq (, $(shell which zip))
$(error "No zip in $(PATH), install zip package")
endif
	cp ../bin/aerolab-linux-amd64 ../bin/aerolab
	bash -ce "cd ../bin && zip packages/aerolab-linux-amd64-${ver}.zip aerolab"
	rm -f ../bin/aerolab

.PHONY: pkg-zip-arm64
pkg-zip-arm64:
ifeq (, $(shell which zip))
$(error "No zip in $(PATH), install zip package")
endif
	cp ../bin/aerolab-linux-arm64 ../bin/aerolab
	bash -ce "cd ../bin && zip packages/aerolab-linux-arm64-${ver}.zip aerolab"
	rm -f ../bin/aerolab

.PHONY: pkg-zip
pkg-zip: pkg-zip-amd64 pkg-zip-arm64

.PHONY: pkg-rpm-amd64
pkg-rpm-amd64:
ifeq (, $(shell which rpmbuild))
$(error "No rpmbuild in $(PATH), install rpmbuild package")
endif
	cp -a ../bin/aerolabrpm ../bin/aerolab-rpm-centos
	sed -i.bak "s/VERSIONHERE/${ver}/g" ../bin/aerolab-rpm-centos/aerolab.spec
	cp ../bin/aerolab-linux-amd64 ../bin/aerolab-rpm-centos/usr/bin/aerolab
	bash -ce "cd ../bin && rpmbuild --target=x86_64-redhat-linux --buildroot \$$(pwd)/aerolab-rpm-centos -bb aerolab-rpm-centos/aerolab.spec"
	mv ../bin/aerolab-linux-x86_64.rpm ../bin/packages/aerolab-linux-amd64-${ver}.rpm

.PHONY: pkg-rpm-arm64
pkg-rpm-arm64:
ifeq (, $(shell which rpmbuild))
$(error "No rpmbuild in $(PATH), install rpmbuild package")
endif
	cp -a ../bin/aerolabrpm ../bin/aerolab-rpm-centos
	sed -i.bak "s/VERSIONHERE/${ver}/g" ../bin/aerolab-rpm-centos/aerolab.spec
	cp ../bin/aerolab-linux-arm64 ../bin/aerolab-rpm-centos/usr/bin/aerolab
	bash -ce "cd ../bin && rpmbuild --target=arm64-redhat-linux --buildroot \$$(pwd)/aerolab-rpm-centos -bb aerolab-rpm-centos/aerolab.spec"
	mv ../bin/aerolab-linux-arm64.rpm ../bin/packages/aerolab-linux-arm64-${ver}.rpm

.PHONY: pkg-rpm
pkg-rpm: pkg-rpm-amd64 pkg-rpm-arm64

.PHONY: pkg-linux
pkg-linux: pkg-zip pkg-deb pkg-rpm

### note - static linking
###go build -ldflags="-extldflags=-static"
