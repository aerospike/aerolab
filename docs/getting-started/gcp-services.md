# GCP Services (APIs) Required by AeroLab

AeroLab uses a number of Google Cloud APIs. This page lists every service it
touches, what each one is used for, and whether AeroLab can enable it for you
automatically.

## How AeroLab enables services

Before enabling anything, AeroLab always checks which services are already
enabled in the target project (via the Service Usage API) and only acts on the
ones that are missing. When one or more required services are **not** enabled,
behaviour depends on how AeroLab is being run:

| Situation | Behaviour |
| --- | --- |
| `--gcp-auto-enable-services` set (see below) | AeroLab enables the missing services automatically, without prompting — including in non-interactive contexts (web UI, MCP, CI). |
| Interactive terminal, flag not set | AeroLab lists the missing services and asks for confirmation (`[y/N]`). Answering no aborts with an error. |
| Non-interactive, flag not set (default) | AeroLab does **not** enable anything. It errors and tells you which services to enable manually (or to re-run with `--gcp-auto-enable-services`). |

Enable automatic, prompt-free enablement by configuring the backend with the
flag:

```bash
aerolab config backend -t gcp -r us-central1 -o your-project-id --gcp-auto-enable-services
```

The setting is persisted with the rest of the backend configuration, so it
stays in effect for subsequent commands until you reconfigure without it.

### Required IAM permission to enable services

Enabling APIs requires `roles/serviceusage.serviceUsageAdmin` (or the
equivalent `serviceusage.services.enable` permission) on the calling principal.
If your principal cannot enable APIs, ask a project owner to enable the
services manually (see the `gcloud` commands below), and run AeroLab without
`--gcp-auto-enable-services`.

## Service reference

### Core services

These underpin normal AeroLab operation.

| Service | Auto-enabled by AeroLab? | Used for |
| --- | --- | --- |
| `compute.googleapis.com` | As part of `expiry-install`; otherwise assumed already enabled | Compute Engine — creating and managing VM instances, disks, firewall rules, networks, and querying Cloud Routers / Cloud NAT. The foundation of every cluster command. |
| `serviceusage.googleapis.com` | No (bootstrap dependency) | Lets AeroLab list which APIs are enabled and enable the missing ones, and provision per-service identities. Must already be enabled for the check/auto-enable logic itself to work. |
| `cloudresourcemanager.googleapis.com` | No | Resolving the project number and managing project-level IAM (used by `expiry-install` when granting the Cloud Functions service agent access). |

### Feature-specific services

Enabled only when the relevant feature is used.

| Service | Auto-enabled by AeroLab? | Used for |
| --- | --- | --- |
| `iap.googleapis.com` | Yes, when `--gcp-use-iap` is set | Routing SSH/SFTP through [Identity-Aware Proxy TCP forwarding](https://cloud.google.com/iap/docs/using-tcp-forwarding) instead of dialing instance IPs. Enabled at `config backend` time when the flag is set. |
| `cloudbilling.googleapis.com` | Yes, on first pricing lookup | Retrieving instance-type and volume pricing shown in the inventory / `instance-types` listings. Enabled lazily the first time pricing is fetched if it isn't already on. |

### Resource expiry automation

These are enabled by `aerolab config gcp expiry-install`, which deploys a
Cloud Function that automatically cleans up expired resources.

| Service | Auto-enabled by AeroLab? | Used for |
| --- | --- | --- |
| `cloudfunctions.googleapis.com` | Yes | The Cloud Function that performs expiry cleanup. |
| `run.googleapis.com` | Yes | Cloud Run — the runtime that 2nd-generation Cloud Functions execute on. |
| `cloudbuild.googleapis.com` | Yes | Cloud Build — builds the expiry function deployment. |
| `artifactregistry.googleapis.com` | Yes | Artifact Registry — stores the built function container image. |
| `pubsub.googleapis.com` | Yes | Pub/Sub — event/trigger messaging for the function. |
| `cloudscheduler.googleapis.com` | Yes | Cloud Scheduler — the cron schedule that triggers expiry runs. |
| `storage.googleapis.com` | Yes | Cloud Storage — bucket holding the function source and state. |
| `logging.googleapis.com` | Yes | Cloud Logging — logs emitted by the expiry function. |

## Enabling everything manually

If you prefer to enable services ahead of time (for example, when the AeroLab
principal lacks `serviceUsageAdmin`), a project owner can enable them with
`gcloud`.

Core + feature services:

```bash
gcloud services enable \
  compute.googleapis.com \
  serviceusage.googleapis.com \
  cloudresourcemanager.googleapis.com \
  iap.googleapis.com \
  cloudbilling.googleapis.com \
  --project=your-project-id
```

Additionally, for resource expiry automation (`expiry-install`):

```bash
gcloud services enable \
  cloudfunctions.googleapis.com \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  pubsub.googleapis.com \
  cloudscheduler.googleapis.com \
  storage.googleapis.com \
  logging.googleapis.com \
  --project=your-project-id
```

Omit `iap.googleapis.com` if you are not using `--gcp-use-iap`, and omit the
expiry block if you are not using resource expiry automation.
