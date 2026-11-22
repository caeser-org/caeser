# Evaluation status: first five verified cases (now development-exposed)

Run on GitHub Actions (`ceaser-org/ceaser`, branch `eval/final-mission-2026-11-22`),
reproduced locally and in CI. Cases were verified before scoring: base fails the
injected regression test as a test failure (not a build error) and the fixed head
passes.

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

Collect ten fresh verified cases from unfamiliar repositories, freeze them, and
score only on those. Do not tune on the same cases used to report accuracy.
