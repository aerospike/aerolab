set -e
GOOS=linux GOARCH=amd64 go build .
docker build -t aerolab:testweb .
docker run -it --rm aerolab:testweb
