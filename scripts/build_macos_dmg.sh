#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_GO="/Users/wutong/code/tmp/go-toolchain/go/bin/go"
APP_NAME="ExcelImageConverter"
APP_DISPLAY_NAME="Excel 图片转换器"
APP_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/VERSION")"
OUT_DIR="$ROOT_DIR/dist/macos"
APP_DIR="$OUT_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
STAGING_DIR="$OUT_DIR/dmg-staging"
DMG_PATH="$OUT_DIR/${APP_NAME}-mac-arm64.dmg"

if command -v go >/dev/null 2>&1; then
  GO_BIN="$(command -v go)"
elif [[ -x "$LOCAL_GO" ]]; then
  GO_BIN="$LOCAL_GO"
else
  echo "go is required to build the bundled converter CLI." >&2
  exit 1
fi

rm -rf "$APP_DIR" "$STAGING_DIR" "$DMG_PATH"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR" "$STAGING_DIR"

cd "$ROOT_DIR"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 "$GO_BIN" build \
  -ldflags="-s -w -X github.com/wutong/excel-image-converter/internal/buildinfo.Version=$APP_VERSION" \
  -o "$RESOURCES_DIR/excel-image-converter-cli" \
  ./cmd/excel-image-converter

swiftc \
  -module-cache-path "$ROOT_DIR/tmp/clang-module-cache" \
  -target arm64-apple-macos12 \
  -O \
  "$ROOT_DIR/macos/ExcelImageConverter/main.swift" \
  "$ROOT_DIR/macos/ExcelImageConverter/AppInfo.swift" \
  "$ROOT_DIR/macos/ExcelImageConverter/AppDelegate.swift" \
  "$ROOT_DIR/macos/ExcelImageConverter/ConverterView.swift" \
  -o "$MACOS_DIR/$APP_NAME"

cp "$ROOT_DIR/macos/ExcelImageConverter/Info.plist" "$CONTENTS_DIR/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $APP_VERSION" "$CONTENTS_DIR/Info.plist"
cp "$ROOT_DIR/assets/app-icon.icns" "$RESOURCES_DIR/AppIcon.icns"
chmod +x "$MACOS_DIR/$APP_NAME" "$RESOURCES_DIR/excel-image-converter-cli"

codesign --force --deep --sign - "$APP_DIR" >/dev/null

cp -R "$APP_DIR" "$STAGING_DIR/$APP_DISPLAY_NAME.app"
ln -s /Applications "$STAGING_DIR/Applications"
hdiutil create \
  -volname "$APP_DISPLAY_NAME" \
  -srcfolder "$STAGING_DIR" \
  -ov \
  -format UDZO \
  "$DMG_PATH"

echo "Built: $DMG_PATH"
