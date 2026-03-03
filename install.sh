#!/bin/bash
set -euo pipefail

# usectl installer
# Usage: curl -fsSL https://usectl.com/install.sh | bash

REPO="https://usectl.com/releases"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="usectl"

# --- Detect OS & Architecture ---

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    echo "Error: unsupported OS: $OS"
    exit 1
    ;;
esac

DOWNLOAD_URL="${REPO}/latest/${BINARY_NAME}-${OS}-${ARCH}"

echo "==> Downloading usectl for ${OS}/${ARCH}..."
echo "    ${DOWNLOAD_URL}"

TMP_DIR="$(mktemp -d)"
TMP_FILE="${TMP_DIR}/${BINARY_NAME}"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if command -v curl &>/dev/null; then
  curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL"
elif command -v wget &>/dev/null; then
  wget -qO "$TMP_FILE" "$DOWNLOAD_URL"
else
  echo "Error: curl or wget is required"
  exit 1
fi

chmod +x "$TMP_FILE"

# --- Install ---

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
else
  echo "==> Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo ""
echo "✓ usectl installed to ${INSTALL_DIR}/${BINARY_NAME}"
echo ""
echo "Get started:"
echo "  usectl login"
echo "  usectl projects list"
echo ""
