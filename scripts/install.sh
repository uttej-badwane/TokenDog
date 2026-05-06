#!/usr/bin/env bash
# TokenDog installer — downloads the appropriate release artifact for the
# host OS/arch, verifies the checksum, and installs to /usr/local/bin.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/uttej-badwane/TokenDog/main/scripts/install.sh | sh
#   curl -fsSL .../install.sh | TD_VERSION=v0.7.0 sh   # pin version
#   curl -fsSL .../install.sh | INSTALL_DIR=~/.local/bin sh   # custom dir

set -euo pipefail

REPO="uttej-badwane/TokenDog"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
TD_VERSION="${TD_VERSION:-latest}"

# --- platform detection -----------------------------------------------------

uname_os() {
    case "$(uname -s)" in
        Linux*)  echo linux ;;
        Darwin*) echo darwin ;;
        *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac
}

uname_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo amd64 ;;
        arm64|aarch64) echo arm64 ;;
        *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
    esac
}

OS=$(uname_os)
ARCH=$(uname_arch)

# --- version resolution -----------------------------------------------------

if [ "$TD_VERSION" = "latest" ]; then
    TD_VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4)
    if [ -z "$TD_VERSION" ]; then
        echo "could not resolve latest version (rate-limited? try TD_VERSION=v0.7.0)" >&2
        exit 1
    fi
fi

# --- download and verify ----------------------------------------------------

ARCHIVE="TokenDog_${OS}_${ARCH}.tar.gz"
DL_BASE="https://github.com/$REPO/releases/download/$TD_VERSION"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading TokenDog $TD_VERSION for $OS/$ARCH..."
curl -fsSL "$DL_BASE/$ARCHIVE" -o "$TMPDIR/$ARCHIVE"
curl -fsSL "$DL_BASE/checksums.txt" -o "$TMPDIR/checksums.txt"

# Verify checksum. shasum is BSD/macOS, sha256sum is GNU. Either should work.
echo "Verifying checksum..."
cd "$TMPDIR"
if command -v sha256sum >/dev/null 2>&1; then
    grep "  $ARCHIVE\$" checksums.txt | sha256sum -c -
else
    EXPECTED=$(grep "  $ARCHIVE\$" checksums.txt | awk '{print $1}')
    ACTUAL=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
    if [ "$EXPECTED" != "$ACTUAL" ]; then
        echo "checksum mismatch! expected $EXPECTED, got $ACTUAL" >&2
        exit 1
    fi
fi
cd - >/dev/null

# --- extract and install ----------------------------------------------------

tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"

# Need write permission on INSTALL_DIR. Use sudo if directory isn't writable.
INSTALL_PATH="$INSTALL_DIR/td"
if [ -w "$INSTALL_DIR" ]; then
    install -m 0755 "$TMPDIR/td" "$INSTALL_PATH"
elif command -v sudo >/dev/null 2>&1; then
    echo "Installing to $INSTALL_DIR (sudo required)..."
    sudo install -m 0755 "$TMPDIR/td" "$INSTALL_PATH"
else
    echo "$INSTALL_DIR is not writable and sudo is unavailable." >&2
    echo "Try: INSTALL_DIR=\$HOME/.local/bin curl ... | sh" >&2
    exit 1
fi

# Symlink tokendog → td for parity with the brew install.
if [ -w "$INSTALL_DIR" ]; then
    ln -sf "$INSTALL_PATH" "$INSTALL_DIR/tokendog"
elif command -v sudo >/dev/null 2>&1; then
    sudo ln -sf "$INSTALL_PATH" "$INSTALL_DIR/tokendog"
fi

echo
echo "Installed: $("$INSTALL_PATH" --version)"
echo "Run 'td welcome' to see setup status."
