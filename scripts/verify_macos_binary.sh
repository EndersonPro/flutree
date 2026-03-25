#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: scripts/verify_macos_binary.sh <tarball-path>"
  exit 2
fi

TARBALL_PATH="$1"

if [[ ! -f "${TARBALL_PATH}" ]]; then
  echo "Tarball not found: ${TARBALL_PATH}"
  exit 2
fi

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

tar -xzf "${TARBALL_PATH}" -C "${WORK_DIR}"

if [[ ! -x "${WORK_DIR}/flutree" ]]; then
  echo "Extracted binary is missing or not executable"
  exit 2
fi

"${WORK_DIR}/flutree" --help >/dev/null

echo "Smoke verification passed: flutree --help"
