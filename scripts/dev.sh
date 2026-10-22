#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
DATA_DIR=${MAZIMA_DEV_DATA:-$ROOT/.dev-data}
PORT=${MAZIMA_DEV_PORT:-8088}

mkdir -p "$DATA_DIR"

cd "$ROOT/daemon"
go build -trimpath -o "$DATA_DIR/mazimad" ./cmd/mazimad

echo "MazimaOS dev server: http://127.0.0.1:$PORT"
echo "Data directory: $DATA_DIR"
exec "$DATA_DIR/mazimad" \
  --dev \
  --data-dir "$DATA_DIR" \
  --ui-dir "$ROOT/ui" \
  --listen "127.0.0.1:$PORT"
