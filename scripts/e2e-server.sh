#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
DATA_DIR=${E2E_DATA_DIR:-$ROOT/.e2e-data}
PORT=${E2E_PORT:-18090}

rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR"

cd "$ROOT/daemon"
go build -trimpath -o "$DATA_DIR/mazimad" ./cmd/mazimad
exec "$DATA_DIR/mazimad" \
  --dev \
  --data-dir "$DATA_DIR" \
  --ui-dir "$ROOT/ui" \
  --listen "127.0.0.1:$PORT"
