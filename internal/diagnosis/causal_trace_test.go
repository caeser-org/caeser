package diagnosis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/caeser-org/ceaser/internal/domain"
	"github.com/caeser-org/ceaser/internal/localization"
	programrepo "github.com/caeser-org/ceaser/internal/repository"
)

const traceSource = `package pkg

func Caller(x int) {
	v := Produce(x)
	panic(v)
}

func Produce(x int) int {
	y := Helper(x)
	return y
}

func Helper(x int) int {
	return Helper2(x)
}

func Helper2(x int) int {
	return 0
}

func Fallback(x int) int {
	return x
}
`

func traceWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "a.go"), []byte(traceSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func traceSnapshot(typedEdges bool, includeFallback bool) programrepo.Snapshot {
	nodes := []domain.ProgramNode{
		{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "pkg.Caller", FilePath: "pkg/a.go", StartLine: 3, EndLine: 6},
		{ID: "n-produce", Type: "Function", Name: "Produce", Identity: "pkg.Produce", FilePath: "pkg/a.go", StartLine: 8, EndLine: 11},
		{ID: "n-helper", Type: "Function", Name: "Helper", Identity: "pkg.Helper", FilePath: "pkg/a.go", StartLine: 13, EndLine: 15},
		{ID: "n-helper2", Type: "Function", Name: "Helper2", Identity: "pkg.Helper2", FilePath: "pkg/a.go", StartLine: 17, EndLine: 19},
	}
	var rels []domain.Relationship
	if typedEdges {
		rels = []domain.Relationship{
			{SourceID: "n-caller", TargetID: "n-produce", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",4,"}},
			{SourceID: "n-produce", TargetID: "n-helper", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",9,"}},
			{SourceID: "n-helper", TargetID: "n-helper2", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",14,"}},
		}
	} else {
		rels = []domain.Relationship{
			{SourceID: "n-caller", TargetID: "n-produce", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",4,"}},
			{SourceID: "n-produce", TargetID: "n-helper", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",9,"}},
		}
	}
	if includeFallback {
		nodes = append(nodes, domain.ProgramNode{ID: "n-fallback", Type: "Function", Name: "Fallback", Identity: "pkg.Fallback", FilePath: "pkg/a.go", StartLine: 21, EndLine: 23})
	}
	return programrepo.Snapshot{Nodes: nodes, Relationships: rels}
}

func anchorNode() domain.ProgramNode {
	return domain.ProgramNode{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "pkg.Caller", FilePath: "pkg/a.go", StartLine: 3, EndLine: 6}
}

func TestCausalTraceFollowsPanicValueToProducer(t *testing.T) {
	ws := traceWorkspace(t)
	trace := traceCausalOrigin(ws, traceSnapshot(true, true), anchorNode(), 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Status != "traced" {
		t.Fatalf("expected traced, got %q: %+v", trace.Status, trace)
	}
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Produce" {
		t.Fatalf("nearest producer must be Produce, got %+v", trace.Producer)
	}
	if len(trace.Edges) != 3 {
		t.Fatalf("expected the full chain recorded, got %d edges: %+v", len(trace.Edges), trace.Edges)
	}
	for _, edge := range trace.Edges {
		if edge.Status != causalTraceStatusStatic {
			t.Fatalf("verified edge not static: %+v", edge)
		}
	}
	if !trace.Supersedes {
		t.Fatal("a panic that propagates the producer's value must supersede the manifestation frame")
	}
}

// The producer is the nearest statically-resolved operation, not the deepest
// reachable function. Deeper nodes are retained as alternates.
func TestCausalTraceDoesNotPreferDeepest(t *testing.T) {
	ws := traceWorkspace(t)
	trace := traceCausalOrigin(ws, traceSnapshot(true, true), anchorNode(), 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) == "Helper2" {
		t.Fatalf("deepest function was preferred: %+v", trace.Producer)
	}
	last := trace.Edges[len(trace.Edges)-1]
	if shortFunctionName(last.To.Function) != "Helper2" {
		t.Fatalf("deeper node not retained in the chain: %+v", last)
	}
}

func TestCausalTraceUnindexedTargetIsUnresolved(t *testing.T) {
	ws := traceWorkspace(t)
	snap := traceSnapshot(false, false)
	// Remove the target node so the call cannot resolve.
	var keep []domain.ProgramNode
	for _, n := range snap.Nodes {
		if n.ID != "n-produce" {
			keep = append(keep, n)
		}
	}
	snap.Nodes = keep
	trace := traceCausalOrigin(ws, snap, anchorNode(), 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Producer != nil {
		t.Fatalf("unresolved target must not produce a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Status != causalTraceStatusUnres {
		t.Fatalf("expected an unresolved boundary edge: %+v", trace.Edges)
	}
	if trace.Status != "unresolved" {
		t.Fatalf("expected unresolved status: %q", trace.Status)
	}
}

// Same-package name resolution is conservative: it is inferred, not static, and
// it does not supersede the manifestation frame.
func TestCausalTraceSamePackageFallbackIsInferred(t *testing.T) {
	ws := traceWorkspace(t)
	// No typed edge from Caller to Helper; make Caller call Helper directly.
	src := `package pkg

func Caller(x int) {
	v := Helper(x)
	panic(v)
}

func Helper(x int) int {
	return x
}
`
	if err := os.WriteFile(filepath.Join(ws, "pkg", "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := programrepo.Snapshot{Nodes: []domain.ProgramNode{
		{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "pkg.Caller", FilePath: "pkg/a.go", StartLine: 3, EndLine: 6},
		{ID: "n-helper", Type: "Function", Name: "Helper", Identity: "pkg.Helper", FilePath: "pkg/a.go", StartLine: 8, EndLine: 10},
	}}
	trace := traceCausalOrigin(ws, snap, anchorNode(), 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != causalTraceTextualCall || trace.Edges[0].Status != causalTraceStatusInfer {
		t.Fatalf("fallback not marked inferred: %+v", trace.Edges)
	}
	if trace.Producer != nil {
		t.Fatalf("inferred-only path must not yield a static producer: %+v", trace.Producer)
	}
	if trace.Supersedes {
		t.Fatal("an inferred path must not supersede the manifestation frame")
	}
}

// A failing operation that performs the invalid operation itself (not a pure
// propagation) must not be superseded by its producer.
func TestCausalTraceDoesNotSupersedeNonPropagation(t *testing.T) {
	ws := traceWorkspace(t)
	trace := traceCausalOrigin(ws, traceSnapshot(true, true), anchorNode(), 4, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Supersedes {
		t.Fatal("anchor that performs the operation must not be superseded")
	}
}

func TestCausalTraceBudgetStops(t *testing.T) {
	ws := traceWorkspace(t)
	trace := traceCausalOrigin(ws, traceSnapshot(true, true), anchorNode(), 5, causalTraceBudget{maxDepth: 1, maxEdges: 8, visited: map[string]bool{}})
	if len(trace.Edges) != 1 {
		t.Fatalf("maxDepth=1 should stop after one edge: %+v", trace.Edges)
	}
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Produce" {
		t.Fatalf("producer after one hop: %+v", trace.Producer)
	}
}

// A wrapper that returns its parameter unchanged must not be promoted over the
// anchor: the invalid value likely came from the argument, which is not traced.
func TestCausalTraceWrapperIsNotPromoted(t *testing.T) {
	ws := t.TempDir()
	src := "package pkg\n\nfunc Caller(x int) {\n\tv := Wrapper(x)\n\tpanic(v)\n}\n\nfunc Wrapper(x int) int {\n\treturn x\n}\n"
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := programrepo.Snapshot{
		Nodes: []domain.ProgramNode{
			{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "go:a.go:Function:Caller", FilePath: "a.go", StartLine: 3, EndLine: 6},
			{ID: "n-wrapper", Type: "Function", Name: "Wrapper", Identity: "go:a.go:Function:Wrapper", FilePath: "a.go", StartLine: 8, EndLine: 10},
		},
		Relationships: []domain.Relationship{
			{SourceID: "n-caller", TargetID: "n-wrapper", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",4,"}},
		},
	}
	anchor := snap.Nodes[0]
	trace := traceCausalOrigin(ws, snap, anchor, 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Supersedes {
		t.Fatalf("wrapper pass-through must not supersede the anchor: %+v", trace)
	}
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Wrapper" {
		t.Fatalf("wrapper should still be recorded as the resolved producer: %+v", trace.Producer)
	}
}

// A correct callee that receives an invalid argument is a real mis-attribution
// risk. The trace must remain an unverified lead and record that arguments are
// not traced.
func TestCausalTraceCorrectCalleeWithInvalidArgumentStaysHypothesis(t *testing.T) {
	ws := traceWorkspace(t)
	trace := traceCausalOrigin(ws, traceSnapshot(true, true), anchorNode(), 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	report := Report{}
	report.CausalTrace = &trace
	admitCausalProducer(&report, traceSnapshot(true, true), trace)
	report.CandidateReport = func() *CandidateReport { r := BuildCandidateReport(report); return &r }()
	lf := BuildLikelyFunction(report)
	report.LikelyFunction = &lf
	if status := assessRootCause(report).Status; status == "established" || status == "confirmed" {
		t.Fatalf("a resolved producer must not establish a root cause: %s", status)
	}
	exp := BuildCausalExplanation(report)
	if exp.Status != "hypothesis" || !strings.Contains(exp.Hypothesis, "not established") {
		t.Fatalf("producer explanation must stay unverified: %+v", exp)
	}
}

// When several values feed the failing operation, no unique producer is chosen.
func TestCausalTraceAmbiguousProducersNotChosen(t *testing.T) {
	ws := t.TempDir()
	src := "package pkg\n\nfunc Caller(a, b int) {\n\tx := One(a)\n\ty := Two(b)\n\tpanic(x + y)\n}\n\nfunc One(a int) int { return a }\n\nfunc Two(b int) int { return b }\n"
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	anchor := domain.ProgramNode{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "go:a.go:Function:Caller", FilePath: "a.go", StartLine: 3, EndLine: 7}
	snap := programrepo.Snapshot{Nodes: []domain.ProgramNode{anchor}}
	trace := traceCausalOrigin(ws, snap, anchor, 6, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Producer != nil {
		t.Fatalf("ambiguous producers must not yield a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Relationship != "ambiguous" {
		t.Fatalf("expected an ambiguous boundary edge: %+v", trace.Edges)
	}
	if trace.Supersedes {
		t.Fatal("ambiguous trace must not supersede")
	}
}

// A call whose result is not used at the failing operation is not a producer.
func TestCausalTraceIgnoresUnrelatedCalls(t *testing.T) {
	ws := t.TempDir()
	src := "package pkg\n\nfunc Caller(a int) int {\n\t_ = Unrelated(a)\n\treturn a\n}\n\nfunc Unrelated(a int) int { return a }\n"
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	anchor := domain.ProgramNode{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "go:a.go:Function:Caller", FilePath: "a.go", StartLine: 3, EndLine: 6}
	snap := programrepo.Snapshot{Nodes: []domain.ProgramNode{anchor}}
	trace := traceCausalOrigin(ws, snap, anchor, 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Producer != nil {
		t.Fatalf("unrelated call must not yield a producer: %+v", trace.Producer)
	}
	for _, edge := range trace.Edges {
		if edge.Relationship == causalTraceRelationship {
			t.Fatalf("discarded call result was traced: %+v", edge)
		}
	}
}

// Two same-package functions with the same name are not a unique resolution and
// must not be followed.
func TestCausalTraceAmbiguousNameNotFollowed(t *testing.T) {
	ws := t.TempDir()
	src := "package pkg\n\nfunc Caller(x int) {\n\tv := helper(x)\n\tpanic(v)\n}\n"
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := programrepo.Snapshot{Nodes: []domain.ProgramNode{
		{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "go:a.go:Function:Caller", FilePath: "a.go", StartLine: 3, EndLine: 6},
		{ID: "h1", Type: "Function", Name: "helper", Identity: "go:a.go:Function:helper.A", FilePath: "a.go", StartLine: 8, EndLine: 9},
		{ID: "h2", Type: "Function", Name: "helper", Identity: "go:a.go:Function:helper.B", FilePath: "a.go", StartLine: 11, EndLine: 12},
	}}
	anchor := snap.Nodes[0]
	trace := traceCausalOrigin(ws, snap, anchor, 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if trace.Producer != nil {
		t.Fatalf("ambiguous name must not yield a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Status != causalTraceStatusUnres {
		t.Fatalf("expected an unresolved boundary: %+v", trace.Edges)
	}
}

// An inferred (same-package name) edge must not be extended into a static
// producer downstream.
func TestCausalTraceDoesNotExtendThroughInferredEdge(t *testing.T) {
	ws := t.TempDir()
	src := "package pkg\n\nfunc Caller(x int) {\n\tv := Mid(x)\n\tpanic(v)\n}\n\nfunc Mid(x int) int {\n\treturn Deep(x)\n}\n\nfunc Deep(x int) int {\n\treturn x + 1\n}\n"
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// No typed edge Caller->Mid; Mid resolves by name (inferred). Mid->Deep is
	// type-resolved, but the trace must stop at Mid.
	snap := programrepo.Snapshot{
		Nodes: []domain.ProgramNode{
			{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "go:a.go:Function:Caller", FilePath: "a.go", StartLine: 3, EndLine: 6},
			{ID: "n-mid", Type: "Function", Name: "Mid", Identity: "go:a.go:Function:Mid", FilePath: "a.go", StartLine: 8, EndLine: 10},
			{ID: "n-deep", Type: "Function", Name: "Deep", Identity: "go:a.go:Function:Deep", FilePath: "a.go", StartLine: 12, EndLine: 14},
		},
		Relationships: []domain.Relationship{
			{SourceID: "n-mid", TargetID: "n-deep", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",9,"}},
		},
	}
	anchor := snap.Nodes[0]
	trace := traceCausalOrigin(ws, snap, anchor, 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if len(trace.Edges) != 1 {
		t.Fatalf("trace must stop at the inferred edge, got %d edges: %+v", len(trace.Edges), trace.Edges)
	}
	if trace.Producer != nil {
		t.Fatalf("no static producer may be claimed past an inferred edge: %+v", trace.Producer)
	}
	if trace.Status != "partial" {
		t.Fatalf("expected partial status: %q", trace.Status)
	}
}
func TestCausalTraceCrossPackageTypedEdge(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "dep"), 0o755); err != nil {
		t.Fatal(err)
	}
	caller := "package pkg\n\nfunc Caller(x int) {\n\tv := dep.Produce(x)\n\tpanic(v)\n}\n"
	if err := os.WriteFile(filepath.Join(ws, "pkg", "a.go"), []byte(caller), 0o644); err != nil {
		t.Fatal(err)
	}
	producer := "package dep\n\nfunc Produce(x int) int {\n\treturn x\n}\n"
	if err := os.WriteFile(filepath.Join(ws, "dep", "b.go"), []byte(producer), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := programrepo.Snapshot{
		Nodes: []domain.ProgramNode{
			{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "go:pkg/a.go:Function:Caller", FilePath: "pkg/a.go", StartLine: 3, EndLine: 6},
			{ID: "n-produce", Type: "Function", Name: "Produce", Identity: "go:dep/b.go:Function:Produce", FilePath: "dep/b.go", StartLine: 3, EndLine: 5},
		},
		Relationships: []domain.Relationship{
			{SourceID: "n-caller", TargetID: "n-produce", Type: "CALLS", Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": ",4,"}},
		},
	}
	anchor := domain.ProgramNode{ID: "n-caller", Type: "Function", Name: "Caller", Identity: "go:pkg/a.go:Function:Caller", FilePath: "pkg/a.go", StartLine: 3, EndLine: 6}
	trace := traceCausalOrigin(ws, snap, anchor, 5, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != causalTraceRelationship || trace.Edges[0].Status != causalTraceStatusStatic {
		t.Fatalf("cross-package typed edge not followed as static: %+v", trace.Edges)
	}
	if trace.Producer == nil || trace.Producer.File != "dep/b.go" {
		t.Fatalf("cross-package producer not identified: %+v", trace.Producer)
	}
	report := Report{}
	admitCausalProducer(&report, snap, trace)
	found := false
	for _, c := range report.Candidates {
		if c.Location.SymbolIdentity == "go:dep/b.go:Function:Produce" && c.Location.Provenance == "causal_producer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cross-package producer not admitted: %+v", report.Candidates)
	}
}
func TestCausalTraceAdmissionAndRanking(t *testing.T) {
	report := Report{
		Candidates: []domain.CandidateLocation{
			{Location: domain.CodeLocation{FilePath: "pkg/a.go", SymbolIdentity: "pkg.Caller", StartLine: 3, Provenance: "enclosing_symbol", LocationType: "Function"},
				Score: domain.ScoreVector{ExecutionRelevance: 1, AvailableDimensions: []string{"execution"}}},
		},
	}
	trace := CausalTrace{
		Status:     "traced",
		Anchor:     &CausalTraceNode{File: "pkg/a.go", Function: "Caller", Line: 5},
		Producer:   &CausalTraceNode{Identity: "pkg.Produce", File: "pkg/a.go", Function: "Produce", Line: 8},
		Supersedes: true,
		Edges:      []CausalTraceEdge{{To: CausalTraceNode{Identity: "pkg.Produce", File: "pkg/a.go", Function: "Produce"}, Status: causalTraceStatusStatic, Relationship: causalTraceRelationship, EvidenceIDs: []string{"E-causal-x"}}},
	}
	report.CausalTrace = &trace
	admitCausalProducer(&report, traceSnapshot(true, true), trace)
	found := false
	for _, c := range report.Candidates {
		if c.Location.SymbolIdentity == "pkg.Produce" && c.Location.Provenance == "causal_producer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("producer not admitted to candidates: %+v", report.Candidates)
	}
	shortlist := BuildCandidateReport(report)
	report.CandidateReport = &shortlist
	if len(shortlist.Entries) == 0 || shortFunctionName(shortlist.Entries[0].Function) != "Produce" || shortlist.Entries[0].Kind != "causal_producer" {
		t.Fatalf("causal producer did not lead when it supersedes: %+v", shortlist.Entries)
	}
	likely := BuildLikelyFunction(report)
	if likely.Primary == nil || shortFunctionName(likely.Primary.Function) != "Produce" {
		t.Fatalf("likely-function tier did not follow the causal producer: %+v", likely.Primary)
	}
	// Without supersession, the manifestation frame stays ahead.
	report.CausalTrace.Supersedes = false
	shortlist2 := BuildCandidateReport(report)
	if len(shortlist2.Entries) == 0 || shortFunctionName(shortlist2.Entries[0].Function) != "Caller" {
		t.Fatalf("non-superseding producer must not displace the frame: %+v", shortlist2.Entries)
	}
}

// A causal-producer explanation must stay explicitly unverified.
func TestCausalProducerExplanationStaysUnverified(t *testing.T) {
	report := Report{
		Candidates: []domain.CandidateLocation{
			{Location: domain.CodeLocation{FilePath: "pkg/a.go", SymbolIdentity: "pkg.Produce", StartLine: 8, Provenance: "causal_producer", LocationType: "Function"},
				Score: domain.ScoreVector{DependencyRelevance: 1, AvailableDimensions: []string{"dependency"}}},
		},
	}
	shortlist := BuildCandidateReport(report)
	report.CandidateReport = &shortlist
	report.CausalTrace = &CausalTrace{
		Status: "traced", Supersedes: true,
		Producer: &CausalTraceNode{Identity: "pkg.Produce", File: "pkg/a.go", Function: "Produce", Line: 8},
		Edges:    []CausalTraceEdge{{To: CausalTraceNode{Identity: "pkg.Produce", Function: "Produce"}, Status: causalTraceStatusStatic}},
	}
	likely := BuildLikelyFunction(report)
	report.LikelyFunction = &likely
	exp := BuildCausalExplanation(report)
	if exp.Status != "hypothesis" {
		t.Fatalf("expected hypothesis: %+v", exp)
	}
	if exp.OperationSupport != "evidence_supported" {
		t.Fatalf("static producer should be evidence-supported: %q", exp.OperationSupport)
	}
	if status := assessRootCause(report).Status; status == "established" || status == "confirmed" {
		t.Fatalf("causal producer promoted a root cause: %s", status)
	}
}

// helper: write one Go file, build a minimal snapshot with type-resolved call
// edges between its functions, and trace from an anchor at anchorLine.
func traceSingleFile(t *testing.T, src string, anchorLine int) CausalTrace {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "a.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	nodes := []domain.ProgramNode{}
	byName := map[string]domain.ProgramNode{}
	var anchor domain.ProgramNode
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		n := domain.ProgramNode{ID: "go:a.go:Function:" + fd.Name.Name, Type: "Function", Name: fd.Name.Name,
			Identity: "go:a.go:Function:" + fd.Name.Name, FilePath: "a.go",
			StartLine: fset.Position(fd.Pos()).Line, EndLine: fset.Position(fd.End()).Line}
		nodes = append(nodes, n)
		byName[fd.Name.Name] = n
		if anchorLine >= n.StartLine && anchorLine <= n.EndLine {
			anchor = n
		}
	}
	var rels []domain.Relationship
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		srcNode := byName[fd.Name.Name]
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			tgt, ok := byName[calleeName(call)]
			if !ok {
				return true
			}
			line := fset.Position(call.Pos()).Line
			rels = append(rels, domain.Relationship{SourceID: srcNode.ID, TargetID: tgt.ID, Type: "CALLS",
				Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": "," + strconv.Itoa(line) + ","}})
			return true
		})
	}
	snap := programrepo.Snapshot{Nodes: nodes, Relationships: rels}
	return traceCausalOrigin(ws, snap, anchor, anchorLine, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
}

func TestCausalTraceNilDerefFromParameterIsBoundary(t *testing.T) {
	trace := traceSingleFile(t, "package pkg\n\ntype T struct{ Field int }\n\nfunc f(p *T) int {\n\treturn p.Field\n}\n", 6)
	if trace.Producer != nil {
		t.Fatalf("parameter origin must not yield a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != "parameter" {
		t.Fatalf("expected a parameter boundary: %+v", trace.Edges)
	}
}

func TestCausalTraceNilDerefLocalZeroValue(t *testing.T) {
	trace := traceSingleFile(t, "package pkg\n\ntype T struct{ Field int }\n\nfunc f() int {\n\tvar p *T\n\treturn p.Field\n}\n", 7)
	if trace.Producer != nil {
		t.Fatalf("local zero value must not yield a producer function: %+v", trace.Producer)
	}
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != "local_zero_value" {
		t.Fatalf("expected a local zero-value origin: %+v", trace.Edges)
	}
}

func TestCausalTraceBranchDependentAssignmentIsAmbiguous(t *testing.T) {
	src := "package pkg\n\nfunc f(c bool) int {\n\tvar x int\n\tif c {\n\t\tx = A()\n\t} else {\n\t\tx = B()\n\t}\n\tpanic(x)\n}\n\nfunc A() int { return 1 }\n\nfunc B() int { return 2 }\n"
	trace := traceSingleFile(t, src, 10)
	if trace.Producer != nil {
		t.Fatalf("branch-dependent assignment must not yield a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Relationship != "ambiguous" {
		t.Fatalf("expected an ambiguous boundary: %+v", trace.Edges)
	}
}

func TestCausalTraceComparisonOperandProducer(t *testing.T) {
	src := "package pkg\n\nfunc f(want int) {\n\tgot := Compute()\n\tif got != want {\n\t\tpanic(got)\n\t}\n}\n\nfunc Compute() int { return 1 }\n"
	trace := traceSingleFile(t, src, 6)
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Compute" {
		t.Fatalf("comparison operand producer not resolved: %+v", trace.Producer)
	}
}

func TestCausalTraceIndexOperandProducer(t *testing.T) {
	src := "package pkg\n\nfunc f(s []int) int {\n\ti := Compute()\n\treturn s[i]\n}\n\nfunc Compute() int { return 0 }\n"
	trace := traceSingleFile(t, src, 5)
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Compute" {
		t.Fatalf("index operand producer not resolved: %+v", trace.Producer)
	}
}

func TestCausalTraceConversionOperandProducer(t *testing.T) {
	src := "package pkg\n\nfunc f() int64 {\n\tn := Compute()\n\treturn int64(n)\n}\n\nfunc Compute() int { return 1 }\n"
	trace := traceSingleFile(t, src, 5)
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Compute" {
		t.Fatalf("conversion operand producer not resolved: %+v", trace.Producer)
	}
}

func TestCausalTraceLocalAliasChain(t *testing.T) {
	src := "package pkg\n\ntype T struct{ F int }\n\nfunc f() {\n\tp := Get()\n\tq := p\n\tpanic(q.F)\n}\n\nfunc Get() *T { return nil }\n"
	trace := traceSingleFile(t, src, 8)
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Get" {
		t.Fatalf("alias chain not resolved to producer: %+v", trace.Producer)
	}
}

// The tracer relies on AST value provenance and does not require SSA; a call
// resolved only by typed edges still works with no SSA trace present.
func TestCausalTraceWorksWithoutSSA(t *testing.T) {
	trace := traceSingleFile(t, "package pkg\n\nfunc f() {\n\tv := Produce()\n\tpanic(v)\n}\n\nfunc Produce() int { return 1 }\n", 5)
	if trace.Producer == nil || shortFunctionName(trace.Producer.Function) != "Produce" {
		t.Fatalf("producer not resolved without SSA: %+v", trace.Producer)
	}
}

// traceWithCallers builds a two-function snapshot A -> B and traces from B's
// failing line, exercising parameter-to-argument provenance.
func traceWithCallers(t *testing.T, src string, anchorLine int) CausalTrace {
	return traceSingleFile(t, src, anchorLine)
}

func TestArgumentProvenanceSingleCaller(t *testing.T) {
	src := "package pkg\n\nfunc B(p int) {\n\tpanic(p)\n}\n\nfunc A() {\n\tx := Produce()\n\tB(x)\n}\n\nfunc Produce() int { return 1 }\n"
	trace := traceWithCallers(t, src, 4)
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != "caller_argument" {
		t.Fatalf("expected a caller-argument edge: %+v", trace.Edges)
	}
	if trace.Edges[0].Status != causalTraceStatusInfer {
		t.Fatalf("caller-derived edge must be inferred, not observed/static: %+v", trace.Edges[0])
	}
	if shortFunctionName(trace.Edges[0].To.Function) != "Produce" {
		t.Fatalf("argument producer not resolved: %+v", trace.Edges[0].To)
	}
	if trace.Supersedes {
		t.Fatal("a caller-derived producer must not supersede the failure frame")
	}
	if trace.Producer != nil {
		t.Fatalf("caller-derived producer must not become the primary producer: %+v", trace.Producer)
	}
}

func TestArgumentProvenanceCallerPassedNil(t *testing.T) {
	src := "package pkg\n\ntype T struct{ F int }\n\nfunc B(p *T) int {\n\treturn p.F\n}\n\nfunc A() int {\n\treturn B(nil)\n}\n"
	trace := traceWithCallers(t, src, 6)
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != "caller_argument" {
		t.Fatalf("expected a caller-argument edge: %+v", trace.Edges)
	}
	if !strings.Contains(trace.Edges[0].Operation, "nil") {
		t.Fatalf("nil argument not reported: %+v", trace.Edges[0])
	}
}

func TestArgumentProvenanceCallerLocalZeroValue(t *testing.T) {
	src := "package pkg\n\ntype T struct{ F int }\n\nfunc B(p *T) int {\n\treturn p.F\n}\n\nfunc A() int {\n\tvar q *T\n\treturn B(q)\n}\n"
	trace := traceWithCallers(t, src, 6)
	if len(trace.Edges) != 1 || !strings.Contains(trace.Edges[0].Operation, "zero value") {
		t.Fatalf("caller-local zero value not reported: %+v", trace.Edges)
	}
}

func TestArgumentProvenanceMultipleCallersAmbiguous(t *testing.T) {
	src := "package pkg\n\nfunc B(p int) {\n\tpanic(p)\n}\n\nfunc A1() { B(1) }\n\nfunc A2() { B(2) }\n"
	trace := traceWithCallers(t, src, 4)
	if trace.Producer != nil {
		t.Fatalf("ambiguous callers must not yield a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Relationship != "ambiguous" {
		t.Fatalf("expected an ambiguous caller boundary: %+v", trace.Edges)
	}
}

func TestArgumentProvenanceMethodReceiver(t *testing.T) {
	src := "package pkg\n\ntype R struct{ x int }\n\nfunc (r *R) M() int {\n\treturn r.x\n}\n\nfunc A() int {\n\tr := NewR()\n\treturn r.M()\n}\n\nfunc NewR() *R { return &R{} }\n"
	trace := traceWithCallers(t, src, 6)
	if len(trace.Edges) == 0 {
		t.Fatalf("no trace from method receiver: %+v", trace)
	}
	if trace.Edges[0].Relationship == "caller_argument" && shortFunctionName(trace.Edges[0].To.Function) != "NewR" {
		t.Fatalf("receiver argument not resolved to producer: %+v", trace.Edges[0].To)
	}
}

func TestArgumentProvenanceVariadicIsAmbiguous(t *testing.T) {
	src := "package pkg\n\nfunc B(xs ...int) {\n\tpanic(xs[0])\n}\n\nfunc A() {\n\tB(1, 2, 3)\n}\n"
	trace := traceWithCallers(t, src, 4)
	if trace.Producer != nil {
		t.Fatalf("variadic span must not yield a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Relationship != "ambiguous" {
		t.Fatalf("expected an ambiguous variadic boundary: %+v", trace.Edges)
	}
}

func TestArgumentProvenanceNestedCallArgument(t *testing.T) {
	src := "package pkg\n\nfunc B(p int) {\n\tpanic(p)\n}\n\nfunc A() {\n\tB(Inner())\n}\n\nfunc Inner() int { return 1 }\n"
	trace := traceWithCallers(t, src, 4)
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != "caller_argument" {
		t.Fatalf("nested call argument not mapped: %+v", trace.Edges)
	}
	if shortFunctionName(trace.Edges[0].To.Function) != "Inner" {
		t.Fatalf("nested argument producer not resolved: %+v", trace.Edges[0].To)
	}
}

func TestArgumentProvenanceRecursionIsBounded(t *testing.T) {
	// B calls itself; the trace must not loop.
	src := "package pkg\n\nfunc B(p int) {\n\tif p > 0 {\n\t\tB(p)\n\t}\n\tpanic(p)\n}\n"
	trace := traceWithCallers(t, src, 6)
	if len(trace.Edges) > 8 {
		t.Fatalf("recursion not bounded: %d edges", len(trace.Edges))
	}
}

func traceFramed(t *testing.T, src string, anchorLine int, frameLines []int) CausalTrace {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "a.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	nodes := []domain.ProgramNode{}
	byName := map[string]domain.ProgramNode{}
	var anchor domain.ProgramNode
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		n := domain.ProgramNode{ID: "go:a.go:Function:" + fd.Name.Name, Type: "Function", Name: fd.Name.Name,
			Identity: "go:a.go:Function:" + fd.Name.Name, FilePath: "a.go",
			StartLine: fset.Position(fd.Pos()).Line, EndLine: fset.Position(fd.End()).Line}
		nodes = append(nodes, n)
		byName[fd.Name.Name] = n
		if anchorLine >= n.StartLine && anchorLine <= n.EndLine {
			anchor = n
		}
	}
	var rels []domain.Relationship
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		srcNode := byName[fd.Name.Name]
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			tgt, ok := byName[calleeName(call)]
			if !ok {
				return true
			}
			line := fset.Position(call.Pos()).Line
			rels = append(rels, domain.Relationship{SourceID: srcNode.ID, TargetID: tgt.ID, Type: "CALLS",
				Metadata: map[string]string{"provenance": causalTraceRelationship, "resolution": "go_types", "call_lines": "," + strconv.Itoa(line) + ","}})
			return true
		})
	}
	var frames []localization.Frame
	for _, line := range frameLines {
		frames = append(frames, localization.Frame{File: "a.go", Line: line})
	}
	snap := programrepo.Snapshot{Nodes: nodes, Relationships: rels}
	return traceCausalOriginFramed(ws, snap, anchor, anchorLine, frames, causalTraceBudget{maxDepth: 4, maxEdges: 8, visited: map[string]bool{}})
}

func TestObservedCallerDisambiguatesStaticCallers(t *testing.T) {
	// B has two static callers (A1 first in source, A2 second). The observed
	// stack shows A2 executed, so A2 must be selected.
	src := "package pkg\nfunc B(p int) { panic(p) }\nfunc A1() { B(1) }\nfunc A2() { B(Produce()) }\nfunc Produce() int { return 2 }\n"
	// frames: line 2 (B panic), line 4 (A2 body)
	trace := traceFramed(t, src, 2, []int{2, 4})
	if len(trace.Edges) != 1 || trace.Edges[0].Relationship != "caller_argument" {
		t.Fatalf("observed caller not selected: %+v", trace.Edges)
	}
	if shortFunctionName(trace.Edges[0].To.Function) != "Produce" {
		t.Fatalf("wrong caller's argument producer: %+v", trace.Edges[0].To)
	}
}

func TestCallerAmbiguousWithoutObservedEvidence(t *testing.T) {
	src := "package pkg\nfunc B(p int) { panic(p) }\nfunc A1() { B(1) }\nfunc A2() { B(Produce()) }\nfunc Produce() int { return 2 }\n"
	trace := traceFramed(t, src, 2, nil)
	if trace.Producer != nil {
		t.Fatalf("ambiguous callers must not yield a producer: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Relationship != "ambiguous" {
		t.Fatalf("expected ambiguity with no observed caller: %+v", trace.Edges)
	}
}

func TestObservedCallerAbsentFromStaticSitesIsBoundary(t *testing.T) {
	// Observed caller is C, which does not call B statically -> no selection.
	src := "package pkg\nfunc B(p int) { panic(p) }\nfunc A1() { B(1) }\nfunc C() int { return 0 }\n"
	trace := traceFramed(t, src, 2, []int{2, 4})
	if trace.Producer != nil {
		t.Fatalf("no static caller for the observed frame; must not select: %+v", trace.Producer)
	}
	if len(trace.Edges) == 0 || trace.Edges[0].Relationship != "parameter" {
		t.Fatalf("expected a parameter boundary: %+v", trace.Edges)
	}
}

func TestWrapperChainTracedAcrossHops(t *testing.T) {
	// B(q) <- A(p) <- Root(Produce()); the value producer is two hops up and the
	// intermediate is a pass-through parameter.
	src := "package pkg\nfunc B(q int) { panic(q) }\nfunc A(p int) { B(p) }\nfunc Root() { A(Produce()) }\nfunc Produce() int { return 1 }\n"
	trace := traceFramed(t, src, 2, []int{2, 3, 4})
	if len(trace.Edges) < 2 {
		t.Fatalf("wrapper chain not followed: %+v", trace.Edges)
	}
	last := trace.Edges[len(trace.Edges)-1]
	if shortFunctionName(last.To.Function) != "Produce" {
		t.Fatalf("chain did not reach the producer: %+v", trace.Edges)
	}
	for _, e := range trace.Edges {
		if e.Status == causalTraceStatusStatic {
			t.Fatalf("caller-derived edge must not be static: %+v", e)
		}
	}
}

// Phase E2: with no observed frame, the failing test's assertion line anchors a
// trace (investigation material only).
func TestAssertionAnchorFromFailureText(t *testing.T) {
	ws := t.TempDir()
	src := "package pkg\n\nfunc TestX(t *testing.T) {\n\tif got != want {\n\t\tt.Fatalf(\"boom\")\n\t}\n}\n"
	if err := os.WriteFile(filepath.Join(ws, "a_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	node := domain.ProgramNode{ID: "n", Type: "Test", Name: "TestX", FilePath: "a_test.go", StartLine: 3, EndLine: 7}
	report := Report{Baseline: Baseline{Results: []Attempt{{Stderr: "a_test.go:5: boom"}}}}
	snap := programrepo.Snapshot{Nodes: []domain.ProgramNode{node}}
	anchor, line, ok := assertionAnchor(&report, snap, ws)
	if !ok || line != 5 || anchor == nil || anchor.Name != "TestX" {
		t.Fatalf("assertion anchor not found: ok=%v line=%d anchor=%+v", ok, line, anchor)
	}
}

func TestAssertionAnchorAbsentWithoutTestNode(t *testing.T) {
	report := Report{Baseline: Baseline{Results: []Attempt{{Stderr: "a_test.go:5: boom"}}}}
	if _, _, ok := assertionAnchor(&report, programrepo.Snapshot{}, t.TempDir()); ok {
		t.Fatal("anchor must not be invented without a matching test node")
	}
}
