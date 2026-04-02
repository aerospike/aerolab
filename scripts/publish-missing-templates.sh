#!/bin/bash
#
# Compare `aerolab installer list-versions` against the registry metadata
# and the blacklist, then publish any versions that are missing.
#
# Usage:
#   ./publish-missing-templates.sh --dry-run
#   ./publish-missing-templates.sh --flavor enterprise --arch amd64
#   ./publish-missing-templates.sh --version-prefix 8.
#   ./publish-missing-templates.sh --auto --dry-run   # use aerolab defaults for os/arch
#   ./publish-missing-templates.sh --auto --arch arm64 --dry-run  # auto with arch override
#
# Prerequisites:
#   - aerolab
#   - gcloud CLI (authenticated)
#   - jq
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PUBLISH_SCRIPT="${SCRIPT_DIR}/publish-template-to-registry.sh"

# --- defaults ----------------------------------------------------------------
FLAVOR="enterprise"
OS_NAME="ubuntu"
OS_VERSION="22.04"
ARCH="amd64"
GCS_BUCKET="gs://aerospike-docker-images-na"
BLACKLIST_FILE="${SCRIPT_DIR}/registry-blacklist.txt"
VERSION_PREFIX=""
DRY_RUN=false
AUTO=false
AEROLAB="aerolab"
ARCH_SET=false
OS_SET=false
OS_VERSION_SET=false

# --- parse args --------------------------------------------------------------
while [[ $# -gt 0 ]]; do
    case "$1" in
        --flavor)           FLAVOR="$2"; shift 2 ;;
        --os)               OS_NAME="$2"; OS_SET=true; shift 2 ;;
        --os-version)       OS_VERSION="$2"; OS_VERSION_SET=true; shift 2 ;;
        --arch)             ARCH="$2"; ARCH_SET=true; shift 2 ;;
        --gcs-bucket)       GCS_BUCKET="$2"; shift 2 ;;
        --blacklist)        BLACKLIST_FILE="$2"; shift 2 ;;
        --version-prefix)   VERSION_PREFIX="$2"; shift 2 ;;
        --dry-run)          DRY_RUN=true; shift ;;
        --auto)             AUTO=true; shift ;;
        --aerolab)          AEROLAB="$2"; shift 2 ;;
        -h|--help)
            sed -n '2,/^$/{ s/^# \?//; p }' "$0"
            exit 0
            ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

GCS_BUCKET="${GCS_BUCKET%/}"

for cmd in "$AEROLAB" jq; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: '$cmd' is required but not found in PATH" >&2
        exit 1
    fi
done

if [[ ! -x "$PUBLISH_SCRIPT" ]]; then
    echo "ERROR: publish script not found or not executable: ${PUBLISH_SCRIPT}" >&2
    exit 1
fi

if [[ "$AUTO" == "true" ]]; then
    OVERRIDE_MSG=""
    [[ "$OS_SET" == "true" ]] && OVERRIDE_MSG+=" os=${OS_NAME}"
    [[ "$OS_VERSION_SET" == "true" ]] && OVERRIDE_MSG+=" os-version=${OS_VERSION}"
    [[ "$ARCH_SET" == "true" ]] && OVERRIDE_MSG+=" arch=${ARCH}"
    if [[ -n "$OVERRIDE_MSG" ]]; then
        echo ">>> Auto mode with overrides:${OVERRIDE_MSG}"
    else
        echo ">>> Auto mode: aerolab will pick default os/arch per version"
    fi
fi

# --- step 1: get all available versions from aerolab -------------------------
echo ">>> Fetching available versions from aerolab installer list-versions"

AEROLAB_CMD="${AEROLAB} installer list-versions"
case "$FLAVOR" in
    community) AEROLAB_CMD+=" --community" ;;
    federal)   AEROLAB_CMD+=" --federal" ;;
    enterprise) ;;
    *) echo "ERROR: unknown flavor '${FLAVOR}' (expected enterprise, community, or federal)" >&2; exit 1 ;;
esac

if [[ -n "$VERSION_PREFIX" ]]; then
    AEROLAB_CMD+=" --version ${VERSION_PREFIX}"
fi

ALL_VERSIONS=$(eval "$AEROLAB_CMD" 2>/dev/null)
VERSION_COUNT=$(echo "$ALL_VERSIONS" | wc -l | tr -d ' ')
echo ">>> Found ${VERSION_COUNT} versions"

# --- step 2: prepare blacklist as a temp file for grep -----------------------
BLACKLIST_CLEAN=$(mktemp)
PUBLISHED_VERSIONS=$(mktemp)
REMOTE_METADATA=$(mktemp)
trap 'rm -f "$BLACKLIST_CLEAN" "$PUBLISHED_VERSIONS" "$REMOTE_METADATA"' EXIT

if [[ -f "$BLACKLIST_FILE" ]]; then
    sed 's/#.*//; s/[[:space:]]//g; /^$/d' "$BLACKLIST_FILE" > "$BLACKLIST_CLEAN"
    BLACKLIST_COUNT=$(wc -l < "$BLACKLIST_CLEAN" | tr -d ' ')
    echo ">>> Loaded ${BLACKLIST_COUNT} blacklisted versions"
else
    echo "WARNING: blacklist file not found: ${BLACKLIST_FILE} — proceeding without blacklist" >&2
    : > "$BLACKLIST_CLEAN"
fi

# --- step 3: download remote metadata ----------------------------------------
echo ">>> Downloading remote metadata from ${GCS_BUCKET}/metadata.json"

if gcloud storage cp "${GCS_BUCKET}/metadata.json" "$REMOTE_METADATA" 2>/dev/null; then
    METADATA_COUNT=$(jq length "$REMOTE_METADATA")
    echo ">>> Remote metadata has ${METADATA_COUNT} entries"
else
    echo ">>> No remote metadata found — treating all versions as missing"
    echo "[]" > "$REMOTE_METADATA"
fi

# --- step 4: extract already-published versions into a file ------------------
if [[ "$AUTO" == "true" ]]; then
    # In auto mode, match on version+flavor; also match any explicitly overridden fields
    JQ_FILTER='.[] | select(.flavor == $fl'
    JQ_CMD=(jq -r --arg fl "$FLAVOR")
    [[ "$OS_SET" == "true" ]] && { JQ_FILTER+=' and .osName == $os'; JQ_CMD+=(--arg os "$OS_NAME"); }
    [[ "$OS_VERSION_SET" == "true" ]] && { JQ_FILTER+=' and .osVersion == $ov'; JQ_CMD+=(--arg ov "$OS_VERSION"); }
    [[ "$ARCH_SET" == "true" ]] && { JQ_FILTER+=' and .architecture == $ar'; JQ_CMD+=(--arg ar "$ARCH"); }
    JQ_FILTER+=') | .aerospikeVersion'

    "${JQ_CMD[@]}" "$JQ_FILTER" "$REMOTE_METADATA" | sort -u > "$PUBLISHED_VERSIONS"
    PUBLISHED_COUNT=$(wc -l < "$PUBLISHED_VERSIONS" | tr -d ' ')
    FILTER_DESC="${FLAVOR}"
    [[ "$OS_SET" == "true" ]] && FILTER_DESC+="/${OS_NAME}"
    [[ "$OS_VERSION_SET" == "true" ]] && FILTER_DESC+="/${OS_VERSION}"
    [[ "$ARCH_SET" == "true" ]] && FILTER_DESC+="/${ARCH}"
    echo ">>> Found ${PUBLISHED_COUNT} already-published versions for ${FILTER_DESC} (auto mode)"
else
    jq -r \
        --arg fl "$FLAVOR" \
        --arg os "$OS_NAME" \
        --arg ov "$OS_VERSION" \
        --arg ar "$ARCH" \
        '.[] | select(.flavor == $fl and .osName == $os and .osVersion == $ov and .architecture == $ar) | .aerospikeVersion' \
        "$REMOTE_METADATA" > "$PUBLISHED_VERSIONS"
    PUBLISHED_COUNT=$(wc -l < "$PUBLISHED_VERSIONS" | tr -d ' ')
    echo ">>> Found ${PUBLISHED_COUNT} already-published versions for ${FLAVOR}/${OS_NAME}/${OS_VERSION}/${ARCH}"
fi

# --- step 5: compute missing versions and publish ----------------------------
MISSING=0
SKIPPED_BLACKLIST=0
SKIPPED_PUBLISHED=0

while IFS= read -r version; do
    [[ -z "$version" ]] && continue

    if grep -qxF "$version" "$BLACKLIST_CLEAN"; then
        SKIPPED_BLACKLIST=$((SKIPPED_BLACKLIST + 1))
        continue
    fi

    if grep -qxF "$version" "$PUBLISHED_VERSIONS"; then
        SKIPPED_PUBLISHED=$((SKIPPED_PUBLISHED + 1))
        continue
    fi

    MISSING=$((MISSING + 1))

    if [[ "$AUTO" == "true" ]]; then
        PUBLISH_ARGS=(--version "$version" --flavor "$FLAVOR" --detect-defaults --aerolab "$AEROLAB" --gcs-bucket "$GCS_BUCKET")
        [[ "$OS_SET" == "true" ]] && PUBLISH_ARGS+=(--os "$OS_NAME")
        [[ "$OS_VERSION_SET" == "true" ]] && PUBLISH_ARGS+=(--os-version "$OS_VERSION")
        [[ "$ARCH_SET" == "true" ]] && PUBLISH_ARGS+=(--arch "$ARCH")
        if [[ "$DRY_RUN" == "true" ]]; then
            echo "[dry-run] ${PUBLISH_SCRIPT} ${PUBLISH_ARGS[*]}"
        else
            echo ">>> Publishing ${version} (${FLAVOR}, auto defaults${OVERRIDE_MSG})"
            if ! "${PUBLISH_SCRIPT}" "${PUBLISH_ARGS[@]}"; then
                echo "ERROR: publish failed for version ${version}" >&2
            fi
        fi
    else
        if [[ "$DRY_RUN" == "true" ]]; then
            echo "[dry-run] ${PUBLISH_SCRIPT} --version ${version} --flavor ${FLAVOR} --os ${OS_NAME} --os-version ${OS_VERSION} --arch ${ARCH} --aerolab ${AEROLAB} --gcs-bucket ${GCS_BUCKET}"
        else
            echo ">>> Publishing ${version} (${FLAVOR}/${OS_NAME}/${OS_VERSION}/${ARCH})"
            if ! "${PUBLISH_SCRIPT}" \
                --version "$version" \
                --flavor "$FLAVOR" \
                --os "$OS_NAME" \
                --os-version "$OS_VERSION" \
                --arch "$ARCH" \
                --aerolab "$AEROLAB" \
                --gcs-bucket "$GCS_BUCKET"; then
                echo "ERROR: publish failed for version ${version}" >&2
            fi
        fi
    fi
done <<< "$ALL_VERSIONS"

echo ""
echo "=== Summary ==="
echo "Total versions:     ${VERSION_COUNT}"
echo "Blacklisted:        ${SKIPPED_BLACKLIST}"
echo "Already published:  ${SKIPPED_PUBLISHED}"
echo "Missing (to publish): ${MISSING}"
if [[ "$DRY_RUN" == "true" && "$MISSING" -gt 0 ]]; then
    echo "(dry-run mode — nothing was published)"
fi
