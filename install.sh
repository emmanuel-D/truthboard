#!/bin/sh
# Truthboard installer — https://github.com/emmanuel-D/truthboard
#
#   curl -fsSL https://raw.githubusercontent.com/emmanuel-D/truthboard/main/install.sh | sh
#
# Downloads the release tarball for this machine, verifies it against the
# release's checksums.txt, and installs `truthboard` to /usr/local/bin
# (when writable) or ~/.local/bin — no sudo, no surprises.
#
# Environment overrides (all optional; the last three exist so the script
# is testable against local fixtures before the repo is public):
#   TRUTHBOARD_INSTALL_DIR  install here instead of the default
#   TRUTHBOARD_VERSION      install this tag (e.g. v0.7.0) instead of latest
#   TRUTHBOARD_BASE_URL     fetch assets from here instead of GitHub releases
#   TRUTHBOARD_OS / TRUTHBOARD_ARCH  pretend to be another platform
set -eu

REPO="emmanuel-D/truthboard"

say() { printf '%s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

os="${TRUTHBOARD_OS:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
case "$os" in
  darwin|linux) ;;
  msys*|mingw*|cygwin*|windows)
    die "Windows isn't covered by this script — grab truthboard_<tag>_windows_amd64.tar.gz from https://github.com/$REPO/releases" ;;
  *) die "unsupported OS \"$os\" — releases cover darwin and linux; try: go install github.com/$REPO/cmd/truthboard@latest" ;;
esac

arch="${TRUTHBOARD_ARCH:-$(uname -m)}"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture \"$arch\" — releases cover amd64 and arm64; try: go install github.com/$REPO/cmd/truthboard@latest" ;;
esac

tag="${TRUTHBOARD_VERSION:-}"
if [ -z "$tag" ]; then
  # The releases/latest redirect names the current tag — no API, no auth.
  tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed 's#.*/##') ||
    die "could not resolve the latest release from https://github.com/$REPO/releases/latest"
  case "$tag" in
    v*) ;;
    *) die "https://github.com/$REPO/releases/latest did not name a release — is the repo public yet?" ;;
  esac
fi

base="${TRUTHBOARD_BASE_URL:-https://github.com/$REPO/releases/download/$tag}"
asset="truthboard_${tag}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "downloading $asset ($tag)…"
curl -fsSL -o "$tmp/$asset" "$base/$asset" || die "download failed: $base/$asset"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || die "download failed: $base/checksums.txt"

want=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "checksums.txt has no entry for $asset"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
else
  got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
fi
[ "$got" = "$want" ] || die "checksum mismatch for $asset — got $got, want $want; refusing to install"

dir="${TRUTHBOARD_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  else
    dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$dir"

tar -xzf "$tmp/$asset" -C "$tmp" truthboard
install -m 0755 "$tmp/truthboard" "$dir/truthboard"

say "installed $("$dir/truthboard" version) → $dir/truthboard"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "note: $dir is not on your PATH — add:  export PATH=\"$dir:\$PATH\"" ;;
esac
say 'next:  cd your-project && truthboard init --agents'
