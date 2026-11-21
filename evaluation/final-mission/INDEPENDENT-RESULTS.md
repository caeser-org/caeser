# Independent evaluation (frozen) — first five verified cases

Run on GitHub Actions (`ceaser-org/ceaser`, branch `eval/final-mission-2026-11-22`),
reproduced locally and in CI. Cases were verified before scoring: base fails the
injected regression test **as a test failure (not a build error)** and the fixed
head passes. The engine was **not** tuned against these cases.

## Result

| # | Case (unfamiliar repo) | Failure | Reviewed culprit | Engine primary | Verdict |
|---|---|---|---|---|---|
| 1 | alecthomas/kong #627 | panic, unmappable map key/value | `mapper.go` `mapDecoder` | `mapDecoder` | **hit** |
| 2 | alecthomas/kong #651 | provider panic on value-typed error | `callbacks.go` `callFunction` | `callAnyFunction` | miss (same file, sibling) |
| 3 | alecthomas/kong #637 | build error: duplicate `-f` alias | `build.go` `buildNode` | `New` | miss |
| 4 | urfave/cli #2511 | help-wrap length assertion | `help.go` `wrapLine` | ABSTAIN | miss (abstain) |
| 5 | spf13/cobra #2356 | completions mutate `os.Args` | `completions.go` `getCompletions` | `ExecuteC` | miss |

**Exact-function: 1/5 (20%).** File-level: 2/5 (kong #627, kong #651 — same file).
Abstentions: 1/5. This is the honest independent signal; it does not generalize the
six development-exposed cases (6/6).

## Failure categories observed

1. **Sibling / fan-out ranking** (kong #651, cobra #2356). The culprit sits in a
   large function set with no distinguishing observed frame; the engine ranks a
   sibling or the entry point first. kong #651 is the clearest: `callFunction` and
   `callAnyFunction` are adjacent siblings in the same changed file, and the fix
   touched both hunks; the engine picked the sibling.
2. **No-frame localization** (kong #637, urfave/cli #2511, cobra #2356). The
   failure is an error message or a test assertion with no implementation stack
   frame; the engine abstains or falls back to the package entry point (`New`,
   `ExecuteC`). These are the assertion/entry-point cases the engine has always
   been weakest at.

Both categories match the previously documented bottlenecks; the independent set
confirms them on repositories the engine was never tuned against.

## What this establishes

- The engine **does** localize a class of unfamiliar panics correctly (kong #627,
  a `nil`/unmapped-value panic with an implementation frame).
- It **does not** reliably localize when the culprit is a sibling among many
  functions, or when there is no implementation frame.
- The six-case 6/6 must **not** be read as generalization; 1/5 is the honest
  independent rate on unfamiliar cases.

## Reproduce

```
go build -o /tmp/caeser ./cmd/caeser
CAESER=/tmp/caeser bash evaluation/scripts/holdout-run.sh   # 5 verified cases
```
CI: `.github/workflows/final-eval.yml` (build, full test suite, paired eval,
holdout). Manifest and reviewed labels: `evaluation/holdout/manifest.json`.
