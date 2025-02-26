.NOTPARALLEL:

## requirements: make dpkg rpmbuild upx golang zip wget jq
## macos pkg requirement: https://docker-laptop.s3.eu-west-1.amazonaws.com/Packages.pkg

## environment variable import
SIGNER := "$(aerolab_signer)"
INSTALLSIGNER := "$(aerolab_installsigner)"
APPLEID := "$(aerolab_appleid)"
APPLEPW := "$(aerolab_applepw)"
TEAMID := "$(aerolab_teamid)"

## available make commands
.PHONY: help
help:
	@printf "\nSHORTHANDS:\n\
	\tmake build && make install                 - build and install on current system\n\
	\tmake build-linux && make pkg-linux         - build and package all linux releases\n\
	\n\
	UPDATE COMMANDS:\n\
	\tdeps               - Update all dependencies in the project\n\
	\n\
	BUILD COMMANDS:\n\
	\tbuild              - A version for the current system (linux and mac only)\n\
	\tbuildall           - All versions for all supported systems\n\
	\tbuild-linux-amd64  - Linux on x86_64\n\
	\tbuild-linux-arm64  - Linux on aarch64\n\
	\tbuild-linux        - Linux on x86_64 and aarach64\n\
	\tbuild-darwin-amd64 - MacOS on x86_64\n\
	\tbuild-darwin-arm64 - MacOS on M1/M2 style aarach64\n\
	\tbuild-darwin       - MacOS on x86_64 and aarch64\n\
	\tbuild-windows-amd64- Windows on x86_64 and aarch64\n\
	\tbuild-windows-arm64- Windows on ARM\n\
	\tbuild-windows      - Windows on x86_64 and arm64\n\
	\n\
	INSTALL COMMANDS:\n\
	\tinstall            - Install a previously built aerolab on the current system (linux and mac only)\n\
	\tnosudo-install     - Install a previously built aerolab on the current system (linux and mac only), no sudo\n\
	\n\
	CLEAN COMMANDS:\n\
	\tclean              - Remove remainders of a build and reset source modified during build\n\
	\tcleanall           - Clean and remove all created packages\n\
	\n\
	LINUX PACKAGING COMMANDS:\n\
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
	WINDOWS PACKAGING COMMANDS:\n\
	\tpkg-windows-zip       - Package windows zip\n\
	\tpkg-windows-zip-amd64 - Package windows zip for amd64 only\n\
	\tpkg-windows-zip-arm64 - Package windows zip for arm64 only\n\
	\n\
	MACOS PACKAGING COMMANDS:\n\
	\tmacos-codesign     - Codesign MacOS binaries\n\
	\tmacos-pkg-build    - Create MacOS pkg installer\n\
	\tmacos-pkg-sign     - Productsign MacOS pkg installer\n\
	\tmacos-pkg-notarize - Codesign MacOS binaries\n\
	\tmacos-zip-build    - Create MacOS zip packages\n\
	\tmacos-zip-notarize - Notarize MacOS ZIP packages\n\
	\tmacos-build-all    - Build and sign pkg and zip\n\
	\tmacos-notarize-all - Notarize pkg and zip\n\
	\n\
	OUTPUTS: ../bin/ and ../bin/packages/\n\
	"

.PHONY: deps
deps:
	cd ../expiries/aws && go get -u && go mod tidy && GOWORK=off go mod vendor
	cd ../expiries/gcp && go get -u && go mod tidy
	go get -u && go mod tidy && GOWORK=off go mod vendor

.PHONY: webdebug
webdebug:
	go generate
	[ ! -f webui/www.tgz ] && touch webui/www.tgz || echo
	[ ! -f agiproxy.tgz ] && touch agiproxy.tgz || echo
	go run . webui --web-path ../web/dev --web-no-override --listen 0.0.0.0:3333 --webroot /aerolab/ --debug-requests --real

.PHONY: webdebug-hosted
webdebug-hosted:
	go generate
	[ ! -f webui/www.tgz ] && touch webui/www.tgz || echo
	[ ! -f agiproxy.tgz ] && touch agiproxy.tgz || echo
	go run . webui --web-path ../web/dev --web-no-override --listen 0.0.0.0:3333 --webroot /aerolab/ --debug-requests --real --block-server-ls

.PHONY: macos-build-all
macos-build-all: macos-codesign macos-zip-build macos-pkg-build macos-pkg-sign

.PHONY: macos-notarize-all
macos-notarize-all: macos-pkg-notarize macos-zip-notarize

.PHONY: build
build: run_build

.PHONY: buildall
buildall: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_linux_amd64 compile_linux_arm64 compile_darwin compile_windows reset2

.PHONY: build-linux-amd64
build-linux-amd64: prep compile_linux_wip_arm64 reset1 compile_linux_amd64 reset2

.PHONY: build-linux-arm64
build-linux-arm64: prep compile_linux_wip_amd64 reset1 compile_linux_arm64 reset2

.PHONY: build-linux
build-linux: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_linux_amd64 compile_linux_arm64 reset2

.PHONY: build-darwin-amd64
build-darwin-amd64: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_darwin_amd64 reset2

.PHONY: build-darwin-arm64
build-darwin-arm64: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_darwin_arm64 reset2

.PHONY: build-darwin
build-darwin: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_darwin reset2

.PHONY: build-windows-amd64
build-windows-amd64: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_windows_amd64 reset2

.PHONY: build-windows-arm64
build-windows-arm64: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_windows_arm64 reset2

.PHONY: build-windows
build-windows: prep compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_windows reset2

.PHONY: install
install: run_install

.PHONY: cleanall
cleanall: clean
	rm -f ../bin/packages/*
	rm -f notarize_result_pkg notarize_result_amd64 notarize_result_arm64
	rm -f ../bin/AeroLab.pkg

.PHONY: clean
clean:
	rm -f aerolab-linux-amd64-wip
	rm -f aerolab-linux-arm64-wip
	rm -f *.upx
	rm -f aerolab-linux-amd64
	rm -f aerolab-linux-arm64
	rm -f aerolab-macos-amd64
	rm -f aerolab-macos-arm64
	rm -f aerolab-windows-amd64.exe
	rm -f aerolab-windows-arm64.exe
	rm -f embed_*.txt
	rm -f myFunction.zip
	rm -f gcpMod.txt
	rm -f gcpFunction.txt
	rm -f ../bin/aerolab-*
	rm -f ../bin/deb
	rm -f ../bin/deb.deb
	rm -f ../bin/aerolab
	printf "package main\n\nvar nLinuxBinaryX64 []byte\n\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	cp embed_linux.go embed_darwin.go
	cp embed_linux.go embed_windows.go

## actual code

OS := $(shell uname -o)
CPU := $(shell uname -m)
ver:=$(shell bash -c 'V=$$(git branch --show-current); if [[ $$V == v* ]]; then printf $${V:1} > ../VERSION.md; fi; cat ../VERSION.md')
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

.PHONY: nosudo-install
nosudo-install:
ifeq ($(OS), Darwin)
ifeq ($(CPU), x86_64)
	cp ../bin/aerolab-macos-amd64 /usr/local/bin/aerolab
else
	cp ../bin/aerolab-macos-arm64 /usr/local/bin/aerolab
endif
else
ifeq ($(CPU), x86_64)
	cp ../bin/aerolab-linux-amd64 /usr/local/bin/aerolab
else
	cp ../bin/aerolab-linux-arm64 /usr/local/bin/aerolab
endif
endif

.PHONY: reset1
reset1:
	printf "package main\n\nvar nLinuxBinaryX64 []byte\n\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	cp embed_linux.go embed_darwin.go
	cp embed_linux.go embed_windows.go

.PHONY: reset2
reset2:
	rm -f aerolab-linux-amd64-wip
	rm -f aerolab-linux-arm64-wip
	rm -f *.upx
	rm -f embed_*.txt
	printf "package main\n\nvar nLinuxBinaryX64 []byte\n\nvar nLinuxBinaryArm64 []byte\n" > embed_linux.go
	cp embed_linux.go embed_darwin.go
	cp embed_linux.go embed_windows.go

.PHONY: prep
prep:
	go generate
	cd ../expiries/aws && $(MAKE)
	cd ../web && ./build.sh

.PHONY: compile_linux_wip_amd64
compile_linux_wip_amd64:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-amd64-wip
ifneq (, $(shell which upx))
	upx aerolab-linux-amd64-wip
endif

.PHONY: compile_linux_wip_arm64
compile_linux_wip_arm64:
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-linux-arm64-wip
ifneq (, $(shell which upx))
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

.PHONY: compile_windows
compile_windows:
	printf "package main\n\nimport _ \"embed\"\n\n//go:embed aerolab-linux-amd64-wip\nvar nLinuxBinaryX64 []byte\n\n//go:embed aerolab-linux-arm64-wip\nvar nLinuxBinaryArm64 []byte" > embed_windows.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-windows-amd64.exe
	env CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-windows-arm64.exe
	mv aerolab-windows-amd64.exe ../bin/
	mv aerolab-windows-arm64.exe ../bin/

.PHONY: compile_windows_amd64
compile_windows_amd64:
	printf "package main\n\nimport _ \"embed\"\n\n//go:embed aerolab-linux-amd64-wip\nvar nLinuxBinaryX64 []byte\n\n//go:embed aerolab-linux-arm64-wip\nvar nLinuxBinaryArm64 []byte" > embed_windows.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o aerolab-windows-amd64.exe
	mv aerolab-windows-amd64.exe ../bin/

.PHONY: compile_windows_arm64
compile_windows_arm64:
	printf "package main\n\nimport _ \"embed\"\n\n//go:embed aerolab-linux-amd64-wip\nvar nLinuxBinaryX64 []byte\n\n//go:embed aerolab-linux-arm64-wip\nvar nLinuxBinaryArm64 []byte" > embed_windows.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o aerolab-windows-arm64.exe
	mv aerolab-windows-arm64.exe ../bin/

.PHONY: official
official: prep
	printf "" > embed_tail.txt

.PHONY: prerelease
prerelease: prep
	printf -- "-prerelease" > embed_tail.txt

.PHONY: build-official
build-official: official compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_linux_amd64 compile_linux_arm64 compile_darwin compile_windows reset2

.PHONY: build-prerelease
build-prerelease: prerelease compile_linux_wip_amd64 compile_linux_wip_arm64 reset1 compile_linux_amd64 compile_linux_arm64 compile_darwin compile_windows reset2

RET := $(shell echo)

.PHONY: pkg-deb-amd64
pkg-deb-amd64:
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
	cp ../bin/aerolab-linux-amd64 ../bin/aerolab
	rm -f ../bin/packages/aerolab-linux-amd64-${ver}.zip
	bash -ce "cd ../bin && zip packages/aerolab-linux-amd64-${ver}.zip aerolab"
	rm -f ../bin/aerolab

.PHONY: pkg-zip-arm64
pkg-zip-arm64:
	cp ../bin/aerolab-linux-arm64 ../bin/aerolab
	rm -f ../bin/packages/aerolab-linux-arm64-${ver}.zip
	bash -ce "cd ../bin && zip packages/aerolab-linux-arm64-${ver}.zip aerolab"
	rm -f ../bin/aerolab

.PHONY: pkg-windows-zip
pkg-windows-zip: pkg-windows-zip-amd64 pkg-windows-zip-arm64

.PHONY: pkg-windows-zip-amd64
pkg-windows-zip-amd64:
	cp ../bin/aerolab-windows-amd64.exe ../bin/aerolab.exe
	rm -f ../bin/packages/aerolab-windows-amd64-${ver}.zip
	bash -ce "cd ../bin && zip packages/aerolab-windows-amd64-${ver}.zip aerolab.exe"
	rm -f ../bin/aerolab.exe

.PHONY: pkg-windows-zip-arm64
pkg-windows-zip-arm64:
	cp ../bin/aerolab-windows-arm64.exe ../bin/aerolab.exe
	rm -f ../bin/packages/aerolab-windows-arm64-${ver}.zip
	bash -ce "cd ../bin && zip packages/aerolab-windows-arm64-${ver}.zip aerolab.exe"
	rm -f ../bin/aerolab.exe

.PHONY: pkg-zip
pkg-zip: pkg-zip-amd64 pkg-zip-arm64

.PHONY: pkg-rpm-amd64
pkg-rpm-amd64:
	rm -rf ../bin/aerolab-rpm-centos
	cp -a ../bin/aerolabrpm ../bin/aerolab-rpm-centos
	sed -i.bak "s/VERSIONHERE/${ver}/g" ../bin/aerolab-rpm-centos/aerolab.spec
	cp ../bin/aerolab-linux-amd64 ../bin/aerolab-rpm-centos/usr/bin/aerolab
	rm -f ../bin/aerolab-linux-x86_64.rpm
	bash -ce "cd ../bin && rpmbuild --target=x86_64-redhat-linux --buildroot \$$(pwd)/aerolab-rpm-centos -bb aerolab-rpm-centos/aerolab.spec"
	rm -f ../bin/packages/aerolab-linux-amd64-${ver}.rpm
	mv ../bin/aerolab-linux-x86_64.rpm ../bin/packages/aerolab-linux-amd64-${ver}.rpm

.PHONY: pkg-rpm-arm64
pkg-rpm-arm64:
	rm -rf ../bin/aerolab-rpm-centos
	cp -a ../bin/aerolabrpm ../bin/aerolab-rpm-centos
	sed -i.bak "s/VERSIONHERE/${ver}/g" ../bin/aerolab-rpm-centos/aerolab.spec
	cp ../bin/aerolab-linux-arm64 ../bin/aerolab-rpm-centos/usr/bin/aerolab
	rm -f ../bin/aerolab-linux-arm64.rpm
	bash -ce "cd ../bin && rpmbuild --target=arm64-redhat-linux --buildroot \$$(pwd)/aerolab-rpm-centos -bb aerolab-rpm-centos/aerolab.spec"
	rm -f ../bin/packages/aerolab-linux-arm64-${ver}.rpm
	mv ../bin/aerolab-linux-arm64.rpm ../bin/packages/aerolab-linux-arm64-${ver}.rpm

.PHONY: pkg-rpm
pkg-rpm: pkg-rpm-amd64 pkg-rpm-arm64

.PHONY: pkg-linux
pkg-linux: pkg-zip pkg-deb pkg-rpm

### note - static linking
###go build -ldflags="-extldflags=-static"

.PHONY: macos-codesign
macos-codesign:
ifeq (exists, $(shell [ -f ../bin/aerolab-macos-amd64 ] && echo "exists" || echo "not found"))
	codesign --verbose --deep --timestamp --force --options runtime --sign ${SIGNER} ../bin/aerolab-macos-amd64
	codesign --verbose --verify ../bin/aerolab-macos-amd64
endif
ifeq (exists, $(shell [ -f ../bin/aerolab-macos-arm64 ] && echo "exists" || echo "not found"))
	codesign --verbose --deep --timestamp --force --options runtime --sign ${SIGNER} ../bin/aerolab-macos-arm64
	codesign --verbose --verify ../bin/aerolab-macos-arm64
endif

.PHONY: macos-zip-build
macos-zip-build:
ifeq (exists, $(shell [ -f ../bin/aerolab-macos-amd64 ] && echo "exists" || echo "not found"))
	cp ../bin/aerolab-macos-amd64 ../bin/aerolab
	rm -f ../bin/packages/aerolab-macos-amd64-${ver}.zip
	bash -ce "cd ../bin && zip packages/aerolab-macos-amd64-${ver}.zip aerolab"
	rm -f ../bin/aerolab
endif
ifeq (exists, $(shell [ -f ../bin/aerolab-macos-arm64 ] && echo "exists" || echo "not found"))
	cp ../bin/aerolab-macos-arm64 ../bin/aerolab
	rm -f ../bin/packages/aerolab-macos-arm64-${ver}.zip
	bash -ce "cd ../bin && zip packages/aerolab-macos-arm64-${ver}.zip aerolab"
	rm -f ../bin/aerolab
endif

.PHONY: macos-zip-notarize
macos-zip-notarize:
ifeq (exists, $(shell [ -f ../bin/packages/aerolab-macos-amd64-${ver}.zip ] && echo "exists" || echo "not found"))
	rm -f notarize_result_amd64
	xcrun notarytool submit --apple-id ${APPLEID} --password ${APPLEPW} --team-id ${TEAMID} -f json --wait --timeout 10m ../bin/packages/aerolab-macos-amd64-${ver}.zip > notarize_result_amd64
	if [ "$$(cat notarize_result_amd64 |jq -r .status)" != "Accepted" ] ;\
	then \
		echo "ZIP-AMD FAILED TO NOTARIZE" ;\
		cat notarize_result_amd64 ;\
		exit 1 ;\
	else \
		echo "ZIP-AMD NOTARIZE SUCCESS" ;\
	fi
endif
ifeq (exists, $(shell [ -f ../bin/packages/aerolab-macos-arm64-${ver}.zip ] && echo "exists" || echo "not found"))
	rm -f notarize_result_arm64
	xcrun notarytool submit --apple-id ${APPLEID} --password ${APPLEPW} --team-id ${TEAMID} -f json --wait --timeout 10m ../bin/packages/aerolab-macos-arm64-${ver}.zip > notarize_result_arm64
	if [ "$$(cat notarize_result_arm64 |jq -r .status)" != "Accepted" ] ;\
	then \
		echo "ZIP-ARM FAILED TO NOTARIZE" ;\
		cat notarize_result_arm64 ;\
		exit 1 ;\
	else \
		echo "ZIP-ARM NOTARIZE SUCCESS" ;\
	fi
endif

.PHONY: macos-pkg-build
macos-pkg-build:
	cp -a ../bin/aerolab-macos-amd64 ../bin/macos-pkg/aerolab/
	cp -a ../bin/aerolab-macos-arm64 ../bin/macos-pkg/aerolab/
	sed "s/AEROLABVERSIONHERE/${ver}/g" ../bin/macos-pkg/AeroLab-template.pkgproj > ../bin/macos-pkg/AeroLab.pkgproj
	bash -ce "cd ../bin/macos-pkg && /usr/local/bin/packagesbuild --project AeroLab.pkgproj"
	mv ../bin/macos-pkg/build/AeroLab.pkg ../bin/aerolab-macos-${ver}-unsigned.pkg

.PHONY: macos-pkg-sign
macos-pkg-sign:
	productsign --timestamp --sign ${INSTALLSIGNER} ../bin/aerolab-macos-${ver}-unsigned.pkg ../bin/packages/aerolab-macos-${ver}.pkg

.PHONY: macos-pkg-notarize
macos-pkg-notarize:
	rm -f notarize_result_pkg
	xcrun notarytool submit --apple-id ${APPLEID} --password ${APPLEPW} --team-id ${TEAMID} -f json --wait --timeout 10m ../bin/packages/aerolab-macos-${ver}.pkg > notarize_result_pkg
	if [ "$$(cat notarize_result_pkg |jq -r .status)" != "Accepted" ] ;\
	then \
		echo "PKG FAILED TO NOTARIZE" ;\
		cat notarize_result_pkg ;\
		exit 1 ;\
	else \
		echo "PKG NOTARIZE SUCCESS" ;\
	fi

#.PHONY: tools
#tools: ginkgo

#.PHONY: ginkgo
#ginkgo: ../bin/ginkgo
#../bin/ginkgo:
#	go build -o ../bin/ginkgo github.com/onsi/ginkgo/v2/ginkgo

### make cleanall && make build-prerelease && make pkg-linux && make pkg-windows-zip && make macos-build-all && make macos-notarize-all
### make cleanall && make build-official && make pkg-linux && make pkg-windows-zip && make macos-build-all && make macos-notarize-all
