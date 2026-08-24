#!/usr/bin/env bash
# =================================================================
# ActonOS Plugin SDK - Version Synchronization Tool
# =================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

NEW_VERSION="${1:-}"

if [ -z "${NEW_VERSION}" ]; then
    echo "Usage: ./scripts/set_version.sh <version> [--sync-plugins]"
    echo "Example: ./scripts/set_version.sh 2.0.0"
    exit 1
fi

NEW_VERSION="${NEW_VERSION#v}"

echo "================================================================="
echo "🏷️ ActonOS Plugin SDK - Version Synchronization Tool"
echo "================================================================="
echo "Target Version: v${NEW_VERSION}"
echo ""

# 1. Update root VERSION
echo -n "${NEW_VERSION}" > "${ROOT_DIR}/VERSION"
echo "✅ Updated root VERSION -> ${NEW_VERSION}"

# 2. Update sdk/VERSION
echo -n "${NEW_VERSION}" > "${ROOT_DIR}/sdk/VERSION"
echo "✅ Updated sdk/VERSION  -> ${NEW_VERSION}"

echo ""
echo "Version successfully set to v${NEW_VERSION}!"
echo "================================================================="
