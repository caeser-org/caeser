# Reproducible causal diagnosis evaluation

Goal: make Ceaser's production diagnosis measurable across cases, then measure
whether backward causal tracing improves correctness. No verification gate was
weakened and no ground truth was changed.

## Phase 1 — Reproducible fixtures (delivered)

`internal/evalcorpus` now preserves and verifies the exact failure inputs:

- `Metadata.InjectedTests []InjectedTest{Path, SHA256}` + `Provenance`,
  `GoVersion`, `Environment` (`types.go`).
- `VerifyFixture` hashes each preserved test under `cases/<id>/injected/<path>`
  and refuses a changed, missing, or path-traversing fixture; it requires
  provenance (`fixture.go`).
- `ApplyInjectedTests` copies the preserved tests into a checkout, re-verifying
  each digest; it never derives a test from an upstream revision.
- `Load` calls `VerifyFixture`, so integrity is checked before every evaluation.
- `go run ./cmd/corpus-score --verify` verifies the whole corpus (19 cases pass).
- Schema + protocol documented in
  `evaluation/corpus/cases/go/GO_COLLECTION.md` and the case template.
- Tests: `internal/evalcorpus/fixture_test.go` (verify, tamper rejection,
  provenance required, traversal rejected, `Load` fails on tamper, apply).

## Phase 2 — Fresh independent corpus (not possible in this environment)

**New independent cases collected: 0.** The machine is at **100% disk**
(~1.1 GB free; the Go build cache alone is 902 MB, regenerable). Cloning an
unfamiliar repository plus its module cache does not fit, and this iteration
must not delete unrelated user data. Per the constraints, the honest sample size
is reported rather than approximated.

The collection path is ready and unblocked for anyone with capacity:

1. Pick an unused repo and a real failure-inducing change (base fails, fix
   passes), with an assertion, panic, conversion, cross-package, nil, or
   deeper-helper failure.
2. Preserve the exact injected/regression test(s) under `injected/` and record
   `injected_tests` + `provenance`.
3. Record repo, base/head SHAs, command, environment, `log_sha256`, reviewed
   culprit.
4. `go run ./cmd/corpus-score --verify`.

No synthetic fixture is counted as independent validation, and no case is
reported as evaluated unless it ran.

## Phase 3 — Complete production engine, paired (tracing on/off)

Runner: `evaluation/scripts/causal-trace-eval.sh` clones each case at its frozen
base revision and runs the **pinned production engine** twice — tracing on and
`CAESER_NO_CAUSAL_TRACE=1` — on the same revision and original CI log. The
ablation is a config/env switch only; it does not alter the algorithm.

**Reproducible cases run: 6** (frozen panic/anomaly cases whose tracer anchor is
an implementation frame present in the base source, so the lost injected test is
not required). Excluded: 10 assertion cases (fixture not replayable). These 6
were also used while building the tracer, so they are **development-exposed, not
independent validation**.

| id | repo | reviewed culprit | tracing ON | tracing OFF | trace | hit on/off |
|---|---|---|---|---|---|---|
| go-u005 | node_exporter | `textFileCollector.Update` | Update | Update | none (deref) | ✓/✓ |
| go-u008 | micromdm | `UnmarshalCheckinEvent` | same | same | none (deref) | ✓/✓ |
| go-u009 | mapstructure | `RecursiveStructToMapHookFunc` | same | same | unresolved | ✓/✓ |
| go-u010 | starlight | `toValue` | **toValue** | `GoSlice.Index` | traced (static) | ✓/✗ |
| go-u011 | cli | `addParamsSlice` | same | same | unresolved | ✓/✓ |
| go-u014 | starlight | `toValue` | same | same | unresolved | ✓/✓ |

Aggregate: exact-function **6/6 on vs 5/6 off**; top-3 recall **6/6 vs 5/6**;
fixed **1** (go-u010); regressed **0**; root-cause status changed **0**; all
non-u010 cases identical (the tracer did not supersede). Mean run 2.8 s on /
2.0 s off (indexing-dominated; the tracer adds a small constant). Peak RSS on
the starlight run ≈357 MB, attributed to `go/packages`, not the tracer.

Tracer stopping reasons observed: `none` (deref/anomaly with no value-producing
call — u005, u008), `unresolved` (callee not indexed / reflect — u009, u011,
u014), `traced` (statically resolved producer — u010).

## Phase 4 — Failure analysis

Only one evaluated case differed between on/off, so the miss classification is
small but exact:

| Case | Blocker for the OFF run |
|---|---|
| go-u010 | **ranking/anchoring**: without tracing the primary is the manifestation frame (`GoSlice.Index`); the tracer resolves the value producer (`toValue`) — fixed |

For the 10 excluded assertion cases the blocker is **fixture not replayable**
(lost injected test overlay), not an engine limitation.

Engine-level limitations observed in the tracer's stopping reasons:

| Blocker | Cases |
|---|---|
| No traceable value-producing call (panic at a dereference) | u005, u008 |
| Unindexed / dynamic target (reflect, cross-package) | u009, u011, u014 |
| Ranking/anchor resolved correctly | u010 |

## Bottleneck ranking (for the next investment)

1. **Fixture reproducibility infra is now in place; the corpus itself is empty
   of new independent cases.** Highest value: collect and preserve new cases
   (needs disk/CI capacity, not code).
2. **Unindexed/dynamic targets** stop the trace (3 of 6). Cross-package indexing
   is the next tracer-side investment, but only *after* it can be measured.
3. **Anchor absence** at dereference/assertion sites (2 of 6): no value-producing
   call to follow.
4. **Argument provenance** (correct callee with invalid argument) — mitigated
   (pass-through guard, ambiguity, no promotion) but not solved.
5. **Ranking/ties** — last; only matters once 1–3 are addressed.

## Honest result

On the 6 runnable, development-exposed cases the tracer produces one correct,
low-noise change (go-u010) with zero regressions and no root-cause promotion.
That is **not evidence of broad generalization**; it is evidence the tracer is
safe and can be measured. Broad evaluation requires new, preserved, unfamiliar
cases, which this environment cannot host.

## Deliverables

- Fixture infra + verification: `internal/evalcorpus/{fixture.go,types.go}`,
  `cmd/corpus-score --verify`, tests.
- Collection protocol: `evaluation/corpus/cases/go/GO_COLLECTION.md` + template.
- Paired runner: `evaluation/scripts/causal-trace-eval.sh`.
- Paired results: `evaluation/results/iteration7-causal-trace-paired.json`.
- Ablation toggle: `diagnosis.Config.NoCausalTrace` / `CAESER_NO_CAUSAL_TRACE`.
