#!/usr/bin/env sh

# install.sh — Install Automergent from the latest GitHub release.
# Downloads the prebuilt binary, installs it to /usr/local/bin or ~/.local/bin,
# creates the `amt` alias, and prints a summary card styled like the TUI's
# session-complete banner (same brand mark, rule, and color palette).

set -e

REPO="iSundram/Automergent"
BINARY_NAME="automergent"
ALIAS_NAME="amt"

# --- ANSI palette, mirroring the exit banner in cmd/automergent/main.go ---
RESET="\033[0m"
ACCENT="\033[1;38;5;255m"    # brand wordmark
TEXT="\033[38;5;252m"        # field values
DIM="\033[38;5;245m"         # labels, rules, tips
COMMAND="\033[1;38;5;153m"   # runnable commands
SUCCESS="\033[38;5;114m"     # final status
BRAND_MARK="\033[38;5;255m✦︎$RESET "  # brand glyph + text-presentation selector

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
if [ -n "$INSTALL_DIR" ]; then
    :
elif [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
elif [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
    INSTALL_DIR="$HOME/.local/bin"
else
    echo "Error: Cannot determine install directory. Set INSTALL_DIR env var."
    exit 1
fi

printf "%b✧%b Installing Automergent for %b%s/%s%b...\n" "$DIM" "$RESET" "$TEXT" "$OS" "$ARCH" "$RESET"

# Fetch latest release or use VERSION env var if set
if [ -n "$VERSION" ]; then
    RELEASE_URL="https://api.github.com/repos/$REPO/releases/tags/v$VERSION"
    TAG="v$VERSION"
else
    RELEASE_URL="https://api.github.com/repos/$REPO/releases/latest"
    TAG=""
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

printf "%b✧%b Fetching release metadata...\n" "$DIM" "$RESET"
if ! curl -fsSL "$RELEASE_URL" -o "$TMP_DIR/release.json"; then
    echo "Error: Could not fetch release information."
    exit 1
fi

# The tag names the version on the summary card.
if [ -z "$TAG" ]; then
    TAG=$(grep -o '"tag_name": *"[^"]*"' "$TMP_DIR/release.json" | head -n 1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
fi

DOWNLOAD_URL=$(grep -o "https://github.com/[^\"]*${OS}_${ARCH}[^\"]*\.tar\.gz" "$TMP_DIR/release.json" | head -n 1)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find download URL for ${OS}/${ARCH}."
    exit 1
fi

printf "%b✧%b Downloading %b%s%b...\n" "$DIM" "$RESET" "$TEXT" "${TAG:-latest}" "$RESET"
if ! curl -fSL "$DOWNLOAD_URL" -o "$TMP_DIR/release.tar.gz"; then
    echo "Error: Download failed."
    exit 1
fi

printf "%b✧%b Extracting...\n" "$DIM" "$RESET"
tar -xzf "$TMP_DIR/release.tar.gz" -C "$TMP_DIR"

if [ ! -f "$TMP_DIR/$BINARY_NAME" ]; then
    echo "Error: $BINARY_NAME binary not found in archive."
    exit 1
fi

chmod +x "$TMP_DIR/$BINARY_NAME"
mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"

# Short alias, so `amt -s <id>` from the session-complete banner works.
ln -sf "$INSTALL_DIR/$BINARY_NAME" "$INSTALL_DIR/$ALIAS_NAME"

# --- Summary card, in the session-complete style ---
WIDTH=72
RULE="${DIM}$(printf '%.0s─' $(seq 1 $WIDTH))${RESET}"
LABEL_WIDTH=11

field() {
    # field <label> <value> — one padded label/value row, like the exit card.
    # All four pieces go through %b so the ANSI variables expand.
    printf " %b%-11s%b%b%s%b\n" "$DIM" "$1" "$RESET" "$TEXT" "$2" "$RESET"
}

PATH_NOTE=""
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        PATH_NOTE=" Add ${INSTALL_DIR} to your PATH to use the command."
        ;;
esac

TIPS=$(cat <<'EOF'
Run `amt` inside a project directory to start a session.
Resume any past session with `amt -s <id>` — the exit banner prints the exact command.
Type /help inside a session for commands and shortcuts.
EOF
)
TIP=$(printf '%s\n' "$TIPS" | sed -n "$(( $(date +%s) % 3 + 1 ))p")

printf "\n"
printf " %b%b%b   %bINSTALL COMPLETE%b\n" "$BRAND_MARK" "$ACCENT" "AUTOMERGENT" "$DIM" "$RESET"
printf "%b\n" "$RULE"
printf "\n"
field "Version" "${TAG:-unknown}"
field "Platform" "${OS}/${ARCH}"
field "Binary" "${INSTALL_DIR}/${BINARY_NAME}"
field "Alias" "${INSTALL_DIR}/${ALIAS_NAME} → ${BINARY_NAME}"
printf "\n"
printf " %bGet started%b\n" "$DIM" "$RESET"
printf "   %bautomergent%b          %bstart in your project%b\n" "$COMMAND" "$RESET" "$DIM" "$RESET"
printf "   %bamt%b                  %bshort alias, same binary%b\n" "$COMMAND" "$RESET" "$DIM" "$RESET"
printf "   %bautomergent --help%b   %ball flags%b\n" "$COMMAND" "$RESET" "$DIM" "$RESET"
[ -n "$PATH_NOTE" ] && printf "\n%b\n" " ${DIM}${PATH_NOTE}${RESET}"
printf "\n"
printf " %bTip%b  %b%s%b\n" "$DIM" "$RESET" "$DIM" "$TIP" "$RESET"
printf "\n"
printf "%b\n" "$RULE"
printf " %b✓ Installed%b  %bSee you inside.%b\n" "$SUCCESS" "$RESET" "$DIM" "$RESET"
printf "\n"
