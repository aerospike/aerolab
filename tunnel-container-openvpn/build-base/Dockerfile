FROM ubuntu:18.04
RUN apt-get update
RUN apt-get -y install openvpn easy-rsa vim iptables
RUN make-cadir /root/openvpn-ca
WORKDIR "/root/openvpn-ca"
RUN cp openssl-1.0.0.cnf openssl.cnf
RUN /bin/bash -c "source vars && ./clean-all"
COPY ca-answers /root/ca-answers
