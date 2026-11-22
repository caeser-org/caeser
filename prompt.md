# CEASER — FINAL AUTONOMOUS ENGINEERING MISSION
## Evidence-Grounded Root-Cause Diagnosis, Verification, and Independent Validation

> **For OpenCode:** This file is the persistent mission specification. Read it **completely before editing code**. Then execute the mission end-to-end without asking the user to supply another prompt after each phase. Re-read this file at the beginning of each work session, after context compaction, and before declaring completion. Keep progress and blockers in repository files so the mission survives context resets.
>
> **Important:** “Final” means one comprehensive engineering assignment, **not** a promise that universal root-cause diagnosis is achievable or that every phase will fit one context window. Do not claim success without evidence. Do not run indefinitely when blocked by permissions, missing infrastructure, or genuinely unresolved research problems. Continue autonomously where possible; record precisely what is blocked and why.

---

## 0. Mission, success definition, and non-negotiable principles

**Project:** Ceaser — a Go CI failure diagnosis system.

**North-star outcome:** Given a reproducible failing CI case, Ceaser should identify the most defensible **defect-origin operation or mechanism** it can establish, explain the causal relationship to the failure, differentiate alternatives, and verify causation when the available evidence and controlled experiments support it. When it cannot establish a cause, it must abstain or return calibrated hypotheses, not manufacture certainty.

**This is NOT a mission to make explanations longer, increase arbitrary confidence scores, or turn source-pattern detectors into “verified root causes.”**

### Hard invariants

1. **Inspect the real repository first.** Treat the architecture descriptions in this file as context, not as proof that symbols, types, paths, or capabilities exist exactly as stated. Locate actual implementations, call sites, and tests.
2. **Reuse the production engine.** No parallel diagnosis engine, independent heuristic scoring pipeline, duplicated AST/SSA/CFG implementation, or disconnected proof system.
3. **Preserve existing verification gates** unless a specific new proof rule is independently justified, implemented with negative controls, and documented. Never lower a gate merely to increase the verified count.
4. **Maintain evidence provenance.** Separate *observed execution*, *statically possible relationships*, *control-flow feasibility*, *source changes*, *runtime correlation*, *intervention results*, and *verified causal mechanism*.
5. **A diff is not causation.** A source pattern is not causation. A static path is not proof of execution. A stack frame is not proof that every line in that function executed. A successful patch is not automatically proof of the intended root cause.
6. **No automatic promotion based on path length, closeness, source order, or a changed line alone.**
7. **Protect existing working functionality.** Keep deterministic output, resource budgets, cycle guards, safe sandboxing, and conservative abstention.
8. **No invented metrics.** Report actual observed results, fixture provenance, denominators, and failures. Keep development-exposed cases separate from independent holdouts.
9. **Do not delete unrelated data, wipe caches outside the repo, overwrite user changes, or force-push.** Never silently reset a dirty working tree.
10. **Be autonomous, not reckless.** Continue through all feasible phases; when blocked, document alternatives and proceed with independent tasks. Ask the user only when a genuine external decision, credential, destructive action, or inaccessible resource is required.
11. **No LLM-only root-cause promotion.** An LLM may help articulate evidence or suggest checks only if already permitted by architecture; it is not a verification oracle.
12. **Do not claim “done” solely because tests pass.** Completion requires evidence of the implemented behavior and an honest evaluation report.

### The desired output distinctions

For each relevant finding, distinguish:

- **Manifestation:** operation where failure becomes visible.
- **Invalid operand/state:** value or condition involved in the failure.
- **Provenance:** how that value or state reached the manifestation.
- **Potential defect origin:** operation or missing condition that could have introduced the invalid behavior.
- **Hypothesis:** a testable explanation with explicit uncertainty.
- **Supported causal mechanism:** stronger relationship established by appropriate static/runtime evidence.
- **Verified root cause:** only when existing or newly justified stringent proof requirements are satisfied.

Do not collapse these into one field or confidence score.

---

## 1. Known history — verify against the checkout

The previous iterations reportedly implemented:

- Failure classification, source indexing, AST and SSA analysis, typed `CALLS` edges (`go_types`, `call_lines`, `verified_static_call`), source excerpts, evidence/provenance, candidate admission/ranking, explanations, briefs, and strict root-cause verification.
- Backward `CausalTrace` from observed failures to producing calls.
- Operand provenance using `fnOrigins` / `buildFnOrigins`, aliases, local zero values, nil literals, call producers, and boundaries.
- Cross-function parameter-to-argument provenance, including methods, nested calls, caller ambiguity handling, and a bounded upward walk.
- Invocation-aware caller selection using ordered observed frames and typed call-site relationships; inferred argument flow remains distinct from observed caller participation.
- A `defect_origin.go` assessment that reports **inferred** `later_assignment`, `ignored_error`, `missing_guard`, and `boundary_guard` hypotheses tied to relevant operands.
- Integration into `CausalTrace`, `BuildCausalExplanation`, and `InvestigationBrief`, without promoting hypotheses or weakening verification.

Reported evaluation: six reproducible **development-exposed** real-repository cases, exact-function localization **6/6 with tracing enabled versus 5/6 disabled**, no primary changes in the last several iterations, zero new verified causes, and defect-origin hypotheses on **2/6** in the latest iteration. These numbers do **not** establish defect-operation accuracy or generalization. Some older fixtures lack their original injected tests. Disk space previously blocked new-repo collection.

Likely relevant paths, to confirm rather than assume:

- `internal/diagnosis/causal_trace.go`
- `internal/diagnosis/defect_origin.go`
- `internal/diagnosis/` candidate/evidence/explanation/verification modules
- `internal/evalcorpus/`
- `evaluation/iteration11-defect-origin.md`
- `evaluation/results/iteration7-causal-trace-paired.json`
- `evaluation/scripts/causal-trace-eval.sh`
- `evaluation/causal-trace-manifest.csv`

**Do not use this historical summary as a substitute for reading source.** If the checkout differs, document the discrepancy and adapt.

---

## 2. Operating protocol — how to work until completion

### 2.1 First actions

Before changing code:

1. Read this entire file.
2. Record `git status`, current branch/commit, Go version, free disk, test commands, and relevant repository instructions.
3. Read the README, architecture docs, latest evaluation reports, and the production code that creates/consumes causal traces, SSA evidence, deltas, candidates, and verification decisions.
4. Draw a concise evidence-flow map showing **actual** producers, fields, consumers, and proof gates.
5. Run a reasonable baseline (`go test ./...`, targeted tests, and existing evaluation where feasible). Preserve output and baseline metrics.
6. Identify already-existing implementations for each planned capability; avoid redundant changes.

### 2.2 Persistent progress tracking

Create/update these repo-local files (choose a suitable existing docs/evaluation location if conventions differ):

- `evaluation/final-mission/PLAN.md` — tasks, dependencies, acceptance criteria, status.
- `evaluation/final-mission/PROGRESS.md` — dated work log, commits, tests, current blockers, next exact command/task.
- `evaluation/final-mission/EVIDENCE-MAP.md` — evidence semantics, provenance levels, trust boundaries, proof gates.
- `evaluation/final-mission/RESULTS.md` — baseline, paired evaluations, independent results, regression findings.
- `evaluation/final-mission/LIMITATIONS.md` — unresolved cases, blocked capabilities, false positives, unsafe assumptions.

Maintain these files **during** work, not only at the end. On context compaction or session restart, re-read **this mission file plus PLAN and PROGRESS**, inspect `git status`, and continue from the last incomplete acceptance criterion. Do not restart finished work or declare the project complete prematurely.

### 2.3 Execution discipline

For each phase: audit → define testable hypothesis → add focused negative and positive tests → implement minimal production integration → run tests → inspect real outputs → update docs/results → continue.

If one task fails, investigate and fix the cause. Do not paper over a failure by weakening tests or relabeling an unsupported hypothesis. If a task is blocked, document it and proceed to other feasible work.

**Do not repeatedly ask “shall I continue?”** The user has authorized this entire mission. Stop and request input only for irreversible/destructive operations, unavailable permissions/secrets, costly external resources requiring approval, or genuine ambiguity that cannot safely be resolved.

Use small reviewable commits if the environment permits. Never commit generated junk or secrets. Do not push unless repository instructions or user authorization explicitly permit it.

### 2.4 Resource safety

- Use bounded analysis depth, time, and memory; deterministic ordering.
- Respect CI budgets and existing sandbox restrictions.
- If disk is full, inspect usage, clean only clearly owned temporary artifacts after verifying their ownership, or use an approved larger CI runner. Never delete unrelated files or caches indiscriminately.
- If independent test collection is impossible locally, prepare reproducible CI jobs and exact collection instructions, then continue with tests that are feasible.
- Do not run untrusted code outside existing sandbox isolation.

---

## 3. Phase A — control-flow-sensitive defect hypothesis assessment

**Goal:** Determine whether a reported defect-origin operation can influence the failing operand along a feasible static control/data-flow relationship. Avoid claiming the relevant branch actually executed without runtime evidence.

### A1. Audit existing analysis

Locate existing SSA trace structures, CFG blocks, dominators, phi nodes, data dependencies, reaching definitions, AST-to-SSA source mappings, and how these are represented in evidence. Reuse them. If necessary, add a narrow adapter, not a second general-purpose analyzer.

Determine which functions/packages have SSA coverage, which do not, and what happens on build errors, generics, closures, interfaces, inlining, and missing source.

### A2. Implement defensible feasibility checks

For each `defect_origin` hypothesis, attempt to establish:

- The candidate operation is in the correct function and maps to a relevant SSA instruction/source range.
- A def-use or memory-state relationship can connect the operation to the failing operand.
- The relevant control-flow relationship exists; distinguish dominance, possible reachability, and path-specific execution.
- Reassignments are evaluated using relevant reaching definitions, not simply source-line order.
- Branch-dependent definitions and phi nodes are represented as alternatives unless discriminated.
- A suspected guard actually controls the failing operation; consider early returns, panics, `continue`, `break`, short-circuit expressions, and guards in predecessor blocks.
- The operand is of a type for which the suspected invalid state is possible; `missing_guard` must be type-aware, including pointer/interface/map/slice/function/channel semantics and valid zero-value behavior.
- An ignored error is tied to the consumed value, and the API's error/value contract is not assumed without evidence.
- A boundary comparison has a relevant index, length, and guarded indexing operation; avoid matching unrelated `len` expressions.
- Mutation through pointers, slices, maps, fields, closures, and aliasing is either correctly modeled or explicitly unresolved.

**Do not invent certainty from incomplete SSA.** If SSA is unavailable or source mapping is ambiguous, preserve the AST-only inferred hypothesis with a limitation.

### A3. Evidence semantics

Use explicit evidence categories or relationships (consistent with existing types):

- `source_pattern`: syntax-level suspicion.
- `static_data_dependency`: proven/defensible def-use link.
- `static_control_dependency`: relevant CFG relationship.
- `possible_path`: feasible statically, not observed.
- `observed_invocation`: runtime evidence of participation, if genuinely present.
- `unresolved`: mapping/alias/path ambiguity.

A stronger static relationship may upgrade **the relationship evidence**, not the hypothesis to verified root cause. If existing statuses only allow `inferred`/`static`, document exact semantics before changing status.

### A4. Negative controls

Test at minimum:

- Assignment occurs after failure, or in unrelated branch.
- Earlier assignment overwritten before failure.
- Multiple reaching definitions, phi merge.
- Dominating nil guard that exits on nil.
- Nil guard exists but does not dominate.
- Non-nilable operand; typed nil inside interface.
- `<= len` on an unrelated collection.
- Correct `< len` with invalid input elsewhere.
- Error intentionally ignored when returned value is safe by contract.
- Aliased mutation or unresolved heap effects.
- Missing SSA and unindexed package.
- Ambiguous source-to-instruction mapping.

**Acceptance:** measurable reduction in unsupported defect patterns on adversarial tests; no unsupported “verified” promotions; clean fallback without SSA; no regression in existing production outputs beyond justified changes.

---

## 4. Phase B — base/head CodeDelta correlation

**Goal:** Relate evidenced defect operations to source changes without confusing a regression signal with causal proof.

### B1. Audit

Find actual `CodeDelta`/`report.Deltas` structures, diff line mapping, file rename handling, hunk metadata, changed symbol tracking, and base/head revision semantics. Determine whether analysis source locations refer to base or head.

### B2. Implement linkage

Where base/head information exists, correlate a defect hypothesis to:

- Exact changed operation/range in head.
- Guard added, removed, or modified.
- Assignment/return/conversion/error-handling change.
- Changed callee or call-site relationship on the supported causal path.
- Nearby changed code that **does not** directly overlap the operation (separate category).
- Unchanged code on a causal path.
- Missing/ambiguous mapping (renames, generated code, line shifts).

Use stable source locations or appropriate mapping; do not rely on coincidental line-number equality between revisions.

### B3. Ranking/admission safety

Do not automatically promote a changed hypothesis. A changed operation on a supported path may be a stronger **investigation lead**, but verified cause requires additional evidence.

Preserve no-regression failures and preexisting bugs as valid cases. Explain both changed and unchanged alternatives.

### B4. Tests

Include changed relevant guard, unrelated nearby diff, renamed file, shifted lines, unchanged defect, preexisting failure, missing base, generated code, and multi-hunk ambiguity.

**Acceptance:** accurate and explainable delta linkage, no false assertion that a change caused the failure, graceful behavior without base revision.

---

## 5. Phase C — hypothesis generation and competition

**Goal:** Turn disconnected source-pattern findings into a coherent, testable set of competing defect explanations.

For each supported failure, construct hypotheses that identify:

1. Exact operation and location.
2. Failure manifestation and relevant operand/state.
3. Provenance chain and strength of each edge.
4. Static feasibility status.
5. Base/head relationship, if known.
6. Assumptions and alternative explanations.
7. A concrete observation or intervention that would distinguish the hypothesis.
8. Whether the hypothesis is admissible as an investigation candidate under existing rules.

### Required distinctions

- Incorrect producer output vs valid producer output mishandled by caller.
- Missing guard vs legitimate nil contract.
- Incorrect branch predicate vs invalid upstream input.
- Ignored error vs expected partial result.
- Defect in caller vs defect in callee.
- Changed operation vs unrelated changed code.
- Observed frame participation vs unobserved static call edge.
- Static source pattern vs runtime-correlated behavior.

Avoid duplicate hypotheses for the same underlying mechanism; group alternatives when equivalent. Keep evidence paths bounded.

### Candidate admission

Audit current candidate-admission gates. If a caller-side defect operation is strongly supported, allow **investigation candidate** admission only under explicit, testable conditions; do not keep a blanket prohibition forever if evidence supports it. But do not equate candidate admission with verified root cause.

Record why a hypothesis was admitted, rejected, or kept as a check. If proof is insufficient, retain uncertainty and the observed failure frame as appropriate.

**Acceptance:** coherent hypotheses with precise locations, evidence IDs, competing explanations, and falsifiable checks; no unsupported primary shifts.

---

## 6. Phase D — targeted runtime experiments and causal verification

**Goal:** Use existing sandbox reproduction/probe machinery to collect discriminating evidence and, for selected mechanisms, perform controlled causal tests.

### D1. Audit existing verification and probes

Map the exact current root-cause verification rules, optional instrumentation, base/head replay, test injection, runtime observation, isolation, timeouts, and evidence fields.

Determine which tests can be run safely and reproducibly in the existing environment. Preserve all current proof requirements.

### D2. Runtime observation

Where safe and supported, gather **invocation-specific** evidence such as:

- Operand values at the failing operation.
- Relevant branch outcome.
- Selected caller and call site.
- Return value and error pairing.
- Whether a suspected assignment executed.
- Whether the invalid value was present before/after a candidate operation.

Do not conflate function appearance in a stack with execution of a particular source operation. Instrumentation must avoid materially changing semantics where possible; document perturbation risk.

### D3. Controlled experiments

For suitable, narrowly defined hypotheses, investigate counterfactuals:

- Repair a suspect boundary guard and rerun the same failing test.
- Restore a removed guard in an isolated copy.
- Correct a proven erroneous conversion or error propagation under a precisely specified contract.
- Replace a candidate invalid value with a valid one to test whether that specific failure disappears.
- Compare base and head under equivalent environment/test inputs where available.

**Critical proof caveats:**

- A passing test after a patch shows that patch can eliminate the observed failure; it does not necessarily establish uniqueness, intended behavior, or absence of other defects.
- A change may mask the symptom rather than repair the cause.
- Flaky failures, nondeterminism, environment drift, and multiple interacting defects require abstention or explicit qualification.
- Never rewrite the user's working tree for experiments; use the existing sandbox/worktree isolation and clean up owned artifacts safely.
- Never silently alter test expectations to make a hypothesis pass.
- Require baseline reproduction, controlled intervention, rerun, and appropriate negative controls before claiming strong causal support.

### D4. Verification rules

Prefer reusing the existing verification system. Add a **new narrow verifier** only when the causal mechanism, intervention semantics, negative controls, and failure modes are sufficiently explicit to support sound acceptance.

For every verifier, specify:

- Preconditions.
- Required evidence.
- Intervention and comparison.
- Expected and disqualifying observations.
- Flakiness handling and rerun policy.
- Cases requiring abstention.
- How evidence IDs connect to the resulting status.

Never widen “verified” by a confidence threshold, diff overlap, or a single successful patch.

**Acceptance:** at least one genuinely implemented, narrowly scoped new experimental capability **if feasible**; otherwise a documented audit and honest blocker. No weakening of existing verification; meaningful negative controls; reproducible outcomes.

---

## 7. Phase E — close the most important coverage gaps

This phase is **conditional on evidence from earlier phases**, not an invitation to build everything indiscriminately.

### E1. Cross-package source/SSA coverage

Investigate missing indexed targets and bounded cross-package expansion. Reuse the existing snapshot/indexing architecture. Add targeted indexing only where it materially helps an evidenced causal path and fits resource budgets.

Support or explicitly abstain for interfaces, dynamic dispatch, reflection, generics, and unavailable dependencies. Do not claim static resolution of dynamic calls.

### E2. Assertion failures without useful stack frames

Use failing test location, assertion operands, and existing SSA assertion traces as anchors when observed stack evidence is missing. Preserve test/production source distinctions. Do not guess an executed path.

### E3. Multi-function state and aliasing

If evaluation shows a material need, add a bounded, sound subset of interprocedural state provenance. Avoid a general heap analyzer unless justified by independent failure cases.

### E4. Conversion and error-handling defects

Connect existing runtime numeric verifiers and ignored-error machinery to defect hypotheses when source/operand evidence supports it. Prefer tested mechanism-specific rules over generic detectors.

**Prioritize by measured independent-case misses and misleading diagnoses.** Do not spend weeks adding coverage that has no evaluation signal.

---

## 8. Phase F — independent evaluation (mandatory for honest completion)

**This is not optional just because the six familiar cases already score 6/6 at function localization.**

### F1. Fixture integrity

Reuse `internal/evalcorpus` or its actual equivalent. Every case should preserve:

- Repository identity and immutable revision.
- Failing test or exact injected overlay bytes and SHA256.
- Relevant toolchain/environment.
- Failure output and reproducibility metadata.
- Ground-truth defect operation/mechanism, with provenance for that label.
- Whether the case was development-exposed or independent.
- Base/head revisions when applicable.
- Whether the case is flaky or inconclusive.

Do not reconstruct missing overlays from memory. Mark unfaithful historical cases unusable for exact paired evaluation.

### F2. Holdout collection

Collect a practical initial set of unfamiliar real-repository failures with independently reviewable defect origins, using an approved environment with sufficient disk. Prefer diversity of mechanisms, packages, assertion failures, panics, conversions, error handling, and control flow.

A useful **target** is 15–30 independently grounded cases if resources permit; this is a planning target, **not** a requirement to invent cases or block forever. Even a smaller genuinely independent sample is more informative than repeatedly tuning the same six cases.

Keep independent holdouts frozen. Do not inspect their ground truth to tune the engine before scoring. If a holdout is used for development, reclassify it as development-exposed and replace it for future independent assessment.

### F3. Paired experiments

Compare baseline versus the integrated new engine on identical inputs and environment. Report separately:

- Exact-function localization accuracy.
- Exact defect-operation localization accuracy.
- Mechanism classification accuracy, where labeled.
- Top-1 and top-k investigation candidate quality.
- Misleading-primary rate.
- Abstention rate.
- Verified-cause precision and coverage.
- Hypothesis calibration/evidence quality where measurable.
- Runtime, peak memory, indexing overhead, and timeouts.
- Case-level wins, losses, unchanged outcomes, and inconclusive cases.

**Never report 6/6 function accuracy as 6/6 root-cause accuracy.**

Show raw case-level outputs and denominator definitions. Avoid broad claims from small samples.

### F4. Regression gates

Block or revert changes that materially increase misleading primary diagnoses or fabricate verified causes. A more informative explanation alone is not enough to justify degraded accuracy.

If independent evaluation is impossible because of infrastructure, create a reproducible CI/runner workflow and a complete **BLOCKED** report; do not claim independent validation completed. Continue all feasible local tasks.

---

## 9. Phase G — production hardening and launch readiness

Only after core changes are implemented and evaluated:

- Confirm deterministic outputs, stable evidence IDs, and backward-compatible report serialization where needed.
- Ensure no crash on incomplete source, build failures, missing SSA, missing base revision, unindexed dependencies, or ambiguous callers.
- Bound time/memory and sandbox side effects; test cancellation and partial results.
- Confirm no hidden network or unsafe execution requirements in default CI mode.
- Verify GitHub PR comments and agent handoff distinguish observed failures, hypotheses, candidate leads, and verified root causes.
- Prevent unsupported wording such as “root cause confirmed” for inferred patterns.
- Add concise user-facing documentation with a realistic capabilities/limitations matrix.
- Test `go test ./...`, `go vet ./...` (or repo's equivalent), formatting, relevant race tests where feasible, and reproducible evaluation scripts.
- Check migration/backward compatibility for report consumers.
- Audit user-visible latency and peak resource use on realistic cases.

Do not expand scope into UI/design or unrelated refactors.

---

## 10. Explicit adversarial test matrix

Use these as a minimum source of scenarios; add real regressions discovered during implementation.

### Control/data flow

1. Assignment on the executed path causes bad value.
2. Assignment in dead/unrelated branch must not be promoted.
3. Assignment overwritten before failure.
4. Multiple reaching definitions with unresolved branch.
5. Phi-node merge where only one input is bad.
6. Dominating guard with early return.
7. Non-dominating guard.
8. `<= len(items)` permits out-of-range access.
9. Correct `< len(items)` but upstream index is invalid.
10. `len(other)` is unrelated to indexed collection.
11. Pointer/interface typed-nil ambiguity.
12. Valid nil result handled by contract.
13. Nil dereference in a callee with defective caller.
14. Nil dereference in a callee with defective producer.
15. Ignored error actually relevant to consumed value.
16. Ignored error intentionally harmless by contract.
17. Conversion overflow/narrowing supported by concrete values.
18. Conversion syntactically present but safe for observed values.
19. Aliasing, fields, maps, slices, and closure capture (support or abstain).
20. Missing SSA or ambiguous AST-to-SSA mapping.

### Invocation and provenance

21. Multiple static callers, one observed caller.
22. Multiple callers without distinguishing runtime evidence.
23. Observed caller but multiple calls on same source line.
24. Receiver vs ordinary argument indexing.
25. Variadic parameter ambiguity.
26. Recursive/wrapper chain under budget.
27. Interface dispatch without concrete target.
28. Unindexed cross-package producer.
29. Caller-side origin should not be promoted solely by static possibility.
30. Missing/inlined frames must not become invented observations.

### Diff and verification

31. Changed defect operation on supported causal path.
32. Changed but unrelated nearby code.
33. Unchanged preexisting defect.
34. Missing base revision.
35. Renamed file and shifted line numbers.
36. Patch eliminates failure but only masks symptom.
37. Flaky baseline, nondeterministic replay.
38. Two independent causes where fixing one changes failure mode.
39. Repeated tests with different runtime environment.
40. Verification attempt fails to reproduce baseline.

For every test, assert **both** the positive evidence produced and the unsupported conclusion that must **not** be produced.

---

## 11. Engineering quality and decision rules

- Prefer small, coherent production changes over one giant rewrite.
- Inspect existing abstractions before adding new structs, indices, or packages.
- Avoid large AST walks repeated per hypothesis when a reusable index already exists.
- Maintain bounded complexity and profile expensive paths.
- Do not change root-cause labels, confidence, or ranking merely to make benchmarks look better.
- Preserve machine-readable provenance, not only prose explanations.
- Treat missing evidence as missing, not as negative proof.
- Clearly distinguish **may reach**, **did execute**, **changed in revision**, **intervention removes failure**, and **verified root cause**.
- If the strongest evidence supports a candidate outside the observed failure frame, permit investigation of it under explicit admission rules; do not assume the deepest or earliest operation is automatically defective.
- Add focused negative controls whenever a detector is made more permissive.
- If an experimental verifier cannot be made sound, keep it as a diagnostic check, not a verification gate.
- Keep a clear baseline and avoid leaking evaluation labels into algorithm design.

---

## 12. Final acceptance checklist

Do not declare the mission complete until every item below is either **DONE** with supporting evidence or **BLOCKED** with a concrete reason and next action. “Blocked” is not the same as completed.

- [ ] Actual architecture audited and documented.
- [ ] Baseline tests/evaluation captured.
- [ ] Control-flow/SSA feasibility integrated where available, with safe fallback.
- [ ] Type-aware missing-guard and relevant negative controls implemented.
- [ ] Base/head delta correlation implemented with accurate line mapping.
- [ ] Defect hypotheses linked to provenance, feasibility, deltas, and falsification checks.
- [ ] Candidate admission/ranking reviewed for evidence-based caller-side leads.
- [ ] Targeted runtime observation/experiment support implemented where feasible.
- [ ] Verification gates preserved or narrowly extended with proof and negative controls.
- [ ] Major measured coverage gaps addressed or explicitly deferred with evidence.
- [ ] Existing six-case paired evaluation rerun and case-level differences reported.
- [ ] Independent evaluation completed **or explicitly marked blocked**, with runnable collection/evaluation workflow.
- [ ] No misleading promotions or unsupported verified-cause claims.
- [ ] Production tests, formatting, vet, and relevant performance checks pass.
- [ ] PR-comment/report wording reflects evidence strength.
- [ ] Final reports, limitations, reproducibility instructions, and next steps written.

### Completion semantics

Use one of these final outcomes, accurately:

**IMPLEMENTED AND INDEPENDENTLY VALIDATED:** all core work complete, independent tests run, metrics and limitations published.

**IMPLEMENTED, INDEPENDENT VALIDATION BLOCKED:** core work complete, independent evaluation genuinely blocked, exact runnable next steps provided. Do not claim production-ready root-cause accuracy.

**PARTIALLY IMPLEMENTED / BLOCKED:** material engineering work or proof requirements remain. Enumerate unfinished tasks precisely.

Do not use “finished” to conceal unresolved blockers. Do not continue endlessly attempting an impossible external dependency.

---

## 13. Required final response to the user

When the work is genuinely at a stopping point, deliver a single detailed report:

1. **What changed** — concrete files, functions, architecture reuse.
2. **What Ceaser can now establish** — with exact evidence semantics.
3. **What remains hypotheses** — and why.
4. **What was verified** — mechanism, method, controls, evidence IDs.
5. **Before/after results** — six familiar cases and separate independent cases.
6. **Misleading diagnoses/regressions** — include failures, not just successes.
7. **Runtime and resource impact**.
8. **Tests and commands run** — with pass/fail status.
9. **Remaining blockers and limitations** — exact next steps.
10. **Completion status** — one of the three defined outcomes.

Also update `evaluation/final-mission/PROGRESS.md` with the final state so a future session can resume without another user prompt.

---

## 14. START HERE — instruction to OpenCode

**Read this file fully, then begin Phase 2.1 immediately.**

Do not answer with only a plan. Audit the real repository, establish the baseline, create the persistent progress files, and implement the phases in dependency order. Continue autonomously through testing, correction, evaluation, and documentation. After each context reset, re-read this file and the progress log. Do not request a new prompt at the end of each phase.

**The objective is an evidence-grounded causal diagnosis engine that knows when it has identified a root cause — and knows when it has not.**
