#!/usr/bin/env bash
# push-docs.sh — Push docs-site to the automergent.github.io repo.
#
# Prerequisites:
#   1. Create the repo: https://github.com/automergent/automergent.github.io
#   2. Add a deploy key with write access:
#      - Generate: ssh-keygen -t ed25519 -C "docs-deploy" -f deploy_key -N ""
#      - Add the PUBLIC key as a deploy key on automergent.github.io repo
#        (Settings → Deploy keys → Add deploy key → Allow write access)
#   3. Store the PRIVATE key as a repo secret named DEPLOY_KEY:
#      - Copy the contents of deploy_key (not deploy_key.pub)
#      - Settings → Secrets and variables → Actions → New repository secret
#        Name: DEPLOY_KEY, Value: <contents of deploy_key>
#
# Usage:
#   ./push-docs.sh              # push current docs-site/
#   ./push-docs.sh --dry-run    # show what would be pushed

set -euo pipefail

DOCS_DIR="$(cd "$(dirname "$0")/docs-site" && pwd)"
TEMP_DIR=$(mktemp -d)
DRY_RUN=false

[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=true

echo "📦 Preparing docs deployment..."

# Copy docs-site to temp dir
cp -r "$DOCS_DIR"/* "$TEMP_DIR/"

# Ensure .nojekyll exists
touch "$TEMP_DIR/.nojekyll"

if $DRY_RUN; then
  echo "🔍 Dry run — would push these files:"
  find "$TEMP_DIR" -type f | sed "s|$TEMP_DIR/||" | sort
  rm -rf "$TEMP_DIR"
  exit 0
fi

# Clone the target repo (shallow)
TARGET_REPO="git@github.com:automergent/automergent.github.io.git"
echo "📡 Cloning $TARGET_REPO..."
git clone --depth 1 --branch main "$TARGET_REPO" "$TEMP_DIR/target" 2>/dev/null || \
git clone --depth 1 "$TARGET_REPO" "$TEMP_DIR/target"

# Clear old files (preserve .git)
cd "$TEMP_DIR/target"
git rm -rf . 2>/dev/null || true

# Copy new files
cp -r "$TEMP_DIR"/* . 2>/dev/null || true
cp "$TEMP_DIR/.nojekyll" . 2>/dev/null || true
rm -rf target .gitconfig

# Commit and push
git config user.email "deploy@automergent.github.io"
git config user.name "Automergent Docs Bot"
git add -A
git diff --cached --quiet && echo "✅ No changes to deploy." && exit 0

COMMIT_MSG="docs: update $(date -u +%Y-%m-%dT%H:%M:%SZ)"
git commit -m "$COMMIT_MSG"
git push origin main

echo "🚀 Deployed to https://automergent.github.io"
rm -rf "$TEMP_DIR"
