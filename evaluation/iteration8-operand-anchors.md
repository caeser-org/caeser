# Causal investigation beyond call-based failures

Extends the backward tracer to begin at dereference, comparison, indexing, and
conversion operations, not only at a propagated call result. Reuses existing
infrastructure; no parallel engine; no verification change.

## 1. Existing analysis reused

- `repository` snapshot nodes and type-resolved `CALLS` edges (`go_types`,
  `call_lines`, `verified_static_call`) — unchanged resolver.
- Ordered observed frames (`observedFrameOrder` / pipeline `frames`) for anchors.
- `sourceStep`/`sourceExcerpt` for evidence minting.
- AST parsing (the same `go/parser` approach as `assertion_slice.go`); SSA is not
  required and is not duplicated.
- Existing candidate/ranking/explanation/handoff tiers.

`TraceAssertion` (SSA) and `assertionOperations` (syntactic assertion slice)
already identify assertion operands and typed call targets; the tracer adds a
source-level value-provenance walk where SSA is absent, so it does not re-run or
replace them.

## 2. New operand types supported

The anchor is still the innermost observed frame; the resolver
(`resolveOperandOrigin`) now handles the failing operation's operands for:
nil-pointer dereference (`p.Field`), comparisons (`if got != want`), indexing
(`s[i]`), type conversions (`int64(n)` — conversions are not treated as
producers), values returned from calls, and variables assigned earlier.

## 3. Value-provenance relationships implemented

| Relationship | Handling |
|---|---|
| `x := f(...)` / `x, err := f(...)` | call producer (existing) |
| `x := y` | alias chain (bounded, cycle-guarded) |
| `var x T` | **local zero value** (nil/zero origin, no producer function) |
| function parameter / receiver | **parameter boundary** (origin is the caller) |
| `return f(x)` / nested `f(g())` | direct call producer; conversions skipped |
| multiple assignments to one variable | **ambiguous** — no producer chosen |
| multiple operands with distinct origins | **ambiguous** |
| parameters among defined operands | parameters de-prioritised so a `testing.T` argument does not make every assertion ambiguous |

Edges keep the existing structure: `relationship` (`verified_static_call`,
`textual_call`, `local_zero_value`, `parameter`, `ambiguous`, `unindexed`),
`status` (`static|inferred|unresolved`), source location, evidence IDs; cycle
guards and depth/edge budgets are unchanged. A producer is admitted only for a
type-resolved edge; parameters and zero values never yield a producer function,
and nothing is promoted merely for being deeper.

## 4. New causal paths demonstrated

Real repositories (production path, tracing on):

| Case | Previously | Now |
|---|---|---|
| go-u008 micromdm (nil deref) | no trace | `partial`: operand `pb` → `local_zero_value` at `checkin_event.go:153` (declared zero value reaching the dereference) |
| go-u011 cli (nil deref) | no trace | `unresolved`: operand `newkey` → `parameter` boundary |
| go-u014 starlight | no trace | `unresolved`: operand `val` → `parameter` boundary |
| go-u010 starlight | `traced` | unchanged: producer `toValue` |

Unit-tested new paths: nil deref from parameter, nil deref from a local zero
value, comparison-operand producer, index-operand producer, conversion-operand
producer, alias chain, and works-without-SSA.

## 5. Before/after paired results (tracing on vs off)

`evaluation/results/iteration7-causal-trace-paired.json` (6 reproducible
panic/anomaly cases — **development-exposed, not independent validation**):

| Metric | tracing OFF | tracing ON |
|---|---|---|
| exact-function correct | 5/6 | 6/6 |
| top-3 recall | 5/6 | 6/6 |
| misleading primary | 1 | 0 |
| root-cause status changed | — | 0 |
| regressions | — | 0 |
| cases with any causal trace | n/a | 4/6 (u008, u010, u011, u014) |

The primary changed on exactly one case (go-u010, correct); the operand
extension added causal traces to three more cases **without changing any
primary** — investigation depth, not additional suspects.

## 6. Ambiguities correctly rejected

Multiple producers; branch-dependent assignment (`var x; if c { x=A() } else { x=B() }`);
ambiguous same-package name; inferred edge not extended; unrelated/discarded
call result; wrapper pass-through not promoted; correct callee with an invalid
argument stays an unverified hypothesis.

## 7. Runtime overhead

End-to-end unchanged within noise (2–7 s per case, dominated by `go/packages`).
The resolver parses one function body per hop, `maxDepth=4`, `maxEdges=8`,
cycle-guarded.

## 8. Remaining limitations

- **Argument provenance across calls**: a parameter origin is reported as a
  boundary and not traced into the caller's argument (so a nil passed in from a
  caller is not followed to its producer). This is the highest-value next step.
- **No-frame assertion anchors**: an assertion in the test with no observed
  frame still does not start a trace (candidate-level `assertionOperations`
  continues to cover it).
- **Unindexed/dynamic targets**: reflect and non-indexed cross-package callees
  stop the trace (`unresolved`).
- Conversion detection uses predeclared-type names; user type conversions are
  treated as calls and may stop at an unindexed boundary.
