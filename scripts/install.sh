#!/bin/bash
# Port Authorizing installation script
# Usage: GITHUB_TOKEN=<<YOUR_TOKEN>> curl -fsSL https://raw.githubusercontent.com/cohandv/port-authorizing/refs/heads/main/scripts/install.sh | bash

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

# Check for jq
if ! command -v jq >/dev/null 2>&1; then
  echo -e "${RED}❌ jq is required but not installed${NC}"
  echo ""
  echo "Please install jq:"
  echo "  • macOS: brew install jq"
  echo "  • Ubuntu/Debian: sudo apt-get install jq"
  echo "  • CentOS/RHEL: sudo yum install jq"
  echo ""
  exit 1
fi

# Get latest release info
echo "🔍 Fetching latest release..."
RELEASE_JSON=$(curl -s -H "Authorization: token $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" https://api.github.com/repos/cohandv/port-authorizing/releases/latest)

LATEST_VERSION=$(echo "$RELEASE_JSON" | jq -r '.tag_name')

if [ -z "$LATEST_VERSION" ] || [ "$LATEST_VERSION" = "null" ]; then
  echo -e "${RED}❌ Failed to fetch latest version${NC}"
  echo ""
  echo "Possible reasons: "
  echo "  • Network connectivity issues"
  echo "  • GitHub API rate limiting"
  echo "  • No releases published yet"
  echo ""
  echo "Please try one of these alternatives:"
  echo ""
  echo "1. Manual download from GitHub:"
  echo "   https://github.com/cohandv/port-authorizing/releases"
  echo ""
  echo "2. Build from source:"
  echo "   git clone https://github.com/cohandv/port-authorizing.git"
  echo "   cd port-authorizing && make build"
  echo ""
  exit 1
fi

echo -e "${GREEN}✓ Latest version: $LATEST_VERSION${NC}"
echo ""

# Determine binary name
BINARY_NAME="port-authorizing-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  BINARY_NAME="${BINARY_NAME}.exe"
fi
CHECKSUM_NAME="${BINARY_NAME}.sha256"

# Extract asset IDs from release info
echo "🔍 Finding asset IDs..."
BINARY_ASSET_ID=$(echo "$RELEASE_JSON" | jq -r ".assets[] | select(.name == \"$BINARY_NAME\") | .id")
CHECKSUM_ASSET_ID=$(echo "$RELEASE_JSON" | jq -r ".assets[] | select(.name == \"$CHECKSUM_NAME\") | .id")

if [ -z "$BINARY_ASSET_ID" ] || [ "$BINARY_ASSET_ID" = "null" ]; then
  echo -e "${RED}❌ Failed to find asset ID for $BINARY_NAME${NC}"
  exit 1
fi

echo -e "${GREEN}✓ Found binary asset ID: $BINARY_ASSET_ID${NC}"
if [ -n "$CHECKSUM_ASSET_ID" ] && [ "$CHECKSUM_ASSET_ID" != "null" ]; then
  echo -e "${GREEN}✓ Found checksum asset ID: $CHECKSUM_ASSET_ID${NC}"
fi
echo ""

# Construct download URLs using asset IDs
DOWNLOAD_URL="https://api.github.com/repos/cohandv/port-authorizing/releases/assets/${BINARY_ASSET_ID}"
if [ -n "$CHECKSUM_ASSET_ID" ] && [ "$CHECKSUM_ASSET_ID" != "null" ]; then
  CHECKSUM_URL="https://api.github.com/repos/cohandv/port-authorizing/releases/assets/${CHECKSUM_ASSET_ID}"
fi

# Create temp directory
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

# Download binary
echo "📥 Downloading $BINARY_NAME..."
if ! curl -fSL -H "Authorization: token $GITHUB_TOKEN" -H "Accept: application/octet-stream" "$DOWNLOAD_URL" -o "$BINARY_NAME"; then
  echo -e "${RED}❌ Failed to download binary${NC}"
  rm -rf "$TMP_DIR"
  exit 1
fi

# Download checksum
echo "🔐 Downloading checksum..."
if [ -n "$CHECKSUM_URL" ] && curl -fSL -H "Authorization: token $GITHUB_TOKEN" -H "Accept: application/octet-stream" "$CHECKSUM_URL" -o "${BINARY_NAME}.sha256"; then
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
else
  echo -e "${YELLOW}⚠️  Checksum not available or failed to download (continuing anyway)${NC}"
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
echo "Documentation: https://github.com/cohandv/port-authorizing"

