#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
if [[ -z "${PARITY_UPSTREAM_DIR:-}" ]]; then
  PARITY_UPSTREAM_DIR=$(mktemp -d "${TMPDIR:-/tmp}/go-bash-upstream.XXXXXX")
  trap 'rm -rf "$PARITY_UPSTREAM_DIR"' EXIT
  cp internal/parity/upstream/package{,-lock}.json "$PARITY_UPSTREAM_DIR/"
  npm ci --prefix "$PARITY_UPSTREAM_DIR" --ignore-scripts --no-audit --no-fund
fi
export PARITY_UPSTREAM_DIR
node --test internal/parity/compare.test.mjs
node internal/parity/harness.mjs
