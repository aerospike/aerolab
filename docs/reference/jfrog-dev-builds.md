# Deploying JFrog Dev Builds

Aerolab can install **pre-release / development builds** of Aerospike server straight from a JFrog Artifactory instance, instead of the public `download.aerospike.com` release site.

This is useful when you need a specific CI build (identified by a build number and git SHA) that has not been published as a public release.

> **How it works:** Aerolab resolves the build via the JFrog AQL API, downloads the matching `.rpm` / `.deb` package **to your machine**, and uploads it to the instance over SFTP. Your JFrog credentials never touch the target VM.

---

## 1. Setup

Point Aerolab at your JFrog instance and provide credentials:

```bash
export AEROLAB_ARTIFACTS_URL=https://aerospike.jfrog.io
export AEROLAB_ARTIFACTS_AUTH="cmVm...."   # bearer token, API key, or user:pass
```

| Variable | Required | Description |
|----------|----------|-------------|
| `AEROLAB_ARTIFACTS_URL` | ✅ | JFrog base URL. A `.jfrog.io` host activates JFrog mode automatically. |
| `AEROLAB_ARTIFACTS_AUTH` | ✅ | Credentials. Accepts a bearer token, a `Bearer ...`/`Basic ...` header value, a JFrog API key, or `user:pass`. |
| `AEROLAB_ARTIFACTS_BUILD_NAME` | — | JFrog build name to query. Defaults to `aerospike-server`. |

See the [Environment Variables reference](environment-variables.md#aerolab_artifacts_url) for the full details.

---

## 2. List available build versions

```bash
./aerolab installer list-versions
```

Filter to a specific release train with `-v`:

```bash
./aerolab installer list-versions -v 8.1.3
```

The list shows clean, copy-pasteable build numbers, for example:

```
8.1.3.0
8.1.3.0-8-g1d7fcd130
8.1.3.0-9-gc884842d0
8.1.3.0-28-g302194ebc
```

---

## 3. Deploy a specific build

Pick a build number from the list and pass it to `cluster create` with `-v`:

```bash
./aerolab cluster create -n dev -v 8.1.3.0-28-g302194ebc -d ubuntu -i 24.04
```

> **Edition:** enterprise is used by default. Append `:c` (community), `:f` (federal), or `:e` (enterprise) to the version — e.g. `-v 8.1.3.0-28-g302194ebc:c`. A plain trailing `c`/`f` is **not** interpreted as an edition here, because JFrog build numbers can end in a git SHA.

Tear it down when you're done:

```bash
./aerolab cluster destroy -f -n dev
```

---

## 4. Deploy the latest build of a release train

You can also target a "floating" build number that always points at the newest build of a train (e.g. `8.1.3.0`):

```bash
./aerolab cluster create -n dev -v 8.1.3.0 -d ubuntu -i 24.04
```

> ⚠️ **Floating builds change over time.** `8.1.3.0` is rebuilt as new commits land. Because Aerolab caches the built image as a **template**, a later `cluster create` with the same `-v` will reuse the *old* cached image instead of the newer build. To pick up a fresh build, clear the template first (see below).

---

## 5. Clearing cached templates

Aerolab caches each built image as a template so subsequent clusters start faster. Template names use the **canonical build number** (always with the `-artifacts` suffix) plus the edition, e.g. `8.1.3.0-artifacts-enterprise`.

List your templates:

```bash
./aerolab template list --pager
```

Destroy the specific template(s) you want to rebuild:

```bash
# a pinned build
./aerolab template destroy -d ubuntu -i 24.04 -v 8.1.3.0-28-g302194ebc-artifacts-enterprise

# a floating build
./aerolab template destroy -d ubuntu -i 24.04 -v 8.1.3.0-artifacts-enterprise
```

After destroying the template, the next `cluster create` rebuilds it from the current JFrog artifact.

---

## Quick reference

```bash
# setup
export AEROLAB_ARTIFACTS_URL=https://aerospike.jfrog.io
export AEROLAB_ARTIFACTS_AUTH="cmVm...."

# list all build versions on JFrog
./aerolab installer list-versions

# deploy a specific build
./aerolab cluster create -n dev -v 8.1.3.0-28-g302194ebc -d ubuntu -i 24.04

# destroy the cluster
./aerolab cluster destroy -f -n dev

# deploy the latest 8.1.3.0 build instead
./aerolab cluster create -n dev -v 8.1.3.0 -d ubuntu -i 24.04

# floating builds can change between rebuilds — clear cached templates to force a fresh build
./aerolab template list --pager
./aerolab template destroy -d ubuntu -i 24.04 -v 8.1.3.0-28-g302194ebc-artifacts-enterprise
./aerolab template destroy -d ubuntu -i 24.04 -v 8.1.3.0-artifacts-enterprise
```

---

## See also

- [Environment Variables](environment-variables.md) — `AEROLAB_ARTIFACTS_URL` / `AEROLAB_ARTIFACTS_AUTH`
- [Installer Commands](../commands/installer.md) — `list-versions` and `download`
- [Templates](../commands/templates.md) — managing cached build images
- [Cluster Management](../commands/cluster.md) — creating and destroying clusters
