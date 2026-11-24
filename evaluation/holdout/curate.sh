#!/usr/bin/env bash
# Curation helper: for a merged PR, print the head (merge) commit, the buggy base
# (merge's first parent), and each changed test file with its SHA-256 at head.
# Review the diff separately to name the culprit. Verify on CI before admitting.
set -euo pipefail
repo="$1"; pr="$2"
info="$(gh api "repos/$repo/pulls/$pr")"
head="$(jq -r .merge_commit_sha <<<"$info")"
base="$(gh api "repos/$repo/commits/$head" --jq '.parents[0].sha')"
echo "repo=${repo}"
echo "pr=${pr}"
echo "base_sha=${base}"
echo "head_sha=${head}"
while read -r f; do
  [ -z "$f" ] && continue
  sha="$(curl -fsSL "https://raw.githubusercontent.com/$repo/$head/$f" | shasum -a 256 | awk '{print $1}')"
  echo "test=${f} sha256=${sha}"
done < <(gh api "repos/$repo/pulls/$pr/files?per_page=100" --jq '.[].filename' | grep '_test.go$' || true)
