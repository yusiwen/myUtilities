#!/bin/sh
set -eu

NAME="${NAME:-mu}"
PREFIX="${PREFIX:-/usr/local/bin}"
VERSION="v1.0.13"

fail() {
    echo "Error: $*" >&2
    exit 1
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

maybe_sudo() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    else
        sudo "$@"
    fi
}

cleanup() {
    [ -n "${tmpdir:-}" ] && rm -rf "$tmpdir"
}
trap cleanup EXIT

command_exists curl || fail "curl is required"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
raw_arch=$(uname -m)

case "$os" in
    linux) os_name="linux" ;;
    darwin) os_name="darwin" ;;
    mingw*|cygwin|msys) os_name="windows" ;;
    *) fail "Unsupported OS: $os" ;;
esac

case "$raw_arch" in
    x86_64|amd64) arch_name="amd64" ;;
    aarch64|arm64)
        if [ "$os_name" = "darwin" ]; then
            arch_name="arm64"
        else
            arch_name="armv8"
        fi
        ;;
    *) fail "Unsupported architecture: $raw_arch" ;;
esac

case "$os_name" in
    windows) ext="zip" ;;
    *) ext="gz" ;;
esac

bin_name="${NAME}-${os_name}-${arch_name}-${VERSION}"
download_url="https://github.com/yusiwen/myUtilities/releases/download/${VERSION}/${bin_name}.${ext}"

tmpdir=$(mktemp -d) || fail "Failed to create temp directory"

echo "Downloading mu ${VERSION} for ${os_name}-${arch_name} ..."
(cd "$tmpdir" && curl -fL -o "${bin_name}.${ext}" "$download_url") || fail "Download failed"

case "$ext" in
    gz)
        command_exists gunzip || fail "gunzip is required"
        gunzip "$tmpdir/${bin_name}.${ext}"
        bin_path="$tmpdir/$bin_name"
        ;;
    zip)
        command_exists unzip || fail "unzip is required"
        unzip -q "$tmpdir/${bin_name}.${ext}" -d "$tmpdir"
        bin_path="$tmpdir/$bin_name"
        if [ ! -f "$bin_path" ] && [ -f "${bin_path}.exe" ]; then
            bin_path="${bin_path}.exe"
        fi
        if [ ! -f "$bin_path" ]; then
            echo "Error: could not find binary in zip archive" >&2
            ls -la "$tmpdir" >&2
            exit 1
        fi
        ;;
esac

echo "Installing $bin_name to $PREFIX/ ..."
maybe_sudo install -m 755 "$bin_path" "$PREFIX/$bin_name"
maybe_sudo ln -sf "$PREFIX/$bin_name" "$PREFIX/$NAME"

echo "Installation complete."
maybe_sudo "$PREFIX/$NAME" version 2>/dev/null || echo "Run '$NAME version' to verify."
