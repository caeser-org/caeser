package diagnosis

import (
	"strings"
	"testing"

	"github.com/caeser-org/ceaser/internal/domain"
	programrepo "github.com/caeser-org/ceaser/internal/repository"
)

// causalFixture is a generic, non-project-specific panic failure: a panic names
// a qualified operation whose method call is visible in the suspect's source
// excerpt, but the failure is not asserted anywhere.
func causalFixture() Report {
	shortlist := CandidateReport{Status: "available", Entries: []CandidateReportEntry{
		{File: "widget.go", Function: "WidgetConvert", Line: 37, Kind: "observed_stack",
			Reason: "location observed in the failure evidence", EvidenceIDs: []string{"E-frame"}},
		{File: "widget.go", Function: "helper", Line: 80, Kind: "inferred_call",
			Reason: "candidate inferred from a resolved call relationship", EvidenceIDs: []string{"E-call"}},
	}}
	log := strings.Join([]string{
		"--- FAIL: TestWidgetConvert (0.00s)",
		"    --- FAIL: TestWidgetConvert/composed (0.00s)",
		"panic: widget: call of widget.Value.Convert on zero Value [recovered]",
		"\tpanic: widget: call of widget.Value.Convert on zero Value",
		"FAIL\twidget.example/pkg\t0.005s",
		"FAIL",
	}, "\n")
	excerpt := strings.Join([]string{
		"checked-out source widget.go",
		"37| func WidgetConvert() ConvertFunc {",
		"38| \treturn func(v Value) (any, error) {",
		"39| \t\tif v.Kind() != Struct {",
		"40| \t\t\treturn v.Convert(), nil",
		"41| \t\t}",
	}, "\n")
	return Report{
		CandidateReport: &shortlist,
		Evidence: []EvidenceItem{
			{ID: "E-ci", Kind: "ci_log", Source: "ci", Detail: log},
			{ID: "E-source", Kind: "source_excerpt", Source: "widget.go", Detail: excerpt},
		},
	}
}

func TestCausalExplanationExtractsObservedFailure(t *testing.T) {
	exp := BuildCausalExplanation(causalFixture())
	if exp.Status != "hypothesis" {
		t.Fatalf("expected a hypothesis, got %q: %+v", exp.Status, exp)
	}
	if exp.Failure == nil || !strings.Contains(exp.Failure.Panic, "widget.Value.Convert") {
		t.Fatalf("panic semantics not extracted: %+v", exp.Failure)
	}
	if exp.Failure.Test != "TestWidgetConvert" || exp.Failure.Subtest != "composed" {
		t.Fatalf("test/subtest context lost: %+v", exp.Failure)
	}
	if exp.Observed == "" {
		t.Fatal("observed failure summary missing")
	}
}

func TestCausalExplanationLinksFailureToSuspectOperation(t *testing.T) {
	exp := BuildCausalExplanation(causalFixture())
	if exp.Suspect != "WidgetConvert" {
		t.Fatalf("wrong suspect: %q", exp.Suspect)
	}
	if !strings.Contains(exp.Hypothesis, "WidgetConvert") {
		t.Fatalf("hypothesis does not name the suspect: %q", exp.Hypothesis)
	}
	// The concrete operation must be attributed to the suspect's source.
	if !strings.Contains(exp.Hypothesis, "v.Convert()") || !strings.Contains(exp.Hypothesis, "widget.go:40") {
		t.Fatalf("hypothesis does not cite the concrete operation: %q", exp.Hypothesis)
	}
	if !strings.Contains(exp.Hypothesis, "not established") {
		t.Fatalf("hypothesis presented as established: %q", exp.Hypothesis)
	}
	if !strings.Contains(exp.Unresolved, "invariant") {
		t.Fatalf("unresolved question missing for a panic: %q", exp.Unresolved)
	}
	if len(exp.Checks) == 0 {
		t.Fatal("no investigation checks derived")
	}
	for _, check := range exp.Checks {
		if check.Target == "" || check.EvidenceNeeded == "" || check.ConfirmsIf == "" || check.RejectsIf == "" {
			t.Fatalf("check is incomplete: %+v", check)
		}
	}
	if len(exp.EvidenceIDs) == 0 {
		t.Fatal("explanation carries no evidence ID")
	}
}

func TestCausalExplanationUsesSSAOperation(t *testing.T) {
	report := causalFixture()
	report.SSAAssertionTrace = &programrepo.SSATrace{
		Status:   "traced",
		AnchorID: "assert",
		Values: []programrepo.SSAValue{
			{ID: "call", Kind: "*ssa.Call", File: "widget.go", Line: 40,
				Function: "widget.example/pkg.WidgetConvert", Callee: "widget.example/pkg.Convert", Text: "t0 = Convert(v)"},
		},
	}
	exp := BuildCausalExplanation(report)
	if exp.Status != "hypothesis" {
		t.Fatalf("expected hypothesis: %+v", exp)
	}
	if !strings.Contains(exp.Hypothesis, "Convert(v)") {
		t.Fatalf("SSA operation not used: %q", exp.Hypothesis)
	}
	foundSSACheck := false
	for _, check := range exp.Checks {
		if strings.Contains(check.Target, "SSA operand path") {
			foundSSACheck = true
		}
	}
	if !foundSSACheck {
		t.Fatalf("no SSA-specific check: %+v", exp.Checks)
	}
}

func TestCausalExplanationAbstainsWithoutSuspect(t *testing.T) {
	report := Report{
		Localization: Localization{LikelyFiles: []string{"pkg/a.go", "pkg/b.go"}},
		Evidence:     []EvidenceItem{{ID: "e1", Source: "pkg/a.go", Kind: "source_excerpt", Detail: "1| package a"}},
	}
	exp := BuildCausalExplanation(report)
	if exp.Status != "unavailable" || exp.Hypothesis != "" || len(exp.Checks) != 0 {
		t.Fatalf("no suspect must abstain from a causal hypothesis: %+v", exp)
	}
	if len(exp.Limitations) == 0 {
		t.Fatal("abstention should record why")
	}
	refused := causalFixture()
	refused.Refused = true
	if got := BuildCausalExplanation(refused); got.Status != "unavailable" || got.Hypothesis != "" {
		t.Fatalf("refused diagnosis must not produce a hypothesis: %+v", got)
	}
}

func TestCausalExplanationDoesNotPromoteRootCause(t *testing.T) {
	report := causalFixture()
	report.FailureIncidents = []FailureIncident{{
		ID: "incident", Category: "test_failure_observed",
		ObservedLocations: []IncidentLocation{{File: "widget.go", Line: 37, Function: "WidgetConvert", Provenance: "direct_stack_frame"}},
		EvidenceIDs:       []string{"E-ci"},
	}}
	before := len(report.Evidence)
	exp := BuildCausalExplanation(report)
	report.CausalExplanation = &exp
	if len(report.Evidence) != before {
		t.Fatalf("causal explanation mutated report evidence: %d -> %d", before, len(report.Evidence))
	}
	if status := assessRootCause(report).Status; status == "established" || status == "confirmed" {
		t.Fatalf("causal explanation promoted a root cause: %s", status)
	}
}

// negative test: the stack-top function is not the culprit. The explanation must
// still be labeled an unverified hypothesis and must not claim the frame caused
// the failure.
func TestCausalExplanationNegativeStackTopNotCulprit(t *testing.T) {
	report := causalFixture()
	// The observed stack frame is a shallow wrapper; the value path goes deeper.
	report.CandidateReport = &CandidateReport{Status: "available", Entries: []CandidateReportEntry{
		{File: "widget.go", Function: "wrapper", Line: 12, Kind: "observed_stack",
			Reason: "location observed in the failure evidence", EvidenceIDs: []string{"E-frame"}},
		{File: "widget.go", Function: "WidgetConvert", Line: 37, Kind: "inferred_source",
			Reason: "function lies on the failed assertion's operand dependency path", EvidenceIDs: []string{"E-ssa"}},
	}}
	// The operand dependency path, not the shallow stack frame, is where the
	// failing value is produced.
	report.SSAAssertionTrace = &programrepo.SSATrace{
		Status: "traced", AnchorID: "assert",
		Values: []programrepo.SSAValue{
			{ID: "call", Kind: "*ssa.Call", File: "widget.go", Line: 40,
				Function: "widget.example/pkg.WidgetConvert", Callee: "widget.example/pkg.Convert"},
		},
	}
	exp := BuildCausalExplanation(report)
	if exp.Status != "hypothesis" {
		t.Fatalf("expected hypothesis: %+v", exp)
	}
	// The wrapper is stack-top but the operand-correlated function is primary.
	if exp.Suspect != "WidgetConvert" {
		t.Fatalf("expected the operand-correlated suspect, got %q", exp.Suspect)
	}
	if strings.Contains(strings.ToLower(exp.Hypothesis), "root cause is") {
		t.Fatalf("hypothesis asserted a root cause: %q", exp.Hypothesis)
	}
}

func TestExtractFailureFactsAssertionExpectedActual(t *testing.T) {
	log := strings.Join([]string{
		"--- FAIL: TestNumbers (0.00s)",
		"    numbers_test.go:42: ",
		"        \tError:   \tNot equal:",
		"        \texpected: 7",
		"        \tactual  : -9223372036854775808",
	}, "\n")
	report := Report{Evidence: []EvidenceItem{{ID: "E-ci", Kind: "ci_log", Detail: log}}}
	facts := extractFailureFacts(report)
	if facts.Test != "TestNumbers" {
		t.Fatalf("test not extracted: %+v", facts)
	}
	if facts.Expected != "7" || facts.Actual != "-9223372036854775808" {
		t.Fatalf("expected/actual not extracted: %+v", facts)
	}
	if !strings.Contains(facts.Message, "expected 7") {
		t.Fatalf("composed message missing expected/actual: %q", facts.Message)
	}
}

func TestFailureOperationTokensAreQualifiedOnly(t *testing.T) {
	report := Report{Evidence: []EvidenceItem{{ID: "E-ci", Kind: "ci_log", Detail: strings.Join([]string{
		"--- FAIL: TestX (0.00s)",
		"panic: reflect: call of reflect.Value.Interface on zero Value",
	}, "\n")}}}
	tokens := failureOperationTokens(report)
	if len(tokens) == 0 {
		t.Fatal("no operation tokens extracted from the panic")
	}
	for _, token := range tokens {
		if !strings.Contains(token, ".") {
			t.Fatalf("unqualified token admitted: %q", token)
		}
	}
}

// The failure text naming a qualified operation and the suspect's source
// calling that operation is a textual correlation between two observed
// artifacts. It must be surfaced as a hypothesis, not a data-flow claim.
func TestCausalExplanationCorrelatesFailureTokenWithSourceCall(t *testing.T) {
	report := Report{
		CandidateReport: &CandidateReport{Status: "available", Entries: []CandidateReportEntry{
			{File: "hooks.go", Function: "convertHook", Line: 380, Kind: "observed_stack",
				Reason: "location observed in the failure evidence", EvidenceIDs: []string{"E-frame"}},
		}},
		Evidence: []EvidenceItem{
			{ID: "E-ci", Kind: "ci_log", Detail: strings.Join([]string{
				"--- FAIL: TestHook (0.00s)",
				"panic: reflect: call of reflect.Value.Interface on zero Value [recovered]",
			}, "\n")},
			{ID: "E-source", Kind: "source_excerpt", Source: "hooks.go", Detail: strings.Join([]string{
				"checked-out source hooks.go",
				"380| func convertHook() HookFunc {",
				"381| \treturn func(f Value, t Value) (any, error) {",
				"382| \t\tif f.Kind() != Struct {",
				"383| \t\t\treturn f.Interface(), nil",
				"384| \t\t}",
			}, "\n")},
		},
	}
	exp := BuildCausalExplanation(report)
	if !strings.Contains(exp.Hypothesis, "reflect.Value.Interface") {
		t.Fatalf("failure token not correlated: %q", exp.Hypothesis)
	}
	if !strings.Contains(exp.Hypothesis, "f.Interface()") || !strings.Contains(exp.Hypothesis, "hooks.go:383") {
		t.Fatalf("correlated source call not cited: %q", exp.Hypothesis)
	}
	if !strings.Contains(exp.Hypothesis, "not established") {
		t.Fatalf("correlation presented as established: %q", exp.Hypothesis)
	}
}

func TestCausalExplanationOperationSupport(t *testing.T) {
	// SSA on the assertion path is typed evidence.
	ssa := causalFixture()
	ssa.SSAAssertionTrace = &programrepo.SSATrace{Status: "traced", AnchorID: "a", Values: []programrepo.SSAValue{
		{ID: "call", Kind: "*ssa.Call", File: "widget.go", Line: 40, Function: "widget.example/pkg.WidgetConvert", Text: "t0 = Convert(v)"},
	}}
	if got := BuildCausalExplanation(ssa); got.OperationSupport != "evidence_supported" {
		t.Fatalf("SSA operation not classified as supported: %+v", got)
	}

	// The suspect is a type-resolved failing-test path target: the cited
	// operation is supported by the call path even without SSA.
	path := causalFixture()
	path.FailureEpisodes = []domain.FailureEpisode{{TestIdentifier: "TestWidgetConvert"}}
	path.ExecutionPaths = []domain.ExecutionPath{{Status: "inferred_call_path", Locations: []domain.CodeLocation{
		{FilePath: "widget_test.go", SymbolIdentity: "go:widget_test.go:Test:TestWidgetConvert"},
		{FilePath: "widget.go", SymbolIdentity: "go:widget.go:Function:WidgetConvert"},
	}}}
	if got := BuildCausalExplanation(path); got.OperationSupport != "evidence_supported" {
		t.Fatalf("path target operation not classified as supported: %q", got.OperationSupport)
	}

	// No concrete operation at all: the explanation must require investigation.
	none := Report{CandidateReport: &CandidateReport{Status: "available", Entries: []CandidateReportEntry{
		{File: "widget.go", Function: "WidgetConvert", Line: 37, Kind: "inferred_call", Reason: "candidate inferred from a resolved call relationship"},
	}}}
	if got := BuildCausalExplanation(none); got.OperationSupport != "requires_investigation" {
		t.Fatalf("missing operation not flagged: %q", got.OperationSupport)
	}
}

func TestCausalExplanationDoesNotIncreaseConfidence(t *testing.T) {
	report := causalFixture()
	report.Confidence = 0.73
	report.Refused = false
	exp := BuildCausalExplanation(report)
	report.CausalExplanation = &exp
	if report.Confidence != 0.73 {
		t.Fatalf("explanation changed confidence: %v", report.Confidence)
	}
	if status := assessRootCause(report).Status; status == "established" || status == "confirmed" {
		t.Fatalf("explanation promoted a root cause: %s", status)
	}
}

// A suspect that encloses an observed failure frame has its cited operation
// supported by execution evidence, not merely associated.
func TestCausalExplanationObservedFrameIsSupported(t *testing.T) {
	report := Report{
		CandidateReport: &CandidateReport{Status: "available", Entries: []CandidateReportEntry{
			{File: "widget.go", Function: "WidgetConvert", Line: 37, Kind: "observed_stack",
				Reason: "location observed in the failure evidence", EvidenceIDs: []string{"E-frame"}},
		}},
		Evidence: []EvidenceItem{
			{ID: "E-frame", Kind: "execution_stack_frame", Source: "widget.go",
				Detail: "CI execution stack frame observed at /src/widget.go:38; this establishes a reported location, not causality"},
		},
	}
	exp := BuildCausalExplanation(report)
	if exp.OperationSupport != "evidence_supported" {
		t.Fatalf("observed frame operation not supported: %q", exp.OperationSupport)
	}
	if !strings.Contains(exp.Hypothesis, "not established") {
		t.Fatalf("supported operation presented as established: %q", exp.Hypothesis)
	}
}
