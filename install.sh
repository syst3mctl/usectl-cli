#!/bin/sh
# usectl CLI installer
# Usage: curl -fsSL https://usectl.com/install.sh | bash
set -e

BINARY="usectl"
INSTALL_DIR="/usr/local/bin"
BASE_URL="https://usectl.com/releases/latest"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

FILENAME="${BINARY}-${OS}-${ARCH}"
URL="${BASE_URL}/${FILENAME}"

echo "==> Downloading ${BINARY} for ${OS}/${ARCH}..."
echo "    ${URL}"

TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

curl -fsSL -o "$BINARY" "$URL"
chmod +x "$BINARY"

# Install
echo "==> Installing to ${INSTALL_DIR}/${BINARY}..."
if [ -w "$INSTALL_DIR" ]; then
  mv "$BINARY" "$INSTALL_DIR/"
else
  sudo mv "$BINARY" "$INSTALL_DIR/"
fi

# Cleanup
rm -rf "$TMP_DIR"

echo "==> ${BINARY} installed successfully!"
echo ""
echo "Get started:"
echo "  ${BINARY} login"
echo "  ${BINARY} --help"
