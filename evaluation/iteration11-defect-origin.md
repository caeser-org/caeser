# Defect-origin reasoning

Extends the causal tracer from value provenance toward evidence-grounded
hypotheses about the operation that may have introduced the invalid state.
Reuses the tracer's AST/provenance infrastructure; no parallel engine; no
verification change; defect hypotheses are never promoted.

## 1. Existing infrastructure reused

- `fnOrigins`/`buildFnOrigins` (causal_trace.go) for local definitions; extended
  with `allDefs` (every assignment) and `errorIgnored` (blank error slots).
- `operandIdentifiers`, `sourceStep` evidence, the `CausalTrace` structure.
- `BuildCausalExplanation` and `InvestigationBrief` for rendering.
- No SSA/CFG engine is duplicated; defect assessment is AST-based and works
  without SSA (the SSA operand trace remains the stronger evidence where present).

## 2. New defect-origin mechanisms

`assessDefectOrigins` (defect_origin.go) reports, for the operand(s) consumed by
the failing operation:

| Kind | Mechanism |
|---|---|
| `later_assignment` | the operand is defined then reassigned before the failure (incorrect mutation) |
| `ignored_error` | the producing call's error was assigned to `_` and discarded before use |
| `missing_guard` | the failing operation dereferences the operand and no nil check occurs between its producer and the failure |
| `boundary_guard` | a prior guard compares the failing index against `len(...)` with `<=`/`>=` |

Each carries operand, file, line, the operation text, a rationale, and status
`inferred`. An operation is reported only when a concrete relationship to the
failing operand exists.

## 3. New evidence relationships

Defect hypotheses are emitted as `defect_origin` source evidence
(`E-causal-*` / `sourceStep`) and, in the trace, as `defect_origins` plus
explicit limitations. Relationship kinds: `later_assignment`, `ignored_error`,
`missing_guard`, `boundary_guard` — all `inferred` (hypothesis), never `static`
and never `Supersedes`.

## 4. Candidate admission and ranking

**No change.** Defect origins are recorded in the trace, explanation, and brief,
and are eligible for explanation/next-checks — but they are **not admitted as
candidates** and do not affect the primary, because a source-level defect pattern
is not yet tied to the failing invocation. This is deliberate: the prerequisite
for admission is invocation evidence (runtime probe correlation or a verified
rewrite), not a static pattern. Root-cause gates are untouched.

## 5. Real examples (before/after)

| Case | Before | Now |
|---|---|---|
| go-u008 micromdm (nil panic) | no trace | `missing_guard` hypothesis: operand `pb` is dereferenced with no nil check between producer and failure |
| go-u014 starlight | parameter boundary | `missing_guard` hypothesis for the dereferenced parameter `val` |
| go-u010 starlight | traced producer `toValue` | unchanged producer; no defect pattern |
| synthetic | — | `limit := X; limit = 0; panic(limit)` → `later_assignment`; `v,_ := decode(); panic(v)` → `ignored_error`; `if i <= len(items) { items[i] }` → `boundary_guard` |

## 6. Adversarial tests

Incorrect assignment detected; unrelated assignment not flagged; missing nil
guard detected; present nil guard not flagged; incorrect `<=` boundary detected;
correct `<` comparison not flagged; ignored error detected; branch-dependent
reassignment surfaced as a hypothesis; works without SSA; clean function produces
no hypothesis. (Plus the causal/argument/invocation tests from prior work.)

## 7. Paired evaluation (6 reproducible, development-exposed cases)

Tracing ON vs OFF (`evaluation/results/iteration7-causal-trace-paired.json`):
exact-function **6/6 vs 5/6**; candidate-admission change **0**; misleading
primary **0 vs 1**; root-cause status change **0**; regressions **0**. Defect
origins surfaced on 2/6 cases (`go-u008`, `go-u014`), all as hypotheses; no
primary changed. These six are development-exposed — not independent validation.

## 8. Performance

Within noise (2–6 s per case, indexing-dominated). Defect assessment parses one
function body and walks statements before the failing line; bounded.

## 9. Remaining limitations

- Mechanisms are AST-only for now: no reaching-definition/CFG feasibility
  (Phase 5) or phi-node analysis, so a hypothesis is not proven reachable —
  hence `inferred` and no admission.
- `missing_guard` is pattern-based (selector deref + no nil check); a
  non-nilable struct field dereference could be flagged. Mitigations: it is
  reported only as a hypothesis and never promoted.
- Conversion defects (Phase 2 D) are not yet derived from source; the runtime
  numeric-conversion verifier (`runtime_numeric.go`) remains the only
  conversion mechanism.
- Base/head change linkage for defect operations (Phase 4) is not implemented:
  `CodeDelta`s exist but are not joined to the defect operation line.
- Independent defect-operation ground truth does not exist in the current corpus;
  defect-operation accuracy is therefore not measured, only pattern correctness.

## 10. Next highest-value improvement

**Join defect hypotheses to control-flow feasibility and base/head deltas.**
Concretely: (a) use `SSAAssertionTrace` reaching-definition/dominator data to
confirm that the flagged operation can reach the failing operand before the
failure (upgrading an AST pattern from `inferred` to `static`), and (b) match the
defect operation line against `report.Deltas` so a *changed* operation on an
evidenced path is distinguished from an unchanged one. Prerequisite: SSA coverage
for the enclosing function and a base revision in the run; where absent, the
pattern stays a hypothesis, as today.
