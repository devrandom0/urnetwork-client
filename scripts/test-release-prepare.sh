#!/usr/bin/env bash
# Runs the semantic-release prepareCmd from .releaserc.json with a fake version and checks the staged assets.
set -euo pipefail
cd "$(dirname "$0")/.."
version="0.0.0-prepare-test"
cmd=$(jq -r '.plugins[] | select(type == "array" and .[0] == "@semantic-release/exec") | .[1].prepareCmd' .releaserc.json)
if [ -z "$cmd" ] || [ "$cmd" = "null" ]; then
  echo "no @semantic-release/exec prepareCmd in .releaserc.json" >&2
  exit 1
fi
rm -rf dist
bash -c "$(printf '%s' "$cmd" | sed "s/\${nextRelease.version}/$version/g")"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  [ -f "dist/urnet-client_$p" ] || { echo "missing dist/urnet-client_$p" >&2; exit 1; }
done
rm -rf dist
echo "release prepare: ok"
