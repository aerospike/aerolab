#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AEROLAB="${SCRIPT_DIR}/../src/tests/cli/aerolab"

ARCH=""
DRY_RUN=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch)     ARCH="$2"; shift 2 ;;
        --aerolab)  AEROLAB="$2"; shift 2 ;;
        --dry-run|dry-run)  DRY_RUN=(--dry-run); shift ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$ARCH" || "$ARCH" == "arm64" ]]; then
    "$AEROLAB" config backend -t docker -a ""
    "${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch arm64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor enterprise
    "${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch arm64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor community
fi

if [[ -z "$ARCH" || "$ARCH" == "amd64" ]]; then
    "$AEROLAB" config backend -t docker -a "amd64"
    "${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch amd64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor enterprise
    "${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch amd64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor community
fi

"$AEROLAB" config backend -t docker -a ""
