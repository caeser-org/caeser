# Iteration 3 — remaining incorrect primary audit

Scope: the six incorrect primaries in the frozen 16-case set after iteration 2.
Reviewed culprit labels are from `evaluation/go-probe-preflight.md`. Each case
below answers: admitted? primary evidence? culprit evidence? error origin?
distinguishing signal available?

## Failure-mechanism table

| Case | Reviewed culprit | Selected primary | Culprit admitted? | Primary's evidence | Culprit's evidence | Origin | Distinguishable from existing evidence? |
|---|---|---|---|---|---|---|---|
| go-u003 | `api-put-object.go` `Client.PutObject` | `Client.PutObjectFanOut` | no (named in causal chain only) | failing test's direct typed callee (verified path) | causal-chain `assertion_dependency` at `api-put-object.go:332`; no SSA/edge admitting it | admission | no — four sibling assertion-dependency ops (FanOut, AppendObject, PutObject, New); no dataflow to pick one |
| go-u004 | `config/config.go` `Config.Clone` | `…CreatePosthogPropertiesForProduct` | no | failing test's typed callee, in-package | none: `config` is referenced only as a parameter type; no node or call edge to `Clone` | indexing scope | no — package not indexed |
| go-u007 | `distinct.go` `probeTable.hashCodeForRow` | `mstream.go` `MStream.TryExecute` | no | textual `TryExecute` fan-out (24 siblings) | causal chain names `Distinct.TryExecute` (`distinct.go`); helper is a deeper callee | admission depth | partially — same file/class, but the exact helper needs a deeper verified-call hop |
| go-u009 | `decode_hooks.go` `RecursiveStructToMapHookFunc` | `mapstructure.go` `Decoder.decode` | yes (exec) | all 7 enclosing frames scored `execution=1`, tie broken by saved order | innermost panic frame `decode_hooks.go:383` | ranking | **yes** — ordered stack frames (innermost first) |
| go-u010 | `convert/conv.go` `toValue` | `convert/slice.go` `GoSlice.Index` | no | innermost observed panic frame `slice.go:111` | `slice.go:109` calls `toValue(...)`; error panicked at 111; no SSA, no edge to `toValue` | insufficient evidence / fix-location vs panic-site | no — `toValue` is not on the stack and no verified path reaches it |
| go-u012 | `orca/server_metrics.go` setters (`SetCPU…`) | `NewServerMetricsRecorder` | no | only candidate from the test's textual edges | reviewed shared mechanism; setters absent and `relationships` has 0 CALLS edges | collection/admission | no — the call edges needed to reach the setters are absent from the report |

## Possible generalizable signals, evaluated

1. **Observed stack-frame order (innermost first).** The failure happened at the
   deepest reported frame. `go-u009` had seven tied `execution=1` candidates and
   the ordered frame evidence (`RecursiveStructToMapHookFunc` at `decode_hooks.go:383`)
   was ignored. **Shipped** — pure reuse of ordered execution evidence.
2. **Causal-chain assertion-dependency admission.** The deterministic causal
   chain is source-verified and already names culprits for `go-u003`/`go-u007`.
   But for `go-u003` it names four sibling operations with no dataflow to choose
   between them, and for `go-u007` it names the operator, not the helper.
   Admitting them cannot uniquely recover a correct primary and would add
   indistinguishable leads. **Not shipped** (no measured correctness gain;
   would trade precision for coverage, contrary to the mandate).
3. **File-localized function sweep (mechanism A).** Would add every function in
   `distinct.go`; explicitly disallowed without function-level evidence and
   bounded expansion rules. **Not shipped.**
4. **Cross-package import indexing (mechanism C).** No evidence names `Clone`;
   the type appears only in signatures. **Not shipped** — documented limitation.
5. **Source-operation callee (`go-u010`).** `GoSlice.Index` panics on the error
   returned by `toValue(...)` at the immediately preceding line. This is a real
   signal but resolving `toValue` to a function needs the call graph (absent) or
   a same-package symbol lookup that is not exposed at ranking time. **Not
   shipped**; documented as a follow-up.

## Shipped change

`BuildCandidateReport` now orders candidates within observed execution evidence
by the innermost reported frame they enclose (structured incident locations
preferred, else ordered `execution_stack_frame` evidence). This is bounded
(`O(candidates × frames)`), deterministic, evidence-cited, and adds no
candidates.

## Frozen-16 result (engine path, `--rebuild-candidate-report`)

| Metric | iter 2 | iter 3 |
| --- | ---: | ---: |
| primary suspect (fn_top1) | 10/16 | **11/16** |
| misleading primary | 6 | **5** |
| top-3 candidate recall | 11/16 | **12/16** |
| top-5 candidate recall | 12/16 | 12/16 |
| unsupported causal claims | 0 | 0 |
| root-cause promotion regressions | 0 | 0 |

Only `go-u009` changed primary (now the reviewed culprit).

## Independent holdout result

The `eval/unseen-prep` holdout (`evaluation/unseen`, 16 independently prepared
bundles with separately reviewed labels and reports from the frozen baseline
engine) was evaluated after the change and was **not** used to tune it:

| Metric | original engine | iterations 2+3 |
| --- | ---: | ---: |
| primary suspect (fn_top1) | 7/16 | **10/16** |
| misleading primary | 9 | **6** |
| top-3 candidate recall | 8/16 | **10/16** |
| top-5 candidate recall | 10/16 | **11/16** |
| coverage | 16/16 | 16/16 |

Changed primaries on the holdout: `go-u001` (validateKinds → validateWildcard),
`go-u009` (Decoder.decode → RecursiveStructToMapHookFunc), `go-u016`
(Column.Format → NullsafeHashcode128); `go-u004`/`go-u015` moved between
`CreatePosthogProperties*` siblings that the holdout label treats equivalently.
No primary regressed. The holdout reuses the same repository set as the frozen
corpus (reports regenerated on the frozen baseline engine), so it measures
generalization across report regeneration and independent labels, not across new
repositories.

## Remaining unsolved (with reason)

- `go-u003`: ambiguous sibling assertion-dependency operations; no dataflow.
- `go-u004`: culprit package outside the indexed graph; no evidence links it.
- `go-u007`: culprit helper is a deeper callee; no SSA and no function-level
  evidence names it.
- `go-u010`: panic site (`GoSlice.Index`) vs reviewed fix location (`toValue`);
  the value-producing callee is named in source but not resolvable at ranking.
- `go-u012`: the report carries no call edges to the setter methods (collection).

## Explanation reliability

`CausalExplanation` records `operation_support` and the exact basis phrase:
`evidence_supported` (typed SSA value, verified failing-test call path, or an
observed failure frame), `source_correlated` (textual match), `source_associated`
(operation present but unlinked), or `requires_investigation`. On the frozen 16:
14 `evidence_supported`, 2 `source_associated`. `go-u003`/`go-u010` (probe
operation on the verified path; observed panic frame) and `go-u007`/`go-u012`
(textual fan-out) are labelled consistently with their evidence, and the
hypothesis text always ends "It is not established." Confidence, verification and
promotion are unchanged (regression-tested).
