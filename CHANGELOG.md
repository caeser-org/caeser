# Changelog

Caeser uses release notes under `docs/releases/`. This file summarizes them.

## v0.1.0-alpha — open-source developer preview

First public alpha. Go-only. Produces evidence-grounded investigation leads,
not verified root causes. Honest measured results: 6/6 exact-function on six
development-exposed cases, **1/5** on the first five independently verified
unfamiliar repositories. No verification gates were weakened.

- Backward causal trace with operand provenance, caller arguments, observed-frame
  caller selection, and bounded multi-hop wrappers.
- Defect-origin hypotheses (`later_assignment`, `ignored_error`, type-aware
  `missing_guard`, collection-tied `boundary_guard`), always inferred.
- Base/head delta correlation and assertion-site anchors, informational only.
- Competitive hypotheses retained; verdicts abstain when evidence is thin.
- Reproducible verified-overlay independent holdout pipeline on GitHub Actions.
- Properly fixed the flaky process-group test; full suite is the CI gate.

See [v0.1.0-alpha notes](docs/releases/v0.1.0-alpha.md) and
[capabilities](docs/capabilities.md).

## Unreleased — v9.2 (proposed, historical)

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

## v9.1 — localization crash fix

- Fixes a crash when a stack frame matched only a whole-file node.
- Install defaults pinned to v9.1.

See [docs/releases/v9.1.md](docs/releases/v9.1.md).

## v9 — likely-function handoff tier

- Adds the structured `likely_function` tier (primary suspect plus alternates,
  each with file, function, line, reason, provenance kind, confidence and
  evidence IDs).

See [docs/releases/v9.md](docs/releases/v9.md).

## v8 and earlier

See [docs/releases/](docs/releases/).
