FROM centos:7
ENV PRODUCT_NAME=aerospike-kafka-outbound
ENV VERSION=2.0.3-20200703.172404-1
ENV RPM=$PRODUCT_NAME-$VERSION.rpm

COPY $RPM /tmp/
RUN yum updateinfo && \
yum -y install wget which initscripts && \
yum -y install java-1.8.0-openjdk && \
yum -y install net-tools telnet && \
yum clean all && \
cd /tmp && \
rpm2cpio $RPM 2>/dev/null | cpio -idmv 1>/dev/null 2>&1 && \
mv opt/aerospike-kafka-outbound /opt && \
mkdir /etc/aerospike-kafka-outbound
RUN mkdir /var/log/aerospike-kafka-outbound

COPY application.yml /etc/aerospike-kafka-outbound
CMD /opt/aerospike-kafka-outbound/bin/aerospike-kafka-outbound -f /etc/aerospike-kafka-outbound/application.yml