#!/usr/bin/env sh

# install.sh — Install Automergent from the latest GitHub release.
# Downloads the prebuilt binary and installs it to /usr/local/bin or ~/.local/bin.

set -e

REPO="iSundram/Automergent"
BINARY_NAME="automergent"

# Detect OS and Arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Error: Unsupported architecture: $ARCH"; exit 1 ;;
esac

case $OS in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    *) echo "Error: Unsupported OS: $OS"; exit 1 ;;
esac

# Determine install directory
if [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
elif [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
    INSTALL_DIR="$HOME/.local/bin"
else
    echo "Error: Cannot determine install directory. Set INSTALL_DIR env var."
    exit 1
fi

echo "✧ Installing Automergent for ${OS}/${ARCH}..."

# Fetch latest release or use VERSION env var if set
if [ -n "$VERSION" ]; then
    RELEASE_URL="https://api.github.com/repos/$REPO/releases/tags/v$VERSION"
else
    RELEASE_URL="https://api.github.com/repos/$REPO/releases/latest"
fi

DOWNLOAD_URL=$(curl -fsSL "$RELEASE_URL" | \
    grep -o "https://github.com/[^\"]*${OS}_${ARCH}.tar.gz" | \
    head -n 1)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find download URL for ${OS}/${ARCH}."
    exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "✧ Downloading from $DOWNLOAD_URL..."
if ! curl -fSL "$DOWNLOAD_URL" -o "$TMP_DIR/release.tar.gz"; then
    echo "Error: Download failed."
    exit 1
fi

echo "✧ Extracting..."
tar -xzf "$TMP_DIR/release.tar.gz" -C "$TMP_DIR"

if [ ! -f "$TMP_DIR/$BINARY_NAME" ]; then
    echo "Error: $BINARY_NAME binary not found in archive."
    exit 1
fi

chmod +x "$TMP_DIR/$BINARY_NAME"
mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"

echo "✓ Installed $BINARY_NAME to $INSTALL_DIR/$BINARY_NAME"
echo "  Run '$BINARY_NAME' to get started."
