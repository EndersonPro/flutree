#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ARCH=arm64 scripts/package_macos.sh [build|contract]
  VERSION=<semver> ARCH=arm64 scripts/package_macos.sh [build|contract]

Modes:
  build     Build Go binary and package tarball + sha256 (default)
  contract  Print expected artifact filenames and exit

Env vars:
  VERSION   Optional override. Defaults to VERSION file in repository root
  ARCH      Required. Must be arm64 for initial release scope
  OUTPUT_DIR Optional. Defaults to dist
EOF
}

MODE="${1:-build}"

if [[ "${MODE}" != "build" && "${MODE}" != "contract" ]]; then
  usage
  exit 2
fi

VERSION="${VERSION:-}"
ARCH="${ARCH:-}"
OUTPUT_DIR="${OUTPUT_DIR:-dist}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="${ROOT_DIR}/VERSION"

if [[ -z "${VERSION}" ]]; then
  if [[ ! -f "${VERSION_FILE}" ]]; then
    echo "VERSION file not found: ${VERSION_FILE}"
    exit 2
  fi
  VERSION="$(tr -d '[:space:]' < "${VERSION_FILE}")"
fi

if [[ -z "${ARCH}" ]]; then
  echo "ARCH is required"
  exit 2
fi

if [[ "${ARCH}" != "arm64" ]]; then
  echo "Unsupported ARCH '${ARCH}'. Initial release supports darwin-arm64 only."
  exit 2
fi

BASE_NAME="flutree-${VERSION}-macos-${ARCH}"
TARBALL_NAME="${BASE_NAME}.tar.gz"
SHA_NAME="${BASE_NAME}.sha256"

if [[ "${MODE}" == "contract" ]]; then
  echo "${TARBALL_NAME}"
  echo "${SHA_NAME}"
  exit 0
fi

OUTPUT_PATH="${ROOT_DIR}/${OUTPUT_DIR}"

mkdir -p "${OUTPUT_PATH}"

BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "${BUILD_DIR}"' EXIT

CGO_ENABLED=0 GOOS=darwin GOARCH="${ARCH}" \
  go build -ldflags="-s -w -X main.version=${VERSION}" -o "${BUILD_DIR}/dist/flutree" ./cmd/flutree

mkdir -p "${BUILD_DIR}/package"
cp "${BUILD_DIR}/dist/flutree" "${BUILD_DIR}/package/flutree"

tar -czf "${OUTPUT_PATH}/${TARBALL_NAME}" -C "${BUILD_DIR}/package" flutree
shasum -a 256 "${OUTPUT_PATH}/${TARBALL_NAME}" | cut -d ' ' -f 1 > "${OUTPUT_PATH}/${SHA_NAME}"

echo "Created ${OUTPUT_PATH}/${TARBALL_NAME}"
echo "Created ${OUTPUT_PATH}/${SHA_NAME}"
