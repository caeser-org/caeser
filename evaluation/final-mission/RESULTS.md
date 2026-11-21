# FINAL MISSION — RESULTS

## Baseline
`go test ./...` PASS (branch `main`, `fb6edeb`, go1.26.2). No test regressions.

## Phase A — control-flow/type-aware defect feasibility
- `missing_guard` is now type-aware: if `SSAAssertionTrace` records a value type at
  the operand's producer line,
  - a known **non-nilable** type (e.g. `pkg.T`) suppresses the hypothesis (negative
    control), and
  - a known **nilable** type (`*T`, `interface`, `map[/[]/chan/func`, `error`)
    increases the relationship evidence to `static_data_dependency`.
- Without SSA/type the hypothesis stays `source_pattern` with an explicit ambiguity
  limitation ("nil-ability not established") — no invented certainty.
- Any defect whose line is on the SSA assertion dependency path is upgraded to
  `static_data_dependency` (relationship evidence only).

## Phase B — base/head delta correlation
- Each defect origin is classified against `report.Deltas`:
  `changed` (same symbol), `changed_file` (same file, different symbol),
  `unchanged`. Empty when no base revision is available.
- The correlation is informational only: it never changes admission, ranking, or
  verification (no diff-as-causation).

## Phase A2 — control-flow feasibility (this session, additional)
- Nil-guard suppression is now **dominance-aware**: a guard suppresses the
  `missing_guard` hypothesis only when it actually protects the failing operation —
  a top-level `name != nil` whose then-block contains the dereference, or a
  top-level `name == nil` with a terminating block before the dereference. Guards
  nested inside another branch (non-dominating) no longer suppress it.
- Branch-dependent / multiple reaching definitions are recorded as explicit
  alternatives on `later_assignment` rather than a silent "first then last".

## Adversarial tests (defect_origin_test.go)
Later assignment detected; unrelated assignment not flagged; missing guard detected;
present nil guard not flagged; `<= len` boundary detected; correct `<` not flagged;
ignored error detected; branch-dependent reassignment surfaced; works without SSA;
clean function reports nothing; **non-nilable typed deref not flagged**; nilable
typed deref upgraded; unknown type stays `source_pattern`; delta `changed` /
`changed_file` / `unchanged`; **non-dominating guard not suppressed**; dominating
`!= nil` then-body honored; branch alternatives recorded.

## Phase E2 — assertion anchors (this session)
When no observed implementation frame exists, the failing test's assertion line
anchors a `CausalTrace` (`assertion_anchored: true`). It reuses the reported
`file:line` and the existing tracer; it carries an explicit limitation and never
admits candidates or supersedes the primary. This covers assertion failures that
previously produced no trace.

## Phase G — wording/docs
`docs/capabilities.md` documents the capabilities/limitations matrix and evidence
semantics. The PR-comment details now list defect hypotheses. No output labels an
inferred pattern as a confirmed root cause.

## Paired evaluation (6 reproducible, development-exposed cases)
`evaluation/results/iteration7-causal-trace-paired.json`:

| Metric | OFF | ON |
|---|---:|---:|
| exact-function correct | 5/6 | 6/6 |
| misleading primary | 1 | 0 |
| candidate-admission change | — | 0 |
| root-cause status change | — | 0 |
| regressions | — | 0 |

Defect hypotheses: `go-u008`, `go-u014` (`missing_guard`, `source_pattern`).
**Not** root-cause accuracy; these six are development-exposed and do not
establish generalization or defect-operation accuracy.

## Independent evaluation
Independent **infrastructure is now runnable in CI** (GitHub Actions), which was
the disk blocker:
- `evaluation/scripts/holdout-run.sh` + `evaluation/holdout/manifest.json` +
  `evaluation/holdout/README.md`: clone base revision, fetch the exact injected
  test(s) from the fix revision, **verify SHA-256**, run the production engine,
  score against the reviewed culprit. Manifest starts empty; no fabricated cases.
- `.github/workflows/final-eval.yml` runs the six-case paired evaluation and the
  holdout job on a GitHub-hosted runner.

CI evidence (clean runners, `ceaser-org/ceaser`):
- Run `44958895228` — paired ON/OFF: **6/6 ON, 5/6 OFF, 0 regressions, 0
  root-cause changes** (independently reproduced, matches local).
- Run `44959160207` — same, plus the holdout job ran with **0 configured cases**
  (infrastructure verified; no independent metric claimed yet).

The only CI test failure encountered was a **pre-existing environment-flaky**
`internal/execution` process-group test on the runner, unrelated to this work;
the workflow gates strictly on the mission packages and runs the full suite
informationally.

## Independent evaluation — first five verified cases (frozen)
On `ceaser-org/ceaser` CI (and reproduced locally), five verified unfamiliar
cases (kong #627/#651/#637, urfave/cli #2511, spf13/cobra #2356):
**exact-function 1/5**, file-level 2/5, 1 abstention. Internal reproduction and CI
agree. See `INDEPENDENT-RESULTS.md`. Failure categories: sibling/fan-out ranking
and no-frame localization. The six-case 6/6 is **not** generalization accuracy.

## Runtime
Within noise (1–5 s per case, `go/packages`-dominated); defect assessment is a
bounded single-function AST walk plus an O(values) SSA scan.
