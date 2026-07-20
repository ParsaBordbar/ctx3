#!/bin/sh
# ctx3 installer — downloads a prebuilt binary from GitHub Releases.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/parsabordbar/ctx3/main/install.sh | sh
# Options (env):
#   CTX3_INSTALL_DIR   target directory (default: /usr/local/bin, else ~/.local/bin)
#   CTX3_VERSION       tag to install (default: latest)
set -eu

REPO="parsabordbar/ctx3"
BIN="ctx3"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*) echo "ctx3: unsupported architecture '$arch' — try 'go install github.com/parsabordbar/ctx3@latest'" >&2; exit 1 ;;
esac

case "$os" in
	linux | darwin) ;;
	*) echo "ctx3: unsupported OS '$os' — try 'go install github.com/parsabordbar/ctx3@latest'" >&2; exit 1 ;;
esac

asset="${BIN}_${os}_${arch}.tar.gz"
version="${CTX3_VERSION:-latest}"
if [ "$version" = "latest" ]; then
	url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
	url="https://github.com/${REPO}/releases/download/${version}/${asset}"
fi

# Choose an install dir we can write to.
dest="${CTX3_INSTALL_DIR:-/usr/local/bin}"
if [ ! -d "$dest" ] || [ ! -w "$dest" ]; then
	dest="$HOME/.local/bin"
	mkdir -p "$dest"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "ctx3: downloading $asset ($version) ..."
if command -v curl >/dev/null 2>&1; then
	curl -fSL "$url" -o "$tmp/$asset"
elif command -v wget >/dev/null 2>&1; then
	wget -qO "$tmp/$asset" "$url"
else
	echo "ctx3: need curl or wget installed" >&2
	exit 1
fi

tar -xzf "$tmp/$asset" -C "$tmp"
chmod +x "$tmp/$BIN"

if install -m 0755 "$tmp/$BIN" "$dest/$BIN" 2>/dev/null; then
	:
else
	mv "$tmp/$BIN" "$dest/$BIN"
	chmod 0755 "$dest/$BIN"
fi

echo "ctx3: installed to $dest/$BIN"

case ":$PATH:" in
	*":$dest:"*) ;;
	*) echo "ctx3: $dest is not on your PATH — add it with:  export PATH=\"$dest:\$PATH\"" ;;
esac

"$dest/$BIN" version 2>/dev/null || true
