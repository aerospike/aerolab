# syntax=docker/dockerfile:experimental
# Do not alter the above line

# Must build using
# docker build --ssh default . -t aeromon
# and set environment variable DOCKER_BUILDKIT=1

FROM centos:7

ENV AEROSPIKE_HOST=
ARG GOROOT=/usr/local/go
ENV GOROOT=${GOROOT}
ARG GOPATH=${GOROOT}/packages
ENV GOPATH=${GOPATH}

ENV PATH=${GOPATH}/bin:${GOROOT}/bin:${PATH}

RUN yum updateinfo && \
# Get binaries - which can be useful
yum -y install wget git which && \
# Get Go & install
wget https://dl.google.com/go/go1.12.7.linux-amd64.tar.gz && \
tar -xzf go1.12.7.linux-amd64.tar.gz && \
mv go /usr/local && \
mkdir -p $GOPATH

# Some ssh setup stuff
RUN mkdir -p -m 0600 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts && \
git config --global url."git@github.com:".insteadOf "https://github.com/"

# Below are useful if your authentication is not working out
# RUN --mount=type=ssh git clone git@github.com:citrusleaf/aeromon.git aeromon
# Note below will cause build to fail as it exits with status 1 as github.com does not permit interactive login
# but it lets you see if your credentials are being used
# RUN --mount=type=ssh ssh -v git@github.com 
# RUN cat ~/.gitconfig

RUN --mount=type=ssh \ 
/usr/local/go/bin/go get github.com/citrusleaf/aeromon
RUN cd $GOPATH/src/github.com/citrusleaf/aeromon && \
go build

CMD cd $GOPATH/src/github.com/citrusleaf/aeromon  && ./aeromon -h $AEROSPIKE_HOST -p 3000 -b :9145 -tags $AEROSPIKE_HOST
