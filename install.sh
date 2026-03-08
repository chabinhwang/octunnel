#!/bin/bash
set -euo pipefail

REPO="chabinhwang/octunnel"
BINARY="octunnel"
INSTALL_DIR="/usr/local/bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

info() { echo -e "${GREEN}[octunnel]${NC} $1"; }
warn() { echo -e "${YELLOW}[octunnel]${NC} $1"; }
error() { echo -e "${RED}[octunnel]${NC} $1" >&2; exit 1; }

# Detect OS and architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      error "Unsupported OS: $OS" ;;
esac

case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)             error "Unsupported architecture: $ARCH" ;;
esac

info "Detected: ${OS}/${ARCH}"

# Get latest release tag
info "Fetching latest release..."
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  error "Could not determine latest release. Check https://github.com/${REPO}/releases"
fi

info "Latest version: ${LATEST}"

# Download
ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${LATEST}/${ASSET}"

info "Downloading ${URL}..."
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

if ! curl -fsSL "$URL" -o "${TMP_DIR}/${ASSET}"; then
  error "Download failed. Check if release exists: https://github.com/${REPO}/releases/tag/${LATEST}"
fi

# Extract
info "Extracting..."
tar -xzf "${TMP_DIR}/${ASSET}" -C "$TMP_DIR"

if [ ! -f "${TMP_DIR}/${BINARY}" ]; then
  error "Binary not found in archive"
fi

chmod +x "${TMP_DIR}/${BINARY}"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  info "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

info "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
echo ""
"${INSTALL_DIR}/${BINARY}" --help 2>/dev/null | head -5 || true
echo ""
info "Done! Run 'octunnel' to get started."
