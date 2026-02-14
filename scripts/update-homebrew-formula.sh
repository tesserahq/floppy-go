#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
FORMULA="$ROOT_DIR/packaging/homebrew/floppy.rb"
VERSION="${1:-}"  # optional, defaults to formula version if empty

if [[ ! -f "$FORMULA" ]]; then
  echo "Formula not found: $FORMULA" >&2
  exit 1
fi

if [[ ! -f "$DIST_DIR/floppy-checksums.txt" ]]; then
  echo "Checksums file not found: $DIST_DIR/floppy-checksums.txt" >&2
  exit 1
fi

get_sha() {
  local fname="$1"
  awk -v f="$fname" '$2==f {print $1}' "$DIST_DIR/floppy-checksums.txt"
}

sha_darwin_arm64="$(get_sha floppy-darwin-arm64.tar.gz)"
sha_darwin_amd64="$(get_sha floppy-darwin-amd64.tar.gz)"

if [[ -z "$sha_darwin_arm64" || -z "$sha_darwin_amd64" ]]; then
  echo "Missing checksums for darwin artifacts in floppy-checksums.txt" >&2
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  VERSION="$(awk '/version/ {print $2}' "$FORMULA" | tr -d '"')"
fi

if [[ -z "$VERSION" ]]; then
  echo "Could not determine version. Pass it as the first argument." >&2
  exit 1
fi

sed -i '' \
  -e "s|/releases/download/v[0-9.\-]*/floppy-darwin-arm64.tar.gz|/releases/download/v${VERSION}/floppy-darwin-arm64.tar.gz|" \
  -e "s|/releases/download/v[0-9.\-]*/floppy-darwin-amd64.tar.gz|/releases/download/v${VERSION}/floppy-darwin-amd64.tar.gz|" \
  -e "s/sha256 \"[0-9a-f]\{64\}\"/sha256 \"${sha_darwin_arm64}\"/1" \
  -e "s/sha256 \"[0-9a-f]\{64\}\"/sha256 \"${sha_darwin_amd64}\"/2" \
  "$FORMULA"

echo "Updated $FORMULA with version v${VERSION} and darwin checksums."
