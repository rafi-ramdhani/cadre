#!/bin/sh
# Installs cadre without Homebrew: downloads the release binary for this
# machine, checks it against the release's checksums.txt, and puts it in
# ~/.local/bin/cadre. Running it again upgrades. It does nothing else: the
# first run of `cadre` does the setup.
#
#   curl -fsSL https://raw.githubusercontent.com/rafi-ramdhani/cadre/main/install.sh | sh
#
# With Homebrew, use instead: brew install rafi-ramdhani/cadre/cadre
#
# Environment:
#   CADRE_VERSION       a version to install (default: the latest release)
#   CADRE_INSTALL_DIR   where to put cadre (default: ~/.local/bin)
#   CADRE_DOWNLOAD_URL  where the release files are (default: the GitHub
#                       release of that version)

set -eu

repo=rafi-ramdhani/cadre
dir=${CADRE_INSTALL_DIR:-$HOME/.local/bin}

say() { printf '%s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is needed"
command -v tar >/dev/null 2>&1 || die "tar is needed"

case $(uname -s) in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "cadre runs on macOS and Linux only" ;;
esac
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "no cadre release for $(uname -m)" ;;
esac

version=${CADRE_VERSION:-}
if [ -z "$version" ]; then
  # The latest release's page redirects to .../tag/v<version>.
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
    die "could not find the latest release"
  version=${url##*/v}
  case $version in
    '' | */*) die "could not find the latest release" ;;
  esac
fi
version=${version#v}
base=${CADRE_DOWNLOAD_URL:-https://github.com/$repo/releases/download/v$version}
archive="cadre_${version}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
say "Downloading cadre $version for $os/$arch..."
curl -fsSL -o "$tmp/$archive" "$base/$archive" || die "could not download $base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || die "could not download $base/checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "checksums.txt does not list $archive"
if command -v sha256sum >/dev/null 2>&1; then
  have=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
  have=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
[ "$have" = "$want" ] || die "$archive does not match its checksum; nothing was installed"

tar -xzf "$tmp/$archive" -C "$tmp" cadre || die "$archive holds no cadre"
mkdir -p "$dir"
# A new file renamed over the old one, so a running cadre keeps working.
cp "$tmp/cadre" "$dir/.cadre.new"
chmod 755 "$dir/.cadre.new"
mv -f "$dir/.cadre.new" "$dir/cadre"

say "Installed $("$dir/cadre" --version) in $dir/cadre."
case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "Add $dir to your PATH (for example in ~/.zshrc: export PATH=\"$dir:\$PATH\")." ;;
esac
say "Run: cadre"
