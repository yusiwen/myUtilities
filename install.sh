#!/bin/sh
set -eu

NAME="${NAME:-mu}"
PREFIX="${PREFIX:-/usr/local/bin}"

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
command_exists gunzip || fail "gunzip is required"
command_exists git || fail "git is required"

remote=$(git remote get-url origin 2>/dev/null) || fail "Not a git repository (no 'origin' remote found)"
owner_repo=$(echo "$remote" | sed -nE 's#.*[:/]([^/]+)/([^/.]+)(\.git)?$#\1/\2#p')
[ -n "$owner_repo" ] || fail "Could not extract owner/repo from: $remote"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
raw_arch=$(uname -m)

case "$os" in
    linux) os_asset="linux" ;;
    darwin) os_asset="darwin" ;;
    *) fail "Unsupported OS: $os" ;;
esac

case "$raw_arch" in
    x86_64|amd64) arch_asset="amd64" ;;
    aarch64|arm64)
        if [ "$os_asset" = "darwin" ]; then
            arch_asset="arm64"
        else
            arch_asset="armv8"
        fi
        ;;
    *) fail "Unsupported architecture: $raw_arch" ;;
esac

api_url="https://api.github.com/repos/$owner_repo/releases/latest"
echo "Fetching latest release from $owner_repo ..."

if [ -n "${GITHUB_TOKEN:-}" ]; then
    release_json=$(curl -sf -H "Authorization: token $GITHUB_TOKEN" "$api_url")
else
    release_json=$(curl -sf "$api_url")
fi
[ -n "$release_json" ] || fail "GitHub API request failed (try setting GITHUB_TOKEN)"

tag=$(echo "$release_json" | grep -o '"tag_name":"[^"]*"' | head -1 | cut -d'"' -f4) || true
[ -n "$tag" ] || fail "Could not extract tag from release"

asset_name="${NAME}-${os_asset}-${arch_asset}-${tag}.gz"
download_url=$(echo "$release_json" | grep -A 3 "\"name\":\"$asset_name\"" | grep "browser_download_url" | sed 's/.*"browser_download_url":"\([^"]*\)".*/\1/') || true

if [ -z "$download_url" ]; then
    echo "No asset found for $asset_name." >&2
    echo "Available assets:" >&2
    echo "$release_json" | grep '"name":' | sed 's/.*"name":"\([^"]*\)".*/\1/' >&2
    exit 1
fi

tmpdir=$(mktemp -d) || fail "Failed to create temp directory"

echo "Downloading $asset_name ..."
(cd "$tmpdir" && curl -fL -O "$download_url") || fail "Download failed"

binary_name="${asset_name%.gz}"
echo "Installing $binary_name to $PREFIX/ ..."
gunzip "$tmpdir/$asset_name"
maybe_sudo install -m 755 "$tmpdir/$binary_name" "$PREFIX/$binary_name"
maybe_sudo ln -sf "$PREFIX/$binary_name" "$PREFIX/$NAME"

echo "Installation complete."
maybe_sudo "$PREFIX/$NAME" version 2>/dev/null || echo "Run '$NAME version' to verify."
