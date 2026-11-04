#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ $# -ne 1 ]]; then
  echo "Usage: bash scripts/aihubkit/verify-release-main.sh <release-commit-or-tag>" >&2
  exit 2
fi

fetch_options=(--no-tags)
if [[ "$(git -C "$ROOT_DIR" rev-parse --is-shallow-repository)" == true ]]; then
  fetch_options+=(--unshallow)
fi
if ! git -C "$ROOT_DIR" fetch "${fetch_options[@]}" origin +refs/heads/main:refs/remotes/origin/main; then
  echo "Cannot refresh origin/main; stopping release validation." >&2
  exit 1
fi

release_commit="$(git -C "$ROOT_DIR" rev-parse --verify --end-of-options "${1}^{commit}")"
if ! git -C "$ROOT_DIR" merge-base --is-ancestor "$release_commit" refs/remotes/origin/main; then
  echo "Release commit $release_commit is not merged into origin/main." >&2
  echo "Merge the features, AIHUBKIT_VERSION and AIHUBKIT_CHANGELOG.md before creating a release tag." >&2
  exit 1
fi

echo "Release commit $release_commit is included in origin/main."
