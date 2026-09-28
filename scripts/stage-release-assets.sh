#!/usr/bin/env bash
# Stage per-platform release assets from dist/<os>_<arch>/ and prove the host binary reports the release version.
set -euo pipefail
version="${1:?usage: stage-release-assets.sh <version>}"
host="$(go env GOOS)_$(go env GOARCH)"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  src="dist/$p/urnet-client"
  dst="dist/urnet-client_$p"
  [ -f "$src" ] || { echo "missing $src" >&2; exit 1; }
  cp "$src" "$dst"
  if [ "$p" = "$host" ]; then
    got="$("./$dst" --version)"
    [ "$got" = "$version" ] || { echo "$dst reports '$got', want '$version'" >&2; exit 1; }
  fi
done
echo "staged release assets for $version"
