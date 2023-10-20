FROM ubuntu:22.04

RUN apt update && apt -y install iproute2 && mkdir /app

WORKDIR /app

ADD test-web-server /app/

EXPOSE 8080

CMD ["/app/test-web-server"]
