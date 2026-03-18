#!/bin/bash
# Bootstrap script for eksctl client configuration

set -e

# Retry helper function: tries command once, sleeps 1s, then retries once on failure
retry_cmd() {
    "$@" || { sleep 1; "$@"; }
}

# Parse arguments
AWS_REGION=""
AWS_KEY_ID=""
AWS_SECRET_KEY=""

while getopts "r:k:s:" opt; do
  case $opt in
    r) AWS_REGION="$OPTARG" ;;
    k) AWS_KEY_ID="$OPTARG" ;;
    s) AWS_SECRET_KEY="$OPTARG" ;;
    *) echo "Usage: $0 -r AWS_REGION [-k AWS_KEY_ID -s AWS_SECRET_KEY]" >&2; exit 1 ;;
  esac
done

if [ -z "$AWS_REGION" ]; then
  echo "Error: AWS region is required (-r)" >&2
  exit 1
fi

echo "Configuring AWS CLI and kubectl..."

# Install AWS CLI
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
  AWSCLI_ARCH="x86_64"
elif [ "$ARCH" = "aarch64" ]; then
  AWSCLI_ARCH="aarch64"
else
  echo "Unsupported architecture: $ARCH" >&2
  exit 1
fi

echo "Installing AWS CLI..."
retry_cmd curl -sL "https://awscli.amazonaws.com/awscli-exe-linux-${AWSCLI_ARCH}.zip" -o "/tmp/awscliv2.zip"
cd /tmp
unzip -q -o awscliv2.zip
if [ -f /usr/local/bin/aws ]; then
  ./aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli --update
else
  ./aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli
fi
rm -rf awscliv2.zip aws
cd -

# Install kubectl if not present
if ! command -v kubectl &> /dev/null; then
  echo "Installing kubectl..."
  if [ "$ARCH" = "x86_64" ]; then
    K8S_ARCH="amd64"
  elif [ "$ARCH" = "aarch64" ]; then
    K8S_ARCH="arm64"
  else
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
  fi
  
  retry_cmd curl -sLO "https://dl.k8s.io/release/$(retry_cmd curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${K8S_ARCH}/kubectl"
  chmod +x kubectl
  mv kubectl /usr/local/bin/
fi

# Install Helm 3 (optional)
echo "Installing helm (optional)..."
set +e
retry_cmd curl -fsSL -o /tmp/get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
if [ -f /tmp/get_helm.sh ]; then
  chmod 700 /tmp/get_helm.sh
  /tmp/get_helm.sh
  rm -f /tmp/get_helm.sh
fi
set -e

# Clone deploy-olm-ako helper scripts
echo "Getting deploy-olm-ako script..."
set +e
if [ ! -d /root/deploy-olm-ako ]; then
  cd /root && git clone -b eksctl https://github.com/colton-aerospike/deploy-olm-ako 2>/dev/null
else
  cd /root/deploy-olm-ako && git pull 2>/dev/null
fi
set -e

# Configure AWS credentials
mkdir -p ~/.aws

if [ -n "$AWS_KEY_ID" ] && [ -n "$AWS_SECRET_KEY" ]; then
  echo "Configuring AWS credentials with Key ID and Secret Key..."
  cat > ~/.aws/credentials <<EOF
[default]
aws_access_key_id = ${AWS_KEY_ID}
aws_secret_access_key = ${AWS_SECRET_KEY}
EOF
  chmod 600 ~/.aws/credentials
fi

# Configure AWS region
echo "Configuring AWS default region to ${AWS_REGION}..."
cat > ~/.aws/config <<EOF
[default]
region = ${AWS_REGION}
output = text
EOF
chmod 600 ~/.aws/config

# Enable tmux auto-attach on login
if ! grep -q 'exec tmux new-session -A -s eksctl' /root/.bashrc 2>/dev/null; then
  cat <<'TMUX_EOF' >> /root/.bashrc
if command -v tmux &> /dev/null && [ -n "$PS1" ] && [[ ! "$TERM" =~ screen ]] && [[ ! "$TERM" =~ tmux ]] && [ -z "$TMUX" ] && [ $# -eq 0 ]; then
  exec tmux new-session -A -s eksctl
fi
TMUX_EOF
fi

# Verify installation
echo ""
echo "Verifying installations..."
aws --version
kubectl version --client
eksctl version

echo ""
echo "Bootstrap complete!"
echo "AWS Region configured: ${AWS_REGION}"
if [ -n "$AWS_KEY_ID" ]; then
  echo "AWS credentials configured"
else
  echo "AWS credentials: Using instance profile"
fi

