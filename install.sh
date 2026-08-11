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
    VERSION=$(curl -s "https://api.github.com/repos/$REPO/tags" | grep -o '"name": "installer/v[^"]*"' | head -n 1 | sed 's/"name": "\(.*\)"/\1/')
fi

if [ -z "$VERSION" ]; then
    echo "Error: Could not detect latest installer version."
    exit 1
fi

echo "✧ Detecting release assets..."
# URL-encode the version (slash to %2F)
ENCODED_VERSION=$(echo "$VERSION" | sed 's|/|%2F|g')

# Fetch the release information and find the URL of the asset matching our platform
# We search for the browser_download_url that contains our OS, Arch, and .tar.gz
DOWNLOAD_URL=$(curl -s "https://api.github.com/repos/$REPO/releases/tags/$ENCODED_VERSION" | \
    grep -o "https://github.com/[^\"]*${OS}_${ARCH}.tar.gz" | \
    head -n 1)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find installer binary for ${OS}/${ARCH}."
    exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "✧ Downloading Installer $VERSION..."
if ! curl -fSL "$DOWNLOAD_URL" -o "$TMP_DIR/installer.tar.gz"; then
    echo "Error: Failed to download installer from $DOWNLOAD_URL"
    exit 1
fi

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
