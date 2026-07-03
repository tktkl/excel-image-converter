#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/dist/windows"
LOCAL_RSRC="/Users/wutong/code/tmp/go-bin/rsrc"
LOCAL_GO="/Users/wutong/code/tmp/go-toolchain/go/bin/go"
APP_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/VERSION")"

mkdir -p "$OUT_DIR"

cd "$ROOT_DIR"
if command -v rsrc >/dev/null 2>&1; then
  RSRC_BIN="$(command -v rsrc)"
elif [[ -x "$LOCAL_RSRC" ]]; then
  RSRC_BIN="$LOCAL_RSRC"
else
  echo "rsrc is required to embed the Windows Common Controls v6 manifest." >&2
  echo "Install it with: go install github.com/akavel/rsrc@latest" >&2
  exit 1
fi

"$RSRC_BIN" -manifest "$ROOT_DIR/cmd/excel-image-converter/ExcelImageConverter.exe.manifest" \
  -ico "$ROOT_DIR/assets/app-icon.ico" \
  -o "$ROOT_DIR/cmd/excel-image-converter/rsrc_windows_amd64.syso"

if command -v go >/dev/null 2>&1; then
  GO_BIN="$(command -v go)"
elif [[ -x "$LOCAL_GO" ]]; then
  GO_BIN="$LOCAL_GO"
else
  echo "go is required to build the Windows executable." >&2
  exit 1
fi

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 "$GO_BIN" build \
  -ldflags="-s -w -H=windowsgui -X github.com/wutong/excel-image-converter/internal/buildinfo.Version=$APP_VERSION" \
  -o "$OUT_DIR/ExcelImageConverter.exe" \
  ./cmd/excel-image-converter

echo "Built: $OUT_DIR/ExcelImageConverter.exe"
