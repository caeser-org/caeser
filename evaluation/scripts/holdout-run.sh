#!/usr/bin/env bash
# Independent holdout evaluation for Ceaser.
#
# For each curated case in evaluation/holdout/manifest.json:
#   1. clone the repository at the buggy base revision,
#   2. fetch the exact injected regression test(s) from the fix/head revision and
#      verify each SHA-256 against the manifest (no approximation),
#   3. run the production engine (reproduce + diagnose) on that revision,
#   4. record the primary suspect and whether it matches the reviewed culprit.
#
# Cases must be real: base fails, fix passes, culprit independently reviewed.
# The manifest starts empty on purpose; do not fabricate cases.
set -uo pipefail

ROOT="${ROOT:-$(git rev-parse --show-toplevel)}"
CAESER="${CAESER:-/tmp/caeser}"
MANIFEST="${MANIFEST:-$ROOT/evaluation/holdout/manifest.json}"
CACHE="${CACHE:-/tmp/holdout}"
OUT="${OUT:-$ROOT/evaluation/results/holdout-results.json}"

mkdir -p "$CACHE" "$(dirname "$OUT")"
n="$(jq '.cases | length' "$MANIFEST" 2>/dev/null || echo 0)"
printf '[' > "$OUT"
first=1
for i in $(seq 0 $((n - 1))); do
  c="$(jq -c ".cases[$i]" "$MANIFEST")"
  repo="$(jq -r .repo <<<"$c")"
  base="$(jq -r .base_sha <<<"$c")"
  head="$(jq -r .head_sha <<<"$c")"
  cmd="$(jq -r .command <<<"$c")"
  culprit="$(jq -r .reviewed_culprit_function <<<"$c")"
  dir="$CACHE/${repo//\//_}-$base"
  if [ ! -d "$dir/.git" ]; then
    mkdir -p "$dir"
    git -C "$dir" init -q
    git -C "$dir" remote add origin "https://github.com/$repo.git"
    if ! timeout 300 git -C "$dir" fetch -q --depth 1 origin "$base"; then echo "SKIP $repo fetch" >&2; continue; fi
    if ! git -C "$dir" checkout -q FETCH_HEAD; then echo "SKIP $repo checkout" >&2; continue; fi
  fi
  ok=1
  while read -r path want; do
    [ -z "$path" ] && continue
    mkdir -p "$dir/$(dirname "$path")"
    if ! curl -fsSL "https://raw.githubusercontent.com/$repo/$head/$path" -o "$dir/$path"; then
      echo "SKIP $repo test $path (fetch)" >&2; ok=0; break
    fi
    got="$(shasum -a 256 "$dir/$path" | awk '{print $1}')"
    if [ "$got" != "$want" ]; then echo "SKIP $repo test $path (digest)" >&2; ok=0; break; fi
  done < <(jq -r '.injected_tests[] | "\(.path) \(.sha256)"' <<<"$c")
  [ "$ok" = 1 ] || continue

  "$CAESER" diagnose --repo "$dir" --command "$cmd" --attempts 1 --json > "$CACHE/$base.json" 2>"$CACHE/$base.err" || true
  primary="$(jq -r '.likely_function.primary.function // "ABSTAIN"' "$CACHE/$base.json" 2>/dev/null)"
  trace="$(jq -r '.causal_trace.status // "-"' "$CACHE/$base.json" 2>/dev/null)"
  hit=false; [[ "$primary" == *"$culprit"* ]] && hit=true
  [[ $first == 1 ]] || printf ',' >> "$OUT"; first=0
  printf '\n {"repo":"%s","base_sha":"%s","reviewed_culprit":"%s","primary":"%s","trace":"%s","hit":%s,"exposed":false}' \
    "$repo" "$base" "$culprit" "$primary" "$trace" "$hit" >> "$OUT"
  echo "$repo@$base primary=$primary culprit=$culprit hit=$hit" >&2
done
printf '\n]\n' >> "$OUT"
echo "holdout: $n configured -> $OUT" >&2
