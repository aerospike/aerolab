# AGI Troubleshooting Guide

This guide covers common issues and solutions when working with AGI.

## Table of Contents

- [Instance Creation Issues](#instance-creation-issues)
- [Log Ingestion Problems](#log-ingestion-problems)
- [Access and Authentication Issues](#access-and-authentication-issues)
- [Service Status Problems](#service-status-problems)
- [Performance Issues](#performance-issues)
- [Volume and Storage Issues](#volume-and-storage-issues)
- [Backend-Specific Issues](#backend-specific-issues)
- [Diagnostic Commands](#diagnostic-commands)

---

## Instance Creation Issues

### Template Creation Takes Too Long

**Symptom:** `agi template create` or first `agi create` takes more than 20 minutes.

**Possible Causes:**
1. Slow network connection for downloading packages
2. Cloud provider quota limits
3. Instance type unavailable

**Solutions:**
```bash
# Check template creation progress
aerolab agi template list

# Try with longer timeout
aerolab agi create --timeout 40 --source-local /path/to/logs

# For AWS, try different availability zone
aerolab agi create --aws-subnet-id us-east-1b --source-local /path/to/logs
```

### "No Suitable Instance Type Found"

**Symptom:** Error about instance type selection.

**Solutions:**
```bash
# Specify instance type explicitly
aerolab agi create --aws-instance-type t3a.xlarge --source-local /path/to/logs

# For GCP
aerolab agi create --gcp-instance e2-highmem-4 --source-local /path/to/logs
```

### "Not Enough Memory"

**Symptom:** AGI requires minimum 8GB RAM.

**Solutions:**
```bash
# For Docker, increase memory limit
aerolab agi create --docker-ram-limit 10g --source-local /path/to/logs

# Use --no-dim for lower memory requirement
aerolab agi create --no-dim --source-local /path/to/logs
```

### Template Creation Fails

**Symptom:** Template creation fails partway through.

**Solutions:**
```bash
# Check for lingering resources
aerolab instances list
aerolab agi template vacuum

# Retry with no-vacuum for debugging
aerolab agi template create --no-vacuum

# Then attach to inspect
aerolab attach shell -n <instance-name>

# Clean up after debugging
aerolab instances destroy -n <instance-name> --force
```

---

## Log Ingestion Problems

### Ingest Stuck at "Init"

**Symptom:** `agi details` shows ingest stuck at initialization.

**Possible Causes:**
1. Aerospike not running
2. Configuration error

**Solutions:**
```bash
# Check service status
aerolab agi status -n myagi

# Attach and check logs
aerolab agi attach -n myagi -- tail -100 /var/log/agi-ingest.log

# Restart ingest service
aerolab agi attach -n myagi -- systemctl restart agi-ingest
```

### "SFTP Connection Failed"

**Symptom:** SFTP source fails to connect.

**Solutions:**
```bash
# Verify SFTP accessibility manually
ssh user@sftp-host

# Check credentials
echo $SFTP_PASSWORD  # if using ENV::

# Skip check if firewall issues
aerolab agi create --source-sftp-skipcheck \
  --source-sftp-enable \
  --source-sftp-host ... \
  ...
```

### "S3 Access Denied"

**Symptom:** S3 source returns access denied errors.

**Solutions:**
```bash
# Verify credentials
aws s3 ls s3://your-bucket --region us-east-1

# Check environment variables
echo $AWS_ACCESS_KEY_ID
echo $AWS_SECRET_ACCESS_KEY

# Try with explicit credentials
export S3_KEY="your-key"
export S3_SECRET="your-secret"
aerolab agi create \
  --source-s3-enable \
  --source-s3-bucket your-bucket \
  --source-s3-region us-east-1 \
  --source-s3-key-id ENV::S3_KEY \
  --source-s3-secret-key ENV::S3_SECRET
```

### No Logs Found / Empty Ingest

**Symptom:** Ingest completes but no data appears in Grafana.

**Possible Causes:**
1. Wrong source path
2. File format not recognized
3. Time range filter excluding all logs

**Solutions:**
```bash
# Check files uploaded
aerolab agi attach -n myagi -- ls -la /opt/agi/files/input/

# Check for recognized log formats
aerolab agi attach -n myagi -- cat /var/log/agi-ingest.log | grep -i "file"

# Disable time range filter
aerolab agi run-ingest -n myagi --source-local /path/to/logs
```

### Ingest Taking Too Long

**Symptom:** Large log sets take hours to process.

**Solutions:**
```bash
# Use larger instance type
aerolab agi create --aws-instance-type m5.xlarge ...

# Use time range filtering
aerolab agi create \
  --ingest-timeranges-enable \
  --ingest-timeranges-from "2024-01-01T00:00:00Z" \
  --ingest-timeranges-to "2024-01-02T00:00:00Z" \
  ...
```

---

## Access and Authentication Issues

### Cannot Access Web Interface

**Symptom:** Browser cannot connect to AGI URL.

**Possible Causes:**
1. Instance not running
2. Security group not configured
3. No public IP

**Solutions:**
```bash
# Check instance state
aerolab agi list

# Verify instance is running
aerolab agi start -n myagi

# For AWS, check security groups
aerolab config aws list-security-groups

# Add public IP
aerolab instances add public-ip -n myagi
```

### SSL Certificate Warning

**Symptom:** Browser shows certificate warning.

This is normal for self-signed certificates. For production:

```bash
# Use custom certificates
aerolab agi create \
  --proxy-ssl-cert /path/to/fullchain.pem \
  --proxy-ssl-key /path/to/privkey.pem \
  --source-local /path/to/logs
```

### Token Not Working

**Symptom:** Access URL with token returns 401.

**Solutions:**
```bash
# List tokens
aerolab agi add-auth-token -n myagi --list

# Generate new token
aerolab agi add-auth-token -n myagi --url

# Check token minimum length (64 chars)
```

### "Unauthorized" with Correct Token

**Symptom:** Token authentication fails despite correct token.

**Solutions:**
```bash
# Attach and check token file
aerolab agi attach -n myagi -- ls -la /opt/agi/tokens/

# Reload tokens
aerolab agi attach -n myagi -- kill -HUP $(cat /opt/agi/proxy.pid)

# Check proxy logs
aerolab agi attach -n myagi -- tail -100 /var/log/agi-proxy.log
```

---

## Service Status Problems

### Aerospike Not Starting

**Symptom:** Aerospike service is inactive.

**Solutions:**
```bash
# Check aerospike logs
aerolab agi attach -n myagi -- journalctl -u aerospike -n 100

# Check configuration
aerolab agi attach -n myagi -- cat /etc/aerospike/aerospike.conf

# Try manual start
aerolab agi attach -n myagi -- systemctl start aerospike

# Check disk space
aerolab agi attach -n myagi -- df -h
```

### Grafana Not Accessible

**Symptom:** Grafana returns errors or doesn't load.

**Solutions:**
```bash
# Check grafana service
aerolab agi attach -n myagi -- systemctl status grafana-server

# Check grafana logs
aerolab agi attach -n myagi -- tail -100 /var/log/grafana/grafana.log

# Restart grafana
aerolab agi attach -n myagi -- systemctl restart grafana-server
```

### Plugin Not Responding

**Symptom:** Grafana dashboards show "No data" or plugin errors.

**Solutions:**
```bash
# Check plugin service
aerolab agi attach -n myagi -- systemctl status agi-plugin

# Check plugin logs
aerolab agi attach -n myagi -- tail -100 /var/log/agi-plugin.log

# Verify aerospike connection
aerolab agi attach -n myagi -- asinfo -v status
```

---

## Performance Issues

### Slow Query Response

**Symptom:** Grafana dashboards load slowly.

**Solutions:**
```bash
# Check system resources
aerolab agi status -n myagi

# Consider upgrading instance
aerolab agi stop -n myagi
# Note: you'll need to recreate with larger instance

# Use --no-dim for very large datasets
aerolab agi create --no-dim --source-local /path/to/logs
```

### High Memory Usage

**Symptom:** Instance running out of memory.

**Solutions:**
```bash
# Check memory usage
aerolab agi attach -n myagi -- free -h

# Use no-dim mode
aerolab agi create --no-dim ...

# Reduce data with time filtering
aerolab agi run-ingest -n myagi \
  --ingest-timeranges-enable \
  --ingest-timeranges-from "2024-01-01T00:00:00Z" \
  ...
```

### Disk Full

**Symptom:** Ingest fails with disk space errors.

**Solutions:**
```bash
# Check disk usage
aerolab agi attach -n myagi -- df -h

# For AWS, increase EBS
# (Requires recreating instance with larger disk)

# For GCP with monitor, disk will auto-expand
```

---

## Volume and Storage Issues

### EFS Mount Failed

**Symptom:** EFS volume not mounting correctly.

**Solutions:**
```bash
# Check EFS status
aerolab volumes list

# Verify security groups allow NFS (port 2049)
aerolab config aws list-security-groups

# Attach and check mount
aerolab agi attach -n myagi -- mount | grep efs
```

### Volume Data Not Persisting

**Symptom:** Data lost after restart.

**Solutions:**
```bash
# Ensure using persistent volume
aerolab agi create --aws-with-efs ...
# or
aerolab agi create --gcp-with-vol ...

# Check volume tags
aerolab volumes list
```

### Cannot Delete Volume

**Symptom:** `agi delete` fails to delete volume.

**Solutions:**
```bash
# Force delete
aerolab agi delete -n myagi --force

# Manually delete volume
aerolab volumes delete -n myagi --force
```

---

## Backend-Specific Issues

### AWS

#### Security Group Not Found
```bash
# List security groups
aerolab config aws list-security-groups

# Create if needed
aerolab config aws create-security-groups -n aerolab-agi -p 80,443
```

#### Subnet/VPC Issues
```bash
# List subnets
aerolab config aws list-subnets

# Specify subnet explicitly
aerolab agi create --aws-subnet-id subnet-12345 ...
```

### GCP

#### Permission Denied
```bash
# Re-authenticate
gcloud auth application-default login

# Check project
gcloud config get-value project
```

#### Firewall Issues
```bash
# List firewalls
gcloud compute firewall-rules list

# Create AGI firewall
gcloud compute firewall-rules create aerolab-agi \
  --allow tcp:80,tcp:443 \
  --source-ranges 0.0.0.0/0
```

### Docker

#### Container Won't Start
```bash
# Check Docker resources
docker system info

# Increase Docker memory (Docker Desktop settings)

# Check container logs
docker logs <container-id>
```

#### Port Already in Use

AGI automatically allocates ports starting at 9443 (HTTPS) or 9080 (HTTP), incrementing if in use. If you need a specific port:

```bash
# Check port usage
lsof -i :9443

# Use specific port
aerolab agi create --docker-expose-ports 8443:443 ...
```

**Note:** Multiple AGI instances can run simultaneously - each gets the next available port automatically.

#### Volume Settings Not Persisting (Docker Limitation)

**Known Limitation:** Docker bind mounts do not support the same metadata/tagging capabilities as AWS EFS or GCP persistent volumes.

**Symptom:** When you stop and restart a Docker AGI instance, or create a new instance pointing to an existing Docker volume, settings like label, instance type, and SSL configuration are not automatically restored.

**Workaround:** For Docker AGI instances, you must specify all settings explicitly on each `aerolab agi create` command. The data in the volume will persist, but the instance configuration must be provided again.

```bash
# AWS/GCP: Settings are restored from volume tags automatically
aerolab agi create --name myagi --aws-with-efs

# Docker: Must specify all settings each time
aerolab agi create --name myagi \
  --bind-files-dir /path/to/agi-output \
  --source-local bind:/path/to/logs \
  --agi-label "My AGI Instance" \
  ...
```

**Note:** This only affects Docker backend. AWS and GCP backends store settings in volume tags and restore them automatically.

#### Recommended Docker Bind Mount Setup

For the best Docker AGI experience with persistent data:

```bash
# Create directories on host
mkdir -p ~/agi-data/output ~/agi-data/logs

# Create AGI with bind mounts
aerolab agi create -n myagi \
  --bind-files-dir ~/agi-data/output \
  --source-local bind:~/agi-data/logs
```

This setup:
- Persists AGI output (processed logs, collect info) in `~/agi-data/output`
- Reads input logs from `~/agi-data/logs` (read-only)
- Allows you to add new logs and re-run ingest without recreating the instance

---

## Diagnostic Commands

### Quick Health Check

```bash
# Instance status
aerolab agi list

# Service status
aerolab agi status -n myagi

# Ingest progress
aerolab agi details -n myagi
```

### Detailed Diagnostics

```bash
# All service logs
aerolab agi attach -n myagi -- journalctl -n 200

# Specific service logs
aerolab agi attach -n myagi -- tail -200 /var/log/agi-ingest.log
aerolab agi attach -n myagi -- tail -200 /var/log/agi-proxy.log
aerolab agi attach -n myagi -- tail -200 /var/log/agi-plugin.log

# System resources
aerolab agi attach -n myagi -- free -h
aerolab agi attach -n myagi -- df -h
aerolab agi attach -n myagi -- top -bn1 | head -20

# Network
aerolab agi attach -n myagi -- ss -tlnp

# Aerospike status
aerolab agi attach -n myagi -- asinfo -v status
aerolab agi attach -n myagi -- asadm -e info
```

### File System Check

```bash
# Check AGI directories
aerolab agi attach -n myagi -- ls -la /opt/agi/

# Check uploaded files
aerolab agi attach -n myagi -- ls -la /opt/agi/files/input/

# Check processed files
aerolab agi attach -n myagi -- ls -la /opt/agi/files/logs/

# Check ingest progress
aerolab agi attach -n myagi -- cat /opt/agi/ingest/steps.json
```

### Configuration Check

```bash
# Ingest config
aerolab agi attach -n myagi -- cat /opt/agi/ingest.yaml

# Plugin config
aerolab agi attach -n myagi -- cat /opt/agi/plugin.yaml

# Aerospike config
aerolab agi attach -n myagi -- cat /etc/aerospike/aerospike.conf
```

---

## Getting Help

If issues persist:

1. **Check logs** - Service logs often contain detailed error messages
2. **Verify configuration** - Ensure all required options are set
3. **Check resources** - Memory and disk space are common bottlenecks
4. **Use `--dry-run`** - Validate commands before execution
5. **Use `--no-vacuum`** - Debug failed creations without cleanup

For further assistance:
- Use `aerolab agi <command> --help` for detailed option documentation
- Check the [Configuration Reference](configuration.md) for all available options

