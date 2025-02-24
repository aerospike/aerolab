FROM centos:8
COPY centos-gpg-keys-8-3.el8.noarch.rpm /root/centos-gpg-keys-8-3.el8.noarch.rpm
RUN rpm -i /root/centos-gpg-keys-8-3.el8.noarch.rpm && dnf --disablerepo '*' --enablerepo=extras swap centos-linux-repos centos-stream-repos -y && dnf distro-sync -y && yum -y remove snappy trousers 
RUN yum -y install wget && echo "Downloading Go:" && wget https://go.dev/dl/go1.24.0.darwin-amd64.pkg >/dev/null 2>&1 && tar -C /usr/local -xf go1.24.0.darwin-amd64.pkg && echo "export GOROOT=/usr/local/go" >> /root/.bashrc; echo "export PATH=$PATH:/usr/local/go/bin" >> /root/.bashrc && echo "export GOPATH=/root/go" >>/root/.bashrc
ENTRYPOINT ["tail", "-f", "/dev/null"]
