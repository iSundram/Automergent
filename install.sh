#!/usr/bin/env sh

# install.sh — Download the Automergent TUI Installer.
# This script bootstraps the installation by downloading the Automergent TUI Installer
# which then handles the full setup of the Automergent agent.

set -e

REPO="iSundram/Automergent"
PROJECT_NAME="automergent"

# Detect OS and Arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case $OS in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

echo "✧ Bootstrapping Automergent Installer for ${OS}/${ARCH}..."

# Get latest installer release tag (e.g., installer/v0.0.1)
if [ -z "$VERSION" ]; then
    VERSION=$(curl -s "https://api.github.com/repos/$REPO/tags" | grep '"name": "installer/v' | head -n 1 | sed -E 's/.*"([^"]+)".*/\1/')
fi

if [ -z "$VERSION" ]; then
    echo "Error: Could not detect latest installer version."
    exit 1
fi

# Extract the raw version number (e.g., 0.0.1 from installer/v0.0.1)
# Note: we need to handle both 'v0.0.1' and 'installer/v0.0.1' formats
RAW_VER=$(echo $VERSION | sed -E 's|.*v||')

# Download URL for the TUI Installer
BINARY_NAME="installer_${RAW_VER}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$BINARY_NAME"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "✧ Downloading Installer $VERSION..."
curl -sSL "$DOWNLOAD_URL" -o "$TMP_DIR/installer.tar.gz"

# Extract only the installer binary
tar -xzf "$TMP_DIR/installer.tar.gz" -C "$TMP_DIR"

if [ ! -f "$TMP_DIR/installer" ]; then
    echo "Error: Installer binary not found in archive."
    exit 1
fi

chmod +x "$TMP_DIR/installer"

echo "✧ Launching TUI Installer..."
# Run the installer
"$TMP_DIR/installer"
