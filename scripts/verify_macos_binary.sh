#!/usr/bin/env bash

set -euo pipefail

EXPECTED_VERSION=""

usage() {
  echo "Usage: scripts/verify_macos_binary.sh <tarball-path> [--expected-version <semver>]"
}

if [[ $# -lt 1 ]]; then
  usage
  exit 2
fi

TARBALL_PATH="$1"
shift

while [[ $# -gt 0 ]]; do
  case "$1" in
    --expected-version)
      if [[ $# -lt 2 ]]; then
        usage
        exit 2
      fi
      EXPECTED_VERSION="$2"
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

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
ACTUAL_VERSION="$(${WORK_DIR}/flutree --version | tr -d '[:space:]')"

if [[ -n "${EXPECTED_VERSION}" && "${ACTUAL_VERSION}" != "${EXPECTED_VERSION}" ]]; then
  echo "Version mismatch. expected=${EXPECTED_VERSION} actual=${ACTUAL_VERSION}"
  exit 1
fi

echo "Smoke verification passed: flutree --help and --version=${ACTUAL_VERSION}"
