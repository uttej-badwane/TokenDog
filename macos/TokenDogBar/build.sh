#!/usr/bin/env bash
#
# Build TokenDogBar.app — a menu-bar agent that shows Claude API spend from
# `td spend --json`.
#
# We compile with `swiftc` directly rather than `swift build` so this works on
# machines that only have the Command Line Tools (whose SwiftPM manifest runner
# is sometimes broken) as well as full Xcode. The Package.swift is kept for
# IDE / `swift run` development on machines with full Xcode.
#
# Usage:
#   ./build.sh            # build .app into ./dist
#   ./build.sh --run      # build, then launch the app
#   ./build.sh --install  # build, then copy to /Applications
set -euo pipefail

cd "$(dirname "$0")"

APP_NAME="TokenDogBar"
BUNDLE_ID="com.tokendog.menubar"
DIST="dist"
APP="$DIST/$APP_NAME.app"
MACOS_DIR="$APP/Contents/MacOS"

SDK="$(xcrun --show-sdk-path)"
ARCH="$(uname -m)"            # arm64 or x86_64
TARGET="${ARCH}-apple-macosx13.0"

echo "› Compiling ($TARGET)…"
rm -rf "$APP"
mkdir -p "$MACOS_DIR"

swiftc -O \
    -sdk "$SDK" \
    -target "$TARGET" \
    -o "$MACOS_DIR/$APP_NAME" \
    Sources/$APP_NAME/*.swift

echo "› Copying resources…"
RES_DIR="$APP/Contents/Resources"
mkdir -p "$RES_DIR"
cp Resources/*.png "$RES_DIR/" 2>/dev/null || true

echo "› Writing Info.plist…"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>            <string>$APP_NAME</string>
    <key>CFBundleDisplayName</key>     <string>TokenDog Bar</string>
    <key>CFBundleIdentifier</key>      <string>$BUNDLE_ID</string>
    <key>CFBundleExecutable</key>      <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>     <string>APPL</string>
    <key>CFBundleShortVersionString</key> <string>0.1.0</string>
    <key>CFBundleVersion</key>         <string>1</string>
    <key>LSMinimumSystemVersion</key>  <string>13.0</string>
    <!-- Menu-bar agent: no Dock icon, no main menu. -->
    <key>LSUIElement</key>             <true/>
</dict>
</plist>
PLIST

# Ad-hoc sign so launchd/Gatekeeper is happy on the local machine. A real
# release would sign + notarize with a Developer ID instead.
echo "› Ad-hoc signing…"
codesign --force --sign - "$APP" >/dev/null 2>&1 || echo "  (codesign skipped)"

echo "✓ Built $APP"

case "${1:-}" in
    --run)
        echo "› Launching…"
        open "$APP"
        ;;
    --install)
        echo "› Installing to /Applications…"
        rm -rf "/Applications/$APP_NAME.app"
        cp -R "$APP" "/Applications/"
        echo "✓ Installed. Launch it from Spotlight or /Applications."
        ;;
esac
