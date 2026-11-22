package diagnosis

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/caeser-org/ceaser/internal/domain"
)

// CandidateReport is a bounded shortlist of places to look next. It reuses
// ranked candidates, observed failure locations and the selected probe
// operation, but never upgrades any of them into a verified mechanism.
type CandidateReport struct {
	Status      string                 `json:"status"`
	Reason      string                 `json:"reason,omitempty"`
	ProbeStatus string                 `json:"probe_status"`
	Entries     []CandidateReportEntry `json:"entries,omitempty"`
	Limitations []string               `json:"limitations,omitempty"`
}

// CandidateReportEntry keeps the kind explicit so an observed stack frame can
// be told apart from an inferred call or source relationship.
type CandidateReportEntry struct {
	Rank        int      `json:"rank"`
	File        string   `json:"file"`
	Function    string   `json:"function,omitempty"`
	Line        int      `json:"line,omitempty"`
	Kind        string   `json:"kind"`
	Reason      string   `json:"reason"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

const candidateReportLimit = 5

// candidateCollectLimit bounds how many entries are gathered before the
// implementation-first reordering, so the report can keep implementation
// candidates even when test-file entries sort ahead of them.
const candidateCollectLimit = 40

// BuildCandidateReport produces the evidence-backed shortlist. It is derived
// from existing report fields and does not change classification, promotion,
// confidence, or any verification gate.
func BuildCandidateReport(report Report) CandidateReport {
	result := CandidateReport{Status: "unavailable", ProbeStatus: "disabled"}
	if obs := report.MechanismObservation; obs != nil {
		result.ProbeStatus = obs.Status
		if obs.Status != "observed" && obs.Status != "" {
			result.Limitations = append(result.Limitations,
				fmt.Sprintf("runtime observation %s is inconclusive; no mechanism is established", obs.Status))
		}
		if obs.Correlation == nil || !obs.Correlation.Verified {
			result.Limitations = append(result.Limitations,
				"probe values are not correlated with the failing invocation")
		}
	}

	type key struct {
		file, function string
		line           int
	}
	seen := map[key]bool{}
	entries := []CandidateReportEntry{}
	add := func(file, function string, line int, kind, reason string, ids []string) {
		file = strings.TrimSpace(file)
		if file == "" || len(entries) >= candidateCollectLimit {
			return
		}
		k := key{file: filepathSlash(file), function: function, line: line}
		if seen[k] {
			return
		}
		seen[k] = true
		entries = append(entries, CandidateReportEntry{
			Rank:        len(entries) + 1,
			File:        filepathSlash(file),
			Function:    function,
			Line:        line,
			Kind:        kind,
			Reason:      reason,
			EvidenceIDs: boundedEvidenceIDs(ids, 3),
		})
	}

	// Observed stack locations come from failure evidence, not from inference.
	for _, incident := range report.FailureIncidents {
		for _, loc := range incident.ObservedLocations {
			add(loc.File, loc.Function, loc.Line, "observed_stack",
				"location observed in the failure evidence", incident.EvidenceIDs)
		}
	}

	// The selected probe operation is concrete, but inconclusive observations
	// remain only a lead. GetInt's line 255 fallback, for example, is not a
	// verified mechanism when its values are uncorrelated.
	if obs := report.MechanismObservation; obs != nil && obs.OperationFile != "" {
		reason := "selected probe operation; observation inconclusive"
		if obs.Status == "observed" && !obs.ValueMatched {
			reason = "selected probe operation; value did not match the failure"
		}
		if obs.Correlation == nil || !obs.Correlation.Verified {
			reason = "selected probe operation; values not correlated with the failing invocation"
		}
		add(obs.OperationFile, obs.OperationName, obs.OperationLine, "probe_operation", reason, obs.EvidenceIDs)
	}

	// Ranked candidates may be verified call edges or weaker source relations.
	// Order them by evidence strength before the bounded report is assembled:
	// directly observed execution evidence first, then functions the named
	// failing test actually reaches through a type-resolved (verified) call
	// path, then the remaining inferred fan-out. The failing test's verified
	// path is existing evidence (execution_paths); using it to order leads is
	// not a new analysis and does not change any candidate's score or kind.
	pathTargets := failingTestPathTargets(report)
	frames := observedFrameOrder(report)
	candidatesByEvidence := append([]domain.CandidateLocation(nil), report.Candidates...)
	evidenceRank := func(candidate domain.CandidateLocation) int {
		provenance := candidate.Location.Provenance
		if provenance == "assertion_mention" {
			return 1
		}
		if provenance == "causal_dependency" {
			return 1
		}
		if provenance == "causal_producer" {
			if report.CausalTrace != nil && report.CausalTrace.Supersedes && report.CausalTrace.Producer != nil &&
				candidate.Location.SymbolIdentity == report.CausalTrace.Producer.Identity {
				return -1
			}
			return 1
		}
		if candidate.Score.ExecutionRelevance > 0 || provenance == "enclosing_symbol" ||
			provenance == "direct_stack_frame" || provenance == "observed_frame" {
			return 0
		}
		if pathTargets[fileFunctionKey(candidate.Location.FilePath, candidateFunction(candidate.Location.SymbolIdentity))] {
			return 1
		}
		return 2
	}
	sort.SliceStable(candidatesByEvidence, func(i, j int) bool {
		a, b := candidatesByEvidence[i], candidatesByEvidence[j]
		if ra, rb := evidenceRank(a), evidenceRank(b); ra != rb {
			return ra < rb
		}
		// A function named at the failing assertion is the most specific
		// source-grounded lead once observed frames are accounted for.
		aMention := a.Location.Provenance == "assertion_mention"
		bMention := b.Location.Provenance == "assertion_mention"
		if aMention != bMention {
			return aMention
		}
		// Among assertion-dependency leads, prefer the step closest to the
		// value's origin. The chain orders these steps from the assertion
		// outward; the score captures that depth. Deeper means a stronger
		// lead, never a verified cause.
		if a.Location.Provenance == "causal_dependency" && b.Location.Provenance == "causal_dependency" {
			if a.Score.DependencyRelevance != b.Score.DependencyRelevance {
				return a.Score.DependencyRelevance > b.Score.DependencyRelevance
			}
		}
		// Within observed execution evidence, prefer the innermost reported
		// frame. The failure happened at the deepest frame, so the function that
		// encloses it is stronger evidence than an enclosing caller. Frame order
		// comes from the original failure evidence, never from inference.
		fa, sa, oka := enclosingFrameRank(a.Location, frames)
		fb, sb, okb := enclosingFrameRank(b.Location, frames)
		if oka != okb {
			return oka
		}
		if oka && fa != fb {
			return fa < fb
		}
		if oka && sa != sb {
			return sa < sb
		}
		return false
	})
	for _, candidate := range candidatesByEvidence {
		loc := candidate.Location
		kind, reason := inferredCandidateKind(loc.Provenance, loc.LocationType)
		add(loc.FilePath, candidateFunction(loc.SymbolIdentity), loc.StartLine, kind, reason, candidateIDs(candidate.SupportingEvidenceIDs))
	}

	// Localization stays useful when probing is disabled or cannot start.
	for _, file := range report.Localization.LikelyFiles {
		add(file, "", 0, "inferred_source", "ranked among localized likely files", evidenceForFile(report.Evidence, file))
	}

	if len(entries) == 0 {
		result.Reason = "no observed location, ranked candidate, or localized file was recorded"
		return result
	}
	// The bounded report is the developer handoff. A failing test's own file is
	// evidence but never the implementation to fix, so keep implementation
	// candidates ahead of test-file entries instead of letting test candidates
	// consume the limited slots.
	impl := make([]CandidateReportEntry, 0, len(entries))
	tests := make([]CandidateReportEntry, 0, len(entries))
	for _, e := range entries {
		if isTestFilePath(e.File) {
			tests = append(tests, e)
		} else {
			impl = append(impl, e)
		}
	}
	ordered := append(impl, tests...)
	if len(ordered) > candidateReportLimit {
		ordered = ordered[:candidateReportLimit]
	}
	for i := range ordered {
		ordered[i].Rank = i + 1
	}
	result.Status = "available"
	result.Entries = ordered
	return result
}

// isTestFilePath reports whether a location is a test file. Test files are
// evidence about where a failure surfaced, not implementation to inspect.
func isTestFilePath(file string) bool {
	return strings.HasSuffix(filepathSlash(file), "_test.go")
}

func inferredCandidateKind(provenance, locationType string) (kind, reason string) {
	p := strings.ToLower(provenance + " " + locationType)
	switch {
	case strings.Contains(p, "assertion_mention"):
		return "assertion_mention", "function named by the failing test's source at the assertion"
	case strings.Contains(p, "causal_dependency"):
		return "causal_dependency", "function on the failed assertion's dependency path (source-verified)"
	case strings.Contains(p, "causal_producer"):
		return "causal_producer", "value producer on the failure's causal path; the observed failure propagates its result or error"
	case strings.Contains(p, "stack"), strings.Contains(p, "observed_frame"), strings.Contains(p, "enclosing_symbol"), strings.Contains(p, "frame"):
		return "observed_stack", "candidate derived from an observed stack location"
	case strings.Contains(p, "call"):
		return "inferred_call", "candidate inferred from a resolved call relationship"
	case strings.Contains(p, "ssa"), strings.Contains(p, "value"), strings.Contains(p, "dependency"):
		return "inferred_source", "candidate inferred from a static value dependency"
	default:
		return "inferred_source", "ranked source candidate"
	}
}

func candidateFunction(identity string) string {
	s := strings.TrimSpace(identity)
	if s == "" || strings.HasPrefix(s, "observed:") {
		return ""
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && i+1 < len(s) {
		tail := s[i+1:]
		if _, err := strconv.Atoi(tail); err != nil && !strings.Contains(tail, "(") {
			s = tail
		}
	}
	if i := strings.LastIndex(s, "."); i > 0 {
		method := s[i+1:]
		if j := strings.LastIndex(s[:i], "."); j >= 0 {
			s = s[j+1:i] + "." + method
		}
	}
	s = strings.TrimPrefix(s, "(*")
	s = strings.TrimSuffix(s, ")")
	return bound(s, 96)
}

func filepathSlash(file string) string {
	return strings.ReplaceAll(strings.TrimSpace(file), "\\", "/")
}

func candidateIDs(ids []domain.ID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if v := strings.TrimSpace(string(id)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func boundedEvidenceIDs(ids []string, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func evidenceForFile(evidence []EvidenceItem, file string) []string {
	want := filepathSlash(file)
	var ids []string
	for _, item := range evidence {
		if filepathSlash(item.Source) == want {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

// failingTestPathTargets returns the file|function keys of implementation
// functions that are on a type-resolved (verified) call path originating at a
// named failing test. Execution paths are already built from verified static
// call edges; this only reuses them to order existing candidates, so it neither
// invents a relationship nor changes a candidate's evidence.
func failingTestPathTargets(report Report) map[string]bool {
	failing := map[string]bool{}
	add := func(identifier string) {
		name := testNameFromIdentifier(identifier)
		if name == "" || name == "Test" {
			return
		}
		failing[name] = true
	}
	for _, episode := range report.FailureEpisodes {
		add(episode.TestIdentifier)
	}
	for _, incident := range report.FailureIncidents {
		for _, test := range incident.AffectedTests {
			add(test)
		}
	}
	targets := map[string]bool{}
	if len(failing) == 0 {
		return targets
	}
	for _, path := range report.ExecutionPaths {
		if len(path.Locations) < 2 {
			continue
		}
		origin := path.Locations[0].SymbolIdentity
		matched := false
		for name := range failing {
			if strings.HasSuffix(origin, ":"+name) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, location := range path.Locations[1:] {
			if location.FilePath == "" || isTestFilePath(location.FilePath) {
				continue
			}
			key := fileFunctionKey(location.FilePath, candidateFunction(location.SymbolIdentity))
			if key != "" {
				targets[key] = true
			}
		}
	}
	return targets
}

// frameEvidenceLine extracts the file and line from an observed stack-frame
// evidence detail. Stack frames are recorded innermost-first.
var frameEvidenceLine = regexp.MustCompile(`observed at (.+):(\d+);`)

type observedFrame struct {
	file string
	line int
}

// observedFrameOrder returns the reported stack frames in evidence order
// (innermost first). It prefers the structured incident locations and falls back
// to the ordered stack-frame evidence. Toolchain paths are harmless: they never
// match a repository candidate.
func observedFrameOrder(report Report) []observedFrame {
	var frames []observedFrame
	for _, incident := range report.FailureIncidents {
		for _, location := range incident.ObservedLocations {
			if location.File != "" && location.Line > 0 {
				frames = append(frames, observedFrame{file: location.File, line: location.Line})
			}
		}
	}
	if len(frames) > 0 {
		return frames
	}
	for _, item := range report.Evidence {
		if item.Kind != "execution_stack_frame" {
			continue
		}
		if match := frameEvidenceLine.FindStringSubmatch(item.Detail); match != nil {
			frames = append(frames, observedFrame{file: match[1], line: atoi(match[2])})
		}
	}
	return frames
}

// enclosingFrameRank returns the index (innermost first) and span of the first
// observed frame a candidate location encloses. It only matches a candidate to a
// frame in the same file whose line falls within the candidate's declaration.
func enclosingFrameRank(location domain.CodeLocation, frames []observedFrame) (int, int, bool) {
	if location.FilePath == "" || len(frames) == 0 {
		return 0, 0, false
	}
	base := sourceBase(location.FilePath)
	span := location.EndLine - location.StartLine
	for i, frame := range frames {
		if sourceBase(frame.file) != base {
			continue
		}
		if frame.line < location.StartLine {
			continue
		}
		if location.EndLine > 0 && frame.line > location.EndLine {
			continue
		}
		return i, span, true
	}
	return 0, 0, false
}

func sourceBase(path string) string {
	path = filepathSlash(path)
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
