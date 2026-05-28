#!/bin/bash
set -e

REPO="D8-X/d8x-cli"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin) PLATFORM="macos" ;;
  linux)  PLATFORM="linux" ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release"
  exit 1
fi

FILE="d8x-${PLATFORM}-${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$TAG/$FILE"

echo "Downloading d8x $TAG for $PLATFORM/$ARCH..."
curl -sL "$URL" -o "/tmp/$FILE"
tar -xzf "/tmp/$FILE" -C /tmp

if [ "$OS" = "darwin" ]; then
  codesign -s - /tmp/d8x 2>/dev/null || true
  xattr -d com.apple.quarantine /tmp/d8x 2>/dev/null || true
fi

sudo mv /tmp/d8x "$INSTALL_DIR/d8x"
rm -f "/tmp/$FILE"

echo "d8x $TAG installed to $INSTALL_DIR/d8x"
