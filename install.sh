#!/usr/bin/env bash
# Install ghs (GitHub account switcher)
# Usage: curl -sL https://raw.githubusercontent.com/cli/ghs/main/install.sh | bash
# Or:   curl -sL https://raw.githubusercontent.com/cli/ghs/main/install.sh | bash -s -- /usr/local/bin

set -e

INSTALL_DIR="${1:-/usr/local/bin}"
REPO="ru-yaka/ghs"
BINARY="ghs"

# Detect OS and ARCH
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64|amd64)   ARCH="amd64" ;;
    aarch64|arm64)  ARCH="arm64" ;;
    *)              echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux|darwin)   ;;
    mingw*|msys*|windows) OS="windows"; BINARY="ghs.exe" ;;
    *)              echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest release URL
URL=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -o "\"browser_download_url\": \"[^\"]*${OS}-${ARCH}[^\"]*\"" \
    | head -1 \
    | sed 's/.*: "\(.*\)"/\1/')

if [ -z "$URL" ]; then
    echo "No release found for ${OS}-${ARCH}. Try: go install github.com/${REPO}@latest"
    exit 1
fi

echo "Downloading ${BINARY} for ${OS}-${ARCH}..."
TMP="$(mktemp -d)/${BINARY}"
curl -sL "$URL" -o "$TMP"
chmod +x "$TMP"

# Move to install dir
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
    echo "Need sudo to install to ${INSTALL_DIR}:"
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed: ${INSTALL_DIR}/${BINARY}"
echo "Run 'ghs help' to get started."
