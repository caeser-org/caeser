package diagnosis

import (
	"fmt"
	"regexp"
	"strings"

	programrepo "github.com/caeser-org/ceaser/internal/repository"
)

// FailureFacts are the directly observed, provider-neutral semantics of a
// failure. Every field is read from evidence that already exists in the report
// (normalized CI observations or the original CI log) using the same parsers the
// rest of the pipeline uses. Nothing here is inferred.
type FailureFacts struct {
	Test     string `json:"test,omitempty"`
	Subtest  string `json:"subtest,omitempty"`
	Message  string `json:"message,omitempty"`
	Panic    string `json:"panic,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// InvestigationCheck is one concrete next step derived from the specific
// unresolved causal question. It names the operation to inspect, the evidence
// that would resolve it, and how that evidence would confirm or reject the
// hypothesis. It is not generic reproduction advice.
type InvestigationCheck struct {
	Target         string `json:"target"`
	EvidenceNeeded string `json:"evidence_needed"`
	ConfirmsIf     string `json:"confirms_if"`
	RejectsIf      string `json:"rejects_if"`
}

// CausalExplanation is an evidence-grounded, explicitly unverified hypothesis
// that connects the observed failure semantics to the primary suspect. Observed
// facts and the inferred hypothesis are separate fields so a hypothesis is never
// presented as an established root cause. It never changes classification,
// confidence, verification, promotion, or abstention.
type CausalExplanation struct {
	Status     string        `json:"status"`
	Failure    *FailureFacts `json:"failure,omitempty"`
	Suspect    string        `json:"suspect,omitempty"`
	Observed   string        `json:"observed,omitempty"`
	Hypothesis string        `json:"hypothesis,omitempty"`
	// OperationSupport records how well the cited operation is connected to the
	// failure, so a hypothesis is never read as a causally connected fact:
	//   evidence_supported   - typed SSA value on the assertion path, a function
	//                          on a type-resolved call path from the failing
	//                          test, or the function enclosing an observed frame
	//   source_correlated    - the failure text names an operation the suspect's
	//                          source calls, but the connection is textual
	//   source_associated    - the operation is in the suspect's source/SSA
	//                          evidence but is not shown to feed the failure
	//   requires_investigation - no concrete operation was isolated
	OperationSupport string               `json:"operation_support,omitempty"`
	Unresolved       string               `json:"unresolved,omitempty"`
	Checks           []InvestigationCheck `json:"checks,omitempty"`
	EvidenceIDs      []string             `json:"evidence_ids,omitempty"`
	Limitations      []string             `json:"limitations,omitempty"`
}

// goSourceLine matches both evidence excerpt formats used in the report
// ("checked-out source" and "checked-out revision") because both write
// "<line>| <text>" rows.
var goSourceLine = regexp.MustCompile(`^(\d+)\| (.*)$`)

// causalToken matches a qualified Go identifier (a.b.C) in failure text. Only
// dotted tokens are used for correlation, which keeps common English words out.
var causalToken = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+)\b`)

// methodCall matches a selector call such as "v.Convert(" in a source line.
var methodCall = regexp.MustCompile(`\.[A-Za-z_][A-Za-z0-9_]*\(`)

// BuildCausalExplanation produces the causal hypothesis tier from evidence that
// already exists in the report. It is deterministic and read-only: it does not
// append evidence, add relationships, or upgrade any root-cause status.
func BuildCausalExplanation(report Report) CausalExplanation {
	result := CausalExplanation{Status: "unavailable"}
	if report.Refused {
		result.Limitations = append(result.Limitations,
			"diagnosis abstained, so no causal hypothesis is offered")
		return result
	}
	facts := extractFailureFacts(report)
	if hasFailureFacts(facts) {
		result.Failure = &facts
		result.Observed = facts.summary()
	}

	lf := report.LikelyFunction
	if lf == nil {
		built := BuildLikelyFunction(report)
		lf = &built
	}
	if lf.Primary == nil {
		result.Limitations = append(result.Limitations,
			"no implementation function was admitted, so no causal hypothesis is offered")
		return result
	}
	primary := lf.Primary

	op := suspectOperation(report, *primary)
	result.Suspect = primary.Function
	if op.evidenceID != "" {
		result.EvidenceIDs = appendUniqueString(result.EvidenceIDs, op.evidenceID)
	}

	support, supportPhrase := classifyOperationSupport(report, *primary, op)
	result.OperationSupport = support
	result.Hypothesis = buildHypothesis(facts, *primary, op, supportPhrase)
	result.Unresolved = unresolvedQuestion(facts, *primary, op)
	result.Checks = buildChecks(facts, *primary, op)
	// When the manifestation frame is not the producer, surface the producer the
	// backward trace resolved so the explanation points at the defect origin
	// rather than only where the failure surfaced.
	if trace := report.CausalTrace; trace != nil && trace.Producer != nil &&
		shortFunctionName(trace.Producer.Function) != shortFunctionName(primary.Function) {
		prod := trace.Producer
		result.Hypothesis += fmt.Sprintf(" The failing operation propagated a value or error produced by %s (%s:%d); inspect that producer too.",
			prod.Function, prod.File, prod.Line)
		result.Checks = append([]InvestigationCheck{{
			Target:         fmt.Sprintf("producer %s at %s:%d", prod.Function, prod.File, prod.Line),
			EvidenceNeeded: fmt.Sprintf("the value %s returns during the failing invocation", prod.Function),
			ConfirmsIf:     "that value equals the failure the caller propagated",
			RejectsIf:      "the producer cannot return the failing value or is not on the failing path",
		}}, result.Checks...)
	}
	// Defect-origin hypotheses: the operation that may have introduced the
	// invalid state. Reported as an unverified hypothesis with a concrete check.
	if trace := report.CausalTrace; trace != nil {
		for _, d := range trace.Defects[:min(len(trace.Defects), 2)] {
			result.Hypothesis += " Potential defect origin: " + bound(d.Rationale, 400)
			result.Checks = append(result.Checks, InvestigationCheck{
				Target:         fmt.Sprintf("operation %s at %s:%d", bound(d.Operation, 200), d.File, d.Line),
				EvidenceNeeded: "whether this operation can produce the invalid value the failing operand used",
				ConfirmsIf:     "this operation's result reaches the failing operand on the failing path",
				RejectsIf:      "this operation is not on the failing path or its result does not reach the operand",
			})
		}
		// Retain competing hypotheses rather than presenting one as the cause.
		if len(trace.Defects) > 1 {
			var alts []string
			for _, d := range trace.Defects[:min(len(trace.Defects), 4)] {
				alts = append(alts, fmt.Sprintf("%s at %s:%d", d.Kind, d.File, d.Line))
			}
			result.Limitations = append(result.Limitations, "competing defect hypotheses retained: "+strings.Join(alts, "; "))
		}
	}
	result.Limitations = append(result.Limitations,
		"this is an investigation hypothesis derived from observed evidence, not a verified root cause")
	if result.OperationSupport != "evidence_supported" {
		result.Limitations = append(result.Limitations,
			"the cited operation is associated with the suspect but is not connected to the failure by verified evidence")
	}
	result.Status = "hypothesis"
	return result
}

// classifyOperationSupport separates an operation that is directly supported by
// evidence (a typed SSA value on the assertion path, a function on a
// type-resolved call path from the failing test, or the function enclosing an
// observed failure frame) from one that is merely associated with the suspect.
// It returns the category and the exact basis phrase, and never upgrades the
// suspect's confidence: "evidence_supported" describes the operation's
// evidentiary basis, not a proven root cause.
func classifyOperationSupport(report Report, primary LikelyFunctionLead, op suspectOp) (string, string) {
	if primary.Kind == "causal_producer" {
		if trace := report.CausalTrace; trace != nil {
			for _, edge := range trace.Edges {
				if shortFunctionName(edge.To.Function) == shortFunctionName(primary.Function) {
					if edge.Status == causalTraceStatusStatic {
						return "evidence_supported", "a type-resolved call edge from the failing operation"
					}
					return "source_associated", "a same-package name match, not a type-resolved call"
				}
			}
		}
		return "source_associated", "the failure's producer path, without a type-resolved edge"
	}
	if op.fromSSA {
		return "evidence_supported", "a typed SSA value on the failed assertion's path"
	}
	if primary.Kind == "observed_stack" {
		return "evidence_supported", "an observed failure frame"
	}
	if op.expr != "" && failingTestPathTargets(report)[fileFunctionKey(primary.File, primary.Function)] {
		return "evidence_supported", "a type-resolved call path from the failing test"
	}
	if op.token != "" {
		return "source_correlated", "a textual match with the failure text, not a proven data flow"
	}
	if op.expr != "" {
		return "source_associated", "its presence in the suspect's source, without a shown link to the failure"
	}
	return "requires_investigation", ""
}

type suspectOp struct {
	expr       string
	file       string
	line       int
	evidenceID string
	fromSSA    bool
	token      string // the failure-text token this operation matched, when any
}

// suspectOperation finds the concrete operation to examine in the primary
// suspect, using evidence that already exists. SSA values in the trace take
// precedence (they are typed static dependencies); otherwise a parsed source
// excerpt around the suspect is used.
func suspectOperation(report Report, primary LikelyFunctionLead) suspectOp {
	if value, ok := ssaOperationFor(report, primary); ok {
		return suspectOp{expr: value.Text, file: value.File, line: value.Line, fromSSA: true,
			evidenceID: ssaEvidenceID(report, value)}
	}
	excerpt, evidenceID := sourceExcerptEvidence(report, primary)
	if excerpt == "" {
		return suspectOp{}
	}
	lines := parseExcerptLines(excerpt)
	if len(lines) == 0 {
		return suspectOp{}
	}
	// Prefer a line whose call name matches an operation named by the failure
	// text. This is a textual correlation between two observed artifacts (the
	// CI message and the checked-out source), not a data-flow claim.
	for _, token := range failureOperationTokens(report) {
		segment := token
		if idx := strings.LastIndex(token, "."); idx >= 0 {
			segment = token[idx+1:]
		}
		if line, text, ok := excerptLineWithCall(lines, segment, primary.Line); ok {
			return suspectOp{expr: text, file: primary.File, line: line, evidenceID: evidenceID, token: token}
		}
	}
	if line, text, ok := excerptLineWithCall(lines, "", primary.Line); ok {
		return suspectOp{expr: text, file: primary.File, line: line, evidenceID: evidenceID}
	}
	return suspectOp{file: primary.File, line: primary.Line, evidenceID: evidenceID}
}

func buildHypothesis(facts FailureFacts, primary LikelyFunctionLead, op suspectOp, supportPhrase string) string {
	label := primaryLabel(primary)
	var b strings.Builder
	b.WriteString(label)
	b.WriteString(" is the leading suspect because ")
	b.WriteString(leadReasonPhrase(primary))
	b.WriteString(".")
	if op.expr != "" {
		b.WriteString(" The concrete operation to examine is ")
		b.WriteString(op.expr)
		b.WriteString(" at ")
		b.WriteString(fmt.Sprintf("%s:%d", op.file, op.line))
		if op.token != "" {
			b.WriteString(fmt.Sprintf("; the failure text names %s, which this function's source calls here", op.token))
		}
		b.WriteString(".")
	}
	if supportPhrase != "" {
		b.WriteString(" This operation is associated with ")
		b.WriteString(supportPhrase)
		b.WriteString(". ")
	}
	if hasFailureFacts(facts) {
		b.WriteString("The hypothesis is that it produces or passes the value that the failure rejected (")
		b.WriteString(facts.summary())
		b.WriteString("). It is not established.")
	} else {
		b.WriteString("The hypothesis is that this function produced the value the failure rejected. It is not established.")
	}
	return b.String()
}

func unresolvedQuestion(facts FailureFacts, primary LikelyFunctionLead, op suspectOp) string {
	switch {
	case op.fromSSA:
		return fmt.Sprintf("whether the static SSA operand path into %s is feasible at runtime and its value equals the observed failure", primary.Function)
	case op.expr != "" && facts.Panic != "":
		return fmt.Sprintf("which operation in %s produced the value reported by the panic, and which invariant it violated", primary.Function)
	case op.expr != "":
		return fmt.Sprintf("whether %s produced the value compared by the failed assertion", op.expr)
	default:
		return fmt.Sprintf("which operation in %s produced the failing value", primary.Function)
	}
}

func buildChecks(facts FailureFacts, primary LikelyFunctionLead, op suspectOp) []InvestigationCheck {
	test := facts.Test
	if test == "" {
		test = "the failing test"
	}
	fail := shortFact(facts)
	var checks []InvestigationCheck

	if op.expr != "" {
		loc := fmt.Sprintf("%s:%d", op.file, op.line)
		target := fmt.Sprintf("operation %s at %s in %s", op.expr, loc, primary.Function)
		checks = append(checks, InvestigationCheck{
			Target:         target,
			EvidenceNeeded: fmt.Sprintf("the runtime argument or return value observed at %s during %s", loc, test),
			ConfirmsIf:     fmt.Sprintf("the value observed at this operation matches the recorded failure (%s)", fail),
			RejectsIf:      "this operation is not executed on the failing path, or the value it produces differs from the recorded failure",
		})
	}
	if op.fromSSA {
		checks = append(checks, InvestigationCheck{
			Target:         fmt.Sprintf("the SSA operand path feeding the failed assertion in %s", primary.Function),
			EvidenceNeeded: fmt.Sprintf("branch feasibility and the runtime value of that operand path during %s", test),
			ConfirmsIf:     "the operand value equals the recorded failing value and the branch executes",
			RejectsIf:      "the branch is infeasible or the operand value differs from the recorded failure",
		})
	}
	if len(checks) == 0 {
		checks = append(checks, InvestigationCheck{
			Target:         fmt.Sprintf("the code path from %s:%d (%s) to the failure", primary.File, primary.Line, primary.Function),
			EvidenceNeeded: fmt.Sprintf("the value this frame hands to its caller during %s", test),
			ConfirmsIf:     fmt.Sprintf("that value matches the recorded failure (%s)", fail),
			RejectsIf:      "this frame is not on the failing path or its value differs from the recorded failure",
		})
	}
	return checks
}

func shortFact(facts FailureFacts) string {
	switch {
	case facts.Panic != "":
		return bound(facts.Panic, 160)
	case facts.Message != "":
		return bound(facts.Message, 160)
	default:
		return facts.Test
	}
}

func primaryLabel(primary LikelyFunctionLead) string {
	if primary.Function != "" {
		return fmt.Sprintf("%s (%s:%d)", primary.Function, primary.File, primary.Line)
	}
	return fmt.Sprintf("%s:%d", primary.File, primary.Line)
}

func leadReasonPhrase(primary LikelyFunctionLead) string {
	switch primary.Kind {
	case "observed_stack":
		return "it appears in the observed failure stack"
	case "causal_producer":
		return "it is the statically-resolved producer of the value or error the failing operation propagated"
	case "inferred_call":
		return "a resolved call relationship connects it to the failure"
	case "inferred_source":
		if strings.Contains(primary.Reason, "operand dependency path") {
			return "it is on the failed assertion's operand dependency path"
		}
		return "it is on the value dependency path reported for the failed assertion"
	case "probe_operation":
		return "a bounded runtime probe selected it"
	default:
		return "the reported evidence ranks it as a lead"
	}
}

// ssaOperationFor returns the highest-signal SSA value in the trace that belongs
// to the suspect function.
func ssaOperationFor(report Report, primary LikelyFunctionLead) (programrepo.SSAValue, bool) {
	trace := report.SSAAssertionTrace
	if trace == nil {
		return programrepo.SSAValue{}, false
	}
	var best programrepo.SSAValue
	found := false
	for _, value := range trace.Values {
		if value.Line <= 0 || value.File == "" || strings.HasSuffix(value.File, "_test.go") {
			continue
		}
		if filepathSlash(value.File) != filepathSlash(primary.File) {
			continue
		}
		if shortSSAFunctionName(value.Function) != primary.Function {
			continue
		}
		if !found || ssaValueRank(value) > ssaValueRank(best) {
			best = value
			found = true
		}
	}
	return best, found
}

func ssaEvidenceID(report Report, value programrepo.SSAValue) string {
	for _, item := range report.Evidence {
		if strings.Contains(item.Detail, value.ID) || (value.Text != "" && strings.Contains(item.Detail, value.Text)) {
			return item.ID
		}
	}
	return "ssa:" + value.ID
}

// sourceExcerptEvidence returns the parsed source excerpt that encloses the
// suspect's location, when the report carries one.
func sourceExcerptEvidence(report Report, primary LikelyFunctionLead) (string, string) {
	want := filepathSlash(primary.File)
	for _, item := range report.Evidence {
		if item.Kind != "source_excerpt" && item.Kind != "source_code" {
			continue
		}
		if filepathSlash(item.Source) != want {
			continue
		}
		lines := parseExcerptLines(item.Detail)
		if excerptCovers(lines, primary.Line) {
			return item.Detail, item.ID
		}
	}
	return "", ""
}

func parseExcerptLines(detail string) map[int]string {
	out := map[int]string{}
	for _, line := range strings.Split(detail, "\n") {
		if match := goSourceLine.FindStringSubmatch(line); match != nil {
			out[atoi(match[1])] = match[2]
		}
	}
	return out
}

func excerptCovers(lines map[int]string, target int) bool {
	if target <= 0 || len(lines) == 0 {
		return false
	}
	if _, ok := lines[target]; ok {
		return true
	}
	// The excerpt spans a few lines around the reported location; accept it when
	// it brackets the target within a small window.
	for line := target - 2; line <= target+4; line++ {
		if _, ok := lines[line]; ok {
			return true
		}
	}
	return false
}

// excerptLineWithCall returns the first line at or after the suspect's location
// that looks like a call or value-producing statement, optionally requiring a
// specific call name. Lines are scanned in source order.
func excerptLineWithCall(lines map[int]string, callName string, from int) (int, string, bool) {
	var bestLine int
	var bestText string
	for line, text := range lines {
		if line < from {
			continue
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "func ") {
			continue
		}
		if !strings.Contains(trimmed, "(") {
			continue
		}
		if callName != "" {
			if !strings.Contains(trimmed, "."+callName+"(") && !strings.Contains(trimmed, callName+"(") {
				continue
			}
		} else if !methodCall.MatchString(trimmed) && !strings.Contains(trimmed, "return ") {
			continue
		}
		if bestText == "" || line < bestLine {
			bestLine, bestText = line, trimmed
		}
	}
	return bestLine, bestText, bestText != ""
}

// failureOperationTokens collects qualified identifiers named in the observed
// failure text. They are only used to correlate the failure message with an
// operation in the suspect's source.
func failureOperationTokens(report Report) []string {
	facts := extractFailureFacts(report)
	text := strings.Join([]string{facts.Panic, facts.Message}, " ")
	seen := map[string]bool{}
	var tokens []string
	for _, match := range causalToken.FindAllStringSubmatch(text, -1) {
		token := match[1]
		if len(token) < 5 || seen[token] {
			continue
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	return tokens
}

// extractFailureFacts reads the observed failure from the report's own evidence.
// It reuses the canonical Go test parser so the semantics match every other
// consumer of the failure log.
func extractFailureFacts(report Report) FailureFacts {
	facts := FailureFacts{}
	text := failureText(report.Baseline, report.Evidence)

	// The canonical parser already composes panic/expected/actual messages.
	var chosen *TestObservation
	for _, obs := range goTestFailures(text) {
		if obs.State != StateFailed {
			continue
		}
		if strings.Contains(obs.Name, "\t") || strings.HasPrefix(obs.Name, "FAIL") {
			continue
		}
		if chosen == nil || len(obs.Message) > len(chosen.Message) {
			copy := obs
			chosen = &copy
		}
	}
	if chosen != nil {
		facts.Test = chosen.Name
		if idx := strings.Index(facts.Test, "/"); idx > 0 {
			facts.Subtest = facts.Test[idx+1:]
			facts.Test = facts.Test[:idx]
		}
		facts.Message = strings.TrimSpace(chosen.Message)
	}

	// Fall back to the failure episodes when the CI log is absent (for example a
	// JUnit-only input).
	if facts.Message == "" {
		for _, episode := range report.FailureEpisodes {
			name := strings.TrimSpace(episode.TestIdentifier)
			if name == "" || strings.Contains(name, "\t") || strings.HasPrefix(name, "FAIL") {
				continue
			}
			msg := strings.TrimSpace(episode.FailureMessage)
			if msg == name {
				msg = ""
			}
			facts.Test = testNameFromIdentifier(name)
			facts.Message = msg
			break
		}
	}

	for _, match := range runtimeDetailLine.FindAllStringSubmatch(text, -1) {
		if detail := strings.TrimSpace(match[1]); detail != "" {
			facts.Panic = detail
			break
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if facts.Expected == "" {
			if match := goAssertionExpected.FindStringSubmatch(line); match != nil {
				facts.Expected = strings.TrimSpace(match[1])
			}
		}
		if facts.Actual == "" {
			if match := goAssertionActual.FindStringSubmatch(line); match != nil {
				facts.Actual = strings.TrimSpace(match[1])
			}
		}
	}
	return facts
}

func hasFailureFacts(facts FailureFacts) bool {
	return facts.Test != "" || facts.Message != "" || facts.Panic != ""
}

func (facts FailureFacts) summary() string {
	subject := facts.Test
	if facts.Subtest != "" {
		subject += "/" + facts.Subtest
	}
	switch {
	case facts.Panic != "":
		return fmt.Sprintf("%s failed with %s", subject, bound(facts.Panic, 200))
	case facts.Message != "":
		return fmt.Sprintf("%s failed: %s", subject, bound(facts.Message, 200))
	default:
		return subject + " failed"
	}
}
