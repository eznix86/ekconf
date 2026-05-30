#!/usr/bin/env bash

set -euo pipefail

REPO="eznix86/ekconf"
BIN_DIR="${HOME}/.local/bin"
BIN_NAME="ekconf"

if [[ $# -gt 0 ]]; then
	VERSION="${1#v}"
else
	VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"].lstrip("v"))')"
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
	darwin|linux) ;;
	*) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

case "$arch" in
	x86_64|amd64) arch="amd64" ;;
	arm64|aarch64) arch="arm64" ;;
	*) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="${BIN_NAME}_${VERSION}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/v${VERSION}/${asset}"
checksums_url="https://github.com/${REPO}/releases/download/v${VERSION}/checksums.txt"

mkdir -p "$BIN_DIR"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL "$url" -o "$tmpdir/$asset"
curl -fsSL "$checksums_url" -o "$tmpdir/checksums.txt"
tar -xzf "$tmpdir/$asset" -C "$tmpdir"

expected="$(grep "$asset" "$tmpdir/checksums.txt" | awk '{print $1}')"
actual="$(openssl dgst -sha256 "$tmpdir/$asset" | awk '{print $NF}')"
if [[ "$expected" != "$actual" ]]; then
	echo "Checksum mismatch: expected $expected, got $actual" >&2
	exit 1
fi
install -m 0755 "$tmpdir/$BIN_NAME" "$BIN_DIR/$BIN_NAME"

echo "Installed $BIN_NAME to $BIN_DIR/$BIN_NAME"
