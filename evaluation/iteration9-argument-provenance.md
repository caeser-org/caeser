# Cross-function argument provenance

Extends the backward tracer so a parameter used by a failing operation is mapped
to its argument at a caller call site and resolved there, instead of stopping at
the parameter. Reuses the existing snapshot and typed-call evidence; no parallel
engine; no verification change.

## 1. Infrastructure reused

- Type-resolved `CALLS` edges (`go_types`/`call_lines`/`verified_static_call`) and
  a new `edgesByTarget` index over the same edges (no new analysis).
- Snapshot function metadata (`ProgramNode`), AST parsing, `sourceStep` evidence,
  cycle guards, depth/edge budgets, candidate admission rules.
- `fnOrigins` (refactored from the operand resolver) so operand resolution and
  argument resolution share one local value-provenance implementation.

`resolveCallee` now also matches the target function name, not only the call
line, so two calls on one line cannot resolve to the wrong callee (a real bug
found by the nested-argument test).

## 2. Parameter-to-argument tracing

When the failing operand is a parameter of function `B`:
1. `parameterCallerSite` selects the caller: the call site the trace traversed
   from, or the unique statically-resolved caller of `B` from `edgesByTarget`
   (`callerSites`).
2. `argumentOrigin` locates the call at that line, maps the parameter by index
   (receiver parameters map to the selector receiver; `call.Args` excludes the
   receiver), and classifies the argument expression.
3. The argument's local provenance is resolved in the caller with the same
   `fnOrigins` (call producer, alias chain, local zero value, caller parameter,
   nil literal, nested call).

Ordinary calls, methods with receivers, and nested-call arguments are supported;
a variadic argument span that cannot be mapped one-to-one is reported ambiguous.

## 3. Causal evidence strength

- Caller-derived edges use relationship `caller_argument` and status
  **`inferred`** — a statically possible caller, not an observed invocation.
- A caller-derived producer is **never** assigned to `Producer` and never sets
  `Supersedes`; the observed failure frame keeps the lead and is not admitted as
  a candidate. `unindexed` caller producers stay unresolved boundaries.
- An inferred/traversed caller is not treated as verified causality.

## 4. Ambiguity handling (reported, never resolved by source order)

Multiple plausible callers; multiple call sites on one line; variadic span;
missing or unmappable argument; caller producer outside the indexed graph;
recursion (cycle guard) and depth/edge budget exhaustion.

## 5. Integration

All changes are in `causal_trace.go`: `fnOrigins`/`buildFnOrigins`,
`argumentOrigin`, `calleeParams`, `classifyArgument`, `callerSites`,
`parameterCallerSite`, plus the loop's caller-context and parameter branch.
Existing edge statuses, evidence IDs, cycle detection, deterministic ordering,
budgets, candidate admission and verification gates are preserved.

## 6. Adversarial tests (`causal_trace_test.go`)

One unambiguous caller (argument producer resolved, edge `inferred`, no
supersede); caller passed `nil`; caller-local zero value; multiple callers →
ambiguous; method receiver; variadic → ambiguous; nested call argument;
recursion bounded; plus the prior deref/comparison/index/conversion/zero/branch
tests.

## 7. Paired evaluation (6 reproducible cases, development-exposed)

`evaluation/results/iteration7-causal-trace-paired.json`:

| Metric | tracing OFF | tracing ON |
|---|---|---|
| exact-function correct | 5/6 | 6/6 |
| misleading primary | 1 | 0 |
| root-cause status changed | — | 0 |
| regressions | — | 0 |

No primary changed in this iteration. The parameter boundary improved from a
generic stop to a caller-aware result: `go-u011` and `go-u014` now report
"multiple plausible callers; no caller selected" instead of an opaque parameter
boundary. No suspect was added.

## 8. Runtime

Within noise (2–8 s per case, indexing-dominated).

## 9. Remaining limitations

- Caller provenance is one hop: the caller's argument is resolved locally but
  not chained further up through the caller's own parameters.
- Caller-derived producers are recorded as causal paths/checks, not admitted as
  candidates (conservative).
- Interface dispatch without a concrete typed edge and unindexed callers remain
  unresolved boundaries.
- Argument flow uses AST provenance, not SSA; SSA operand flow is not consulted
  for cross-function arguments.
