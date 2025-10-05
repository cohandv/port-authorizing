#!/bin/bash
# Port Authorizing installation script
# Usage: curl -fsSL https://raw.githubusercontent.com/davidcohan/port-authorizing/main/scripts/install.sh | bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🚀 Port Authorizing Installer"
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  linux*)
    OS="linux"
    ;;
  darwin*)
    OS="darwin"
    ;;
  msys*|mingw*|cygwin*)
    OS="windows"
    ;;
  *)
    echo -e "${RED}❌ Unsupported OS: $OS${NC}"
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    echo -e "${RED}❌ Unsupported architecture: $ARCH${NC}"
    exit 1
    ;;
esac

echo "📍 Detected: $OS-$ARCH"
echo ""

# Get latest release version
echo "🔍 Fetching latest release..."
LATEST_VERSION=$(curl -s https://api.github.com/repos/davidcohan/port-authorizing/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_VERSION" ]; then
  echo -e "${RED}❌ Failed to fetch latest version${NC}"
  echo ""
  echo "Possible reasons:"
  echo "  • Network connectivity issues"
  echo "  • GitHub API rate limiting"
  echo "  • No releases published yet"
  echo ""
  echo "Please try one of these alternatives:"
  echo ""
  echo "1. Manual download from GitHub:"
  echo "   https://github.com/davidcohan/port-authorizing/releases"
  echo ""
  echo "2. Use Docker:"
  echo "   docker pull cohandv/port-authorizing:latest"
  echo ""
  echo "3. Build from source:"
  echo "   git clone https://github.com/davidcohan/port-authorizing.git"
  echo "   cd port-authorizing && make build"
  echo ""
  exit 1
fi

echo -e "${GREEN}✓ Latest version: $LATEST_VERSION${NC}"
echo ""

# Construct download URL
BINARY_NAME="port-authorizing-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  BINARY_NAME="${BINARY_NAME}.exe"
fi

DOWNLOAD_URL="https://github.com/davidcohan/port-authorizing/releases/download/${LATEST_VERSION}/${BINARY_NAME}"
CHECKSUM_URL="${DOWNLOAD_URL}.sha256"

# Create temp directory
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

# Download binary
echo "📥 Downloading $BINARY_NAME..."
if ! curl -fSL "$DOWNLOAD_URL" -o "$BINARY_NAME"; then
  echo -e "${RED}❌ Failed to download binary${NC}"
  rm -rf "$TMP_DIR"
  exit 1
fi

# Download checksum
echo "🔐 Downloading checksum..."
if ! curl -fSL "$CHECKSUM_URL" -o "${BINARY_NAME}.sha256"; then
  echo -e "${YELLOW}⚠️  Failed to download checksum (continuing anyway)${NC}"
else
  # Verify checksum
  echo "✅ Verifying checksum..."
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c "${BINARY_NAME}.sha256" || {
      echo -e "${RED}❌ Checksum verification failed${NC}"
      rm -rf "$TMP_DIR"
      exit 1
    }
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -c "${BINARY_NAME}.sha256" || {
      echo -e "${RED}❌ Checksum verification failed${NC}"
      rm -rf "$TMP_DIR"
      exit 1
    }
  else
    echo -e "${YELLOW}⚠️  sha256sum not found, skipping verification${NC}"
  fi
fi

# Make executable
chmod +x "$BINARY_NAME"

# Install to /usr/local/bin (or appropriate location)
INSTALL_DIR="/usr/local/bin"
INSTALL_PATH="$INSTALL_DIR/port-authorizing"

echo ""
echo "📦 Installing to $INSTALL_PATH..."

if [ -w "$INSTALL_DIR" ]; then
  mv "$BINARY_NAME" "$INSTALL_PATH"
else
  echo "🔑 Requesting sudo access for installation..."
  sudo mv "$BINARY_NAME" "$INSTALL_PATH"
fi

# Cleanup
cd - > /dev/null
rm -rf "$TMP_DIR"

echo ""
echo -e "${GREEN}✅ Installation complete!${NC}"
echo ""
echo "Verify installation:"
echo "  port-authorizing --version"
echo ""
echo "Get started:"
echo "  port-authorizing -h"
echo ""
echo "Documentation: https://github.com/davidcohan/port-authorizing"

