# Ceaser final mission — PLAN

Mission spec: `prompt.md` (repo root). North star: identify the most defensible
**defect-origin operation/mechanism**, explain causation, differentiate
alternatives, verify when evidence supports it, else abstain.

## Status legend
DONE / PARTIAL / BLOCKED / DEFERRED

## Phases

| Phase | Item | Status | Acceptance |
|---|---|---|---|
| 2.1 | Audit + baseline + progress files | DONE | evidence-flow map, baseline tests green |
| A | Control-flow-sensitive defect feasibility | PARTIAL | type-aware missing_guard; guard dominance; SSA path/coverage annotation; negative controls |
| B | Base/head CodeDelta correlation | DONE | defect op classified changed/unchanged/unmapped; no auto-promotion |
| C | Hypothesis generation/competition | PARTIAL | precise locations + evidence + falsifiable checks; competing hypotheses retained; no unsupported primary shifts |
| D | Targeted runtime experiments/verification | PARTIAL (pre-existing) | existing probe/verifier preserved; no widening; no sound new verifier identified |
| E | Coverage gaps | PARTIAL | E2 assertion anchors (material only) DONE; E1/E3/E4 DEFERRED pending evaluation signal |
| F | Independent evaluation | NEXT | collect ten fresh verified cases; freeze before first evaluation |
| G | Production hardening/docs | DONE (docs) / PARTIAL | `docs/capabilities.md`; comment wording; report wording verified |

## This implementation increment
- Phase A: `missing_guard` becomes type-aware using SSA `SSAValue.Type` when
  available; relationship evidence `source_pattern` vs `static_data_dependency`;
  type-unknown keeps an inferred hypothesis with a limitation.
- Phase B: each defect origin is classified against `report.Deltas`
  (`changed` / `changed_file` / `unchanged`), never used to promote.

## Next batch protocol

Ten fresh cases from repositories the engine has not seen. For each case, verify
base fails as a test failure and head passes, pin the reviewed culprit function,
record source SHAs, then freeze the manifest. Run the first evaluation once,
unchanged. Do not inspect these cases to design or tune a fix before that first
evaluation. The five earlier cases are development-exposed and are not reused as
independent evidence.

## Constraints honored
No parallel engine; reuse `fnOrigins`/`causal_trace`/SSA trace/`CodeDelta`; no
verification widening; hypotheses never promoted; deterministic and bounded.
