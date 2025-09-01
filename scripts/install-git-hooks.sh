#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
hooks_dir="$root_dir/.git/hooks"
mkdir -p "$hooks_dir"

cat >"$hooks_dir/commit-msg" <<'EOF'
#!/usr/bin/env bash
"$(git rev-parse --show-toplevel)/scripts/commit-msg-lint.sh" "$@"
EOF
chmod +x "$hooks_dir/commit-msg"

echo "Installed commit-msg hook -> $hooks_dir/commit-msg"
