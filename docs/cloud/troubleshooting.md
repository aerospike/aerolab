# Cloud Clusters Troubleshooting

Troubleshooting tips and advanced usage for Aerospike Cloud clusters.

## Accessing Cloud Cluster Logs with AGI

Aerospike Cloud clusters store their logs in S3 buckets. You can use AGI (Aerospike Grafana Integration) to analyze these logs.

### Prerequisites

1. Enable log access for your cluster (done automatically during cluster creation, or manually):
   ```bash
   aerolab cloud clusters enable-logs-access -n myClusterName
   ```

2. Ensure your AWS credentials have access to the log bucket (the account root ARN is authorized by default)

### Creating an AGI from Cloud Cluster Logs

```bash
# Get logging bucket and extract region from cluster name
CLOUD_BUCKET=$(aerolab cloud clusters list --output json | jq -r '.clusters[] | select(.name == "myClusterName") | .logging.logBucket | split(":") | .[-1]')
CLOUD_BUCKET_REGION=$(echo $CLOUD_BUCKET | awk -F'-' '{print $(NF-2)"-"$(NF-1)"-"$NF}')

# Get current AWS credentials
export AWS_ACCESS_KEY_ID=$(aws configure get aws_access_key_id)
export AWS_SECRET_ACCESS_KEY=$(aws configure get aws_secret_access_key)

# Create AGI, getting logs from the bucket for a specific date
aerolab agi create \
  --name myagi \
  --source-s3-enable \
  --source-s3-bucket "${CLOUD_BUCKET}" \
  --source-s3-region "${CLOUD_BUCKET_REGION}" \
  --source-s3-key-id "ENV::AWS_ACCESS_KEY_ID" \
  --source-s3-secret-key "ENV::AWS_SECRET_ACCESS_KEY" \
  --source-s3-path "logs/2026/01/20/"
```

### Notes

- The `--source-s3-path` option filters logs by date path (format: `logs/YYYY/MM/DD/`)
- Use `ENV::VAR_NAME` syntax to read credentials from environment variables
- The bucket region is embedded in the bucket name (last 3 dash-separated segments)

### Using a Specific AWS Profile

If you have multiple AWS profiles:

```bash
export AWS_ACCESS_KEY_ID=$(aws configure get aws_access_key_id --profile myprofile)
export AWS_SECRET_ACCESS_KEY=$(aws configure get aws_secret_access_key --profile myprofile)
```

### Time Range Filtering

To filter logs by time range:

```bash
aerolab agi create \
  --name myagi \
  --source-s3-enable \
  --source-s3-bucket "${CLOUD_BUCKET}" \
  --source-s3-region "${CLOUD_BUCKET_REGION}" \
  --source-s3-key-id "ENV::AWS_ACCESS_KEY_ID" \
  --source-s3-secret-key "ENV::AWS_SECRET_ACCESS_KEY" \
  --ingest-timeranges-enable \
  --ingest-timeranges-from "2026-01-19T00:00:00Z" \
  --ingest-timeranges-to "2026-01-20T23:59:59Z"
```

### Alternative: Download Logs First, Then Create AGI

You can also download logs manually using the AWS CLI and then use `--source-local` to create an AGI:

```bash
# Get logging bucket and extract region
CLOUD_BUCKET=$(aerolab cloud clusters list --output json | jq -r '.clusters[] | select(.name == "myClusterName") | .logging.logBucket | split(":") | .[-1]')
CLOUD_BUCKET_REGION=$(echo $CLOUD_BUCKET | awk -F'-' '{print $(NF-2)"-"$(NF-1)"-"$NF}')

# Create a local directory for logs
mkdir -p ./cloud-logs

# Download logs from S3 (for a specific date)
aws s3 sync "s3://${CLOUD_BUCKET}/logs/2026/01/20/" ./cloud-logs/ --region "${CLOUD_BUCKET_REGION}"

# Or download all logs
aws s3 sync "s3://${CLOUD_BUCKET}/logs/" ./cloud-logs/ --region "${CLOUD_BUCKET_REGION}"

# Create AGI from local logs
aerolab agi create \
  --name myagi \
  --source-local ./cloud-logs/
```

This approach is useful when:
- You want to inspect the logs before processing
- You need to process the same logs multiple times
- You're working offline or have limited S3 access
- You want to archive the logs locally

## Checking Log Access Configuration

To verify log access is enabled and see the bucket details:

```bash
# List clusters with logging details (JSON output includes full logging configuration)
aerolab cloud clusters list --output json | jq '.clusters[] | {name, logging}'
```

## Manually Enabling Log Access

If log access wasn't enabled during cluster creation:

```bash
# Enable using cluster name
aerolab cloud clusters enable-logs-access -n myClusterName

# Or using cluster ID
aerolab cloud clusters enable-logs-access -c <cluster-id>

# Authorize a specific IAM role
aerolab cloud clusters enable-logs-access -n myClusterName --role arn:aws:iam::123456789012:role/MyRole

# Append additional roles to existing authorized roles
aerolab cloud clusters enable-logs-access -n myClusterName --role arn:aws:iam::123456789012:role/AnotherRole --append
```
