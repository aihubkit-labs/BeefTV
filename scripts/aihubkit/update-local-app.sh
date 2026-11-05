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
for tool in go node bun python3 bash zip rsync xcrun codesign; do
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
echo "Preparing dependencies and current protocol packages"
python3 "$ROOT_DIR/scripts/aihubkit/build.py" --prepare-only

echo "Checking, building and updating the local app"
"$ROOT_DIR/scripts/update-local-beeftv-app.sh"
