#!/usr/bin/env bash
# Runs the end-to-end CLI suite: builds the real flutree binary and exercises it
# as a user would. This is the gate the pre-push hook runs before every push.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

echo "e2e: building ./cmd/flutree"
go build -o /dev/null ./cmd/flutree

echo "e2e: running go test ./integration/..."
go test -count=1 ./integration/... "$@"
