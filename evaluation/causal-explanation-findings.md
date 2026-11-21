# Causal-explanation findings (frozen 16 cases + curated demos)

This documents a presentation-and-diagnosis-quality change: a deterministic
**causal hypothesis** tier that connects the observed failure semantics to the
primary suspect using evidence that already exists in the report. It adds no new
analysis stage, no model call, no relationships, and no verification power.

## What changed

`internal/diagnosis/causal_explanation.go` derives, per report:

- `failure` — directly observed semantics (`panic`, composed assertion message,
  `expected`/`actual`, test and subtest) read with the canonical Go test parser
  (`goTestFailures`) and the existing panic/assertion regexes. Observation only.
- `hypothesis` — one sentence tying the concrete operation in the primary
  suspect to the failure text, labelled unverified.
- `unresolved` — the specific causal question that remains.
- `checks` — evidence-specific steps (`target`, `evidence_needed`, `confirms_if`,
  `rejects_if`) derived from that question. Generic reproduction steps are only
  used when no suspect operation exists at all.

The concrete operation is chosen from evidence already in the report, in this
order: a typed SSA value on the failed assertion's path that lives in the suspect
function; then a parsed source-excerpt line whose call matches a qualified
operation named in the failure text; then the first call/value-producing line at
or after the suspect. The token match is a textual correlation between two
observed artifacts (the CI message and the checked-out source), not a data-flow
claim.

## Guardrails (unchanged)

- No project, function, panic-string, or test name is hardcoded; the module
  contains no repository identifiers.
- `BuildCausalExplanation` is read-only: it appends no evidence, adds no
  relationships, and cannot change classification. A regression test asserts the
  evidence count is unchanged and that `assessRootCause` stays non-established.
- A refused diagnosis yields `status: unavailable` with no hypothesis.
- No suspect yields `status: unavailable` with a typed reason (abstention kept).
- `root_cause`, confidence, verification, promotion, and the candidate/likely
  tiers are untouched; ranking is byte-for-byte unchanged.

## Frozen-16 result (ranking stage held fixed)

`go run ./cmd/likely-bench --artifacts evaluation/probe-rerun/artifacts --out /tmp/causal.json`

| Metric | Value |
| --- | ---: |
| hypotheses produced | 14/16 |
| typed abstentions (no function admitted) | 2/16 (`go-u004`, `go-u015`) |
| hypotheses citing a concrete source operation | 12/16 |
| hypotheses citing an SSA operation | 3/16 |
| checks per hypothesis | 1–2 |

Primary suspects are unchanged from the frozen ranking baseline (this change
does not touch ranking). The improvement is in explanation and next-step
specificity, not localization.

Representative cases:

- `go-u009` (mapstructure): panic `reflect: call of reflect.Value.Interface on
  zero Value` is extracted; the hypothesis names the suspect and the concrete
  source operation to examine, and leaves the violated invariant unresolved.
- `go-u010` (starlight): suspect `GoSlice.Index`, but the cited operation is the
  call `toValue(g.v.Index(i), g.tag)` — the reviewed deeper helper — so the check
  points at the useful function even when the primary is the panic site.
- `go-u002` (mcp-go): SSA operation `convert int <- float64` in `GetInt`, with an
  SSA-specific feasibility check.

## Causal gaps resolved

- Reports no longer stop at "Observed stack frame": the primary suspect now
  carries an evidence-grounded explanation and a concrete operation.
- Generic "run the smallest failing test" advice is replaced by checks that name
  the operation, the evidence needed, and the confirm/reject condition.
- Failure semantics (panic/assertion/expected/actual/subtest) are surfaced from
  evidence that already existed but was discarded by the presentation.

## Causal gaps that remain

- `go-u004`, `go-u015`: the culprit function is not admitted to the candidate
  tier (mechanism C, analysis scope), so no hypothesis is offered. Unchanged.
- `go-u007`, `go-u016`: the primary suspect is a deeper/other function; the
  explanation is still a hypothesis and the check is specific, but the primary is
  wrong (a ranking issue, mechanism A, not an explanation issue).
- The violated invariant is never *established* — only hypothesised — because the
  data needed to verify it (runtime value, branch feasibility) is not in the
  original CI evidence. That remains the correct, honest limit.

## Negative test

`causal_explanation_test.go` includes a case where the stack-top frame is a
shallow wrapper and the operand-correlated function is the true lead; the
explanation follows the ranked suspect, is labelled a hypothesis, and never
claims a root cause.

## Ranking-reliability update (suspect correctness)

A follow-up targeted the two wrong primaries (`go-u007`, `go-u016`) and the two
missing culprits (`go-u004`, `go-u015`). Precise mechanisms:

- **go-u016** — the reviewed culprit `api_hash.go NullsafeHashcode128` was
  admitted with provenance `inferred_caller` (a type-resolved `verified_static_call`
  edge) but carried the *same* behavioural score as 16 textual `inferred_callee`
  fan-out entries, so the tie broke on saved candidate order and the culprit
  landed fourth. The report already contained the discriminator:
  `execution_paths` has `TestHashCodesUnknownTemporal → NullsafeHashcode128`, the
  only non-test target.
- **go-u015** — the reviewed culprit `reporter_properties.go
  CreatePosthogPropertiesForSettings` was admitted at rank 2 but shared identical
  behavioural evidence with its sibling `CreatePosthogPropertiesFor*` functions,
  because `Generate` adds every `Test` node whose *file path* appears in the
  failure text and then expands all of their callees. The failing test's own
  verified path `TestCreatePosthogPropertiesForSettingsValues →
  CreatePosthogPropertiesForSettings` is the only discriminator, and it was again
  unused.
- **go-u007** — `distinct.go probeTable.hashCodeForRow` was never admitted: the
  failing test has no type-resolved call path and the candidate set is a 24-entry
  textual interface fan-out of `*.TryExecute`. Localization ranks `distinct.go`
  first, but no function-level evidence admits the culprit.
- **go-u004** — `config/config.go Config.Clone` is in a package outside the
  indexed graph for a `./internal/analytics` run; no execution path or evidence
  names it.

**Generalizable fix (shipped).** `BuildCandidateReport` now orders existing
candidates by evidence strength: directly observed execution evidence first, then
functions on the failing test's type-resolved call path (`ExecutionPaths`, built
only from `verified_static_call` edges), then the remaining inferred fan-out.
This reuses existing, evidence-cited data; it adds no candidates, changes no
score or kind, and does not touch confidence, verification, or promotion.

Frozen-16 result (ranking stage, `--rebuild-candidate-report`):

| Metric | Before | After |
| --- | ---: | ---: |
| fn_top1 (reviewed culprit) | 8 | **10** |
| misleading primary | 8 | **6** |
| candidate recall (top-5 entries) | 12 | 12 |
| top-3 recall | 10 | 11 |
| coverage | 16 | 16 |

Only `go-u004`, `go-u015`, `go-u016` changed primary; `go-u015` and `go-u016`
are now correct, and `go-u004` moved between two functions that are both not the
culprit (the culprit is out of scope).

**Explanation reliability.** `CausalExplanation` now records
`operation_support`, separating an operation directly supported by evidence
(typed SSA value, or a function on the failing test's verified call path) from
one that is merely `source_correlated` / `source_associated`, and from
`requires_investigation` when no concrete operation is isolated. On the frozen 16:
8 `evidence_supported`, 5 `source_associated`, 3 `requires_investigation`. The
tier still cannot change confidence or promotion (regression-tested).

**Remaining unsolved.** `go-u007` and `go-u004` are admission/scope limits, not
ranking: they need file-localized function admission (mechanism A) and broader
indexing scope (mechanism C) respectively, neither justified by function-level
evidence in the current reports.

