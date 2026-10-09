#!/bin/sh
# Writes the Homebrew formula for a release: cadrei.rb.in with the version
# and each platform's checksum from the release's checksums.txt.
#
#   packaging/homebrew/formula.sh <version> <checksums.txt> > cadrei.rb

set -eu

[ $# -eq 2 ] || { echo "usage: $0 <version> <checksums.txt>" >&2; exit 1; }
version=${1#v}
sums=$2
here=$(cd "$(dirname "$0")" && pwd)

script="s|@VERSION@|$version|g"
for platform in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64; do
  file="cadrei_${version}_${platform}.tar.gz"
  sum=$(awk -v f="$file" '$2 == f { print $1 }' "$sums")
  case $sum in
    [0-9a-f]*) [ ${#sum} -eq 64 ] || { echo "$0: bad checksum for $file" >&2; exit 1; } ;;
    *) echo "$0: $sums does not list $file" >&2; exit 1 ;;
  esac
  script="$script;s|@SHA256_${platform}@|$sum|g"
done
sed "$script" "$here/cadrei.rb.in"
