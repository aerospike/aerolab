FROM centos:7
ARG citrusleaf_pass=password_not_set_yet
ENV user citrusleaf
ENV pass $citrusleaf_pass

RUN yum updateinfo && \
yum -y install wget which initscripts && \
yum -y install java-1.8.0-openjdk && \
yum -y install net-tools telnet && \
yum clean all && \
wget --user $user --password $pass https://www.aerospike.com/artifacts/enterprise/aerospike-kafka/1.0.1/aerospike-kafka-1.0.1-1.x86_64.rpm && \
rpm -i aerospike-kafka-1.0.1-1.x86_64.rpm && \
rm aerospike-kafka-1.0.1-1.x86_64.rpm 

COPY features.conf /etc/aerospike-kafka/
COPY application.yml /etc/aerospike-kafka/
CMD service aerospike-kafka supervise
