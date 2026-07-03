#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/dist/local"
APP_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/VERSION")"

mkdir -p "$OUT_DIR"

cd "$ROOT_DIR"
go build \
  -ldflags="-X github.com/wutong/excel-image-converter/internal/buildinfo.Version=$APP_VERSION" \
  -o "$OUT_DIR/excel-image-converter" \
  ./cmd/excel-image-converter

echo "Built: $OUT_DIR/excel-image-converter"
