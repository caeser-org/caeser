# FINAL MISSION — PROGRESS

## Session 2026-11-22 (final)
- Baseline: branch `main`, HEAD `fb6edeb`, go1.26.2, disk free 1.7 Gi, `go test ./...` PASS.
- Created `PLAN.md`, `EVIDENCE-MAP.md`, this file, `RESULTS.md`, `LIMITATIONS.md`.
- Audit confirmed the historical summary in `prompt.md` §1 matches the checkout.
- Implemented **Phase A** (control-flow/type-aware defect feasibility) and **Phase B**
  (base/head delta correlation) in `internal/diagnosis/defect_origin.go`.
- Added negative controls (`defect_origin_test.go`).
- `go test ./...` PASS; `go vet` clean; gofmt clean.
- Paired 6-case eval rerun: 6/6 ON vs 5/6 OFF, 0 regressions, 0 root-cause changes;
  defect hypotheses on 2/6 (`go-u008`, `go-u014`), all `source_pattern` (no SSA type,
  no base revision in artifact-first runs).
- Independent evaluation: **BLOCKED** by disk (1.7 Gi free cannot host a new-repo
  corpus + module cache). Reproducible workflow already exists
  (`evaluation/scripts/causal-trace-eval.sh`, `internal/evalcorpus` fixture
  verification, `cmd/corpus-score --verify`).

## Session 2026-11-22 (final, continued — Phase A2)
- Implemented dominance-aware nil-guard suppression (`nilGuardDominates`,
  `blockContains`, `blockTerminates`) and branch-alternative recording
  (`definitionsAreBranchDependent`) in `defect_origin.go`.
- Added negative controls: non-dominating guard not suppressed; `!= nil` then-body
  honored; branch alternatives recorded. `go test ./...` PASS, vet/gofmt clean.
- Paired 6-case eval unchanged (6/6 ON, 5/6 OFF, 0 regressions, 0 root changes).
- **Next**: full reaching-definition/dominator feasibility from `SSAAssertionTrace`
  to upgrade `source_pattern`→`static_data_dependency` only when reachability is
  proven (prerequisite: SSA coverage for the enclosing function).

## Session 2026-11-22 (final, continued — Phase A2 SSA annotation, C, E2, G)
- Phase A2: SSA coverage annotation — an operation in an SSA-traced file that is
  **not** on the assertion dependency path is annotated (never suppressed); type/path
  upgrades to `static_data_dependency` retained.
- Phase C: competing defect hypotheses are retained in `CausalExplanation`
  limitations ("competing defect hypotheses retained: …"); no collapsing to one cause.
- Phase E2: assertion-site anchors — when no observed implementation frame exists,
  the failing test's assertion line anchors a trace (reusing the reported
  `file:line`). Such traces are `assertion_anchored`, carry an explicit limitation,
  and **never admit candidates or supersede**.
- Phase G: added `docs/capabilities.md` (capabilities/limitations matrix, evidence
  semantics, wording rules); PR-comment details now include defect hypotheses.
- Tests added for SSA off-path annotation, competing-hypothesis retention, and
  assertion anchors. `go test ./...` PASS, `go vet ./...` clean, gofmt clean.
- Paired 6-case eval unchanged (6/6 ON, 5/6 OFF, 0 regressions, 0 root changes).

## Session 2026-11-22 (final, continued — boundary-guard tie)
- `boundary_guard` now requires the guard to compare the failing index against
  `len(<the same indexed collection>)`; an unrelated `len(other)` no longer matches
  (adversarial #10). Removed the now-unused `indexOperands`/`usesLen`.
- Negative + positive tests added. `go test ./...` PASS, vet/gofmt clean.

## Next exact command for a future session
```
go test ./... && go run ./cmd/corpus-score --verify && bash evaluation/scripts/causal-trace-eval.sh
```
Then, if disk ≥ ~10 GiB or a CI runner is available: collect independent cases per
`evaluation/corpus/cases/go/GO_COLLECTION.md` (preserve injected test overlays +
hashes), run `bash evaluation/scripts/causal-trace-eval.sh`, and complete Phase F.

## Blockers
- Disk space for independent holdout collection (Phase F).
- SSA coverage is sparse; most real cases have no `SSAAssertionTrace`, so type-aware
  feasibility falls back to `source_pattern`.
