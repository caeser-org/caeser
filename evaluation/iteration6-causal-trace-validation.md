# Backward causal tracing — soundness and reproducibility validation

Scope: validate the backward tracer before expanding it. No new tracing
subsystems; only correctness fixes, tests, and evaluation infrastructure. No
verification gate was weakened.

## 1. Soundness review and fixes (`internal/diagnosis/causal_trace.go`)

| Issue found | Risk | Fix |
|---|---|---|
| The trace recursed through a same-package **inferred** edge and could set a "static" producer downstream | inferred structure presented as evidence | stop at an inferred edge with a limitation; never extend past it |
| Multiple assignments fed the failing line; the closest was chosen silently | arbitrary producer | return the candidate list; the tracer emits an `ambiguous` boundary and chooses no producer |
| The callee's failing operation was assumed to be the **first** return | wrong branch chosen silently | count returns; record "multiple return paths; branch feasibility unverified" |
| `Supersedes` treated any `return` as pure propagation | a pass-through wrapper could be promoted over the anchor | `producerIsPassThrough` (all returns are a bare parameter) blocks promotion |
| `resolveCallee` accepted a typed edge with empty `call_lines` | wrong call matched | require `call_lines` present and containing the call line |
| Anchor came only from persisted incident/evidence frames | live frames weren't available (u005/u008/u014 not anchored) | pass the pipeline's `frames` explicitly, with the evidence fallback |

`trace.Status` now distinguishes `traced` (static producer, no inferred/unresolved
edge), `partial` (inferred hop or static producer with a boundary), and
`unresolved` (unresolved boundary only).

## 2. Can the frozen/holdout fixtures be reconstructed without changing truth?

**Partially, and only for panic/anomaly cases.**

- Each corpus/frozen case was produced by injecting a **comment-redacted** copy
  of the PR's regression test onto the buggy base. Those exact files are not
  preserved in the corpus. Re-applying the upstream test misaligns failure-log
  line numbers (verified: `gorilla/mux` PR #748 reports `mux_test.go:2771`, the
  upstream test is at line 2805), so **assertion** cases cannot be reproduced
  faithfully offline.
- **Panic/anomaly** cases reproduce without the injected test: the tracer anchors
  on the implementation frame in the original stack (e.g. `convert/slice.go:111`),
  which exists in the base source. Ground truth and revisions are unchanged; the
  engine receives only repo, revision and the original CI log.

No fixture was fabricated and no expected label was altered.

## 3. Reproducible evaluation path

`evaluation/scripts/causal-trace-eval.sh` + `evaluation/causal-trace-manifest.csv`:
clone each repository at its frozen base revision, run the pinned production
engine (`caeser diagnose --repo … --ci-log … --artifact-first --json`), and
record primary, kind, candidate entries and the causal trace. Results:
`evaluation/results/iteration6-causal-trace.json`. The manifest pins repo,
revision, log path and reviewed culprit.

Reproducible set (panic/anomaly): **6 cases**. Excluded: **10 assertion cases**
(missing injected tests). Not attempted: holdups require the same missing tests.

## 4. Before/after (production path, real repositories)

| Case | Category | Reviewed culprit | Before primary | After primary | Trace | Hit |
|---|---|---|---|---|---|---|
| go-u005 node_exporter | panic | `textFileCollector.Update` | Update | Update | none (deref, no call) | yes |
| go-u008 micromdm | nil panic | `UnmarshalCheckinEvent` | UnmarshalCheckinEvent | same | none (deref) | yes |
| go-u009 mapstructure | panic | `RecursiveStructToMapHookFunc` | same | same | unresolved (reflect) | yes |
| go-u010 starlight | panic | `toValue` | `GoSlice.Index` | **`toValue`** | traced (static) | yes |
| go-u011 cli | nil panic | `addParamsSlice` | same | same | unresolved | yes |
| go-u014 starlight | anomaly | `toValue` | toValue | same | unresolved | yes |

Exact-function accuracy: **5/6 → 6/6**. Exactly **one** case changed (go-u010,
now correct); **zero regressions**. Root-cause status unchanged everywhere
(`localized`/`partially_established`); `operation_support` for go-u010 is
`evidence_supported`; no false verified claims.

## 5. Does tracing improve correctness or just add candidates?

For go-u010 it changes the primary to the reviewed culprit and retains the
previous primary (`GoSlice.Index`) as an observed-stack alternate. It added one
pass-through helper (`passThroughOrNone`) as a candidate; that was ranked too
high initially and is now demoted below observed frames by only promoting the
**nearest** producer. Net effect on this set: a correct change, +0 misleading
primaries, +1 low-ranked alternate.

## 6. Adversarial tests (`causal_trace_test.go`)

Deepest-not-defect; wrapper pass-through not promoted; correct callee with an
invalid argument stays an unverified hypothesis; multiple producers →
`ambiguous`, no producer; ambiguous same-package name not followed; unrelated
call ignored; inferred edge not extended; cross-package typed edge followed;
unindexed boundary; budget stop; race exclusion (integration).

## 7. Profiling

End-to-end (dominated by `go/packages` indexing, not the tracer): starlight
1–2 s, mapstructure 1 s, micromdm 2 s, node_exporter 4 s, cli 3–43 s (cold vs
warm module cache). Peak RSS ≈ 357 MB on starlight. The tracer itself parses at
most one function body per hop with `maxDepth=4`, `maxEdges=8`, cycle-guarded;
no measurable fraction of the run. SSA/indexing budget exhaustion is unchanged
and still reported (`coverage_notes`).

## 8. Remaining bottlenecks (ranked)

1. **Assertion-case reproducibility** — the missing injected tests block 10/16
   frozen cases and all holdouts from real re-evaluation.
2. **Anchor selection** — the tracer only fires when an implementation frame is
   on the stack; assertion/panic-at-deref cases (u005, u008) have no
   value-producing call to follow.
3. **Cross-package / unindexed targets** — the producer path stops when the
   callee package is not indexed (u009 reflect; several others).
4. **Argument provenance** — a correct callee receiving an invalid argument is
   still a mis-attribution risk (mitigated, not solved).
5. **Ranking/ties** — unchanged; only matters once 1–4 are addressed.

## 9. Next highest-value change

**Not** deeper tracing or cross-package indexing yet — both are blocked by the
inability to measure them. The highest-value change is infrastructure:

> Preserve each case's **injected regression-test overlay and a content digest**
> in the corpus bundle at generation time, so the production pipeline (snapshot,
> SSA, tracer, verification) can be re-run offline for all 16 frozen cases and
> the holdouts.

With that, cross-package indexing becomes measurable; without it, any further
tracing change cannot be validated beyond the 6 panic cases and unit tests.

## 10. Independent-evaluation verdict

Independent, real-repository evaluation is **possible only for the 6 panic/
anomaly cases**; it is not possible for the 10 assertion cases or the holdouts
because their exact regression-test overlays are lost. The smallest
infrastructure change to make full independent evaluation possible is stated in
§9. On the reproducible subset the tracer is a **correct, low-noise, single-case
improvement** (go-u010), not a broad gain, and that is the honest result.
