# Your first investigation

This walkthrough takes one small Go failure from a failure log to a report you
can read. It uses the fixture in [`examples/first-investigation`](examples/first-investigation/)
and needs no model credentials. By the end you should be able to point at the
failure, its evidence ID, the function to inspect first, and the next check.

The fixture is an order package. A paid order should be `"ready"`, but
`Status` returns `"pending"`, so `TestStatusPaid` fails. The fixture is broken
on purpose. Do not fix it.

## Before you start

Build or install `caeser` as described in
[Try it locally](../README.md#try-it-locally), and check that `caeser version`
works. Run the steps below from a clean checkout of this repository.

## 1. Generate a sample failure log

This step reproduces a failure locally instead of collecting a real CI log.
Run the fixture's tests and save their output as the log:

```sh
cd docs/examples/first-investigation
go test ./... > ci.log 2>&1
```

The command exits with an error because the test fails. `ci.log` should
contain:

```text
--- FAIL: TestStatusPaid (0.00s)
    order_test.go:8: Status() = "pending", want "ready"
FAIL
```

The timing on each line can differ.

## 2. Generate the report and the brief

Run the diagnosis against the same checkout that produced the log, then export
the Markdown brief:

```sh
caeser diagnose --repo . --ci-log ci.log --artifact-first --json > report.json
caeser brief --report report.json > caeser-brief.md
```

`--artifact-first` analyzes the log you already have instead of rerunning the
test, which is what the default CI path does. Progress messages go to stderr.
The run also creates a `.caeser/` working directory. `ci.log`, `report.json`,
`caeser-brief.md` and `.caeser/` are ignored by git in this folder.

## 3. Read the brief

Open `caeser-brief.md`. Evidence IDs and the source revision depend on your
checkout, so yours will not match the examples below.

**The failure.** Under **Failed checks**:

```text
TestStatusPaid at order_test.go:8 — Status() = "pending", want "ready" [ci-evidence-f4ca8a539d3f]
```

The ID in brackets is the evidence ID for the log excerpt. Search the brief for
it to see the excerpt under **Evidence excerpts**.

**Where to start.** Under **Suspects**:

```text
Primary: Status — order.go:14 [inferred_call, medium confidence]
Why: function contains an SSA value in the failed assertion dependency trace
```

**The next check.** The first line under **Next checks** names the operation
to inspect and what would confirm or reject the lead:

```text
Investigate operation return "ready":string at order.go:16 in Status — confirm if the value observed at this operation matches the recorded failure ...
```

The suggested operation is a starting point for the investigation, not
necessarily the faulty statement. Here `return "ready"` is not what went wrong:
it only runs when the condition above it is true. Inspect the condition that
controls the return value.

**How far Caeser got.** Under **Verdict**, `Cause: unverified` means Caeser
did not establish the root cause. `Verification: source and static analysis
only` means nothing was rerun.

The same facts are in `report.json`:

```sh
jq '.normalized_evidence.executions[].tests[] | select(.state == "failed")' report.json
jq '.likely_function.primary' report.json
jq '.root_cause | {status, verification_status, next_actions}' report.json
```

## 4. Failure location, ranked function, proven cause

This example shows three different kinds of answer:

| Kind | In this example | What it tells you |
| --- | --- | --- |
| Failure location (observed) | `order_test.go:8`, from the log | Where the test reported the failed assertion. It is an observed location, not necessarily a stack frame, and here it is not the bug. |
| Ranked function (inferred) | `Status` at `order.go:14` | Where to look first. Caeser traced the failed value back through the call, but did not run anything to prove it. |
| Proven cause | None | `root_cause.status` is `localized` and `verification_status` is `not_attempted`. No mechanism was established. |

The ranked function is a lead to check. Open `order.go` at line 14: the
condition on line 15 also requires `Shipped`, so a paid but unshipped order
falls through to `"pending"`. Confirming that is your job. See
[Read the evidence](reports.md#read-the-evidence) for what observed, inferred
and established mean.

## 5. Find the PR comment and the artifact

Steps 1–4 used the local order fixture. This step uses a separate, real CI
failure: `TestLateRouteRegistrationSkippedNodes` in
[caeser-org/ceaser-demo pull request #1](https://github.com/caeser-org/ceaser-demo/pull/1).

1. Open the pull request. Caeser posts or updates one comment headed
   **Caeser diagnosis**. It shows the failure, triage, root cause status,
   **Start here** (the ranked function), and **Next checks**. Expand
   **Diagnosis details** for the evidence IDs.
2. Follow **View the workflow run and download the investigation brief** in
   the comment.
3. On the [workflow run](https://github.com/caeser-org/ceaser-demo/actions/runs/45677116286),
   under **Artifacts**, download `caeser-diagnosis-45677116286`. In other
   runs the name is `caeser-diagnosis-<run-id>`. It contains
   `caeser-brief.md` and `caeser-report.json`, the same kinds of files you
   made in step 2.

See [Reports and briefs](reports.md) for every output and what to do if the
brief is missing.

## Check yourself

After these steps you should be able to answer, from your own brief:

- Which test failed, at which line, and with which message?
- Which evidence ID holds the log excerpt?
- Which function should you inspect first, and is that observed or inferred?
- What is the next check, and was the root cause verified?
