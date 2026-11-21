# Iteration 4 — independent validation (measurement only)

Engine frozen for this evaluation: repository HEAD `fb6edeb` plus iterations 2–3
(evidence-ordered candidate ranking, innermost-frame ordering,
`operation_support`). Evaluated worktree diff SHA-256
`ada10445…f617`. No ranking, admission, explanation, or verification logic was
changed during this iteration. Base comparison = `fb6edeb` alone.

Production path used: the engine's candidate/likely/causal tiers rebuilt from
each **sealed** diagnosis report via `cmd/likely-bench
--rebuild-candidate-report` (identical inputs to the production
`BuildCandidateReport`/`BuildLikelyFunction`/`BuildCausalExplanation`). No
evidence was added and no scope widened per case.

## 1. Dataset and the new-repository shortfall

| Set | Cases | Repos | Repos not in frozen benchmark | Independently labeled | Reproducible offline |
|---|---:|---:|---:|---|---|
| `eval/unseen` (sealed bundles + versioned labels) | 16 | 12 | 0 | yes | yes |
| `eval/hy4-go-holdout` (sealed blind reports + truth) | 11 | 11 | 0 | yes | yes |
| `evaluation/corpus` (new repos, reviewed labels, real logs) | 15 | 8 | **8** | yes | **no** |

**Actual new-repository cases reproducibly evaluated in this iteration: 0 of the
target 25–30.** The obstacle is not label quality but reproduction: each corpus
case was produced by injecting a *comment-redacted* copy of the PR's regression
test onto the buggy base, and those exact files are not preserved in the corpus.
Re-running the production evidence-first path with the un-redacted upstream test
file misaligns the failure-log line numbers (e.g. `go-014` reports
`mux_test.go:2771`, while the upstream test sits at line 2805), so the wrong
enclosing test is selected and source analysis is not faithful. The environment
also lacks a container runtime for the Go cold path, and the seven
unrelated-repository replacement PRs previously collected all failed
qualification (test files reference fix-introduced symbols and do not compile on
the buggy base, or require unavailable native libraries). No weak cases were
substituted to reach the target.

The two sealed holdouts reuse repositories from the frozen benchmark but are
otherwise independent: separately prepared revisions, injected tests, and
independently reviewed culprit labels (e.g. HY4 labels `RequireInt`,
`hashCodeForRow`, `SetCPUUtilization`, `setBody`, `Unflatten` — different from the
frozen labels for the same repositories). They therefore measure generalization
across **new failures, revisions, and labels**, not across new **repositories**.

## 2. Results (current engine)

Function match: receiver-qualified substring equality of the reviewed culprit
function against the primary. File match: basename equality. `top-k` counts a hit
in any of the first k entries.

| Metric | unseen (16) base → current | HY4 (11) base → current |
|---|---:|---:|
| Primary, function exact | 7 → **10** | 3 → **5** |
| Primary, function-or-file | 10 → **12** | 8 → 8 |
| Top-3 (function-or-file) | 11 → **12** | 8 → 8 |
| Top-5 (function-or-file) | 12 → **13** | 9 → 9 |
| Misleading primary (function) | 9 → **6** | 8 → **6** |
| Coverage | 16/16 | 11/11 |
| False root-cause verifications | 0 | 0 |

Rates (current engine): primary function exact **10/16 = 62.5%** (unseen) and
**5/11 = 45.5%** (HY4); function-or-file **75%** and **73%**; function-level
misleading **37.5%** and **54.5%**.

Per-case (current engine, function match):

- unseen hits: u001,u002,u004,u005,u008,u009,u011,u014,u015,u016.
  misses: u003 (`Version` vs `PutObject`), u006 (`newContext.func1` vs
  `patchContext`), u007 (`Delete.TryExecute` vs `Distinct`), u010
  (`GoSlice.Index` vs `toValue`), u012 (`NewServerMetricsRecorder` vs
  `SetCPU…`), u013 (`PutObjectOptions` vs `maxNumThreads`).
- HY4 hits: h4-02,h4-06,h4-07,h4-08,h4-09. misses: h4-01 (`GetInt` vs
  `RequireInt`), h4-03 (`CreatePosthogPropertiesForProduct` vs `config.New`),
  h4-04 (`Lock.TryStreamExecute` vs `hashCodeForRow`), h4-12
  (`newServerMetricsRecorder` vs `SetCPUUtilization`), h4-13
  (`newContext.func1` vs `setBody`), h4-14 (`Koanf.Delete` vs `Unflatten`).

## 3. Qualified uncertainty and explanation accuracy

`operation_support` (current engine): unseen 12 `evidence_supported`, 2
`source_associated`, 2 `requires_investigation`; HY4 8 `evidence_supported`, 3
`source_associated`. Of the 12 function-level misses, 6 are explicitly
`source_associated`/`requires_investigation` (weakly supported language), and 6
are `evidence_supported` (observed frame or verified failing-test path) that
still name the wrong function. `evidence_supported` describes the operation's
evidentiary basis, not a proven cause; every hypothesis text ends
"It is not established."

Root-cause status of the sealed reports: no case is `established` or `confirmed`
(unseen 11 localized / 4 partially_established / 1 unknown; HY4 7 / 3 / 1), so
**false root-cause verification = 0** and **unsupported causal claims = 0**.
Abstention behavior is preserved: no case is force-promoted; root cause stays
behavioral/localized.

## 4. Failure-mechanism analysis (no fixes applied)

| Mechanism | Cases | Description |
|---|---|---|
| Admission / sibling ambiguity | u003, u013, h4-01 | culprit never admitted, or a same-domain sibling (`GetInt` vs `RequireInt`, `PutObjectOptions` vs the cap) ranks first |
| Interface fan-out ranking | u007, h4-04 | an interface implementation (`Delete.TryExecute`, `Lock.TryStreamExecute`) outranks the reviewed helper |
| Panic-site vs deeper helper | u010, h4-13, h4-14 | the innocuous observed frame (`GoSlice.Index`, `newContext.func1`, `Koanf.Delete`) is reported; the reviewed cause is a callee |
| No call edges / shared concurrency | u012, h4-12 | the report carries no edges to the setter methods; only the constructor is reachable |
| Indexing scope | u004, h4-03 | culprit in an unindexed package (`config`), referenced only as a parameter type |
| Ambiguous ground truth | u015, h4-03 | several `CreatePosthogProperties*` siblings, or a generic `New` |

These group into six generalizable mechanisms: missing evidence (no edges/SSA),
indexing scope, candidate admission, ranking, insufficient data flow, and
ambiguous labels. Only one (innermost-frame ordering) was shown earlier to be
resolvable from existing evidence; the rest remain open and were **not** fixed in
this iteration.

## 5. Cost

Full diagnosis (baseline reports, runtime observation enabled): unseen mean
10.7 s / max 30.2 s; HY4 mean 14.1 s / max 34.6 s. Corpus prior baseline mean
3.9 s / max 9.5 s. The ranking rebuild itself is ≈0.15 s for 16 cases. No
indexing scope was changed; no additional repository scan is performed by the
evaluated logic.

## 6. Reproduction

```
# labels + reports are sealed on their own branches; extract read-only:
git archive eval/unseen-prep evaluation/unseen | tar -x -C /tmp
git archive eval/hy4-go-holdout evaluation/holdout-prep | tar -x -C /tmp
# place each report as <dir>/go-XXXX/caeser-report.json and rebuild current tiers:
go run ./cmd/likely-bench --rebuild-candidate-report \
  --artifacts <dir> --out <out.json>
```
Machine-readable results: `evaluation/iteration4-summary.json`.

## 7. Recommendation — **limited alpha**

Distinguish the two products the engine produces:

- **Likely investigation location (what it does today):** it localizes the
  culprit *file* in ~73–75% of independently prepared cases and the exact
  *function* in ~45–63%, and it orders leads with explicit, usually correct
  uncertainty labels. This is useful for a developer or an assistant to start an
  investigation.
- **Verified root cause (what it does not do from CI evidence):** zero false
  root-cause verifications across both holdouts; the engine correctly leaves the
  cause unverified.

Because true cross-repository generalization is **unmeasured** here (0 of the
target new-repository cases could be reproduced offline) and function-level
accuracy on unfamiliar failures is ~50% with a high function-level misleading
rate, public beta is not justified. Recommended: **limited alpha** for
investigation-location assistance, gated on (a) a reproducibly runnable
new-repository corpus that preserves injected tests, and (b) reducing the
admission/fan-out and panic-site-vs-helper mechanisms above.
