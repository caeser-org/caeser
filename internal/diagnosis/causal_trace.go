package diagnosis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/caeser-org/ceaser/internal/domain"
	"github.com/caeser-org/ceaser/internal/localization"
	programrepo "github.com/caeser-org/ceaser/internal/repository"
)

// CausalTraceNode is a revision-scoped location on a backward causal trace.
type CausalTraceNode struct {
	Identity string `json:"identity,omitempty"`
	File     string `json:"file,omitempty"`
	Function string `json:"function,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// CausalTraceEdge is one evidence-carrying link in a backward trace. Status
// separates directly observed evidence from static resolution and from an
// unresolved boundary; a chain containing unresolved or inferred edges is never
// presented as verified causality.
type CausalTraceEdge struct {
	From         CausalTraceNode `json:"from"`
	To           CausalTraceNode `json:"to"`
	Operation    string          `json:"operation,omitempty"`
	Line         int             `json:"line,omitempty"`
	Relationship string          `json:"relationship,omitempty"` // verified_static_call | textual_call | unindexed
	Status       string          `json:"status"`                 // static | inferred | unresolved
	EvidenceIDs  []string        `json:"evidence_ids,omitempty"`
	Ambiguity    []string        `json:"ambiguity,omitempty"`
}

// CausalTrace is the bounded backward investigation from a failure's
// manifestation site toward the operation that produced the invalid value or
// state. It is evidence about relationships, not a root-cause claim.
type CausalTrace struct {
	Status      string            `json:"status"` // traced | partial | unresolved
	Anchor      *CausalTraceNode  `json:"anchor,omitempty"`
	Producer    *CausalTraceNode  `json:"producer,omitempty"`
	Supersedes  bool              `json:"supersedes_manifestation"`
	Edges       []CausalTraceEdge `json:"edges,omitempty"`
	Budget      []string          `json:"budget,omitempty"`
	Limitations []string          `json:"limitations,omitempty"`
	Defects     []DefectOrigin    `json:"defect_origins,omitempty"`
	// AssertionAnchored is true when no observed implementation frame existed and
	// the trace started from the failing test's assertion line. Such a trace is
	// investigation material only and never admits candidates.
	AssertionAnchored bool `json:"assertion_anchored,omitempty"`
}

const (
	causalTraceDefaultDepth = 4
	causalTraceMaxEdges     = 8
	causalTraceRelationship = "verified_static_call"
	causalTraceTextualCall  = "textual_call"
	causalTraceUnindexed    = "unindexed"
	causalTraceStatusStatic = "static"
	causalTraceStatusInfer  = "inferred"
	causalTraceStatusUnres  = "unresolved"
)

type causalTraceBudget struct {
	maxDepth int
	maxEdges int
	visited  map[string]bool
}

// buildFailureOriginTrace runs the production backward investigation and stores
// it on the report. It mutates only presentation/candidate fields; it never
// approves a root cause and never touches verification.
func buildFailureOriginTrace(report *Report, workspace string, snapshot programrepo.Snapshot, frames []localization.Frame) {
	if report == nil || workspace == "" || len(report.FailureIncidents) == 0 {
		return
	}
	// Races keep their concurrency-specific analysis; a linear value-propagation
	// trace would misrepresent a happens-before/ownership defect.
	for _, incident := range report.FailureIncidents {
		if incident.Category == "go_data_race" {
			return
		}
	}
	order := frames
	if len(order) == 0 {
		for _, frame := range observedFrameOrder(*report) {
			order = append(order, localization.Frame{File: frame.file, Line: frame.line})
		}
	}
	if len(order) == 0 {
		return
	}
	nodesByID := map[string]domain.ProgramNode{}
	for _, node := range snapshot.Nodes {
		nodesByID[node.ID] = node
	}
	// Anchor: the innermost reported frame that resolves to a non-test symbol.
	var anchor *domain.ProgramNode
	var anchorLine int
	for _, frame := range order {
		node, ok := enclosingNode(nodesByID, frame.File, frame.Line)
		if !ok || node.Type == "Test" || strings.HasSuffix(node.FilePath, "_test.go") {
			continue
		}
		copy := node
		anchor = &copy
		anchorLine = frame.Line
		break
	}
	assertionAnchored := false
	if anchor == nil {
		if a, l, ok := assertionAnchor(report, snapshot, workspace); ok {
			anchor = a
			anchorLine = l
			assertionAnchored = true
		}
	}
	if anchor == nil {
		return
	}
	trace := traceCausalOriginFramed(workspace, snapshot, *anchor, anchorLine, order, causalTraceBudget{maxDepth: causalTraceDefaultDepth, maxEdges: causalTraceMaxEdges, visited: map[string]bool{}})
	trace.AssertionAnchored = assertionAnchored
	if assertionAnchored {
		trace.Limitations = append(trace.Limitations,
			"anchored on the failing test's assertion line (no observed implementation frame); this is investigation material, not execution evidence")
	}
	// Defect-origin hypotheses for operations feeding the failing operand. They
	// are evidence-grounded but never even a candidate unless a supported causal
	// relationship exists; they never change verification.
	trace.Defects = assessDefectOrigins(workspace, *anchor, anchorLine, defectContext{ssa: report.SSAAssertionTrace, deltas: report.Deltas})
	if len(trace.Edges) == 0 && len(trace.Defects) == 0 {
		return
	}
	// Mint source evidence for each traced node so every edge is auditable.
	for i := range trace.Edges {
		edge := &trace.Edges[i]
		if edge.Line <= 0 || edge.To.File == "" || edge.To.Function == "" {
			continue
		}
		_, ev := sourceStep(report, workspace, "causal_producer", edge.To.File, edge.Line,
			edge.Relationship+" edge: "+edge.Operation+" in "+edge.From.Function+" depends on "+edge.To.Function)
		if ev != nil {
			report.Evidence = append(report.Evidence, *ev)
			edge.EvidenceIDs = appendUniqueString(edge.EvidenceIDs, ev.ID)
		}
	}
	for i := range trace.Defects {
		d := &trace.Defects[i]
		_, ev := sourceStep(report, workspace, "defect_origin", d.File, d.Line, d.Rationale)
		if ev != nil {
			report.Evidence = append(report.Evidence, *ev)
			d.EvidenceID = ev.ID
		}
		trace.Limitations = append(trace.Limitations, "defect-origin hypothesis ("+d.Kind+"): "+d.Rationale)
	}
	report.CausalTrace = &trace
	if len(trace.Edges) > 0 {
		report.CoverageNotes = append(report.CoverageNotes,
			"backward causal trace from "+trace.Anchor.Function+" reached "+traceEdgeTargets(trace))
	}
	// Admit the trace's statically-supported producer so candidate discovery can
	// see a defect candidate that is absent from the observed stack. Assertion-
	// anchored traces are investigation material only and never admit candidates.
	if !assertionAnchored && trace.Producer != nil && trace.Status != "unresolved" {
		admitCausalProducer(report, snapshot, trace)
	}
}

// traceCausalOrigin walks backward from an anchor operation toward the
// operation that produced the value or error it depends on, using type-resolved
// call edges first and a conservative same-package name resolution second. It
// stops at the first statically-supported producer, an evidence boundary, or a
// budget limit.
func traceCausalOrigin(workspace string, snapshot programrepo.Snapshot, anchor domain.ProgramNode, anchorLine int, budget causalTraceBudget) CausalTrace {
	return traceCausalOriginFramed(workspace, snapshot, anchor, anchorLine, nil, budget)
}

func traceCausalOriginFramed(workspace string, snapshot programrepo.Snapshot, anchor domain.ProgramNode, anchorLine int, frames []localization.Frame, budget causalTraceBudget) CausalTrace {
	trace := CausalTrace{Status: "unresolved"}
	anchorNode := nodeRef(anchor, anchorLine)
	trace.Anchor = &anchorNode

	nodesByID := map[string]domain.ProgramNode{}
	for _, node := range snapshot.Nodes {
		nodesByID[node.ID] = node
	}
	edgesBySource := map[string][]domain.Relationship{}
	for _, rel := range snapshot.Relationships {
		if rel.Type == "CALLS" {
			edgesBySource[rel.SourceID] = append(edgesBySource[rel.SourceID], rel)
		}
	}
	edgesByTarget := map[string][]domain.Relationship{}
	for _, rel := range snapshot.Relationships {
		if rel.Type == "CALLS" {
			edgesByTarget[rel.TargetID] = append(edgesByTarget[rel.TargetID], rel)
		}
	}
	chain := observedFunctionChain(frames, nodesByID)

	current := anchor
	line := anchorLine
	var callerNode *domain.ProgramNode
	callerCallLine := 0
	for depth := 0; depth < budget.maxDepth; depth++ {
		if budget.visited[current.ID] {
			trace.Budget = append(trace.Budget, "recursion cycle detected; stopped at "+current.FilePath)
			break
		}
		budget.visited[current.ID] = true

		call, ok := resolveOperandOrigin(workspace, current, line)
		if !ok {
			trace.Limitations = append(trace.Limitations,
				"no resolvable value-producing call at the failing operation in "+current.FilePath+"; the manifestation site remains the deepest supported step")
			break
		}
		from := nodeRef(current, line)
		if len(call.ambiguous) > 0 {
			// Several values feed the failing operation; do not guess a producer.
			to := CausalTraceNode{Function: call.callee}
			trace.Edges = append(trace.Edges, CausalTraceEdge{
				From: from, To: to, Operation: call.text, Line: call.line,
				Relationship: "ambiguous", Status: causalTraceStatusUnres,
				Ambiguity: append([]string{"multiple values feed the failing operation; producer not uniquely determined"}, call.ambiguous...),
			})
			trace.Limitations = append(trace.Limitations, "multiple candidate producers feed the failing operation; no unique producer")
			break
		}
		if call.kind == "zero_value" {
			// The operand is a locally declared zero value (nil/zero). The value
			// origin is established, but there is no producer function to admit.
			trace.Edges = append(trace.Edges, CausalTraceEdge{
				From: from, To: CausalTraceNode{File: current.FilePath, Line: call.line},
				Operation: call.text, Line: call.line,
				Relationship: "local_zero_value", Status: causalTraceStatusStatic,
			})
			trace.Limitations = append(trace.Limitations,
				"operand "+call.name+" is a locally declared zero value (nil/zero); no valid value reaches the failing operation")
			break
		}
		if call.kind == "parameter" {
			// The operand comes from a caller. Use observed-caller evidence and
			// bounded upward chaining; every resulting edge is inferred and never
			// supersedes the failure frame.
			var upEdges []CausalTraceEdge
			var upLimits []string
			resolveParameterUpward(workspace, snapshot, nodesByID, edgesBySource, edgesByTarget, chain, callerNode, callerCallLine, current, call.name, from, call.line, &budget, &upEdges, &upLimits, 0)
			trace.Edges = append(trace.Edges, upEdges...)
			trace.Limitations = append(trace.Limitations, upLimits...)
			break
		}
		if target, rel, ok := resolveCallee(snapshot, nodesByID, edgesBySource, current.ID, call.line, call.callee); ok {
			to := nodeRef(target, target.StartLine)
			status := causalTraceStatusStatic
			if rel == causalTraceTextualCall {
				status = causalTraceStatusInfer
			}
			edge := CausalTraceEdge{From: from, To: to, Operation: call.text, Line: call.line, Relationship: rel, Status: status}
			if rel == causalTraceTextualCall {
				edge.Ambiguity = append(edge.Ambiguity, "same-package name resolution; not type-resolved")
			}
			trace.Edges = append(trace.Edges, edge)
			// The producer is the nearest statically-supported operation that
			// produced the value the failing operation consumed. Deeper nodes
			// remain in Edges and are admitted as alternates; depth alone is
			// never treated as stronger evidence.
			if trace.Producer == nil && status == causalTraceStatusStatic {
				producer := to
				trace.Producer = &producer
			}
			if len(trace.Edges) >= budget.maxEdges {
				trace.Budget = append(trace.Budget, "edge budget reached; the producer path may continue")
				break
			}
			if status == causalTraceStatusInfer {
				// A same-package name match is not a proven edge. Extending the
				// trace past it would present inferred structure as evidence.
				trace.Limitations = append(trace.Limitations, "trace stopped at a same-package name resolution; extension past it is not statically supported")
				break
			}
			if producerIsPassThrough(workspace, &to) {
				trace.Limitations = append(trace.Limitations, to.Function+" returns its parameter unchanged; there is no deeper value producer to trace")
				break
			}
			if target.Type == "Test" || strings.HasSuffix(target.FilePath, "_test.go") {
				break
			}
			if functionReturnCount(workspace, target) > 1 {
				trace.Limitations = append(trace.Limitations,
					target.FilePath+" has multiple return paths; branch feasibility is unverified and the trace follows the first")
			}
			prev := current
			prevLine := call.line
			current = target
			line = functionReturnLine(workspace, target)
			callerNode = &prev
			callerCallLine = prevLine
			continue
		}
		// The call exists but its target is not in the indexed graph. Record the
		// boundary instead of inventing an edge.
		to := CausalTraceNode{Function: call.callee}
		edge := CausalTraceEdge{From: from, To: to, Operation: call.text, Line: call.line, Relationship: causalTraceUnindexed, Status: causalTraceStatusUnres,
			Ambiguity: []string{"call target is outside the indexed graph; not admitted as a candidate"}}
		trace.Edges = append(trace.Edges, edge)
		trace.Limitations = append(trace.Limitations, "call to "+call.callee+" is not indexed; cross-package or dynamic target unresolved")
		break
	}

	switch {
	case trace.Producer != nil && !hasUnresolved(trace.Edges):
		trace.Status = "traced"
	case trace.Producer != nil:
		trace.Status = "partial"
	case hasStatus(trace.Edges, causalTraceStatusStatic), hasStatus(trace.Edges, causalTraceStatusInfer):
		trace.Status = "partial"
	default:
		trace.Status = "unresolved"
	}
	trace.Supersedes = trace.Status == "traced" && producerSupersedesManifestation(workspace, anchor, anchorLine) && trace.Producer != nil
	if trace.Supersedes {
		if producerIsPassThrough(workspace, trace.Producer) {
			// The producer only forwards a parameter; the invalid value likely
			// originated in the argument, which this trace does not follow.
			trace.Supersedes = false
			trace.Limitations = append(trace.Limitations,
				"the producer returns its parameter unchanged; its argument's origin is not traced, so the producer is not promoted over the anchor")
		} else {
			trace.Limitations = append(trace.Limitations,
				"the anchor frame propagates the producer's value or error; the producer is the stronger defect candidate, not a verified cause")
		}
	}
	return trace
}

type producerCallResult struct {
	kind      string // "call" | "zero_value" | "parameter"
	line      int
	callee    string
	name      string
	text      string
	ambiguous []string
}

// resolveOperandOrigin identifies where the value(s) consumed by the failing
// operation at failLine come from, for any operation kind (a propagated call
// result, a dereference, a comparison, an index, a conversion, a local
// assignment chain, a declared zero value, or a parameter). It returns a single
// origin only when the dependency is unambiguous.
// fnOrigins holds the local value-provenance facts of one function body so they
// can be reused for operand resolution, argument mapping, and chained tracing.
type fnOrigins struct {
	fn           domain.ProgramNode
	data         []byte
	fset         *token.FileSet
	fd           *ast.FuncDecl
	assigned     map[string][]producerCallResult
	aliases      map[string]string
	declared     map[string]int
	params       map[string]bool
	direct       []producerCallResult
	callsByLine  map[int][]*ast.CallExpr
	allDefs      map[string][]producerCallResult
	errorIgnored map[string]string
}

func buildFnOrigins(workspace string, fn domain.ProgramNode, failLine int) (*fnOrigins, bool) {
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(fn.FilePath)))
	if err != nil {
		return nil, false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fn.FilePath, data, 0)
	if err != nil {
		return nil, false
	}
	var fd *ast.FuncDecl
	for _, decl := range file.Decls {
		f, ok := decl.(*ast.FuncDecl)
		if !ok || f.Body == nil {
			continue
		}
		if fset.Position(f.Pos()).Line == fn.StartLine || (failLine >= fset.Position(f.Pos()).Line && failLine <= fset.Position(f.End()).Line) {
			fd = f
			if fset.Position(f.Pos()).Line == fn.StartLine {
				break
			}
		}
	}
	if fd == nil {
		return nil, false
	}
	o := &fnOrigins{fn: fn, data: data, fset: fset, fd: fd,
		assigned: map[string][]producerCallResult{}, aliases: map[string]string{},
		declared: map[string]int{}, params: parameterNames(fd), callsByLine: map[int][]*ast.CallExpr{},
		allDefs: map[string][]producerCallResult{}, errorIgnored: map[string]string{}}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CallExpr:
			line := fset.Position(v.Pos()).Line
			o.callsByLine[line] = append(o.callsByLine[line], v)
			for _, arg := range v.Args {
				inner, ok := arg.(*ast.CallExpr)
				if !ok || fset.Position(inner.Pos()).Line != failLine {
					continue
				}
				if c := calleeName(inner); c != "" && !isConversion(c) {
					o.direct = append(o.direct, producerCallResult{kind: "call", line: failLine, callee: c, text: sourceLine(data, failLine)})
				}
			}
		case *ast.ReturnStmt:
			if len(v.Results) == 1 && fset.Position(v.Pos()).Line == failLine {
				if call, ok := v.Results[0].(*ast.CallExpr); ok {
					if c := calleeName(call); c != "" && !isConversion(c) {
						o.direct = append(o.direct, producerCallResult{kind: "call", line: failLine, callee: c, text: sourceLine(data, failLine)})
					}
				}
			}
		case *ast.AssignStmt:
			line := fset.Position(v.Pos()).Line
			if line > failLine {
				return true
			}
			for i, lhs := range v.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}
				rhs := v.Rhs[min(i, len(v.Rhs)-1)]
				callee := ""
				if call, ok := rhs.(*ast.CallExpr); ok {
					callee = calleeName(call)
				}
				o.allDefs[ident.Name] = append(o.allDefs[ident.Name], producerCallResult{kind: defKind(callee), line: line, callee: callee, name: ident.Name, text: sourceLine(data, line)})
				switch r := rhs.(type) {
				case *ast.CallExpr:
					if callee != "" {
						o.assigned[ident.Name] = append(o.assigned[ident.Name], producerCallResult{kind: "call", line: line, callee: callee, name: ident.Name, text: sourceLine(data, line)})
					}
				case *ast.Ident:
					if r.Name != "_" {
						o.aliases[ident.Name] = r.Name
					}
				}
			}
			// An error result assigned to the blank identifier is discarded.
			if errCall, ok := v.Rhs[0].(*ast.CallExpr); ok && len(v.Lhs) >= 2 && len(v.Rhs) == 1 {
				blank := false
				for _, lhs := range v.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name == "_" {
						blank = true
					}
				}
				if blank {
					if c := calleeName(errCall); c != "" {
						for _, lhs := range v.Lhs {
							if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
								o.errorIgnored[id.Name] = c
							}
						}
					}
				}
			}
		case *ast.GenDecl:
			if v.Tok != token.VAR {
				return true
			}
			for _, spec := range v.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				line := fset.Position(vs.Pos()).Line
				for i, name := range vs.Names {
					if len(vs.Values) == 0 {
						o.declared[name.Name] = line
						continue
					}
					rhs := vs.Values[min(i, len(vs.Values)-1)]
					if call, ok := rhs.(*ast.CallExpr); ok {
						if c := calleeName(call); c != "" && line <= failLine {
							o.assigned[name.Name] = append(o.assigned[name.Name], producerCallResult{kind: "call", line: line, callee: c, name: name.Name, text: sourceLine(data, line)})
						}
					}
				}
			}
		}
		return true
	})
	return o, true
}

// resolve resolves a local name through alias chains to a call, a declared zero
// value, a parameter, or nothing. Multiple assignments are reported ambiguous.
func (o *fnOrigins) resolve(name string) (producerCallResult, bool) {
	return o.resolveSeen(name, map[string]bool{})
}

func (o *fnOrigins) resolveSeen(name string, seen map[string]bool) (producerCallResult, bool) {
	if seen[name] {
		return producerCallResult{}, false
	}
	seen[name] = true
	if list, ok := o.assigned[name]; ok && len(list) > 0 {
		if len(list) == 1 {
			return list[0], true
		}
		best := list[len(list)-1]
		for _, x := range list {
			best.ambiguous = append(best.ambiguous, "assignment "+originLabel(x)+" at line "+strconv.Itoa(x.line))
		}
		return best, true
	}
	if line, ok := o.declared[name]; ok {
		return producerCallResult{kind: "zero_value", line: line, name: name, text: sourceLine(o.data, line)}, true
	}
	if o.params[name] {
		return producerCallResult{kind: "parameter", line: o.fset.Position(o.fd.Pos()).Line, name: name}, true
	}
	if alias, ok := o.aliases[name]; ok {
		return o.resolveSeen(alias, seen)
	}
	return producerCallResult{}, false
}

func resolveOperandOrigin(workspace string, fn domain.ProgramNode, failLine int) (producerCallResult, bool) {
	o, ok := buildFnOrigins(workspace, fn, failLine)
	if !ok {
		return producerCallResult{}, false
	}
	used := operandIdentifiers(o.fd.Body, o.fset, failLine)
	var origins []producerCallResult
	seenOrigin := map[string]bool{}
	addOrigin := func(x producerCallResult) {
		key := x.kind + "|" + x.callee + "|" + x.name + "|" + strconv.Itoa(x.line)
		if seenOrigin[key] {
			return
		}
		seenOrigin[key] = true
		origins = append(origins, x)
	}
	for name := range used {
		if x, ok := o.resolve(name); ok {
			addOrigin(x)
		}
	}
	// A value-producing call on the failing line itself (a returned call or a
	// nested argument call) is the producer before any operand variable.
	seenDirect := map[string]bool{}
	var uniq []producerCallResult
	for _, d := range o.direct {
		key := d.callee + "|" + strconv.Itoa(d.line)
		if seenDirect[key] {
			continue
		}
		seenDirect[key] = true
		uniq = append(uniq, d)
	}
	switch {
	case len(uniq) == 1:
		return uniq[0], true
	case len(uniq) > 1:
		best := uniq[0]
		for _, d := range uniq {
			best.ambiguous = append(best.ambiguous, "call "+d.callee)
		}
		return best, true
	}
	// Prefer defined values over parameters so a testing.T argument does not
	// make every assertion ambiguous.
	var nonParam []producerCallResult
	for _, x := range origins {
		if x.kind != "parameter" {
			nonParam = append(nonParam, x)
		}
	}
	pool := origins
	if len(nonParam) > 0 {
		pool = nonParam
	}
	switch {
	case len(pool) == 1:
		return pool[0], true
	case len(pool) > 1:
		best := pool[0]
		for _, x := range pool[1:] {
			if x.line > best.line {
				best = x
			}
		}
		for _, x := range pool {
			best.ambiguous = append(best.ambiguous, x.kind+" "+originLabel(x))
		}
		return best, true
	default:
		return producerCallResult{}, false
	}
}

func originLabel(o producerCallResult) string {
	if o.callee != "" {
		return o.callee
	}
	if o.name != "" {
		return o.name
	}
	return o.text
}

func parameterNames(fd *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	add := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				out[name.Name] = true
			}
		}
	}
	add(fd.Recv)
	add(fd.Type.Params)
	return out
}

var operandBuiltins = map[string]bool{
	"panic": true, "len": true, "cap": true, "make": true, "new": true,
	"append": true, "copy": true, "delete": true, "close": true, "print": true,
	"println": true, "recover": true, "true": true, "false": true, "nil": true,
	"iota": true,
}

// builtinTypes are Go predeclared types. A conversion T(x) is not a function
// call and must not be treated as a value producer.
var builtinTypes = map[string]bool{
	"bool": true, "string": true, "byte": true, "rune": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true, "float32": true, "float64": true, "complex64": true,
	"complex128": true, "any": true, "error": true,
}

func isConversion(callee string) bool { return builtinTypes[callee] }

// operandIdentifiers returns the identifiers used at a line, excluding selector
// field/method names and built-in functions, so they can be resolved as values.
func operandIdentifiers(body *ast.BlockStmt, fset *token.FileSet, line int) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if fset.Position(n.Pos()).Line > line || fset.Position(n.End()).Line < line {
			return false
		}
		switch v := n.(type) {
		case *ast.SelectorExpr:
			// The selected field/method name is not a value operand.
			if ident, ok := v.X.(*ast.Ident); ok && ident.Name != "" && !operandBuiltins[ident.Name] {
				out[ident.Name] = true
			}
			ast.Inspect(v.X, func(m ast.Node) bool {
				if ident, ok := m.(*ast.Ident); ok && !operandBuiltins[ident.Name] {
					out[ident.Name] = true
				}
				return true
			})
			return false
		case *ast.Ident:
			if !operandBuiltins[v.Name] {
				out[v.Name] = true
			}
		}
		return true
	})
	return out
}

func functionBodyFor(fn domain.ProgramNode, workspace string) (*ast.FuncDecl, *token.FileSet, []byte, bool) {
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(fn.FilePath)))
	if err != nil {
		return nil, nil, nil, false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fn.FilePath, data, 0)
	if err != nil {
		return nil, nil, nil, false
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if ok && fd.Body != nil && fset.Position(fd.Pos()).Line == fn.StartLine {
			return fd, fset, data, true
		}
	}
	return nil, nil, nil, false
}

func identifiersAtLine(body *ast.BlockStmt, fset *token.FileSet, line int) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if fset.Position(n.Pos()).Line > line || fset.Position(n.End()).Line < line {
			return false
		}
		if ident, ok := n.(*ast.Ident); ok && ident.Name != "_" {
			out[ident.Name] = true
		}
		return true
	})
	return out
}

// resolveCallee resolves a call at callLine from sourceID in preference order:
// a type-resolved CALLS edge for that exact line, then a same-package function
// node by name.
func resolveCallee(snapshot programrepo.Snapshot, nodesByID map[string]domain.ProgramNode, edgesBySource map[string][]domain.Relationship, sourceID string, callLine int, callee string) (domain.ProgramNode, string, bool) {
	needle := "," + strconv.Itoa(callLine) + ","
	for _, rel := range edgesBySource[sourceID] {
		if rel.Metadata["provenance"] != causalTraceRelationship {
			continue
		}
		callLines := rel.Metadata["call_lines"]
		if callLines == "" || !strings.Contains(callLines, needle) {
			continue
		}
		target, ok := nodesByID[rel.TargetID]
		if !ok || (target.Type != "Function" && target.Type != "Method") {
			continue
		}
		if callee != "" && shortFunctionName(target.Name) != shortFunctionName(callee) {
			continue
		}
		return target, causalTraceRelationship, true
	}
	// Conservative fallback: a unique same-package function/method with this
	// name. Never claims type-resolved resolution.
	source := nodesByID[sourceID]
	sourceDir := filepath.Dir(filepathSlash(source.FilePath))
	var match *domain.ProgramNode
	dupes := 0
	for _, node := range snapshot.Nodes {
		if node.Type != "Function" && node.Type != "Method" {
			continue
		}
		if shortFunctionName(node.Name) != shortFunctionName(callee) {
			continue
		}
		if filepath.Dir(filepathSlash(node.FilePath)) != sourceDir {
			continue
		}
		copy := node
		match = &copy
		dupes++
	}
	if match != nil && dupes == 1 {
		return *match, causalTraceTextualCall, true
	}
	return domain.ProgramNode{}, "", false
}

func enclosingNode(nodesByID map[string]domain.ProgramNode, file string, line int) (domain.ProgramNode, bool) {
	base := sourceBase(file)
	var best *domain.ProgramNode
	for _, node := range nodesByID {
		if node.Type == "File" || node.Type == "Dependency" {
			continue
		}
		if sourceBase(node.FilePath) != base {
			continue
		}
		if line < node.StartLine || (node.EndLine > 0 && line > node.EndLine) {
			continue
		}
		if best == nil || (node.EndLine-node.StartLine) < (best.EndLine-best.StartLine) {
			copy := node
			best = &copy
		}
	}
	if best == nil {
		return domain.ProgramNode{}, false
	}
	return *best, true
}

// producerSupersedesManifestation reports whether the anchor's failing statement
// merely propagates the producer's value or error (a panic or return), rather
// than the anchor itself performing the invalid operation.
func producerSupersedesManifestation(workspace string, anchor domain.ProgramNode, line int) bool {
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(anchor.FilePath)))
	if err != nil {
		return false
	}
	text := strings.TrimSpace(sourceLine(data, line))
	if text == "" {
		return false
	}
	if strings.HasPrefix(text, "panic(") {
		return true
	}
	if strings.HasPrefix(text, "return") {
		return true
	}
	return false
}

func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func sourceLine(data []byte, line int) string {
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return lines[line-1]
}

func functionReturnLine(workspace string, fn domain.ProgramNode) int {
	lines := functionReturnLines(workspace, fn)
	if len(lines) == 0 {
		return fn.StartLine
	}
	return lines[0]
}

func functionReturnLines(workspace string, fn domain.ProgramNode) []int {
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(fn.FilePath)))
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fn.FilePath, data, 0)
	if err != nil {
		return nil
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil || fset.Position(fd.Pos()).Line != fn.StartLine {
			continue
		}
		var lines []int
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if ret, ok := n.(*ast.ReturnStmt); ok {
				lines = append(lines, fset.Position(ret.Pos()).Line)
			}
			return true
		})
		return lines
	}
	return nil
}

func functionReturnCount(workspace string, fn domain.ProgramNode) int {
	return len(functionReturnLines(workspace, fn))
}

// producerIsPassThrough reports whether a function returns a parameter
// unchanged (a wrapper), in which case the invalid value likely came from the
// argument rather than the function body.
func producerIsPassThrough(workspace string, node *CausalTraceNode) bool {
	if node == nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(node.File)))
	if err != nil {
		return false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, node.File, data, 0)
	if err != nil {
		return false
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		if shortFunctionName(fd.Name.Name) != shortFunctionName(node.Function) {
			continue
		}
		params := map[string]bool{}
		if fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				for _, name := range field.Names {
					params[name.Name] = true
				}
			}
		}
		passThrough := 0
		total := 0
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			total++
			if len(ret.Results) == 1 {
				if ident, ok := ret.Results[0].(*ast.Ident); ok && params[ident.Name] {
					passThrough++
				}
			}
			return true
		})
		return total > 0 && passThrough == total
	}
	return false
}

func nodeRef(node domain.ProgramNode, line int) CausalTraceNode {
	return CausalTraceNode{Identity: node.Identity, File: node.FilePath, Function: node.Name, Line: line}
}

func hasUnresolved(edges []CausalTraceEdge) bool {
	for _, edge := range edges {
		if edge.Status == causalTraceStatusUnres || edge.Status == causalTraceStatusInfer {
			return true
		}
	}
	return false
}

func hasStatus(edges []CausalTraceEdge, status string) bool {
	for _, edge := range edges {
		if edge.Status == status {
			return true
		}
	}
	return false
}

func traceEdgeTargets(trace CausalTrace) string {
	var parts []string
	for _, edge := range trace.Edges {
		target := edge.To.Function
		if target == "" {
			target = edge.To.Identity
		}
		if target != "" {
			parts = append(parts, target)
		}
	}
	return strings.Join(parts, " -> ")
}

// admitCausalProducer adds every statically-supported node on the trace as a
// candidate so candidate discovery can surface a defect candidate absent from
// the observed stack. Each carries the causal evidence and a dependency score
// only; none is verified. The nearest producer is the primary lead; deeper
// nodes are alternates (depth is never treated as stronger evidence).
func admitCausalProducer(report *Report, snapshot programrepo.Snapshot, trace CausalTrace) {
	nodesByID := map[string]domain.ProgramNode{}
	for _, node := range snapshot.Nodes {
		nodesByID[node.Identity] = node
	}
	admitted := map[string]bool{}
	for _, existing := range report.Candidates {
		admitted[existing.Location.SymbolIdentity] = true
	}
	for _, edge := range trace.Edges {
		if edge.Status != causalTraceStatusStatic {
			continue
		}
		node, ok := nodesByID[edge.To.Identity]
		if !ok || node.Type == "Test" || node.FilePath == "" {
			continue
		}
		score := 0.9
		if trace.Producer != nil && edge.To.Identity == trace.Producer.Identity {
			score = 1.0
		}
		if index := candidateIndexByIdentity(report.Candidates, node.Identity); index >= 0 {
			// Promote an existing candidate to the causal-producer kind so
			// ordering and provenance reflect the trace evidence.
			report.Candidates[index].Location.Provenance = "causal_producer"
			if report.Candidates[index].Score.DependencyRelevance < score {
				report.Candidates[index].Score.DependencyRelevance = score
			}
			if !sliceContains(report.Candidates[index].Score.AvailableDimensions, "dependency") {
				report.Candidates[index].Score.AvailableDimensions = append(report.Candidates[index].Score.AvailableDimensions, "dependency")
			}
			if report.Candidates[index].Score.EvidenceByDimension == nil {
				report.Candidates[index].Score.EvidenceByDimension = map[string][]domain.ID{}
			}
			report.Candidates[index].Score.EvidenceByDimension["dependency"] = append(report.Candidates[index].Score.EvidenceByDimension["dependency"], stringIDsToDomain(edge.EvidenceIDs)...)
			for _, id := range edge.EvidenceIDs {
				report.Candidates[index].SupportingEvidenceIDs = appendUniqueID(report.Candidates[index].SupportingEvidenceIDs, id)
			}
			continue
		}
		if admitted[node.Identity] {
			continue
		}
		admitted[node.Identity] = true
		id := "candidate:causal:" + digestString(node.Identity)
		evidenceIDs := append([]string(nil), edge.EvidenceIDs...)
		report.Candidates = append(report.Candidates, domain.CandidateLocation{
			ID: id,
			Location: domain.CodeLocation{
				ID:             "location:" + id,
				RepositoryID:   node.RepositoryID,
				RevisionID:     node.RevisionID,
				FilePath:       node.FilePath,
				SymbolIdentity: node.Identity,
				StartLine:      node.StartLine,
				EndLine:        node.EndLine,
				LocationType:   node.Type,
				Provenance:     "causal_producer",
			},
			Score: domain.ScoreVector{
				DependencyRelevance:  score,
				AvailableDimensions:  []string{"dependency"},
				EvidenceByDimension:  map[string][]domain.ID{"dependency": stringIDsToDomain(evidenceIDs)},
				ScoringPolicyVersion: "vector-v1",
			},
			Status:                "open",
			SupportingEvidenceIDs: stringIDsToDomain(evidenceIDs),
		})
	}
	if trace.Producer != nil {
		report.CoverageNotes = append(report.CoverageNotes, "causal trace admitted producer "+trace.Producer.Function+" ("+trace.Producer.File+")")
	}
}

func candidateIndexByIdentity(candidates []domain.CandidateLocation, identity string) int {
	for i, candidate := range candidates {
		if candidate.Location.SymbolIdentity == identity {
			return i
		}
	}
	return -1
}

func sliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendUniqueID(ids []domain.ID, id string) []domain.ID {
	for _, existing := range ids {
		if string(existing) == id {
			return ids
		}
	}
	if id != "" {
		ids = append(ids, domain.ID(id))
	}
	return ids
}

// calleeParams returns the callee's parameter names in order and its receiver
// name (empty for a plain function).
func calleeParams(workspace string, callee domain.ProgramNode) ([]string, string, bool) {
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(callee.FilePath)))
	if err != nil {
		return nil, "", false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, callee.FilePath, data, 0)
	if err != nil {
		return nil, "", false
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil || fset.Position(fd.Pos()).Line != callee.StartLine {
			continue
		}
		var names []string
		if fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				for _, n := range field.Names {
					names = append(names, n.Name)
				}
			}
		}
		recv := ""
		if fd.Recv != nil {
			for _, field := range fd.Recv.List {
				for _, n := range field.Names {
					recv = n.Name
				}
			}
		}
		return names, recv, true
	}
	return nil, "", false
}

// argumentOrigin maps a parameter used at a failing operation back to the
// corresponding argument at a specific caller call site, resolving the
// argument's local provenance in the caller. ok=false means the parameter
// cannot be mapped; an ambiguous or unresolved mapping is returned with a
// populated Ambiguity list.
func argumentOrigin(workspace string, caller domain.ProgramNode, callLine int, callee domain.ProgramNode, paramName string) (producerCallResult, bool) {
	o, ok := buildFnOrigins(workspace, caller, callLine)
	if !ok {
		return producerCallResult{}, false
	}
	calls := o.callsByLine[callLine]
	var matching []*ast.CallExpr
	for _, c := range calls {
		if shortFunctionName(calleeName(c)) == shortFunctionName(callee.Name) {
			matching = append(matching, c)
		}
	}
	if len(matching) == 0 && len(calls) == 1 {
		matching = calls
	}
	switch {
	case len(matching) == 0:
		return producerCallResult{kind: "unresolved_argument", ambiguous: []string{"no call site to " + callee.Name + " at line " + strconv.Itoa(callLine)}}, true
	case len(matching) > 1:
		return producerCallResult{kind: "unresolved_argument", ambiguous: []string{"multiple call sites to " + callee.Name + " at line " + strconv.Itoa(callLine)}}, true
	}
	call := matching[0]
	params, recv, ok := calleeParams(workspace, callee)
	if !ok {
		return producerCallResult{}, false
	}
	if recv != "" && paramName == recv {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			return o.classifyArgument(sel.X)
		}
		return producerCallResult{kind: "unresolved_argument", ambiguous: []string{"receiver expression not resolvable"}}, true
	}
	idx := -1
	for i, p := range params {
		if p == paramName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return producerCallResult{}, false
	}
	if idx >= len(call.Args) {
		return producerCallResult{kind: "unresolved_argument", ambiguous: []string{"no argument for parameter " + paramName}}, true
	}
	if idx == len(params)-1 && len(call.Args)-1 > idx {
		return producerCallResult{kind: "unresolved_argument", ambiguous: []string{"variadic argument span is ambiguous"}}, true
	}
	return o.classifyArgument(call.Args[idx])
}

func (o *fnOrigins) classifyArgument(expr ast.Expr) (producerCallResult, bool) {
	line := o.fset.Position(expr.Pos()).Line
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == "nil" {
			return producerCallResult{kind: "nil_literal", line: line, name: "nil", text: sourceLine(o.data, line)}, true
		}
		if operandBuiltins[e.Name] {
			return producerCallResult{kind: "constant", line: line, name: e.Name, text: sourceLine(o.data, line)}, true
		}
		return o.resolve(e.Name)
	case *ast.CallExpr:
		if c := calleeName(e); c != "" && !isConversion(c) {
			return producerCallResult{kind: "call", line: line, callee: c, text: sourceLine(o.data, line)}, true
		}
		return producerCallResult{kind: "constant", line: line, text: sourceLine(o.data, line)}, true
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return o.resolve(id.Name)
		}
		return producerCallResult{kind: "unresolved_argument", line: line, text: sourceLine(o.data, line)}, true
	case *ast.BasicLit:
		return producerCallResult{kind: "constant", line: line, text: sourceLine(o.data, line)}, true
	default:
		return producerCallResult{kind: "unresolved_argument", line: line, text: sourceLine(o.data, line)}, true
	}
}

// callerSite is a statically-resolved call site into a callee.
type callerSite struct {
	source   domain.ProgramNode
	line     int
	verified bool
}

func parseCallLinesCSV(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

// callerSites lists statically-resolved callers of a callee from the snapshot.
func callerSites(nodesByID map[string]domain.ProgramNode, edgesByTarget map[string][]domain.Relationship, calleeID string) []callerSite {
	var out []callerSite
	for _, rel := range edgesByTarget[calleeID] {
		src, ok := nodesByID[rel.SourceID]
		if !ok || (src.Type != "Function" && src.Type != "Method") {
			continue
		}
		if strings.HasSuffix(src.FilePath, "_test.go") {
			continue
		}
		verified := rel.Metadata["provenance"] == "verified_static_call"
		for _, line := range parseCallLinesCSV(rel.Metadata["call_lines"]) {
			out = append(out, callerSite{source: src, line: line, verified: verified})
		}
	}
	return out
}

// parameterCallerSite selects the caller for a parameter. Preference:
//  1. the traversed call site (downward recursion),
//  2. a static caller site whose source matches the caller observed on the stack
//     (invocation evidence),
//  3. a unique statically-resolved caller.
//
// It returns ambiguous=true when several plausible callers remain.
func parameterCallerSite(traversed *domain.ProgramNode, traversedLine int, observed *domain.ProgramNode, nodesByID map[string]domain.ProgramNode, edgesByTarget map[string][]domain.Relationship, calleeID string) (*callerSite, bool) {
	if traversed != nil {
		return &callerSite{source: *traversed, line: traversedLine, verified: true}, false
	}
	sites := callerSites(nodesByID, edgesByTarget, calleeID)
	if observed != nil {
		// Only accept a static site whose source is the caller actually seen on
		// the observed stack. Never fall back to graph proximity.
		var matched []callerSite
		for _, s := range sites {
			if s.source.Identity == observed.Identity {
				matched = append(matched, s)
			}
		}
		switch len(matched) {
		case 1:
			return &matched[0], false
		case 0:
			return nil, false
		default:
			return nil, true
		}
	}
	var verified []callerSite
	for _, s := range sites {
		if s.verified {
			verified = append(verified, s)
		}
	}
	pick := sites
	if len(verified) > 0 {
		pick = verified
	}
	switch len(pick) {
	case 0:
		return nil, false
	case 1:
		return &pick[0], false
	default:
		return nil, true
	}
}

// observedFunctionChain collapses the ordered observed stack frames to the
// sequence of distinct application functions that actually executed, innermost
// first. Test and runtime frames are excluded.
func observedFunctionChain(frames []localization.Frame, nodesByID map[string]domain.ProgramNode) []domain.ProgramNode {
	var chain []domain.ProgramNode
	for _, frame := range frames {
		node, ok := enclosingNode(nodesByID, frame.File, frame.Line)
		if !ok || node.Type == "Test" || strings.HasSuffix(node.FilePath, "_test.go") {
			continue
		}
		if len(chain) > 0 && chain[len(chain)-1].Identity == node.Identity {
			continue
		}
		chain = append(chain, node)
	}
	return chain
}

// observedCallerOf returns the function that called identity according to the
// observed stack chain, or nil when the chain does not establish it.
func observedCallerOf(chain []domain.ProgramNode, identity string) *domain.ProgramNode {
	for i := 0; i+1 < len(chain); i++ {
		if chain[i].Identity == identity {
			caller := chain[i+1]
			return &caller
		}
	}
	return nil
}

const causalTraceMaxUpHops = 3

// resolveParameterUpward maps a parameter used at a failing operation to the
// caller's argument, following observed-caller evidence, and continues through
// wrapper parameters within the incremental hop budget. Every edge is inferred:
// a statically possible or observed caller is not proof the call is the cause.
func resolveParameterUpward(workspace string, snapshot programrepo.Snapshot, nodesByID map[string]domain.ProgramNode, edgesBySource, edgesByTarget map[string][]domain.Relationship, chain []domain.ProgramNode, traversed *domain.ProgramNode, traversedLine int, callee domain.ProgramNode, paramName string, from CausalTraceNode, opLine int, budget *causalTraceBudget, edges *[]CausalTraceEdge, limitations *[]string, depth int) {
	if depth >= causalTraceMaxUpHops {
		*limitations = append(*limitations, "caller chain exceeds the upward hop budget")
		return
	}
	if len(*edges) >= budget.maxEdges {
		*limitations = append(*limitations, "edge budget reached during caller provenance")
		return
	}
	var observed *domain.ProgramNode
	if traversed == nil {
		observed = observedCallerOf(chain, callee.Identity)
	}
	site, ambiguous := parameterCallerSite(traversed, traversedLine, observed, nodesByID, edgesByTarget, callee.ID)
	if ambiguous {
		*edges = append(*edges, CausalTraceEdge{From: from, Operation: "parameter " + paramName, Line: opLine, Relationship: "ambiguous", Status: causalTraceStatusUnres, Ambiguity: []string{"multiple plausible callers for " + callee.Name + "; none established"}})
		*limitations = append(*limitations, "parameter "+paramName+": multiple plausible callers; no caller selected")
		return
	}
	if site == nil {
		*edges = append(*edges, CausalTraceEdge{From: from, Operation: "parameter " + paramName, Line: opLine, Relationship: "parameter", Status: causalTraceStatusUnres})
		*limitations = append(*limitations, "operand "+paramName+" is a parameter; no single caller could be established")
		return
	}
	arg, ok := argumentOrigin(workspace, site.source, site.line, callee, paramName)
	if !ok {
		*edges = append(*edges, CausalTraceEdge{From: from, Operation: "parameter " + paramName, Line: opLine, Relationship: "parameter", Status: causalTraceStatusUnres})
		*limitations = append(*limitations, "parameter "+paramName+": caller argument could not be resolved")
		return
	}
	if len(arg.ambiguous) > 0 {
		*edges = append(*edges, CausalTraceEdge{From: from, Operation: "parameter " + paramName, Line: opLine, Relationship: "ambiguous", Status: causalTraceStatusUnres, Ambiguity: arg.ambiguous})
		*limitations = append(*limitations, "parameter "+paramName+" has multiple or unresolvable caller arguments")
		return
	}
	edge := CausalTraceEdge{From: from, Line: site.line, Relationship: "caller_argument", Status: causalTraceStatusInfer}
	switch arg.kind {
	case "call":
		edge.Operation = "caller calls " + arg.callee
		if target, rel, ok := resolveCallee(snapshot, nodesByID, edgesBySource, site.source.ID, arg.line, arg.callee); ok {
			edge.To = nodeRef(target, target.StartLine)
			if rel == causalTraceTextualCall {
				edge.Ambiguity = append(edge.Ambiguity, "caller call resolved by name, not type")
			}
		} else {
			edge.To = CausalTraceNode{Function: arg.callee}
			edge.Relationship = causalTraceUnindexed
			edge.Status = causalTraceStatusUnres
			edge.Ambiguity = append(edge.Ambiguity, "caller producer is outside the indexed graph")
		}
		*edges = append(*edges, edge)
		*limitations = append(*limitations, "operand "+paramName+" maps to the argument at "+site.source.FilePath+":"+strconv.Itoa(site.line)+"; the caller invocation is statically possible, not observed")
	case "parameter":
		edge.Operation = "caller parameter " + arg.name
		edge.To = CausalTraceNode{File: site.source.FilePath, Line: arg.line}
		*edges = append(*edges, edge)
		nextFrom := nodeRef(site.source, arg.line)
		resolveParameterUpward(workspace, snapshot, nodesByID, edgesBySource, edgesByTarget, chain, nil, 0, site.source, arg.name, nextFrom, arg.line, budget, edges, limitations, depth+1)
	case "nil_literal":
		edge.Operation = "caller passed nil for " + paramName
		edge.To = CausalTraceNode{File: site.source.FilePath, Line: arg.line}
		*edges = append(*edges, edge)
		*limitations = append(*limitations, "operand "+paramName+" maps to a nil argument at "+site.source.FilePath+":"+strconv.Itoa(site.line)+"; the caller invocation is statically possible, not observed")
	case "zero_value":
		edge.Operation = "caller-local zero value for " + paramName
		edge.To = CausalTraceNode{File: site.source.FilePath, Line: arg.line}
		*edges = append(*edges, edge)
		*limitations = append(*limitations, "operand "+paramName+" maps to a caller-local zero value at "+site.source.FilePath+":"+strconv.Itoa(site.line))
	default:
		edge.Operation = "caller argument for " + paramName
		edge.To = CausalTraceNode{File: site.source.FilePath, Line: arg.line}
		*edges = append(*edges, edge)
		*limitations = append(*limitations, "operand "+paramName+" maps to the argument at "+site.source.FilePath+":"+strconv.Itoa(site.line)+"; the caller invocation is statically possible, not observed")
	}
}

func defKind(callee string) string {
	if callee == "" {
		return "assignment"
	}
	return "call"
}

// assertionAnchor locates the failing test's assertion line when no observed
// implementation frame is available, reusing the test node and the file:line
// reported in the failure evidence.
func assertionAnchor(report *Report, snapshot programrepo.Snapshot, workspace string) (*domain.ProgramNode, int, bool) {
	text := failureText(report.Baseline, report.Evidence)
	if strings.TrimSpace(text) == "" {
		return nil, 0, false
	}
	for i := range snapshot.Nodes {
		node := snapshot.Nodes[i]
		if node.Type != "Test" || node.FilePath == "" {
			continue
		}
		if !strings.Contains(text, node.Name) && !strings.Contains(text, node.FilePath) {
			continue
		}
		if line := failedAssertionLine(workspace, node, text); line > 0 {
			copy := node
			return &copy, line, true
		}
	}
	return nil, 0, false
}

func failedAssertionLine(workspace string, testNode domain.ProgramNode, text string) int {
	re := regexp.MustCompile(regexp.QuoteMeta(filepath.Base(filepathSlash(testNode.FilePath))) + `:(\d+):`)
	for _, match := range re.FindAllStringSubmatch(text, -1) {
		line, err := strconv.Atoi(match[1])
		if err != nil || line <= 0 {
			continue
		}
		if line >= testNode.StartLine && (testNode.EndLine == 0 || line <= testNode.EndLine) {
			return line
		}
	}
	return 0
}
