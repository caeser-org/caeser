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
Not run. **BLOCKED** by environment disk (1.7 Gi free). The fixture-integrity
infra (`internal/evalcorpus.VerifyFixture`, `cmd/corpus-score --verify`), the
collection protocol, and the paired runner are prepared and verified; no
independent metric is claimed.

## Runtime
Within noise (1–5 s per case, `go/packages`-dominated); defect assessment is a
bounded single-function AST walk plus an O(values) SSA scan.
