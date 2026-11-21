#!/usr/bin/env bash
# Independent holdout evaluation for Ceaser.
#
# For each curated case:
#   1. clone the buggy base revision,
#   2. fetch the exact injected regression test(s) from the head revision and
#      verify each SHA-256 (no approximation),
#   3. VERIFY: base must fail as a *test* failure (not a build error) and head
#      must pass. Cases that do not verify are rejected, not scored,
#   4. run the production engine on the verified base+overlay and record the
#      primary suspect vs the reviewed culprit.
#
# Cases are real PRs (base fails, fix passes) with an independently reviewed
# culprit. Do not fabricate.
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
emit() { [[ $first == 1 ]] || printf ',' >> "$OUT"; first=0; printf '\n %s' "$1" >> "$OUT"; }

for i in $(seq 0 $((n - 1))); do
  c="$(jq -c ".cases[$i]" "$MANIFEST")"
  repo="$(jq -r .repo <<<"$c")"; base="$(jq -r .base_sha <<<"$c")"; head="$(jq -r .head_sha <<<"$c")"
  cmd="$(jq -r .command <<<"$c")"; culprit="$(jq -r .reviewed_culprit_function <<<"$c")"
  dir="$CACHE/${repo//\//_}-$base"
  if [ ! -d "$dir/.git" ]; then
    mkdir -p "$dir"; git -C "$dir" init -q; git -C "$dir" remote add origin "https://github.com/$repo.git"
    timeout 300 git -C "$dir" fetch -q --depth 1 origin "$base" || { echo "SKIP $repo fetch" >&2; continue; }
    git -C "$dir" checkout -q FETCH_HEAD || { echo "SKIP $repo checkout" >&2; continue; }
  fi
  base_sha="$(git -C "$dir" rev-parse HEAD)"

  apply_overlay() {
    while read -r path want; do
      [ -z "$path" ] && continue
      mkdir -p "$dir/$(dirname "$path")"
      curl -fsSL "https://raw.githubusercontent.com/$repo/$head/$path" -o "$dir/$path" || return 1
      got="$(shasum -a 256 "$dir/$path" | awk '{print $1}')"
      [ "$got" = "$want" ] || { echo "digest mismatch $path" >&2; return 1; }
    done < <(jq -r '.injected_tests[] | "\(.path) \(.sha256)"' <<<"$c")
  }
  run_cmd() { ( cd "$dir" && bash -lc "$cmd" ) >"$CACHE/$base_sha.out" 2>&1; echo $?; }

  git -C "$dir" reset --hard -q "$base_sha" 2>/dev/null; git -C "$dir" clean -fdq
  if ! apply_overlay; then emit "{\"repo\":\"$repo\",\"base_sha\":\"$base\",\"verified\":false,\"reject\":\"overlay\"}"; continue; fi
  rc_base="$(run_cmd)"
  base_out="$(cat "$CACHE/$base_sha.out")"
  base_failed=false; [ "$rc_base" != 0 ] && base_failed=true
  build_error=false
  grep -qE '\[build failed\]|undefined:|cannot use .* as|cannot find package|no required module|syntax error|not enough arguments|too many arguments' <<<"$base_out" && build_error=true

  git -C "$dir" fetch -q --depth 1 origin "$head" || { emit "{\"repo\":\"$repo\",\"base_sha\":\"$base\",\"verified\":false,\"reject\":\"head fetch\"}"; continue; }
  git -C "$dir" reset --hard -q FETCH_HEAD 2>/dev/null; git -C "$dir" clean -fdq
  rc_head="$(run_cmd)"
  head_passed=false; [ "$rc_head" = 0 ] && head_passed=true

  verified=false
  [ "$base_failed" = true ] && [ "$build_error" = false ] && [ "$head_passed" = true ] && verified=true
  if [ "$verified" != true ]; then
    emit "{\"repo\":\"$repo\",\"base_sha\":\"$base\",\"head_sha\":\"$head\",\"verified\":false,\"base_failed\":$base_failed,\"build_error\":$build_error,\"head_passed\":$head_passed,\"reject\":\"failed verification\"}"
    echo "$repo@$base REJECT base_failed=$base_failed build_error=$build_error head_passed=$head_passed" >&2
    continue
  fi

  # Verified: back to base+overlay and run the production engine.
  git -C "$dir" reset --hard -q "$base_sha" 2>/dev/null; git -C "$dir" clean -fdq
  apply_overlay || true
  # The engine reproduces in an isolated git worktree of HEAD, so the injected
  # regression test must be committed to be visible there.
  git -C "$dir" add -A >/dev/null 2>&1
  git -C "$dir" -c user.email=holdout@local -c user.name=holdout commit -qm "holdout: injected regression test" >/dev/null 2>&1 || true
  "$CAESER" diagnose --repo "$dir" --command "$cmd" --attempts 1 --json > "$CACHE/$base_sha.report.json" 2>"$CACHE/$base_sha.err" || true
  primary="$(jq -r '.likely_function.primary.function // "ABSTAIN"' "$CACHE/$base_sha.report.json" 2>/dev/null)"
  trace="$(jq -r '.causal_trace.status // "-"' "$CACHE/$base_sha.report.json" 2>/dev/null)"
  hit=false; [[ "$primary" == *"$culprit"* ]] && hit=true
  emit "{\"repo\":\"$repo\",\"base_sha\":\"$base\",\"head_sha\":\"$head\",\"reviewed_culprit\":\"$culprit\",\"verified\":true,\"primary\":\"$primary\",\"trace\":\"$trace\",\"hit\":$hit,\"exposed\":false}"
  echo "$repo@$base VERIFIED primary=$primary culprit=$culprit hit=$hit" >&2
done
printf '\n]\n' >> "$OUT"
echo "holdout: $n configured -> $OUT" >&2
