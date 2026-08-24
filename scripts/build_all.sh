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
DIST_DIR="${ROOT_DIR}/dist"
mkdir -p "${DIST_DIR}"

INCLUDE_EXAMPLES="${1:-false}"

echo "================================================================="
echo "🚀 ActonOS Plugin SDK - Batch Build & Package Toolchain"
echo "================================================================="
echo "Root Directory: ${ROOT_DIR}"
echo "Output Dist:    ${DIST_DIR}"
echo ""

SEARCH_DIRS=("plugins")

MANIFESTS=()
for dir in "${SEARCH_DIRS[@]}"; do
    if [ -d "${dir}" ]; then
        while IFS= read -r -d '' file; do
            MANIFESTS+=("${file}")
        done < <(find "${dir}" -name "manifest.json" -print0)
    fi
done

TOTAL=${#MANIFESTS[@]}
echo "Found ${TOTAL} plugin(s) to process."
echo ""

SUCCESS_COUNT=0
FAIL_COUNT=0

for manifest in "${MANIFESTS[@]}"; do
    PLUGIN_DIR="$(dirname "${manifest}")"
    PLUGIN_ID="$(grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' "${manifest}" | head -n1 | cut -d'"' -f4)"
    PLUGIN_NAME="$(grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' "${manifest}" | head -n1 | cut -d'"' -f4)"

    if [ -z "${PLUGIN_ID}" ]; then
        echo "❌ [${PLUGIN_DIR}] Could not extract plugin ID from manifest.json"
        ((FAIL_COUNT++))
        continue
    fi

    echo "▶ Building [${PLUGIN_ID}] (${PLUGIN_NAME})..."

    WASM_OUT="${DIST_DIR}/${PLUGIN_ID}.wasm"
    PKG_OUT="${DIST_DIR}/${PLUGIN_ID}.actonpkg"
    LOCAL_DIST="${PLUGIN_DIR}/dist"
    mkdir -p "${LOCAL_DIST}"

    # 1. Compile WASM binary
    if env GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -trimpath -o "${WASM_OUT}" "./${PLUGIN_DIR}"; then
        cp -f "${WASM_OUT}" "${LOCAL_DIST}/plugin.wasm"
    else
        echo "❌ [${PLUGIN_ID}] Compilation failed!"
        ((FAIL_COUNT++))
        continue
    fi

    # 2. Package into .actonpkg (Zip bundle via acton-plugin CLI)
    rm -f "${PKG_OUT}"
    if go run "${ROOT_DIR}/cmd/acton-plugin" pack -manifest "${manifest}" -wasm "${WASM_OUT}" -out "${PKG_OUT}" > /dev/null 2>&1; then
        PKG_SIZE=$(du -h "${PKG_OUT}" | cut -f1)
        echo "   ✅ Compiled & Packaged -> dist/${PLUGIN_ID}.actonpkg (${PKG_SIZE})"
        ((SUCCESS_COUNT++))
    else
        echo "❌ [${PLUGIN_ID}] Packaging failed!"
        ((FAIL_COUNT++))
    fi
done

echo ""
echo "================================================================="
echo "📊 Build & Packaging Summary"
echo "================================================================="
echo "Total: ${TOTAL} | Success: ${SUCCESS_COUNT} | Failed: ${FAIL_COUNT}"
echo "All distributable packages are located in: ${DIST_DIR}"
echo "================================================================="
