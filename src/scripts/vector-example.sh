if [ ! -f /opt/prism-example-installed ]
then
  set -e
  apt -y install python3 python3-pip git
  cd /opt && git clone https://github.com/aerospike/proximus-examples.git
  cd /opt/proximus-examples/prism-image-search/prism
  python3 -m pip install -r requirements.txt
  mkdir -p /opt/proximus-examples/prism-image-search/prism/static/images/data/
  set +e
  python3 -m pip uninstall -y numpy
  set -e
  python3 -m pip install -Iv numpy==1.26.4
  touch /opt/prism-example-installed
fi
if [ "$1" != "install" ]
then
  set -e
  export AVS_PORT=%s
  cd /opt/proximus-examples/prism-image-search/prism
  waitress-serve --host 0.0.0.0 --port %s --threads 32 prism:app
fi
