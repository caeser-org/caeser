# Changelog

Caeser uses release notes under `docs/releases/`. This file summarizes them.

## Unreleased

- Added a frozen independent batch of ten verified cases from unseen
  repositories. First evaluation, before any tuning: 3/10 exact-function with
  three abstentions. The batch is a validation set and must not be used for
  tuning. See `evaluation/holdout/FRESH10-RESULTS.md`.

## v0.1.1

Ships the candidate-admission fix from commit `4a5c4d9`. No verification gate
changed. Exact-function matches: 6 of 6 on the paired development cases, 3 of 5
on the five now-exposed cases after tuning. There is no fresh independent
measurement and no claim of production-ready root-cause diagnosis.

- Admit functions named by causal-chain `assertion_dependency` and
  `implementation_branch` steps as `causal_dependency` leads, ranked by depth.
- Admit functions named at the failing assertion in retrieved test source as
  `assertion_mention` leads, limited to comments with explicit causal language.
- Stop re-injecting a candidate from SSA once the candidate tier contains it.

See [v0.1.1 notes](docs/releases/v0.1.1.md).

## v0.1.0, open source

First public release. Go only. Produces evidence-grounded investigation leads,
not verified root causes. Honest measured results: 6/6 exact-function on six
development-exposed cases, originally 1/5 on five verified unfamiliar
repositories, now 3/5 after an admission fix made those five development-exposed.
No verification gates were weakened.

- Backward causal trace with operand provenance, caller arguments, observed-frame
  caller selection, and bounded multi-hop wrappers.
- Defect-origin hypotheses (`later_assignment`, `ignored_error`, type-aware
  `missing_guard`, collection-tied `boundary_guard`), always inferred.
- Base/head delta correlation and assertion-site anchors, informational only.
- Competitive hypotheses retained; verdicts abstain when evidence is thin.
- Reproducible verified-overlay independent holdout pipeline on GitHub Actions.
- Properly fixed the flaky process-group test; full suite is the CI gate.

See [v0.1.0 notes](docs/releases/v0.1.0.md) and
[capabilities](docs/capabilities.md).

## Unreleased, v9.2 (proposed, historical)

Diagnostics-quality release. No change to verification or promotion.

- Likely-function ranking now preserves the candidate tier's evidence order and
  promotes operand-correlated SSA leads; the operand cone follows the call
  `callee` so the function that produced the assertion's operand is named even
  when it is not in the stack. Framework/runtime stack frames are ordered behind
  application code.
- Candidate ordering is deterministic (previously a Go map order could change
  the primary suspect between identical runs).
- The bounded candidate report keeps implementation candidates ahead of the
  failing test file.
- The concrete panic/exception text is extracted instead of a generic
  placeholder.
- The PR comment leads with the primary suspect and lists what to investigate.
- Repository references use the renamed owner `caeser-org`.
- Go toolchain pinned to 1.26.11 (0 called vulnerabilities).

See [docs/releases/v9.2.md](docs/releases/v9.2.md).

## v9.1, localization crash fix

- Fixes a crash when a stack frame matched only a whole-file node.
- Install defaults pinned to v9.1.

See [docs/releases/v9.1.md](docs/releases/v9.1.md).

## v9, likely-function handoff tier

- Adds the structured `likely_function` tier (primary suspect plus alternates,
  each with file, function, line, reason, provenance kind, confidence and
  evidence IDs).

See [docs/releases/v9.md](docs/releases/v9.md).

## v8 and earlier

See [docs/releases/](docs/releases/).
