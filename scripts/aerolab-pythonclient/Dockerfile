FROM centos:8
COPY centos-gpg-keys-8-3.el8.noarch.rpm /root/centos-gpg-keys-8-3.el8.noarch.rpm
RUN rpm -i /root/centos-gpg-keys-8-3.el8.noarch.rpm && dnf --disablerepo '*' --enablerepo=extras swap centos-linux-repos centos-stream-repos -y && dnf distro-sync -y && yum -y remove snappy trousers
RUN yum -y install wget && yum -y install python3 python3-devel openssl-devel epel-release gcc make && pip3 install aerospike
ENTRYPOINT ["tail", "-f", "/dev/null"]
