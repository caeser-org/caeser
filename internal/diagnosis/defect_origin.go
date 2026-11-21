package diagnosis

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/caeser-org/ceaser/internal/domain"
	programrepo "github.com/caeser-org/ceaser/internal/repository"
)

// DefectOrigin is an evidence-grounded hypothesis about the operation that may
// have introduced the invalid state consumed by the failure. It is never a
// verified cause: status is "inferred" (hypothesis), and it carries the operand,
// location, and the causal relationship to the failing operation.
type DefectOrigin struct {
	Kind       string `json:"kind"` // later_assignment | ignored_error | missing_guard | boundary_guard
	Operand    string `json:"operand,omitempty"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Operation  string `json:"operation,omitempty"`
	Rationale  string `json:"rationale"`
	Status     string `json:"status"`
	EvidenceID string `json:"evidence_id,omitempty"`
	// EvidenceKind is the strength of the relationship: source_pattern (AST-only)
	// or static_data_dependency (an SSA value/type at the location supports it).
	EvidenceKind string `json:"evidence_kind,omitempty"`
	// DeltaRelation links the hypothesis to base/head changes without implying
	// causation: changed (same symbol), changed_file (same file), unchanged.
	DeltaRelation string   `json:"delta_relation,omitempty"`
	Ambiguity     []string `json:"ambiguity,omitempty"`
}

// defectContext carries optional stronger evidence the AST assessment can use.
type defectContext struct {
	ssa    *programrepo.SSATrace
	deltas []domain.CodeDelta
}

// ssaTypeAt returns the SSA-recorded type of a value at file:line, if any.
func (c defectContext) ssaTypeAt(file string, line int) (string, bool) {
	if c.ssa == nil {
		return "", false
	}
	for _, v := range c.ssa.Values {
		if filepathSlash(v.File) == filepathSlash(file) && v.Line == line && v.Type != "" {
			return v.Type, true
		}
	}
	return "", false
}

// ssaOnPath reports whether an SSA value was recorded at file:line, i.e. the
// location participates in the assertion dependency trace.
func (c defectContext) ssaOnPath(file string, line int) bool {
	if c.ssa == nil {
		return false
	}
	for _, v := range c.ssa.Values {
		if filepathSlash(v.File) == filepathSlash(file) && v.Line == line {
			return true
		}
	}
	return false
}

// ssaCoversFile reports whether the assertion dependency trace captured any value
// in the file, i.e. the file was SSA-traced and can be used to judge relevance.
func (c defectContext) ssaCoversFile(file string) bool {
	if c.ssa == nil {
		return false
	}
	for _, v := range c.ssa.Values {
		if filepathSlash(v.File) == filepathSlash(file) {
			return true
		}
	}
	return false
}

// deltaRelation classifies a hypothesis location against base/head changes. It
// never implies causation.
func (c defectContext) deltaRelation(file, symbol string) string {
	if len(c.deltas) == 0 {
		return ""
	}
	sameFile := false
	for _, d := range c.deltas {
		if filepathSlash(d.FilePath) != filepathSlash(file) {
			continue
		}
		sameFile = true
		if symbol != "" && d.SymbolIdentity == symbol {
			return "changed"
		}
	}
	if sameFile {
		return "changed_file"
	}
	return "unchanged"
}

// isNilableType reports whether a Go type can hold a nil/zero value whose
// dereference could fault. Unknown types are handled by the caller.
func isNilableType(typ string) bool {
	typ = strings.TrimSpace(typ)
	switch {
	case strings.HasPrefix(typ, "*"), strings.HasPrefix(typ, "map["),
		strings.HasPrefix(typ, "[]"), strings.HasPrefix(typ, "chan "),
		strings.HasPrefix(typ, "func("), strings.HasPrefix(typ, "interface"),
		typ == "interface{}", typ == "any", typ == "error":
		return true
	}
	return false
}

// assessDefectOrigins inspects the operations that feed the failing operand at
// failLine and returns bounded, evidence-grounded defect-origin hypotheses. It
// reports an operation only when a concrete relationship to the failing operand
// is present; it never reports nearby-but-unrelated code.
func assessDefectOrigins(workspace string, fn domain.ProgramNode, failLine int, ctx defectContext) []DefectOrigin {
	o, ok := buildFnOrigins(workspace, fn, failLine)
	if !ok {
		return nil
	}
	operands := operandIdentifiers(o.fd.Body, o.fset, failLine)
	failText := strings.TrimSpace(sourceLine(o.data, failLine))
	var out []DefectOrigin
	seen := map[string]bool{}
	add := func(d DefectOrigin) {
		key := fmt.Sprintf("%s|%s|%d", d.Kind, d.Operand, d.Line)
		if seen[key] {
			return
		}
		seen[key] = true
		if d.EvidenceKind == "" {
			d.EvidenceKind = "source_pattern"
		}
		if d.EvidenceKind == "source_pattern" && ctx.ssaOnPath(d.File, d.Line) {
			d.EvidenceKind = "static_data_dependency"
		}
		// Honest negative signal: when the enclosing file was SSA-traced but this
		// operation was not on the assertion dependency path, record it. This does
		// not suppress the hypothesis (the operation may be control-only).
		if ctx.ssaCoversFile(d.File) && !ctx.ssaOnPath(d.File, d.Line) {
			d.Ambiguity = append(d.Ambiguity, "the file was SSA-traced but this operation was not on the failing assertion's dependency path")
		}
		if d.DeltaRelation == "" {
			d.DeltaRelation = ctx.deltaRelation(d.File, fn.Identity)
		}
		out = append(out, d)
	}
	for name := range operands {
		defs := definitionsBefore(o.allDefs[name], failLine)
		// A parameter is the original definition even though it has no local
		// assignment record.
		if o.params[name] {
			param := producerCallResult{kind: "parameter", line: o.fset.Position(o.fd.Pos()).Line, name: name}
			defs = append([]producerCallResult{param}, defs...)
		}
		if len(defs) == 0 {
			continue
		}
		last := defs[len(defs)-1]

		// A. Later assignment: the operand is reassigned before the failure. The
		// earlier producer and the later mutation are both reported as context,
		// and multiple reaching definitions are kept as explicit alternatives.
		if len(defs) >= 2 {
			first := defs[0]
			d := DefectOrigin{
				Kind: "later_assignment", Operand: name, File: o.fn.FilePath, Line: last.line,
				Operation: last.text, Status: "inferred",
				Rationale: fmt.Sprintf("operand %s is defined at line %d then reassigned at line %d before the failing operation; the later assignment is a candidate defect-introducing mutation", name, first.line, last.line),
			}
			if len(defs) > 2 || definitionsAreBranchDependent(o, defs) {
				d.Ambiguity = append(d.Ambiguity, "multiple reaching definitions; the definition that executed is not established")
			}
			for _, def := range defs {
				d.Ambiguity = append(d.Ambiguity, fmt.Sprintf("line %d: %s", def.line, strings.TrimSpace(def.text)))
			}
			add(d)
		}

		// E. Ignored/overwritten error from the operand's producing call.
		if callee, ok := o.errorIgnored[name]; ok {
			add(DefectOrigin{
				Kind: "ignored_error", Operand: name, File: o.fn.FilePath, Line: last.line,
				Operation: last.text, Status: "inferred",
				Rationale: fmt.Sprintf("the error result of %s is discarded (assigned to _) before %s is used by the failing operation", callee, name),
			})
		}

		// B. Missing guard: the failing operation dereferences the operand and no
		// nil check on it occurs between the producer and the failure. Type-aware
		// when SSA records the operand's type; otherwise an inferred AST pattern.
		if dereferences(failText, name) && !nilGuardDominates(o, name, last.line, failLine) {
			d := DefectOrigin{
				Kind: "missing_guard", Operand: name, File: o.fn.FilePath, Line: last.line,
				Operation: last.text, Status: "inferred",
			}
			typ, typed := ctx.ssaTypeAt(o.fn.FilePath, last.line)
			switch {
			case typed && !isNilableType(typ):
				// A known non-nilable type cannot produce a nil dereference here.
				continue
			case typed:
				d.EvidenceKind = "static_data_dependency"
				d.Rationale = fmt.Sprintf("the failing operation dereferences %s of type %s produced at line %d with no nil check between producer and failure", name, typ, last.line)
			default:
				d.Rationale = fmt.Sprintf("the failing operation dereferences %s, which is produced at line %d, and no nil check on %s occurs between the producer and the failure", name, last.line, name)
				d.Ambiguity = append(d.Ambiguity, "operand type is unavailable without SSA; nil-ability is not established")
			}
			if ctx.ssaOnPath(o.fn.FilePath, last.line) {
				d.EvidenceKind = "static_data_dependency"
			}
			add(d)
		}
	}

	// C. Boundary guard: an index used at the failing operation is guarded by a
	// prior comparison against len(...) using <= or >=.
	for _, boundary := range boundaryGuards(o, failLine) {
		add(boundary)
	}

	return out
}

func definitionsBefore(defs []producerCallResult, line int) []producerCallResult {
	var out []producerCallResult
	for _, d := range defs {
		if d.line > 0 && d.line <= line {
			out = append(out, d)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].line > out[j].line; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func dereferences(text, name string) bool {
	return strings.Contains(text, name+".")
}

// nilGuardDominates reports whether a nil check on name actually protects the
// failing operation at `to`: either `name != nil` guards a block containing `to`,
// or `name == nil` guards an early-exit block reached before `to`. A nil check in
// a sibling or unrelated branch does not dominate and does not suppress the
// hypothesis.
func nilGuardDominates(o *fnOrigins, name string, from, to int) bool {
	for _, stmt := range o.fd.Body.List {
		ifStmt, ok := stmt.(*ast.IfStmt)
		if !ok {
			continue
		}
		gLine := o.fset.Position(ifStmt.Pos()).Line
		if gLine <= from || gLine >= to {
			continue
		}
		bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || (bin.Op.String() != "==" && bin.Op.String() != "!=") {
			continue
		}
		nilCompare := (identIs(bin.X, name) && identIs(bin.Y, "nil")) || (identIs(bin.Y, name) && identIs(bin.X, "nil"))
		if !nilCompare {
			continue
		}
		switch bin.Op.String() {
		case "!=":
			if blockContains(o.fset, ifStmt.Body, to) {
				return true
			}
		case "==":
			if blockTerminates(o.fset, ifStmt.Body) && to > o.fset.Position(ifStmt.Body.End()).Line {
				return true
			}
		}
	}
	return false
}

func blockContains(fset *token.FileSet, block *ast.BlockStmt, line int) bool {
	if block == nil {
		return false
	}
	return line >= fset.Position(block.Pos()).Line && line <= fset.Position(block.End()).Line
}

// blockTerminates reports whether control cannot fall through the block (it ends
// with a return, panic, or a labeled branch statement).
func blockTerminates(fset *token.FileSet, block *ast.BlockStmt) bool {
	if block == nil || len(block.List) == 0 {
		return false
	}
	last := block.List[len(block.List)-1]
	switch s := last.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
				return true
			}
		}
	}
	return false
}

func identIs(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

func boundaryGuards(o *fnOrigins, failLine int) []DefectOrigin {
	uses := indexUses(o.fd.Body, o.fset, failLine)
	if len(uses) == 0 {
		return nil
	}
	var out []DefectOrigin
	ast.Inspect(o.fd.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || o.fset.Position(ifStmt.Pos()).Line >= failLine {
			return true
		}
		bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if bin.Op.String() != "<=" && bin.Op.String() != ">=" {
			return true
		}
		// The guard must compare the failing index against len(<the same
		// collection>) — an unrelated len() must not match.
		leftLen, rightLen := lenOf(bin.X), lenOf(bin.Y)
		for _, u := range uses {
			matched := (leftLen == u.collection && exprUsesIdent(bin.Y, u.index)) ||
				(rightLen == u.collection && exprUsesIdent(bin.X, u.index))
			if !matched {
				continue
			}
			line := o.fset.Position(ifStmt.Pos()).Line
			out = append(out, DefectOrigin{
				Kind: "boundary_guard", Operand: u.index, File: o.fn.FilePath, Line: line,
				Operation: strings.TrimSpace(sourceLine(o.data, line)), Status: "inferred",
				Rationale: fmt.Sprintf("the guard at line %d compares %s against len(%s) with %q, which permits the failing index value to reach the indexing operation", line, u.index, u.collection, bin.Op.String()),
			})
		}
		return true
	})
	return out
}

type indexUse struct{ collection, index string }

func indexUses(body *ast.BlockStmt, fset *token.FileSet, line int) []indexUse {
	var out []indexUse
	ast.Inspect(body, func(n ast.Node) bool {
		idx, ok := n.(*ast.IndexExpr)
		if !ok || fset.Position(idx.Pos()).Line != line {
			return true
		}
		coll, ok := idx.X.(*ast.Ident)
		if !ok {
			return true
		}
		index, ok := idx.Index.(*ast.Ident)
		if !ok {
			return true
		}
		out = append(out, indexUse{collection: coll.Name, index: index.Name})
		return true
	})
	return out
}

// lenOf returns the argument identifier of a len(x) call, or "".
func lenOf(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return ""
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "len" {
		return ""
	}
	if arg, ok := call.Args[0].(*ast.Ident); ok {
		return arg.Name
	}
	return ""
}

func exprUsesIdent(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// definitionsAreBranchDependent reports whether any definition lies inside an
// if/else block, so the definitions are alternatives rather than a linear chain.
func definitionsAreBranchDependent(o *fnOrigins, defs []producerCallResult) bool {
	inBranch := map[int]bool{}
	ast.Inspect(o.fd.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		mark := func(node ast.Node) {
			if node == nil {
				return
			}
			for l := o.fset.Position(node.Pos()).Line; l <= o.fset.Position(node.End()).Line; l++ {
				inBranch[l] = true
			}
		}
		mark(ifStmt.Body)
		mark(ifStmt.Else)
		return true
	})
	for _, d := range defs {
		if inBranch[d.line] {
			return true
		}
	}
	return false
}
