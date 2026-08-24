#!/usr/bin/env bash
# =================================================================
# ActonOS Plugin SDK - Batch Build & Package Toolchain
# =================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [ ! -f "${ROOT_DIR}/go.mod" ]; then
    ROOT_DIR="${SCRIPT_DIR}"
fi

cd "${ROOT_DIR}"
exec go run "${ROOT_DIR}/cmd/acton-plugin" build-all "$@"
