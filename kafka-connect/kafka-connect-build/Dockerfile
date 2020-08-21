FROM centos:7
ENV PRODUCT_NAME=aerospike-kafka-outbound
ENV VERSION=3.0.0
ENV RPM=$PRODUCT_NAME-$VERSION.rpm

RUN yum updateinfo && \
yum -y install wget which initscripts && \
yum -y install java-1.8.0-openjdk && \
yum -y install net-tools telnet && \
yum clean all && \
cd /tmp && \
wget https://www.aerospike.com/enterprise/download/connectors/aerospike-kafka-outbound/${VERSION}/artifact/rpm -O /tmp/$RPM && \
rpm2cpio $RPM 2>/dev/null | cpio -idmv 1>/dev/null 2>&1 && \
mv /tmp/opt/aerospike-kafka-outbound /opt && \
mkdir /etc/aerospike-kafka-outbound && \
mkdir /var/log/aerospike-kafka-outbound

COPY application.yml /etc/aerospike-kafka-outbound
CMD /opt/aerospike-kafka-outbound/bin/aerospike-kafka-outbound -f /etc/aerospike-kafka-outbound/application.yml
