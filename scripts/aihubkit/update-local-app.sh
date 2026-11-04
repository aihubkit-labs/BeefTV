#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if [[ "$(uname -s)" != Darwin ]]; then
  echo "This entry updates the local macOS BeefTV app." >&2
  exit 1
fi
TARGET="darwin/$(uname -m)"
[[ "$TARGET" != darwin/x86_64 ]] || TARGET=darwin/amd64
if [[ -n "${BEEFTV_WAILS_PLATFORM:-}" && "$BEEFTV_WAILS_PLATFORM" != "$TARGET" ]]; then
  echo "Local installation requires the host target: $TARGET" >&2
  exit 1
fi

if [[ -n "${BEEFTV_GO_DIR:-}" ]]; then
  export PATH="$BEEFTV_GO_DIR/bin:$PATH"
  export GOROOT="$BEEFTV_GO_DIR"
fi
if [[ -n "${BEEFTV_NODE_RUNTIME:-}" ]]; then
  export PATH="$BEEFTV_NODE_RUNTIME/bin:$PATH"
fi
for tool in go node bun zip unzip rsync xcrun codesign; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Required build tool is missing: $tool" >&2; exit 1; }
done

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export CGO_ENABLED=1
# This entry always runs the upstream release gate before building and installing.
export BEEFTV_SKIP_LOCAL_VERIFY=0
if [[ -z "${BEEFTV_NODE_RUNTIME:-}" ]]; then
  BEEFTV_NODE_RUNTIME="$(node -p 'require("node:path").dirname(require("node:path").dirname(require("node:fs").realpathSync(process.execPath)))')"
  export BEEFTV_NODE_RUNTIME
fi
export PATH="$BEEFTV_NODE_RUNTIME/bin:$PATH"

echo "Checking build toolchains"
xcrun --find clang >/dev/null
bun "$ROOT_DIR/scripts/package-agent-host.mjs" "$TARGET" --verify-runtime
(
  cd "$ROOT_DIR/backend"
  go version
  go mod download
)

echo "Installing locked frontend dependencies"
(
  cd "$ROOT_DIR/web"
  bun install --frozen-lockfile
)

echo "Building protocol packages from current sources"
mkdir -p "$ROOT_DIR/.local/cache"
PLUGIN_STAGE="$(mktemp -d "$ROOT_DIR/.local/cache/aihubkit-plugins.XXXXXX")"
trap 'rm -rf -- "$PLUGIN_STAGE"' EXIT
# The upstream packager embeds documentation into manifests. Run it on a fresh
# copy so generated documentation never rewrites the checkout's plugin sources.
rsync -a --exclude='*.beeftv-plugin' --exclude='*.tmp' \
  "$ROOT_DIR/plugin-packages/" "$PLUGIN_STAGE/plugin-packages/"
sh "$PLUGIN_STAGE/plugin-packages/build-packages.sh"
shopt -s nullglob
manifests=("$PLUGIN_STAGE/plugin-packages/"*/manifest.json)
packages=("$PLUGIN_STAGE/plugin-packages/"*.beeftv-plugin)
if [[ ${#manifests[@]} -eq 0 || ${#packages[@]} -ne ${#manifests[@]} ]]; then
  echo "Protocol package generation is incomplete." >&2
  exit 1
fi
for manifest in "${manifests[@]}"; do
  package_dir="${manifest%/manifest.json}"
  unzip -tqq "$PLUGIN_STAGE/plugin-packages/${package_dir##*/}.beeftv-plugin"
done
# Only generated archives are synchronized/deleted; excluded source directories
# stay untouched. A removed plugin must not survive in the app via an old archive.
rsync -a --checksum --delete --include='*.beeftv-plugin' --exclude='*' \
  "$PLUGIN_STAGE/plugin-packages/" "$ROOT_DIR/plugin-packages/"

echo "Checking, building and updating the local app"
"$ROOT_DIR/scripts/update-local-beeftv-app.sh"
