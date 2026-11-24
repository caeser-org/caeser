package diagnosis

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/caeser-org/ceaser/internal/domain"
	programrepo "github.com/caeser-org/ceaser/internal/repository"
)

// DeterministicCausalStep is one evidence-cited link in a source-grounded
// explanation of a deterministic (non-race) Go failure. Each step is a
// reported observation backed by source evidence, never a causal conclusion on
// its own. The chain as a whole may be labeled established only when every
// link is verified by retrieved source.
type DeterministicCausalStep struct {
	Kind       string `json:"kind"`
	Statement  string `json:"statement"`
	Symbol     string `json:"symbol,omitempty"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	EvidenceID string `json:"evidence_id,omitempty"`
}

// DeterministicCausalChain is the generic, evidence-cited path from a failing
// Go test through the functions it calls into the implementation branch that
// produces the observed value.
type DeterministicCausalChain struct {
	Status      string                    `json:"status"`
	Steps       []DeterministicCausalStep `json:"steps"`
	EvidenceIDs []string                  `json:"evidence_ids,omitempty"`
	Limitations []string                  `json:"limitations,omitempty"`
	Operations  []ObservedSourceOperation `json:"operations,omitempty"`
}

// buildDeterministicCausalChain constructs a source-grounded chain for any
// deterministic Go failure. It is generic: it keys on program-graph structure
// (AST call edges, function symbols, retrieved source), never on repository,
// test, or symbol names. It writes the chain into the report and appends the
// source evidence it collects; it never claims a root cause itself.
func buildDeterministicCausalChain(report *Report, workspace string, snapshot programrepo.Snapshot, candidates []domain.CandidateLocation) {
	if report == nil || workspace == "" {
		return
	}
	if len(report.FailureIncidents) == 0 || report.FailureIncidents[0].Category == "go_data_race" {
		return
	}
	failingTests := map[string]bool{}
	for _, episode := range report.FailureEpisodes {
		if name := testNameFromIdentifier(episode.TestIdentifier); name != "" {
			failingTests[name] = true
		}
	}
	for _, incident := range report.FailureIncidents {
		for _, test := range incident.AffectedTests {
			if name := testNameFromIdentifier(test); name != "" {
				failingTests[name] = true
			}
		}
	}
	if len(failingTests) == 0 {
		return
	}

	nodesByID := map[string]domain.ProgramNode{}
	for _, node := range snapshot.Nodes {
		nodesByID[node.ID] = node
	}
	verifiedCallees := map[string][]domain.ProgramNode{}
	typedCallees := map[string][]domain.ProgramNode{}
	for _, rel := range snapshot.Relationships {
		if rel.Type != "CALLS" || rel.Metadata["provenance"] != "verified_static_call" {
			continue
		}
		target, ok := nodesByID[rel.TargetID]
		if !ok || (target.Type != "Function" && target.Type != "Method") {
			continue
		}
		verifiedCallees[rel.SourceID] = append(verifiedCallees[rel.SourceID], target)
		if rel.Metadata["resolution"] == "go_types" {
			typedCallees[rel.SourceID] = append(typedCallees[rel.SourceID], target)
		}
	}

	chain := &DeterministicCausalChain{Status: "unresolved"}
	for _, candidate := range candidates {
		nodeID := strings.TrimPrefix(candidate.ID, "candidate:")
		testNode, ok := nodesByID[nodeID]
		if !ok || testNode.Type != "Test" || !failingTests[testNode.Name] {
			continue
		}

		// Prefer a go/packages type-resolved callee of the failing test itself.
		// This is a verified edge (the test really calls that symbol), so no
		// test-name heuristic is needed. Fall back to the AST name-based path
		// only when type information was unavailable.
		var impl *domain.ProgramNode
		operations := assertionOperations(report, workspace, testNode, snapshot)
		if len(operations) > 0 {
			impl = &operations[0]
		}
		for i := range typedCallees[testNode.ID] {
			if impl != nil {
				break
			}
			c := &typedCallees[testNode.ID][i]
			if c.Type == "Test" || strings.HasSuffix(c.FilePath, "_test.go") {
				continue
			}
			impl = c
			break
		}
		if impl == nil {
			// Only graph edges belonging to this exact failing test are eligible.
			for i := range verifiedCallees[testNode.ID] {
				callee := &verifiedCallees[testNode.ID][i]
				if !strings.HasSuffix(callee.FilePath, "_test.go") {
					impl = callee
					break
				}
			}
		}
		if impl == nil {
			chain.Limitations = append(chain.Limitations, "no indexed implementation symbol matched a call in the failing test")
			break
		}

		testStep, testEvidence := sourceStep(report, workspace, "failing_test", testNode.FilePath, testNode.StartLine,
			fmt.Sprintf("source analysis selected %s as an assertion dependency or reachable-call candidate; path feasibility and causality remain unverified", impl.Name))
		if testEvidence != nil {
			report.Evidence = append(report.Evidence, *testEvidence)
			chain.EvidenceIDs = appendUniqueString(chain.EvidenceIDs, testEvidence.ID)
		}
		implStep, implEvidence := sourceStep(report, workspace, "called_function", impl.FilePath, impl.StartLine,
			fmt.Sprintf("%s is an indexed function on the selected assertion dependency or call path", impl.Identity))
		if implEvidence != nil {
			report.Evidence = append(report.Evidence, *implEvidence)
			chain.EvidenceIDs = appendUniqueString(chain.EvidenceIDs, implEvidence.ID)
		}
		if testEvidence != nil && implEvidence != nil {
			chain.Steps = append(chain.Steps, testStep, implStep)
		}
		for _, operation := range operations {
			chain.Operations = append(chain.Operations, collectSourceOperations(report, workspace, operation)...)
			step, evidence := sourceStep(report, workspace, "assertion_dependency", operation.FilePath, operation.StartLine,
				fmt.Sprintf("%s is a type-resolved call associated with values used by the failed assertion; syntactic may-dependency, not verified causality", operation.Identity))
			if evidence != nil {
				report.Evidence = append(report.Evidence, *evidence)
				chain.EvidenceIDs = appendUniqueString(chain.EvidenceIDs, evidence.ID)
				chain.Steps = append(chain.Steps, step)
			}
		}

		// Follow verified calls. A return statement alone cannot identify the
		// operation responsible for an observed value.
		callees := verifiedCallees[impl.ID]
		for _, operation := range operations {
			if operation.ID != impl.ID {
				callees = append(callees, verifiedCallees[operation.ID]...)
			}
		}
		if len(callees) == 0 {
			chain.Limitations = append(chain.Limitations, "the implementation has no AST-verified callee; its own source is the deepest verified step")
		}
		seenCallees := map[string]bool{}
		for _, callee := range callees {
			if seenCallees[callee.ID] {
				continue
			}
			seenCallees[callee.ID] = true
			statement := fmt.Sprintf("%s is called by %s through an AST-verified call edge", callee.Identity, impl.Identity)
			calleeStep, calleeEvidence := sourceStep(report, workspace, "implementation_branch", callee.FilePath, callee.StartLine, statement)
			if calleeEvidence != nil {
				report.Evidence = append(report.Evidence, *calleeEvidence)
				chain.EvidenceIDs = appendUniqueString(chain.EvidenceIDs, calleeEvidence.ID)
				chain.Steps = append(chain.Steps, calleeStep)
			}
		}

		if len(chain.Steps) >= 2 && len(chain.EvidenceIDs) >= 2 {
			chain.Status = "localized"
			chain.Limitations = append(chain.Limitations, "call reachability does not establish data flow to the failed assertion or a violated invariant")
		} else if len(chain.Steps) > 0 {
			chain.Status = "partial"
		}
		break
	}
	report.CausalChain = chain
	admitCausalChainCandidates(report, snapshot)
	admitAssertionMentionedCandidates(report, snapshot)
}

// identifierPattern extracts Go identifiers from retrieved source excerpts.
var identifierPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{3,}`)

// admitAssertionMentionedCandidates admits implementation functions that the
// failing test's own source names at the assertion. A test comment or expression
// naming a function is retrieved source evidence, not speculation: the report
// cites the evidence ID, and the function must still exist as an indexed,
// non-test symbol in the checked-out revision.
func admitAssertionMentionedCandidates(report *Report, snapshot programrepo.Snapshot) {
	if report == nil {
		return
	}
	byShortName := map[string][]domain.ProgramNode{}
	for _, node := range snapshot.Nodes {
		if node.Type != "Function" && node.Type != "Method" {
			continue
		}
		if strings.HasSuffix(node.FilePath, "_test.go") || strings.Contains(node.FilePath, "/testdata/") {
			continue
		}
		name := candidateFunction(node.Identity)
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if len(name) < 5 || isCommonIdentifier(name) {
			continue
		}
		byShortName[name] = append(byShortName[name], node)
	}
	evidenceByName := map[string][]string{}
	for _, ev := range report.Evidence {
		if ev.Kind != "source_excerpt" || !strings.HasSuffix(ev.Source, "_test.go") {
			continue
		}
		// Only comment lines that explicitly describe a cause name a suspect.
		// Ordinary test code and descriptive comments mention many functions
		// that have nothing to do with the failure, so admitting them would be
		// noisier than the evidence is worth.
		for _, line := range strings.Split(ev.Detail, "\n") {
			if !strings.Contains(line, "//") || !hasCausalPhrase(line) {
				continue
			}
			for _, id := range identifierPattern.FindAllString(line, -1) {
				evidenceByName[id] = appendUniqueString(evidenceByName[id], ev.ID)
			}
		}
	}
	names := make([]string, 0, len(byShortName))
	for name := range byShortName {
		if len(evidenceByName[name]) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		nodes := byShortName[name]
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].FilePath != nodes[j].FilePath {
				return nodes[i].FilePath < nodes[j].FilePath
			}
			return nodes[i].Identity < nodes[j].Identity
		})
		node := nodes[0]
		identity := node.Identity
		ids := evidenceByName[name]
		duplicate := false
		for _, existing := range report.Candidates {
			if existing.Location.SymbolIdentity == identity {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		report.Candidates = append(report.Candidates, domain.CandidateLocation{
			ID: "candidate:assertion-mention:" + digestString(identity),
			Location: domain.CodeLocation{
				ID:           "location:assertion-mention:" + digestString(identity),
				RepositoryID: node.RepositoryID, RevisionID: node.RevisionID,
				FilePath: node.FilePath, SymbolIdentity: identity,
				StartLine: node.StartLine, EndLine: node.EndLine, LocationType: node.Type,
				Provenance: "assertion_mention",
			},
			Score: domain.ScoreVector{
				DependencyRelevance: 0.7, AvailableDimensions: []string{"dependency"},
				EvidenceByDimension:  map[string][]domain.ID{"dependency": stringIDsToDomain(ids)},
				ScoringPolicyVersion: "vector-v1",
			},
			SupportingEvidenceIDs: stringIDsToDomain(ids),
			Status:                "open",
		})
	}
}

func hasCausalPhrase(line string) bool {
	lower := strings.ToLower(line)
	for _, phrase := range []string{"because", "bug:", "bug ", "caused by", "the fix", "without the fix", "regression"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func isCommonIdentifier(name string) bool {
	switch name {
	case "assert", "require", "check", "error", "value", "result", "expect", "actual", "output", "input":
		return true
	}
	return false
}

// admitCausalChainCandidates admits the functions named by the chain's
// assertion-dependency and implementation-branch steps as investigation
// candidates. Those steps are source-verified operations on the failed
// assertion's dependency path; dropping them at admission loses the strongest
// source-level lead the engine has.
//
// Assertion-dependency steps form a path from the failing assertion toward the
// value's origin, so a later step is closer to the origin. Their score grows
// with depth. Implementation-branch steps are a flat set of verified callees,
// so they receive a lower fixed score. This is ranking of existing evidence,
// not a new analysis or a causal conclusion.
func admitCausalChainCandidates(report *Report, snapshot programrepo.Snapshot) {
	chain := report.CausalChain
	if chain == nil {
		return
	}
	nodesByIdentity := map[string]domain.ProgramNode{}
	for _, node := range snapshot.Nodes {
		if node.Type == "Function" || node.Type == "Method" {
			nodesByIdentity[node.Identity] = node
		}
	}
	type admitted struct {
		node  domain.ProgramNode
		score float64
		ids   []string
	}
	byIdentity := map[string]admitted{}
	depIndex := 0
	for _, step := range chain.Steps {
		if step.Kind != "assertion_dependency" && step.Kind != "implementation_branch" {
			continue
		}
		fields := strings.Fields(step.Statement)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "go:") {
			continue
		}
		identity := fields[0]
		node, ok := nodesByIdentity[identity]
		if !ok || strings.HasSuffix(node.FilePath, "_test.go") {
			continue
		}
		score := 0.5
		if step.Kind == "assertion_dependency" {
			score = 0.55 + 0.05*float64(depIndex)
			if score > 0.95 {
				score = 0.95
			}
			depIndex++
		}
		ids := []string{}
		if step.EvidenceID != "" {
			ids = append(ids, step.EvidenceID)
		}
		if prev, exists := byIdentity[identity]; !exists || score > prev.score {
			byIdentity[identity] = admitted{node: node, score: score, ids: ids}
		}
	}
	identities := make([]string, 0, len(byIdentity))
	for identity := range byIdentity {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		item := byIdentity[identity]
		duplicate := false
		for _, existing := range report.Candidates {
			if existing.Location.SymbolIdentity == identity {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		report.Candidates = append(report.Candidates, domain.CandidateLocation{
			ID: "candidate:causal-chain:" + digestString(identity),
			Location: domain.CodeLocation{
				ID:           "location:causal-chain:" + digestString(identity),
				RepositoryID: item.node.RepositoryID, RevisionID: item.node.RevisionID,
				FilePath: item.node.FilePath, SymbolIdentity: identity,
				StartLine: item.node.StartLine, EndLine: item.node.EndLine, LocationType: item.node.Type,
				Provenance: "causal_dependency",
			},
			Score: domain.ScoreVector{
				DependencyRelevance: item.score, AvailableDimensions: []string{"dependency"},
				EvidenceByDimension:  map[string][]domain.ID{"dependency": stringIDsToDomain(item.ids)},
				ScoringPolicyVersion: "vector-v1",
			},
			SupportingEvidenceIDs: stringIDsToDomain(item.ids),
			Status:                "open",
		})
	}
}

// testNameFromIdentifier tolerates subtests ("TestFoo/1e1000") and Go package
// decorations.
func testNameFromIdentifier(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if idx := strings.Index(id, "/"); idx > 0 {
		id = id[:idx]
	}
	if idx := strings.LastIndex(id, "."); idx >= 0 && idx < len(id)-1 {
		id = id[idx+1:]
	}
	return id
}

// calledSymbolsFromSource parses a Go test file and returns the identifiers of
// functions/methods it calls, in source order, de-duplicated.
func calledSymbolsFromSource(workspace, relPath string) []string {
	if relPath == "" || strings.HasSuffix(relPath, "/") {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relPath)))
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, data, 0)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			name = fun.Name
		case *ast.SelectorExpr:
			name = fun.Sel.Name
		}
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		return true
	})
	return names
}

// valueBranchLine finds a return statement in the named Go function, the
// generic shape of a value-producing branch, and returns its line.
func valueBranchLine(workspace, relPath, funcName string) (int, bool) {
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relPath)))
	if err != nil {
		return 0, false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, data, 0)
	if err != nil {
		return 0, false
	}
	found := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if found > 0 {
			return false
		}
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if found > 0 {
				return false
			}
			if ret, ok := n.(*ast.ReturnStmt); ok {
				found = fset.Position(ret.Pos()).Line
				return false
			}
			return true
		})
		return false
	})
	return found, found > 0
}

func sourceStep(report *Report, workspace, kind, file string, line int, statement string) (DeterministicCausalStep, *EvidenceItem) {
	detail := sourceExcerpt(workspace, file, line)
	if detail == "" {
		return DeterministicCausalStep{}, nil
	}
	step := DeterministicCausalStep{Kind: kind, Statement: statement, File: file, Line: line}
	if file != "" {
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			step.Symbol = file[idx+1:]
		}
	}
	sum := sha256.Sum256([]byte(report.InvestigationID + "\x00" + kind + "\x00" + file + "\x00" + fmt.Sprint(line) + "\x00" + detail))
	id := "E-causal-" + hex.EncodeToString(sum[:10])
	step.EvidenceID = id
	ev := &EvidenceItem{
		ID: id, Kind: "source_excerpt", Source: file, Digest: hex.EncodeToString(sum[:]),
		Detail:    detail,
		ClaimKind: "source-backed causal step", Basis: "SourceVerified",
		Provenance: "deterministic source inspection of checked-out revision",
		Revision:   report.Environment.HeadCommit,
	}
	return step, ev
}

func sourceExcerpt(workspace, file string, line int) string {
	if file == "" || filepath.IsAbs(file) || strings.HasPrefix(file, "..") {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(file)))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	start, end := line-2, line+4
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "checked-out source %s\n", file)
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%d| %s\n", i, lines[i-1])
	}
	return b.String()
}

// sortedCausalStepKinds keeps causal step order stable in reports.
func sortedCausalStepKinds(steps []DeterministicCausalStep) []string {
	kinds := make([]string, 0, len(steps))
	for _, step := range steps {
		kinds = append(kinds, step.Kind)
	}
	sort.Strings(kinds)
	return kinds
}
