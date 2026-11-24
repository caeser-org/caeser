package diagnosis

import (
	"strings"
	"testing"

	"github.com/caeser-org/ceaser/internal/domain"
)

func candidateReportFixture() Report {
	return Report{
		FailureIncidents: []FailureIncident{{
			ID: "incident",
			ObservedLocations: []IncidentLocation{{
				File: "pkg/handler/patch.go", Line: 41, Function: "writeChunk",
				Provenance: "direct_stack_frame",
			}},
			EvidenceIDs: []string{"E-stack"},
		}},
		Localization: Localization{LikelyFiles: []string{"pkg/handler/context.go", "pkg/handler/unrouted.go"}},
		Evidence:     []EvidenceItem{{ID: "E-source", Source: "pkg/handler/context.go", Kind: "source_excerpt"}},
		Candidates: []domain.CandidateLocation{
			{Location: domain.CodeLocation{
				FilePath: "mcp/tools.go", SymbolIdentity: "go:pkg:Function:CallToolRequest.GetInt",
				StartLine: 255, LocationType: "Function", Provenance: "verified_call",
			}, SupportingEvidenceIDs: []domain.ID{"E-call", "E-ssa"}},
		},
	}
}

func TestCandidateReportSeparatesObservedFromInferred(t *testing.T) {
	got := BuildCandidateReport(candidateReportFixture())
	if got.Status != "available" || got.ProbeStatus != "disabled" || len(got.Entries) < 3 {
		t.Fatalf("shortlist not available: %#v", got)
	}
	kinds := map[string]CandidateReportEntry{}
	for _, entry := range got.Entries {
		kinds[entry.Kind] = entry
		if entry.Rank == 0 || entry.File == "" || entry.Reason == "" {
			t.Fatalf("incomplete entry: %#v", entry)
		}
	}
	observed, ok := kinds["observed_stack"]
	if !ok || observed.Line != 41 || observed.Function != "writeChunk" || !containsString(observed.EvidenceIDs, "E-stack") {
		t.Fatalf("observed stack location lost: %#v", observed)
	}
	inferred, ok := kinds["inferred_call"]
	if !ok || inferred.File != "mcp/tools.go" || inferred.Line != 255 || inferred.Function != "CallToolRequest.GetInt" {
		t.Fatalf("inferred call entry lost: %#v", inferred)
	}
	if !containsString(inferred.EvidenceIDs, "E-call") || !containsString(inferred.EvidenceIDs, "E-ssa") {
		t.Fatalf("supporting evidence not referenced: %#v", inferred)
	}
	var localized *CandidateReportEntry
	for i := range got.Entries {
		if got.Entries[i].File == "pkg/handler/context.go" {
			localized = &got.Entries[i]
		}
	}
	if localized == nil || !containsString(localized.EvidenceIDs, "E-source") {
		t.Fatalf("localized fallback lost evidence reference: %#v", localized)
	}
}

func TestCandidateReportUncorrelatedProbeStaysALead(t *testing.T) {
	report := candidateReportFixture()
	report.MechanismObservation = &RuntimeMechanismObservation{
		Status:         "observed",
		OperationFile:  "mcp/tools.go",
		OperationLine:  255,
		OperationName:  "CallToolRequest.GetInt",
		SignatureMatch: true,
		// Captured values are not bound to the failing subtest.
		Correlation: &AssertionCorrelation{Verified: false, Test: "TestCallToolRequest_IntegerOutOfRange"},
		Limitations: []string{"values not correlated"},
	}
	got := BuildCandidateReport(report)
	var probe *CandidateReportEntry
	for i := range got.Entries {
		if got.Entries[i].Kind == "probe_operation" {
			probe = &got.Entries[i]
		}
	}
	if probe == nil || probe.Line != 255 || !strings.Contains(probe.Reason, "not correlated") {
		t.Fatalf("uncorrelated probe not presented as a lead: %#v", got)
	}
	if got.ProbeStatus != "observed" || len(got.Limitations) == 0 {
		t.Fatalf("probe limitations missing: %#v", got)
	}
	// The candidate tier is presentation only: promotion requirements unchanged.
	report.FailureIncidents[0].EvidenceIDs = append(report.FailureIncidents[0].EvidenceIDs, got.Entries[0].EvidenceIDs...)
	if status := assessRootCause(report).Status; status == "established" || status == "confirmed" {
		t.Fatalf("candidate tier promoted root cause: %s", status)
	}
}

func TestCandidateReportFallbackWhenProbingStops(t *testing.T) {
	report := Report{
		Localization:         Localization{LikelyFiles: []string{"a.go", "b.go"}},
		MechanismObservation: &RuntimeMechanismObservation{Status: "unavailable"},
	}
	got := BuildCandidateReport(report)
	if got.Status != "available" || got.ProbeStatus != "unavailable" || len(got.Entries) != 2 {
		t.Fatalf("shortlist should survive unavailable probe: %#v", got)
	}
	for _, entry := range got.Entries {
		if entry.Kind != "inferred_source" || entry.File == "" {
			t.Fatalf("unexpected fallback entry: %#v", entry)
		}
	}
	empty := BuildCandidateReport(Report{})
	if empty.Status != "unavailable" || empty.Reason == "" || len(empty.Entries) != 0 {
		t.Fatalf("empty report should be typed unavailable: %#v", empty)
	}
}

func TestCandidateReportBoundedAndDeduplicated(t *testing.T) {
	report := Report{}
	for i := 0; i < 9; i++ {
		report.Candidates = append(report.Candidates, domain.CandidateLocation{
			Location: domain.CodeLocation{FilePath: "pkg/f.go", StartLine: i + 1, LocationType: "Function", Provenance: "reference"},
		})
	}
	report.Candidates = append(report.Candidates, domain.CandidateLocation{
		Location: domain.CodeLocation{FilePath: "pkg/f.go", StartLine: 1, LocationType: "Function", Provenance: "reference"},
	})
	got := BuildCandidateReport(report)
	if len(got.Entries) != candidateReportLimit {
		t.Fatalf("shortlist not bounded: %#v", got.Entries)
	}
	for i, entry := range got.Entries {
		if entry.Rank != i+1 {
			t.Fatalf("ranks not sequential: %#v", got.Entries)
		}
	}
	if len(boundedEvidenceIDs([]string{"a", "a", "b", "c", "d"}, 3)) != 3 {
		t.Fatal("evidence ids not deduplicated and bounded")
	}
}

// A function the named failing test reaches through a type-resolved call path is
// stronger evidence than an equally-scored sibling reached only by fan-out. The
// two candidates here are identical except for that path membership.
func TestCandidateReportPrefersFailingTestVerifiedPathTarget(t *testing.T) {
	report := Report{
		FailureEpisodes: []domain.FailureEpisode{{TestIdentifier: "TestSettings"}},
		ExecutionPaths: []domain.ExecutionPath{{
			Status: "inferred_call_path",
			Locations: []domain.CodeLocation{
				{FilePath: "props_test.go", SymbolIdentity: "go:props_test.go:Test:TestSettings"},
				{FilePath: "props.go", SymbolIdentity: "go:props.go:Method:CreateSettings"},
			},
		}},
		Candidates: []domain.CandidateLocation{
			{Location: domain.CodeLocation{FilePath: "props.go", SymbolIdentity: "go:props.go:Method:CreateRouteSettings", StartLine: 20, Provenance: "inferred_caller", LocationType: "Method"}},
			{Location: domain.CodeLocation{FilePath: "props.go", SymbolIdentity: "go:props.go:Method:CreateSettings", StartLine: 40, Provenance: "inferred_caller", LocationType: "Method"}},
		},
	}
	got := BuildCandidateReport(report)
	if len(got.Entries) == 0 || got.Entries[0].Function != "CreateSettings" {
		t.Fatalf("verified failing-test path target not ranked first: %#v", got.Entries)
	}
	report.CandidateReport = &got
	likely := BuildLikelyFunction(report)
	if likely.Primary == nil || likely.Primary.Function != "CreateSettings" {
		t.Fatalf("likely-function tier did not follow the evidence order: %#v", likely.Primary)
	}
}

// Observed execution evidence must outrank a failing-test path target, so the
// innermost frame keeps the lead when both exist.
func TestCandidateReportKeepsObservedFrameAheadOfPathTarget(t *testing.T) {
	report := Report{
		FailureEpisodes: []domain.FailureEpisode{{TestIdentifier: "TestSettings"}},
		ExecutionPaths: []domain.ExecutionPath{{
			Status: "inferred_call_path",
			Locations: []domain.CodeLocation{
				{FilePath: "props_test.go", SymbolIdentity: "go:props_test.go:Test:TestSettings"},
				{FilePath: "props.go", SymbolIdentity: "go:props.go:Method:CreateSettings"},
			},
		}},
		Candidates: []domain.CandidateLocation{
			{Location: domain.CodeLocation{FilePath: "props.go", SymbolIdentity: "go:props.go:Function:parseFields", StartLine: 16, Provenance: "enclosing_symbol", LocationType: "Function"},
				Score: domain.ScoreVector{ExecutionRelevance: 1, AvailableDimensions: []string{"execution"}}},
			{Location: domain.CodeLocation{FilePath: "props.go", SymbolIdentity: "go:props.go:Method:CreateSettings", StartLine: 40, Provenance: "inferred_caller", LocationType: "Method"}},
		},
	}
	got := BuildCandidateReport(report)
	if len(got.Entries) == 0 || got.Entries[0].Function != "parseFields" {
		t.Fatalf("observed frame lost the lead: %#v", got.Entries)
	}
}

// Within observed execution evidence, the innermost reported frame must win.
// Both candidates enclose a frame in the same file; only the inner function
// encloses the deepest frame, which is where the failure actually happened.
func TestCandidateReportPrefersInnermostObservedFrame(t *testing.T) {
	report := Report{
		Candidates: []domain.CandidateLocation{
			{Location: domain.CodeLocation{FilePath: "x.go", SymbolIdentity: "go:x.go:Function:Outer", StartLine: 10, EndLine: 100, Provenance: "enclosing_symbol", LocationType: "Function"},
				Score: domain.ScoreVector{ExecutionRelevance: 1, AvailableDimensions: []string{"execution"}}},
			{Location: domain.CodeLocation{FilePath: "x.go", SymbolIdentity: "go:x.go:Function:Inner", StartLine: 40, EndLine: 55, Provenance: "enclosing_symbol", LocationType: "Function"},
				Score: domain.ScoreVector{ExecutionRelevance: 1, AvailableDimensions: []string{"execution"}}},
		},
		Evidence: []EvidenceItem{
			{ID: "E-frame-0", Kind: "execution_stack_frame", Source: "/toolchain/runtime/panic.go", Detail: "CI execution stack frame observed at /toolchain/runtime/panic.go:900; this establishes a reported location, not causality"},
			{ID: "E-frame-1", Kind: "execution_stack_frame", Source: "x.go", Detail: "CI execution stack frame observed at /src/x.go:45; this establishes a reported location, not causality"},
		},
	}
	got := BuildCandidateReport(report)
	if len(got.Entries) == 0 || got.Entries[0].Function != "Inner" {
		t.Fatalf("innermost observed frame did not win: %#v", got.Entries)
	}
}
