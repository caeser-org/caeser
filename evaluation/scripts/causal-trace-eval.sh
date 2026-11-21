#!/usr/bin/env bash
# Paired evaluation: run the pinned production engine with backward causal
# tracing ON and OFF on the same cloned revision and CI log, and record both.
#
# Reproducible cases are panic/anomaly failures whose tracer anchor is an
# implementation frame present in the base source, so the (lost) injected
# regression test is not required. Assertion cases are excluded because their
# injected overlays were not preserved.
set -uo pipefail

ROOT="${ROOT:-$(git rev-parse --show-toplevel)}"
CAESER="${CAESER:-/tmp/caeser}"
CACHE="${CACHE:-/tmp/causal-eval}"
MANIFEST="${MANIFEST:-$ROOT/evaluation/causal-trace-manifest.csv}"
OUT="${OUT:-$ROOT/evaluation/results/iteration7-causal-trace-paired.json}"

mkdir -p "$CACHE" "$(dirname "$OUT")"
printf '[' > "$OUT"
first=1

while IFS=, read -r id repo sha log culprit; do
  [[ -z "$id" || "$id" == \#* ]] && continue
  dir="$CACHE/$id"
  if [[ ! -d "$dir/.git" ]]; then
    rm -rf "$dir"; mkdir -p "$dir"
    git -C "$dir" init -q
    git -C "$dir" remote add origin "https://github.com/$repo.git"
    if ! timeout 240 git -C "$dir" fetch -q --depth 1 origin "$sha" 2>/dev/null; then
      echo "SKIP $id: fetch failed" >&2; continue
    fi
    git -C "$dir" checkout -q FETCH_HEAD 2>/dev/null || { echo "SKIP $id: checkout" >&2; continue; }
  fi
  logabs="$ROOT/$log"
  run() { # $1 = on|off ; $2 = outfile
    local start secs
    start=$(date +%s)
    if [[ "$1" == off ]]; then
      CAESER_NO_CAUSAL_TRACE=1 "$CAESER" diagnose --repo "$dir" --ci-log "$logabs" --artifact-first --json > "$2" 2>"$2.err"
    else
      "$CAESER" diagnose --repo "$dir" --ci-log "$logabs" --artifact-first --json > "$2" 2>"$2.err"
    fi
    secs=$(( $(date +%s) - start ))
    echo "$secs"
  }
  on_secs=$(run on "$CACHE/$id.on.json")
  off_secs=$(run off "$CACHE/$id.off.json")

  primary_on=$(jq -r '.likely_function.primary.function // "ABSTAIN"' "$CACHE/$id.on.json" 2>/dev/null)
  primary_off=$(jq -r '.likely_function.primary.function // "ABSTAIN"' "$CACHE/$id.off.json" 2>/dev/null)
  kind_on=$(jq -r '.likely_function.primary.kind // "-"' "$CACHE/$id.on.json" 2>/dev/null)
  trace_status=$(jq -r '.causal_trace.status // "-"' "$CACHE/$id.on.json" 2>/dev/null)
  anchor=$(jq -r '.causal_trace.anchor.function // "-"' "$CACHE/$id.on.json" 2>/dev/null)
  stop=$(jq -r '(.causal_trace.limitations // []) | last // "-"' "$CACHE/$id.on.json" 2>/dev/null)
  supersedes=$(jq -r '.causal_trace.supersedes_manifestation // false' "$CACHE/$id.on.json" 2>/dev/null)
  entries_on=$(jq -c '[.candidate_report.entries[]?.function // "null"]' "$CACHE/$id.on.json" 2>/dev/null)
  entries_off=$(jq -c '[.candidate_report.entries[]?.function // "null"]' "$CACHE/$id.off.json" 2>/dev/null)
  root_on=$(jq -r '.root_cause.status // "?"' "$CACHE/$id.on.json" 2>/dev/null)
  root_off=$(jq -r '.root_cause.status // "?"' "$CACHE/$id.off.json" 2>/dev/null)
  hit_on=false; [[ "$primary_on" == *"$culprit"* ]] && hit_on=true
  hit_off=false; [[ "$primary_off" == *"$culprit"* ]] && hit_off=true
  in3_on=$(jq -r --arg c "$culprit" '[(.candidate_report.entries[0:3][]?.function // "")] | any(contains($c))' "$CACHE/$id.on.json" 2>/dev/null)
  in3_off=$(jq -r --arg c "$culprit" '[(.candidate_report.entries[0:3][]?.function // "")] | any(contains($c))' "$CACHE/$id.off.json" 2>/dev/null)

  [[ $first == 1 ]] || printf ',' >> "$OUT"
  first=0
  printf '\n  {"id":"%s","repo":"%s","reviewed_culprit":"%s","primary_on":"%s","primary_off":"%s","kind_on":"%s","trace_status":"%s","anchor":"%s","supersedes":%s,"stop":"%s","entries_on":%s,"entries_off":%s,"hit_on":%s,"hit_off":%s,"top3_on":%s,"top3_off":%s,"root_on":"%s","root_off":"%s","seconds_on":%s,"seconds_off":%s}' \
    "$id" "$repo" "$culprit" "$primary_on" "$primary_off" "$kind_on" "$trace_status" "$anchor" "$supersedes" "$stop" "$entries_on" "$entries_off" "$hit_on" "$hit_off" "$in3_on" "$in3_off" "$root_on" "$root_off" "$on_secs" "$off_secs" >> "$OUT"
  echo "$id on=$primary_on off=$primary_off trace=$trace_status hit_on=$hit_on hit_off=$hit_off (${on_secs}s/${off_secs}s)" >&2
done < "$MANIFEST"

printf '\n]\n' >> "$OUT"
echo "wrote $OUT" >&2
