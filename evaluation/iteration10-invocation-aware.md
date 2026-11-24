# Invocation-aware causal investigation

Connects the backward tracer to execution evidence so a parameter is resolved to
the caller that actually ran (when the observed stack establishes it), and
extends parameter→argument tracing across bounded hops. Reuses the existing
snapshot, typed-call edges, observed frames, and budgets; no parallel engine; no
verification change.

## 1. Evidence reused

- Ordered observed stack frames (`observedFrameOrder` / pipeline `frames`).
- `observedFunctionChain` / `observedCallerOf`: the observed frames collapsed to
  the application functions that executed, innermost first — invocation evidence.
- Type-resolved `CALLS` edges with `call_lines` and a `edgesByTarget` index.
- `fnOrigins`/`argumentOrigin` (argument provenance) and `resolveCallee`, cycle
  guards, depth/edge budgets.

**Observed vs possible:** the observed frame chain is evidence a function ran;
a `verified_static_call` edge alone is not. The two are kept distinct.

## 2. Caller disambiguation (`parameterCallerSite`)

Preference order:
1. the traversed call site (downward recursion),
2. a **static caller whose source matches the caller observed on the stack** —
   only accepted when exactly one static site matches; zero matches is a
   boundary, several matches is ambiguous,
3. a unique statically-resolved caller when there is no observed caller.

A caller is never selected by graph proximity, source order, or ranking. When the
observed caller is unknown or does not appear among the static sites, the
ambiguity is preserved and reported.

## 3. Bounded multi-hop provenance (`resolveParameterUpward`)

From a parameter at the failing operation, the argument at the evidenced caller
is resolved and, when the argument is itself the caller's parameter, the walk
continues upward through the observed chain (pass-through wrappers), bounded by
`causalTraceMaxUpHops = 3`, the edge budget, and a cycle guard. Argument kinds:
call producer, nested call, alias/local assignment, local zero value, nil
literal, caller parameter, constant, unresolved.

Every upward edge is status **`inferred`** ("statically possible, not observed");
a longer path is never stronger evidence.

## 4. Defect-origin differentiation

The trace distinguishes, via edge relationships: observed failure frame →
immediate operand (`local_zero_value`, `parameter`, `ambiguous`) →
value-producing call (`verified_static_call`/`caller_argument`) → propagating
wrapper (`producerIsPassThrough` stops promotion) → potential defect producer
(the nearest statically-resolved producer). Caller-side producers are recorded as
hypotheses only; they never become `Producer` and never set `Supersedes`.

## 5. Candidate admission

Unchanged and conservative: a caller-derived producer (relationship
`caller_argument`, status `inferred`) is not admitted as a candidate and does not
change the primary. Observed frames keep the lead; unindexed caller producers
stay unresolved. Verification gates and evidence provenance are preserved.

## 6. Adversarial validation (all passing)

Multiple static callers, only one observed → the observed caller is selected;
multiple callers with no observed evidence → ambiguous; observed caller absent
from static sites → boundary (no selection); misleading source-order proximity
(A1 first, A2 observed) → A2 selected; wrapper chain across two hops → producer
reached, all edges inferred; correct callee with invalid argument, recursion, and
budget exhaustion (from prior iterations) remain hypotheses/bounded.

## 7. Paired evaluation (6 reproducible, development-exposed cases)

`evaluation/results/iteration7-causal-trace-paired.json`, tracing OFF vs ON:

| Metric | OFF | ON |
|---|---|---|
| exact-function correct | 5/6 | 6/6 |
| misleading primary | 1 | 0 |
| candidate admission change | — | 0 |
| root-cause status change | — | 0 |
| regressions | — | 0 |

No primary changed in this iteration. `go-u011`/`go-u014` continue to report
"multiple plausible callers; no caller selected" because the observed frame chain
does not establish their caller — an honest boundary, not a guess.

## 8. Performance

Within noise (1–3 s per case); the upward walk is bounded by 3 hops and the edge
budget.

## 9. Remaining blockers

- The observed chain depends on frames being parsed and resolving to indexed
  symbols; inlined or missing frames leave the caller unestablished.
- Interface dispatch and unindexed callers remain unresolved boundaries.
- Caller-side producers are intentionally not admitted; admitting them safely
  needs stronger invocation evidence than a static call site.
- Argument flow uses AST provenance, not SSA.

**Explicit limitation:** when the observed execution evidence does not identify
the caller (missing/inlined frames, no matching static site, or several
candidates), Ceaser reports the ambiguity and what would resolve it (the failing
invocation's caller and call site) rather than inventing a caller.
