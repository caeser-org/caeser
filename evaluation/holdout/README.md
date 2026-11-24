# Independent holdout

This directory holds independently grounded real-failure cases used to measure
whether Ceaser generalizes beyond the six development-exposed cases. It is
deliberately **empty until a case passes review** — do not fabricate cases.

## Adding a case

Append an entry to `manifest.json`:

```json
{
  "cases": [
    {
      "repo": "owner/repo",
      "base_sha": "<40-hex buggy revision>",
      "head_sha": "<40-hex fixed revision>",
      "command": "go test -run TestThing ./pkg",
      "reviewed_culprit_function": "FuncName",
      "reviewed_culprit_file": "pkg/file.go",
      "injected_tests": [
        { "path": "pkg/thing_test.go", "sha256": "<sha256 of that file at head_sha>" }
      ],
      "provenance": "PR #N (unmerged); base fails, head passes; culprit reviewed from the fix diff",
      "exposed": false
    }
  ]
}
```

Requirements (a case is rejected otherwise):

1. **Reproducible**: `command` fails on `base_sha` and passes on `head_sha`.
2. **Exact overlay**: every `injected_tests[].path` is fetched from `head_sha` and
   its **SHA-256 must match** — no reconstruction, no upstream substitution.
3. **Independent culprit**: `reviewed_culprit_function` comes from the fix, not
   from Ceaser's output.
4. **Unexposed**: set `"exposed": true` once a case has informed engine changes,
   so it is no longer counted as independent.

## Running

The evaluation runs in CI (GitHub-hosted runner, ample disk/network) via the
`final-eval.yml` workflow, which calls `evaluation/scripts/holdout-run.sh`. It
clones `base_sha`, applies the verified overlay, runs the production engine, and
records `primary`, `trace`, and `hit` per case in
`evaluation/results/holdout-results.json`.

Locally (needs disk for the clones + module cache):

```sh
go build -o /tmp/caeser ./cmd/caeser
CAESER=/tmp/caeser bash evaluation/scripts/holdout-run.sh
```

## Reporting

Report exact-function accuracy over **independently grounded** cases only, with
denominators, and keep development-exposed cases separate. Never present the
six-case 6/6 as root-cause or generalization accuracy.
