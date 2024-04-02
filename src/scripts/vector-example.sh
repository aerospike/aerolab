if [ ! -f /opt/prism-example-installed ]
then
  set -e
  apt -y install python3 python3-pip git
  cd /opt && git clone https://github.com/aerospike/proximus-examples.git
  cd /opt/proximus-examples/prism-image-search/prism
  python3 -m pip install -r requirements.txt --extra-index-url https://aerospike.jfrog.io/artifactory/api/pypi/aerospike-pypi-dev/simple
  mkdir -p /opt/proximus-examples/prism-image-search/prism/static/images/data/
  touch /opt/prism-example-installed
fi
set -e
export PROXIMUS_PORT=%s
cd /opt/proximus-examples/prism-image-search/prism
waitress-serve --host 0.0.0.0 --port %s --threads 32 prism:app
