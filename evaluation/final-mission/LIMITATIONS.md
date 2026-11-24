# FINAL MISSION — LIMITATIONS

## Verified vs hypothesis (unchanged)
No new verified root causes. `assessRootCause` / `mechanismEvidenceVerified` are
untouched. Every defect origin is a hypothesis (`source_pattern` or
`static_data_dependency` relationship evidence), never `established`.

## Feasibility is partial
- Defect feasibility uses SSA **only** for a type at the operand's producer line
  and for on-path membership. Nil-guard suppression is dominance-aware at the AST
  level (top-level terminating `== nil` / containing `!= nil`), but full
  reaching-definition / dominator / phi analysis (Phase A2) is still **not**
  implemented; where a pattern is not proven reachable it stays a hypothesis.
  This remains the largest gap and the next highest-value improvement.
- SSA coverage is sparse in real cases; type-awareness therefore frequently falls
  back to `source_pattern`.

## Mechanism-specific
- `missing_guard` remains pattern-based when the type is unknown; a non-nilable
  deref is suppressed only when SSA provides the type.
- Conversion defects (Phase E4) are not derived from source.
- `boundary_guard` matches `len(...)` comparisons; unrelated `len` expressions are
  possible (no CFG/type tie to the exact indexed collection yet).
- Mutation through pointers/slices/maps/closures/aliases is not modeled.

## Phase B
Delta correlation is by symbol/file only; no rename or line-shift mapping, and no
hunk-level overlap. It is informational and cannot promote.

## Phase C/E
Hypothesis competition exists implicitly (multiple `DefectOrigin`s + alternatives
preserved) but is not rendered as a ranked competitor set. Cross-package indexing
and assertion-anchor expansion (Phase E) are deferred pending evaluation signal.

## Phase F (independent evaluation) — BLOCKED
Environment disk (1.7 Gi free) prevents cloning unfamiliar repositories and their
module caches. No independent metric is claimed. Exact unblock: a runner with
≥ ~10 GiB free; then follow `evaluation/corpus/cases/go/GO_COLLECTION.md` to
preserve injected test overlays + hashes and run
`bash evaluation/scripts/causal-trace-eval.sh` plus `cmd/corpus-score --verify`.

## Explicitly not done
- No LLM root-cause promotion.
- No widening of verification; no new verifier added (none could be made sound
  within this session's scope).
- No claim of production-ready root-cause accuracy or generalization.
