# Evaluation status: independent batch and exposed early cases

## Current independent measurement

A fresh batch of ten verified cases from repositories Ceaser had not been tuned
against was frozen before its first evaluation. Exact-function: **3/10 (30%)**,
with three abstentions. This is the honest independent signal for the engine as
shipped in v0.1.1. The cases remain frozen and must not be used for tuning. Full
table: [FRESH10-RESULTS.md](../holdout/FRESH10-RESULTS.md). Manifest:
[fresh10-manifest.json](../holdout/fresh10-manifest.json).

## Early case history

The first five cases below were run on GitHub Actions
(`ceaser-org/ceaser`, branch `eval/final-mission-2026-11-22`), reproduced locally
and in CI. Cases were verified before scoring: base fails the injected regression
test as a test failure (not a build error) and the fixed head passes.

## Exposure notice

These five cases were the independent holdout when the 1/5 result below was
frozen. They have since been inspected and used to diagnose and fix a candidate
admission gap:

- the causal chain named the right function as an `assertion_dependency` step,
  but candidate admission dropped it;
- the failing test's own source sometimes names the responsible function in a
  comment at the assertion.

Because the cases informed these changes, they are **development-exposed** and
must no longer be reported as an independent accuracy measurement. A fresh set
of unfamiliar, verified cases is required before claiming any generalization.

## Original frozen result (before exposure)

| # | Case (unfamiliar repo) | Failure | Reviewed culprit | Engine primary | Verdict |
|---|---|---|---|---|---|
| 1 | alecthomas/kong #627 | panic, unmappable map key/value | `mapper.go` `mapDecoder` | `mapDecoder` | hit |
| 2 | alecthomas/kong #651 | provider panic on value-typed error | `callbacks.go` `callFunction` | `callAnyFunction` | miss (same file, sibling) |
| 3 | alecthomas/kong #637 | build error: duplicate `-f` alias | `build.go` `buildNode` | `New` | miss |
| 4 | urfave/cli #2511 | help-wrap length assertion | `help.go` `wrapLine` | ABSTAIN | miss (abstain) |
| 5 | spf13/cobra #2356 | completions mutate `os.Args` | `completions.go` `getCompletions` | `ExecuteC` | miss |

Exact-function: 1/5 (20%). File-level: 2/5. Abstentions: 1/5.

## Result after the admission fix on the now-exposed cases

| # | Case | Reviewed culprit | Primary after fix | Verdict |
|---|---|---|---|---|
| 1 | kong #627 | `mapDecoder` | `mapDecoder` | hit |
| 2 | kong #651 | `callFunction` | `callAnyFunction` | miss (ambiguous sibling) |
| 3 | kong #637 | `buildNode` | `buildNode` | hit |
| 4 | urfave/cli #2511 | `wrapLine` | ABSTAIN | miss (no source evidence) |
| 5 | cobra #2356 | `getCompletions` | `Command.getCompletions` | hit |

Exact-function: 3/5 on the exposed set. This is **not** an independent rate and
must not be presented as one.

## What the fix does

- Admits chain-named functions as `causal_dependency` leads. Assertion-dependency
  steps form a path from the assertion toward the value's origin, so later steps
  rank higher. This recovered kong #637.
- Admits a function named at the failing assertion in retrieved test source as
  an `assertion_mention` lead, only from a comment with explicit causal language.
  This recovered cobra #2356.
- Prevents a function already in the candidate tier from being re-injected from
  SSA with a competing rank, which had let operand distance override the
  evidence-grounded order.

No verification gate was changed. All leads remain investigation leads; none is
promoted to a verified root cause.

## Remaining gaps

- kong #651: two sibling functions in the same changed file; the fix touched both
  and the observed stack names the other one. This label is ambiguous.
- urfave/cli #2511: the report has only git evidence and no source excerpt, so
  the engine correctly abstains. Fixing this needs assertion-anchor analysis on
  tests with thin SSA coverage, which is not implemented.

## Paired development check

The six paired development cases remain 6/6 exact-function with tracing on and
no primary regressions (tracing off: 5/6).

## Reproduce

```
go build -o /tmp/caeser ./cmd/caeser
CAESER=/tmp/caeser bash evaluation/scripts/holdout-run.sh   # 5 now-exposed cases
```
CI: `.github/workflows/final-eval.yml` (build, full test suite, paired eval,
holdout). Manifest and reviewed labels: `evaluation/holdout/manifest.json`.

## Next required step

The ten-case fresh batch is collected, frozen, and evaluated once at 3/10. Keep
it as a validation set and do not tune against it. For any later engine change,
collect a further fresh batch and evaluate once without tuning.
