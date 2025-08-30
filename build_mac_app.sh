#!/bin/bash
set -e

APP_NAME="TouchyTails"
BINARY_NAME="TouchyTails"
ICON_FILE="TouchyTails.icns"   # place in same folder

# Build Go binary for Apple Silicon
echo "Building $BINARY_NAME for darwin/arm64..."
GOOS=darwin GOARCH=arm64 go build -o "$BINARY_NAME" .

# Create .app bundle structure
echo "Creating $APP_NAME.app bundle..."
rm -rf "$APP_NAME.app"
mkdir -p "$APP_NAME.app/Contents/MacOS"
mkdir -p "$APP_NAME.app/Contents/Resources"

# Copy binary
cp "$BINARY_NAME" "$APP_NAME.app/Contents/MacOS/"

# Copy icon
if [ -f "$ICON_FILE" ]; then
    cp "$ICON_FILE" "$APP_NAME.app/Contents/Resources/"
else
    echo "Warning: icon file $ICON_FILE not found!"
fi

# Create minimal Info.plist
cat > "$APP_NAME.app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundleExecutable</key>
    <string>$BINARY_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>com.example.$APP_NAME</string>
    <key>CFBundleIconFile</key>
    <string>$ICON_FILE</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string>13.0</string>
</dict>
</plist>
EOF

# Make binary executable
chmod +x "$APP_NAME.app/Contents/MacOS/$BINARY_NAME"

# Remove extended attributes to avoid Gatekeeper issues
xattr -cr "$APP_NAME.app"

echo "Build complete: $APP_NAME.app"
