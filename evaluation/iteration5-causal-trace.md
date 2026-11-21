# Backward causal investigation (value/operation tracing)

Adds a deterministic, bounded backward trace from a failure's manifestation
site toward the operation that produced the invalid value or error. It reuses
the existing snapshot, typed-call edges, AST parsing and evidence model; it adds
no LLM and never promotes a root cause.

## Reused infrastructure

- `programrepo.Snapshot` nodes and type-resolved `CALLS` edges
  (`repository/typed_calls.go`: `provenance=verified_static_call`,
  `resolution=go_types`, `call_lines`).
- `observedFrameOrder` (ordered stack-frame evidence) for the anchor.
- `sourceStep`/`sourceExcerpt` for evidence minting.
- Existing candidate/ranking/explanation/handoff tiers (`BuildCandidateReport`,
  `BuildLikelyFunction`, `BuildCausalExplanation`, `InvestigationBrief`).

## New file: `internal/diagnosis/causal_trace.go`

`traceCausalOrigin(workspace, snapshot, anchor, anchorLine, budget) CausalTrace`:

1. Anchor = innermost observed non-test frame.
2. `producerCall` parses the enclosing function and finds the call whose result
   feeds the failing operation: a call assigned to a variable used at the
   failing line (e.g. `v, err := toValue(...)` then `panic(err)`), or a call on
   the failing line.
3. `resolveCallee` resolves the target by the type-resolved edge for that exact
   call line; otherwise a unique same-package function of that name (marked
   `textual_call`, `inferred`).
4. Recurse into the callee's return operation, bounded by depth/edges and
   cycle-guarded; record every edge with relationship, status
   (`static|inferred|unresolved`), source location and evidence.
5. `Producer` = the **nearest** statically-resolved producer (not the deepest);
   deeper nodes remain in `Edges` and are admitted as alternates.
6. `Supersedes` is true only when the anchor line merely propagates the
   producer's value/error (`panic(...)` or `return`) and the first edge is
   type-resolved. Then the producer outranks the manifestation frame.

Edges are minted as `E-causal-*` evidence. Unresolved calls (target not indexed)
are recorded as an explicit boundary, never fabricated.

## Integration

- `pipeline.go` calls `buildFailureOriginTrace` right after
  `buildDeterministicCausalChain`.
- `admitCausalProducer` promotes every static path node to a candidate with
  provenance `causal_producer` (dependency score; nearest producer highest).
- `BuildCandidateReport` ranks a superseding `causal_producer` first, else after
  observed frames. `inferredCandidateKind`, `likelyFunctionEntry`,
  `leadConfidence`, `leadReasonPhrase` and `classifyOperationSupport` know the
  new kind.
- `BuildCausalExplanation` names the producer and adds a producer check when it
  differs from the primary.
- `InvestigationBrief` renders the causal path and the "propagates the
  producer" note.
- Races are excluded (concurrency analysis preserved).

## Demonstrated before/after (production path, real repositories)

| Case | Reviewed culprit | Previous primary | New primary | Culprit admitted | Path | Verified? |
|---|---|---|---|---|---|---|
| starlight (`go-u010`, real repo + CI log) | `convert/conv.go` `toValue` | `GoSlice.Index` (manifestation) | **`toValue`** (`causal_producer`) | yes | static: `GoSlice.Index` → `toValue` (slice.go:109) → `passThroughOrNone` | no |
| mapstructure (`go-u009`) | `RecursiveStructToMapHookFunc` | same | same (no regression) | yes | unresolved (reflect call not indexed) | no |
| synthetic module | producer `Produce` | `Caller` | **`Produce`** | yes | static: `Caller`→`Produce`→`Helper` | no |

In every case the root-cause status is unchanged (`localized` /
`partially_established`); the new tier cannot promote.

## New tests (`internal/diagnosis/causal_trace_test.go`)

Nested calls (nearest, not deepest), cross-package typed edge, same-package
fallback is inferred (never static), unindexed boundary, non-propagation anchor
does not supersede, budget stop, admission+ranking+explanation integration,
race exclusion.

## Limitations

- Cross-package targets are followed only when the target package is indexed;
  otherwise the boundary is reported and nothing is admitted.
- Interface dispatch resolves only when the type-resolved edge exists; no
  speculative dispatch expansion.
- Offline report rebuilds (`cmd/likely-bench`) do not exercise the tracer because
  saved reports do not carry the snapshot/workspace; it runs on the live path.
- The producer is a stronger *lead*, not a verified cause; the verification gate
  is unchanged.
