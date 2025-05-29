FROM %s

RUN mkdir -p /opt/aerolab/scripts

COPY userdata.sh /opt/aerolab/scripts/userdata.sh

RUN chmod +x /opt/aerolab/scripts/userdata.sh

RUN /bin/bash /opt/aerolab/scripts/userdata.sh

ENTRYPOINT ["/usr/local/bin/init-docker-systemd"]
