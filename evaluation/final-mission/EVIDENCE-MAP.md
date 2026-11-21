# Evidence-flow map (actual)

Producers → structures → consumers → gates. Confirmed against the checkout.

| Stage | Producer (file/func) | Structure | Consumed by | Verification gate |
|---|---|---|---|---|
| Failure parse | `evidence.go` (`goTestFailures`, `ParseCITextEvidence`) | `NormalizedEvidence`, `FailureEpisode` | classification, localization, probes | observation only |
| Classification | `classify.go:Classify` | `Report.Classification` | report, base/head verdict | none |
| Indexing | `repository/analyzer.go`, targeted (`pipeline.go:stageOverview`) | `Snapshot` nodes + `CALLS` edges (`go_types`,`call_lines`,`verified_static_call`) | candidate gen, tracer, SSA | none |
| SSA | `repository/go_ssa.go:TraceAssertion` | `SSAAssertionTrace` (values with `File,Line,Function,Type,Operands,Callee`) | ranking, probe selection, tracer | exact line only |
| Candidates | `localization/pipeline.go:Generate`, `candidate_report.go`, `likely_function.go` | `CandidateLocation`, `CandidateReport`, `LikelyFunction` | comment/brief; leads only | never a cause |
| Execution paths | `pipeline.go:buildExecutionPaths` | `ExecutionPath` | ranking tie-break | `verified_static_call` only |
| Backward trace | `causal_trace.go:buildFailureOriginTrace`→`traceCausalOriginFramed` | `CausalTrace{Edges,Producer,Supersedes,Defects}` | explanation, brief, `admitCausalProducer` | never verified |
| Argument provenance | `causal_trace.go:argumentOrigin`,`resolveParameterUpward` | `CausalTraceEdge` (`inferred`) | trace/explanation | never supersedes |
| Defect origins | `defect_origin.go:assessDefectOrigins` | `DefectOrigin` (`later_assignment`,`ignored_error`,`missing_guard`,`boundary_guard`) | explanation, brief | hypotheses only |
| Deltas | `pipeline.go` (`Compare`→`CodeDelta`) | `Report.Deltas` (SymbolIdentity, FilePath, HeadStartLine) | localization "changed_file" | not causation |
| Runtime probe | `runtime_observation.go` | `RuntimeMechanismObservation` | `assessRootCause` | gates A/B/C + SSA + correlation |
| Verification | `pipeline.go:assessRootCause`,`mechanismEvidenceVerified` | `RootCauseAssessment` | report | established only via concurrency contract or runtime mechanism |

## Evidence semantics (trust order, weakest→strongest)
`source_pattern` < `static_data_dependency` < `static_control_dependency` <
`possible_path` < `observed_invocation` < `verified causal mechanism`.

A stronger static relationship upgrades the **relationship evidence**, never the
hypothesis to verified root cause. `inferred` and `static` statuses are unchanged.
