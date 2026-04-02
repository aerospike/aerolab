#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AEROLAB="${SCRIPT_DIR}/../src/tests/cli/aerolab"

DRY_RUN=()
if [[ "${1:-}" == "dry-run" ]]; then
    DRY_RUN=(--dry-run)
fi

"$AEROLAB" config backend -t docker -a ""
"${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch arm64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor enterprise
"${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch arm64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor community

"$AEROLAB" config backend -t docker -a "amd64"
"${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch amd64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor enterprise
"${SCRIPT_DIR}/publish-missing-templates.sh" --auto --aerolab "$AEROLAB" --arch amd64 ${DRY_RUN[@]+"${DRY_RUN[@]}"} --flavor community

"$AEROLAB" config backend -t docker -a ""
