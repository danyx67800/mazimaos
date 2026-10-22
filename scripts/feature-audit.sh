#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
EVIDENCE_DIR=${EVIDENCE_DIR:-$ROOT/docs/evidence}
REPORT=${1:-$EVIDENCE_DIR/feature-audit.txt}

mkdir -p "$EVIDENCE_DIR"

{
  echo "MazimaOS feature audit evidence"
  echo "generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "host: $(uname -srm)"
  echo "go: $(go version)"
  echo "node: $(node --version)"
  echo "npm: $(npm --version)"
  echo

  echo "== gofmt =="
  cd "$ROOT/daemon"
  unformatted=$(gofmt -l .)
  if [ -n "$unformatted" ]; then
    echo "$unformatted"
    echo "FAIL: unformatted Go files"
    exit 1
  fi
  echo "ok"

  echo
  echo "== go vet =="
  go vet ./...
  echo "ok"

  echo
  echo "== go test -cover =="
  go test -count=1 -cover ./...

  echo
  echo "== playwright e2e =="
  cd "$ROOT"
  npm run test:e2e
} 2>&1 | tee "$REPORT"

echo
echo "evidence written to $REPORT"
