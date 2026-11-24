package diagnosis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caeser-org/ceaser/internal/domain"
	programrepo "github.com/caeser-org/ceaser/internal/repository"
)

func defectFor(t *testing.T, src string, failLine int) []DefectOrigin {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	fn := domain.ProgramNode{ID: "go:a.go:Function:f", Type: "Function", Name: "f", Identity: "go:a.go:Function:f", FilePath: "a.go", StartLine: 3, EndLine: 60}
	return assessDefectOrigins(ws, fn, failLine, defectContext{})
}

func findDefect(defs []DefectOrigin, kind string) *DefectOrigin {
	for i := range defs {
		if defs[i].Kind == kind {
			return &defs[i]
		}
	}
	return nil
}

// A. Incorrect assignment followed by a failure.
func TestDefectLaterAssignment(t *testing.T) {
	src := "package pkg\n\nfunc f(limit int) {\n\tlimit = 0\n\tpanic(limit)\n}\n"
	defs := defectFor(t, src, 5)
	if findDefect(defs, "later_assignment") == nil {
		t.Fatalf("later assignment not detected: %+v", defs)
	}
}

// 2. Correct assignment followed by an unrelated failure must not be flagged.
func TestDefectUnrelatedAssignmentNotFlagged(t *testing.T) {
	src := "package pkg\n\nfunc f(other int) {\n\tx := 1\n\t_ = x\n\tpanic(other)\n}\n"
	defs := defectFor(t, src, 6)
	if findDefect(defs, "later_assignment") != nil {
		t.Fatalf("unrelated assignment flagged: %+v", defs)
	}
}

// B. Missing nil guard before a dereference.
func TestDefectMissingGuard(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f() string {\n\tu := loadUser()\n\treturn u.Name\n}\n"
	defs := defectFor(t, src, 7)
	if findDefect(defs, "missing_guard") == nil {
		t.Fatalf("missing guard not detected: %+v", defs)
	}
}

// 4. Valid nil handling must not be flagged.
func TestDefectPresentGuardNotFlagged(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f() string {\n\tu := loadUser()\n\tif u == nil {\n\t\treturn \"\"\n\t}\n\treturn u.Name\n}\n"
	defs := defectFor(t, src, 9)
	if findDefect(defs, "missing_guard") != nil {
		t.Fatalf("present nil guard flagged: %+v", defs)
	}
}

// C. Incorrect boundary comparison permits the failing index.
func TestDefectBoundaryComparison(t *testing.T) {
	src := "package pkg\n\nfunc f(items []int, i int) int {\n\tif i <= len(items) {\n\t\treturn items[i]\n\t}\n\treturn 0\n}\n"
	defs := defectFor(t, src, 5)
	if findDefect(defs, "boundary_guard") == nil {
		t.Fatalf("boundary comparison not detected: %+v", defs)
	}
}

// 6. A correct comparison with an invalid caller input must not be flagged as a
// boundary guard.
func TestDefectCorrectComparisonNotFlagged(t *testing.T) {
	src := "package pkg\n\nfunc f(items []int, i int) int {\n\tif i < len(items) {\n\t\treturn items[i]\n\t}\n\treturn 0\n}\n"
	defs := defectFor(t, src, 5)
	if findDefect(defs, "boundary_guard") != nil {
		t.Fatalf("correct strict comparison flagged: %+v", defs)
	}
}

// E. Discarded error result before the failing operation.
func TestDefectIgnoredError(t *testing.T) {
	src := "package pkg\n\nfunc f() int {\n\tv, _ := decode()\n\tpanic(v)\n}\n"
	defs := defectFor(t, src, 5)
	if findDefect(defs, "ignored_error") == nil {
		t.Fatalf("ignored error not detected: %+v", defs)
	}
}

// 9/10. Multiple reaching definitions (branch-dependent) still surface the
// later assignment as a hypothesis, not a chosen producer.
func TestDefectBranchDependentDefinitions(t *testing.T) {
	src := "package pkg\n\nfunc f(c bool) int {\n\tx := 0\n\tif c {\n\t\tx = 1\n\t}\n\treturn 10 / x\n}\n"
	defs := defectFor(t, src, 8)
	if findDefect(defs, "later_assignment") == nil {
		t.Fatalf("branch-dependent reassignment not surfaced: %+v", defs)
	}
}

// 16. No SSA available: defect assessment is AST-only and still works.
func TestDefectWorksWithoutSSA(t *testing.T) {
	src := "package pkg\n\nfunc f(limit int) {\n\tlimit = -1\n\tpanic(limit)\n}\n"
	defs := defectFor(t, src, 5)
	if len(defs) == 0 {
		t.Fatal("defect assessment requires no SSA and should still report the mutation")
	}
}

// 5/12. A clean function with no mutation or guard issue reports nothing.
func TestDefectCleanFunctionNoHypothesis(t *testing.T) {
	src := "package pkg\n\nfunc f(items []int, i int) int {\n\tif i < len(items) {\n\t\treturn items[i]\n\t}\n\treturn -1\n}\n"
	defs := defectFor(t, src, 5)
	if len(defs) != 0 {
		t.Fatalf("clean function produced hypotheses: %+v", defs)
	}
}

func defectForCtx(t *testing.T, src string, failLine int, ctx defectContext) []DefectOrigin {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	fn := domain.ProgramNode{ID: "go:a.go:Function:f", Type: "Function", Name: "f", Identity: "go:a.go:Function:f", FilePath: "a.go", StartLine: 3, EndLine: 60}
	return assessDefectOrigins(ws, fn, failLine, ctx)
}

// Phase A: a known non-nilable type must not be reported as a missing guard.
func TestMissingGuardTypeAwareNonNilable(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f() string {\n\tu := loadUser()\n\treturn u.Name\n}\n"
	ctx := defectContext{ssa: &programrepo.SSATrace{Values: []programrepo.SSAValue{{File: "a.go", Line: 6, Type: "pkg.T"}}}}
	defs := defectForCtx(t, src, 7, ctx)
	if findDefect(defs, "missing_guard") != nil {
		t.Fatalf("non-nilable typed operand flagged: %+v", defs)
	}
}

// Phase A: a known pointer type upgrades the relationship evidence.
func TestMissingGuardTypeAwareNilable(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f() string {\n\tu := loadUser()\n\treturn u.Name\n}\n"
	ctx := defectContext{ssa: &programrepo.SSATrace{Values: []programrepo.SSAValue{{File: "a.go", Line: 6, Type: "*pkg.T"}}}}
	defs := defectForCtx(t, src, 7, ctx)
	d := findDefect(defs, "missing_guard")
	if d == nil || d.EvidenceKind != "static_data_dependency" {
		t.Fatalf("nilable typed operand not upgraded: %+v", defs)
	}
}

// Phase A: unknown type stays a source pattern with explicit ambiguity.
func TestMissingGuardUnknownTypeStaysSourcePattern(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f() string {\n\tu := loadUser()\n\treturn u.Name\n}\n"
	defs := defectForCtx(t, src, 7, defectContext{})
	d := findDefect(defs, "missing_guard")
	if d == nil || d.EvidenceKind != "source_pattern" || len(d.Ambiguity) == 0 {
		t.Fatalf("unknown-type operand not marked as source pattern: %+v", defs)
	}
}

// Phase B: delta correlation classifies the defect location; it never promotes.
func TestDefectDeltaRelation(t *testing.T) {
	src := "package pkg\n\nfunc f(limit int) {\n\tlimit = 0\n\tpanic(limit)\n}\n"
	changed := defectForCtx(t, src, 5, defectContext{deltas: []domain.CodeDelta{{FilePath: "a.go", SymbolIdentity: "go:a.go:Function:f"}}})
	if d := findDefect(changed, "later_assignment"); d == nil || d.DeltaRelation != "changed" {
		t.Fatalf("changed symbol not correlated: %+v", changed)
	}
	sameFile := defectForCtx(t, src, 5, defectContext{deltas: []domain.CodeDelta{{FilePath: "a.go", SymbolIdentity: "go:a.go:Function:other"}}})
	if d := findDefect(sameFile, "later_assignment"); d == nil || d.DeltaRelation != "changed_file" {
		t.Fatalf("nearby change not correlated: %+v", sameFile)
	}
	unchanged := defectForCtx(t, src, 5, defectContext{deltas: []domain.CodeDelta{{FilePath: "b.go", SymbolIdentity: "go:b.go:Function:g"}}})
	if d := findDefect(unchanged, "later_assignment"); d == nil || d.DeltaRelation != "unchanged" {
		t.Fatalf("unchanged defect not classified: %+v", unchanged)
	}
}

// Phase A2 negative control: a nil guard in a non-dominating branch must not
// suppress the missing-guard hypothesis.
func TestMissingGuardNonDominatingNotSuppressed(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f(flag bool) string {\n\tu := loadUser()\n\tif flag {\n\t\tif u == nil {\n\t\t\treturn \"x\"\n\t\t}\n\t}\n\treturn u.Name\n}\n"
	defs := defectFor(t, src, 12)
	if findDefect(defs, "missing_guard") == nil {
		t.Fatalf("non-dominating guard wrongly suppressed the hypothesis: %+v", defs)
	}
}

// Phase A2: a `!= nil` guard whose body contains the dereference protects it.
func TestMissingGuardThenBodyProtects(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f() string {\n\tu := loadUser()\n\tif u != nil {\n\t\treturn u.Name\n\t}\n\treturn \"\"\n}\n"
	defs := defectFor(t, src, 8)
	if findDefect(defs, "missing_guard") != nil {
		t.Fatalf("dominating != nil guard not honored: %+v", defs)
	}
}

// Phase A2: branch-dependent definitions are represented as alternatives.
func TestLaterAssignmentBranchAlternatives(t *testing.T) {
	src := "package pkg\n\nfunc f(c bool) int {\n\tx := 0\n\tif c {\n\t\tx = 1\n\t} else {\n\t\tx = 2\n\t}\n\treturn 10 / x\n}\n"
	defs := defectFor(t, src, 10)
	d := findDefect(defs, "later_assignment")
	if d == nil || len(d.Ambiguity) == 0 {
		t.Fatalf("branch-dependent alternatives not recorded: %+v", defs)
	}
}

// Phase A2: an operation in an SSA-traced file that is not on the assertion
// dependency path is annotated (not suppressed).
func TestDefectNotOnSSACoveredPathAnnotated(t *testing.T) {
	src := "package pkg\n\ntype T struct{ Name string }\n\nfunc f() string {\n\tu := loadUser()\n\treturn u.Name\n}\n"
	ctx := defectContext{ssa: &programrepo.SSATrace{Values: []programrepo.SSAValue{{File: "a.go", Line: 3, Function: "pkg.f"}}}}
	defs := defectForCtx(t, src, 7, ctx)
	d := findDefect(defs, "missing_guard")
	if d == nil {
		t.Fatalf("hypothesis suppressed instead of annotated: %+v", defs)
	}
	found := false
	for _, a := range d.Ambiguity {
		if strings.Contains(a, "dependency path") {
			found = true
		}
	}
	if !found {
		t.Fatalf("off-path annotation missing: %+v", d)
	}
}

// Phase C: competing defect hypotheses are retained, not collapsed to one cause.
func TestExplanationRetainsCompetingHypotheses(t *testing.T) {
	report := Report{}
	report.CausalTrace = &CausalTrace{Status: "partial", Defects: []DefectOrigin{
		{Kind: "later_assignment", Operand: "x", File: "a.go", Line: 4, Rationale: "x reassigned"},
		{Kind: "boundary_guard", Operand: "i", File: "a.go", Line: 6, Rationale: "<= len guard"},
	}}
	report.CandidateReport = &CandidateReport{Status: "available", Entries: []CandidateReportEntry{{File: "a.go", Function: "f", Line: 3, Kind: "observed_stack", Reason: "observed"}}}
	lf := BuildLikelyFunction(report)
	report.LikelyFunction = &lf
	exp := BuildCausalExplanation(report)
	joined := strings.Join(exp.Limitations, " ")
	if !strings.Contains(joined, "competing defect hypotheses") {
		t.Fatalf("competing hypotheses not retained: %+v", exp.Limitations)
	}
	if exp.Status == "verified" {
		t.Fatal("hypotheses must not be verified")
	}
}

// Adversarial #10: a len() on an unrelated collection must not match.
func TestBoundaryGuardUnrelatedLenNotFlagged(t *testing.T) {
	src := "package pkg\n\nfunc f(items, other []int, i int) int {\n\tif i <= len(other) {\n\t\treturn items[i]\n\t}\n\treturn 0\n}\n"
	defs := defectFor(t, src, 5)
	if findDefect(defs, "boundary_guard") != nil {
		t.Fatalf("unrelated len() matched the boundary guard: %+v", defs)
	}
}

// The same collection is still matched.
func TestBoundaryGuardSameCollectionFlagged(t *testing.T) {
	src := "package pkg\n\nfunc f(items []int, i int) int {\n\tif i <= len(items) {\n\t\treturn items[i]\n\t}\n\treturn 0\n}\n"
	defs := defectFor(t, src, 5)
	if findDefect(defs, "boundary_guard") == nil {
		t.Fatalf("same-collection guard not matched: %+v", defs)
	}
}
