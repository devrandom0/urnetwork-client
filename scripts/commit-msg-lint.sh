#!/usr/bin/env bash
set -euo pipefail

# Conventional Commits types supported by .releaserc.json
ALLOWED_TYPES='feat|fix|perf|refactor|docs|chore|ci|build|test'

cc_regex="^(${ALLOWED_TYPES})(\([a-zA-Z0-9_.-]+\))?(!)?: .+"

base_sha="${BASE_SHA:-}"
head_sha="${HEAD_SHA:-}"

if [[ -n "${1-}" && -z "${base_sha}${head_sha}" ]]; then
  # If a file path is provided (hook mode), validate that single message
  msg_file="$1"
  subject=$(head -n1 "$msg_file" | tr -d '\r\n')
  if [[ "$subject" =~ ^Merge\  ]]; then
    exit 0
  fi
  if [[ "$subject" =~ ^Revert\  ]]; then
    exit 0
  fi
  if [[ "$subject" =~ $cc_regex ]]; then
    exit 0
  fi
  echo "Commit message does not follow Conventional Commits:" >&2
  echo "  $subject" >&2
  echo "Expected: <type>[optional scope][!]: <subject>" >&2
  echo "Where <type> is one of: ${ALLOWED_TYPES//|/, }" >&2
  exit 1
fi

# CI mode: determine range
if [[ -z "${head_sha}" ]]; then
  head_sha=$(git rev-parse HEAD)
fi

range_commits=()
if [[ -n "${base_sha}" ]] && git rev-parse --verify --quiet "$base_sha" >/dev/null; then
  # Exclude base commit itself, include head
  mapfile -t range_commits < <(git rev-list --no-merges --reverse "$base_sha..$head_sha")
else
  mapfile -t range_commits < <(git rev-list -n 1 "$head_sha")
fi

bad=0
for c in "${range_commits[@]}"; do
  subject=$(git log -n1 --format=%s "$c" | tr -d '\r\n')
  # Allow merges and GitHub-generated reverts
  if [[ "$subject" =~ ^Merge\  ]]; then
    continue
  fi
  if [[ "$subject" =~ ^Revert\  ]]; then
    continue
  fi
  if [[ "$subject" =~ $cc_regex ]]; then
    continue
  fi
  echo "Invalid commit subject ($c): $subject" >&2
  bad=1
done

if [[ $bad -ne 0 ]]; then
  echo "\nCommit message check failed. Use one of: ${ALLOWED_TYPES//|/, }" >&2
  echo "Example: feat(vpn): add inbound allowlist" >&2
  exit 1
fi

exit 0
