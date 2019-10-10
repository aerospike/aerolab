FROM alpine
RUN apk update
RUN apk add openssh-server
COPY sshd_config /etc/ssh/sshd_config
RUN echo 'root:root' | chpasswd
RUN mkdir /root/.ssh
RUN ssh-keygen -f /etc/ssh/ssh_host_rsa_key -N '' -t rsa
RUN ssh-keygen -f /etc/ssh/ssh_host_dsa_key -N '' -t dsa
CMD /usr/sbin/sshd -D

