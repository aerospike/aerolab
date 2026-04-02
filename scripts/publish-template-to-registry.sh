#!/bin/bash
#
# Build an Aerospike template image, export it as a tarball, and publish
# it to a GCS-backed template registry.
#
# Usage:
#   ./publish-template-to-registry.sh \
#       --version 8.0.0.4           \
#       --flavor enterprise         \
#       --os ubuntu                 \
#       --os-version 22.04          \
#       --arch amd64                \
#       --gcs-bucket gs://aerospike-docker-images-na
#
#   # Let aerolab pick the default os/arch for each version:
#   ./publish-template-to-registry.sh --version 8.0.0.4 --detect-defaults
#
# Prerequisites:
#   - aerolab (configured with docker backend)
#   - docker CLI
#   - gcloud CLI (authenticated with appropriate credentials)
#   - jq
#
set -euo pipefail

# --- defaults ----------------------------------------------------------------
AEROSPIKE_VERSION=""
FLAVOR="enterprise"
OS_NAME="ubuntu"
OS_VERSION="22.04"
ARCH="amd64"
GCS_BUCKET="gs://aerospike-docker-images-na"
OUTPUT_DIR="./registry-output"
SKIP_BUILD=false
FORCE=false
DETECT_DEFAULTS=false
AEROLAB="aerolab"
ARCH_SET=false
OS_SET=false
OS_VERSION_SET=false

# --- parse args --------------------------------------------------------------
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)          AEROSPIKE_VERSION="$2"; shift 2 ;;
        --flavor)           FLAVOR="$2"; shift 2 ;;
        --os)               OS_NAME="$2"; OS_SET=true; shift 2 ;;
        --os-version)       OS_VERSION="$2"; OS_VERSION_SET=true; shift 2 ;;
        --arch)             ARCH="$2"; ARCH_SET=true; shift 2 ;;
        --gcs-bucket)       GCS_BUCKET="$2"; shift 2 ;;
        --output-dir)       OUTPUT_DIR="$2"; shift 2 ;;
        --skip-build)       SKIP_BUILD=true; shift ;;
        --force)            FORCE=true; shift ;;
        --detect-defaults)  DETECT_DEFAULTS=true; shift ;;
        --aerolab)          AEROLAB="$2"; shift 2 ;;
        -h|--help)
            sed -n '2,/^$/{ s/^# \?//; p }' "$0"
            exit 0
            ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$AEROSPIKE_VERSION" ]]; then
    echo "ERROR: --version is required" >&2
    exit 1
fi

# strip trailing slash from bucket path
GCS_BUCKET="${GCS_BUCKET%/}"

for cmd in "$AEROLAB" docker gcloud jq; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: '$cmd' is required but not found in PATH" >&2
        exit 1
    fi
done

# --- derived values (FILENAME/TARBALL deferred when detecting defaults) ------
AV="${AEROSPIKE_VERSION}-${FLAVOR}"
mkdir -p "$OUTPUT_DIR"

# --- check for existing entry in remote metadata ----------------------------
echo ">>> Checking if entry already exists in registry"
REMOTE_METADATA=$(mktemp)
trap 'rm -f "$REMOTE_METADATA"' EXIT
if gcloud storage cp "${GCS_BUCKET}/metadata.json" "$REMOTE_METADATA" 2>/dev/null; then
    if [[ "$DETECT_DEFAULTS" == "true" ]]; then
        # Match version+flavor, plus any explicitly overridden fields
        JQ_FILTER='[.[] | select(.aerospikeVersion == $av and .flavor == $fl'
        JQ_CMD=(jq --arg av "$AEROSPIKE_VERSION" --arg fl "$FLAVOR")
        [[ "$OS_SET" == "true" ]] && { JQ_FILTER+=' and .osName == $os'; JQ_CMD+=(--arg os "$OS_NAME"); }
        [[ "$OS_VERSION_SET" == "true" ]] && { JQ_FILTER+=' and .osVersion == $ov'; JQ_CMD+=(--arg ov "$OS_VERSION"); }
        [[ "$ARCH_SET" == "true" ]] && { JQ_FILTER+=' and .architecture == $ar'; JQ_CMD+=(--arg ar "$ARCH"); }
        JQ_FILTER+=')] | length'
        EXISTS=$("${JQ_CMD[@]}" "$JQ_FILTER" "$REMOTE_METADATA")
    else
        EXISTS=$(jq \
            --arg av "$AEROSPIKE_VERSION" \
            --arg fl "$FLAVOR" \
            --arg os "$OS_NAME" \
            --arg ov "$OS_VERSION" \
            --arg ar "$ARCH" \
            '[.[] | select(.aerospikeVersion == $av and .flavor == $fl and .osName == $os and .osVersion == $ov and .architecture == $ar)] | length' \
            "$REMOTE_METADATA")
    fi
    if [[ "$EXISTS" -gt 0 ]]; then
        if [[ "$FORCE" == "true" ]]; then
            echo ">>> Entry already exists in registry — overriding (--force)"
        else
            echo "ERROR: Entry already exists in registry for ${AEROSPIKE_VERSION} ${FLAVOR}" >&2
            echo "Use --force to override the existing entry" >&2
            exit 1
        fi
    else
        echo ">>> No existing entry found, proceeding"
    fi
else
    echo ">>> No remote metadata found, proceeding with fresh registry"
fi

# --- step 1: build template -------------------------------------------------
VERSION_FLAG="${AEROSPIKE_VERSION}"
case "$FLAVOR" in
    community) VERSION_FLAG="${AEROSPIKE_VERSION}c" ;;
    federal)   VERSION_FLAG="${AEROSPIKE_VERSION}f" ;;
esac

# Snapshot image IDs before build so we can diff afterwards
IMAGES_BEFORE=$(docker images --format '{{.ID}}' \
    --filter "label=aerolab.soft.version=${AV}" \
    --filter "label=aerolab.image.type=aerospike")

if [[ "$SKIP_BUILD" == "false" ]]; then
    if [[ "$DETECT_DEFAULTS" == "true" ]]; then
        BUILD_ARGS=(--aerospike-version "$VERSION_FLAG")
        [[ "$OS_SET" == "true" ]] && BUILD_ARGS+=(--distro "$OS_NAME")
        [[ "$OS_VERSION_SET" == "true" ]] && BUILD_ARGS+=(--distro-version "$OS_VERSION")
        [[ "$ARCH_SET" == "true" ]] && BUILD_ARGS+=(--arch "$ARCH")
        OVERRIDE_INFO=""
        [[ "$OS_SET" == "true" ]] && OVERRIDE_INFO+=" os=${OS_NAME}"
        [[ "$OS_VERSION_SET" == "true" ]] && OVERRIDE_INFO+=" os-version=${OS_VERSION}"
        [[ "$ARCH_SET" == "true" ]] && OVERRIDE_INFO+=" arch=${ARCH}"
        echo ">>> Building template with aerolab defaults: version=${AEROSPIKE_VERSION} flavor=${FLAVOR}${OVERRIDE_INFO}"
        ${AEROLAB} template create "${BUILD_ARGS[@]}"
    else
        echo ">>> Building template: version=${AEROSPIKE_VERSION} flavor=${FLAVOR} os=${OS_NAME}:${OS_VERSION} arch=${ARCH}"
        ${AEROLAB} template create \
            --aerospike-version "$VERSION_FLAG" \
            --distro "$OS_NAME" \
            --distro-version "$OS_VERSION" \
            --arch "$ARCH"
    fi
    echo ">>> Template build complete"
else
    echo ">>> Skipping template build (--skip-build)"
fi

# --- step 2: find the docker image ------------------------------------------
echo ">>> Finding template image for ${AV}"

# Snapshot after build and diff to identify the newly created image
IMAGES_AFTER=$(docker images --format '{{.ID}}' \
    --filter "label=aerolab.soft.version=${AV}" \
    --filter "label=aerolab.image.type=aerospike")

IMAGE_ID=""
if [[ -n "$IMAGES_AFTER" ]]; then
    for id in $IMAGES_AFTER; do
        if [[ -z "$IMAGES_BEFORE" ]] || ! echo "$IMAGES_BEFORE" | grep -qxF "$id"; then
            IMAGE_ID="$id"
            break
        fi
    done
fi

if [[ -z "$IMAGE_ID" ]]; then
    # No new image found (build skipped, or aerolab reused existing template).
    # Fall back to label-based search.
    if [[ "$DETECT_DEFAULTS" == "true" ]]; then
        IMAGE_ID=$(docker images --format '{{.ID}}' \
            --filter "label=aerolab.soft.version=${AV}" \
            --filter "label=aerolab.image.type=aerospike" \
            | head -1)
    else
        IMAGE_ID=$(docker images --format '{{.ID}}' \
            --filter "label=aerolab.soft.version=${AV}" \
            --filter "label=aerolab.image.type=aerospike" \
            --filter "label=aerolab.os.name=${OS_NAME}" \
            --filter "label=aerolab.os.version=${OS_VERSION}" \
            --filter "label=aerolab.architecture=${ARCH}" \
            | head -1)

        if [[ -z "$IMAGE_ID" ]]; then
            echo ">>> Docker label filter returned nothing, trying broader search..."
            IMAGE_ID=$(docker images --format '{{.ID}}' \
                --filter "label=aerolab.soft.version=${AV}" \
                | head -1)
        fi
    fi
fi

if [[ -z "$IMAGE_ID" ]]; then
    echo "ERROR: Could not find Docker image with label aerolab.soft.version=${AV}" >&2
    echo "Available template images:" >&2
    docker images --filter "label=aerolab.image.type=aerospike" --format "table {{.Repository}}\t{{.Tag}}\t{{.ID}}" >&2
    exit 1
fi

echo ">>> Found image: ${IMAGE_ID}"

# --- step 2b: detect os/os-version/arch from image labels -------------------
if [[ "$DETECT_DEFAULTS" == "true" ]]; then
    [[ "$OS_SET" != "true" ]] && OS_NAME=$(docker inspect --format '{{index .Config.Labels "aerolab.os.name"}}' "$IMAGE_ID")
    [[ "$OS_VERSION_SET" != "true" ]] && OS_VERSION=$(docker inspect --format '{{index .Config.Labels "aerolab.os.version"}}' "$IMAGE_ID")
    [[ "$ARCH_SET" != "true" ]] && ARCH=$(docker inspect --format '{{index .Config.Labels "aerolab.architecture"}}' "$IMAGE_ID")

    if [[ -z "$OS_NAME" || -z "$OS_VERSION" || -z "$ARCH" ]]; then
        echo "ERROR: Could not detect os/os-version/arch from image labels" >&2
        echo "Labels on ${IMAGE_ID}:" >&2
        docker inspect --format '{{json .Config.Labels}}' "$IMAGE_ID" | jq . >&2
        exit 1
    fi

    echo ">>> Resolved: os=${OS_NAME} os-version=${OS_VERSION} arch=${ARCH}"
fi

# --- compute filename and tarball path (after os/arch are known) -------------
FILENAME="${AEROSPIKE_VERSION}-${FLAVOR}-${OS_NAME}-${OS_VERSION}-${ARCH}.tar.gz"
TARBALL="${OUTPUT_DIR}/${FILENAME}"

# --- step 3: docker save + gzip ---------------------------------------------
echo ">>> Exporting image to ${TARBALL}"
docker save "$IMAGE_ID" | gzip > "$TARBALL"

FILE_SIZE=$(stat -f%z "$TARBALL" 2>/dev/null || stat -c%s "$TARBALL" 2>/dev/null)
echo ">>> Tarball size: $(( FILE_SIZE / 1024 / 1024 )) MB"

# --- step 4: compute SHA256 -------------------------------------------------
echo ">>> Computing SHA256"
SHA256=$(shasum -a 256 "$TARBALL" | cut -d' ' -f1)
echo ">>> SHA256: ${SHA256}"

# --- step 5: download existing metadata.json from GCS -----------------------
METADATA_FILE="${OUTPUT_DIR}/metadata.json"
echo ">>> Downloading existing metadata from ${GCS_BUCKET}/metadata.json"
if gcloud storage cp "${GCS_BUCKET}/metadata.json" "$METADATA_FILE" 2>/dev/null; then
    echo ">>> Existing metadata downloaded"
else
    echo ">>> No existing metadata found, starting fresh"
    echo "[]" > "$METADATA_FILE"
fi

# --- step 6: update metadata.json -------------------------------------------
echo ">>> Updating metadata.json"

NEW_ENTRY=$(jq -n \
    --arg av "$AEROSPIKE_VERSION" \
    --arg fl "$FLAVOR" \
    --arg os "$OS_NAME" \
    --arg ov "$OS_VERSION" \
    --arg ar "$ARCH" \
    --arg fn "$FILENAME" \
    --arg sha "$SHA256" \
    '{
        aerospikeVersion: $av,
        flavor: $fl,
        osName: $os,
        osVersion: $ov,
        architecture: $ar,
        fileName: $fn,
        sha256: $sha
    }')

# Remove any existing entry with the same version+flavor+os+osVersion+arch, then append new
UPDATED=$(jq \
    --arg av "$AEROSPIKE_VERSION" \
    --arg fl "$FLAVOR" \
    --arg os "$OS_NAME" \
    --arg ov "$OS_VERSION" \
    --arg ar "$ARCH" \
    '[.[] | select(.aerospikeVersion != $av or .flavor != $fl or .osName != $os or .osVersion != $ov or .architecture != $ar)]' \
    "$METADATA_FILE")

echo "$UPDATED" | jq --argjson entry "$NEW_ENTRY" '. + [$entry]' > "$METADATA_FILE"

ENTRY_COUNT=$(jq length "$METADATA_FILE")
echo ">>> Metadata now has ${ENTRY_COUNT} entries"

# --- step 7: upload tarball and metadata to GCS ------------------------------
echo ">>> Uploading ${FILENAME} to ${GCS_BUCKET}/"
gcloud storage cp "$TARBALL" "${GCS_BUCKET}/${FILENAME}"

echo ">>> Uploading metadata.json to ${GCS_BUCKET}/"
gcloud storage cp "$METADATA_FILE" "${GCS_BUCKET}/metadata.json"

# Derive the public HTTPS URL for display
GCS_PATH="${GCS_BUCKET#gs://}"
PUBLIC_URL="https://storage.googleapis.com/${GCS_PATH}"

echo ""
echo "=== Done ==="
echo "GCS Bucket:   ${GCS_BUCKET}"
echo "Public URL:   ${PUBLIC_URL}"
echo "Template:     ${FILENAME}"
echo "SHA256:       ${SHA256}"
echo "Entries:      ${ENTRY_COUNT}"
