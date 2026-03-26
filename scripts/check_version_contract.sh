#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="${ROOT_DIR}/VERSION"
TAG=""
CLI_VERSION=""

usage() {
  echo "Usage: scripts/check_version_contract.sh [--tag <vX.Y.Z|X.Y.Z>] [--cli-version <X.Y.Z>]"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      if [[ $# -lt 2 ]]; then
        usage
        exit 2
      fi
      TAG="$2"
      shift 2
      ;;
    --cli-version)
      if [[ $# -lt 2 ]]; then
        usage
        exit 2
      fi
      CLI_VERSION="$2"
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [[ ! -f "${VERSION_FILE}" ]]; then
  echo "Missing VERSION file: ${VERSION_FILE}"
  exit 1
fi

VERSION="$(tr -d '[:space:]' < "${VERSION_FILE}")"

if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Invalid VERSION value '${VERSION}'. Expected strict semver X.Y.Z"
  exit 1
fi

if [[ -n "${TAG}" ]]; then
  NORMALIZED_TAG="${TAG#v}"
  if [[ "${NORMALIZED_TAG}" != "${VERSION}" ]]; then
    echo "Tag mismatch. tag=${NORMALIZED_TAG} VERSION=${VERSION}"
    exit 1
  fi
fi

if [[ -n "${CLI_VERSION}" ]]; then
  NORMALIZED_CLI="$(printf '%s' "${CLI_VERSION}" | tr -d '[:space:]')"
  if [[ "${NORMALIZED_CLI}" != "${VERSION}" ]]; then
    echo "CLI mismatch. cli=${NORMALIZED_CLI} VERSION=${VERSION}"
    exit 1
  fi
fi

echo "Version contract valid: VERSION=${VERSION}"
