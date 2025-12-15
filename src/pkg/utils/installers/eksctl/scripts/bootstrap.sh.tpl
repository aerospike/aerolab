#!/bin/bash
# Bootstrap script for eksctl client configuration

set -e

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

# Install AWS CLI if not present
if ! command -v aws &> /dev/null; then
  echo "Installing AWS CLI..."
  ARCH=$(uname -m)
  if [ "$ARCH" = "x86_64" ]; then
    curl -sL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "/tmp/awscliv2.zip"
  elif [ "$ARCH" = "aarch64" ]; then
    curl -sL "https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip" -o "/tmp/awscliv2.zip"
  else
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
  fi
  
  cd /tmp
  unzip -q awscliv2.zip
  ./aws/install
  rm -rf awscliv2.zip aws
  cd -
fi

# Install kubectl if not present
if ! command -v kubectl &> /dev/null; then
  echo "Installing kubectl..."
  ARCH=$(uname -m)
  if [ "$ARCH" = "x86_64" ]; then
    K8S_ARCH="amd64"
  elif [ "$ARCH" = "aarch64" ]; then
    K8S_ARCH="arm64"
  else
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
  fi
  
  curl -sLO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${K8S_ARCH}/kubectl"
  chmod +x kubectl
  mv kubectl /usr/local/bin/
fi

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
output = json
EOF
chmod 600 ~/.aws/config

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

