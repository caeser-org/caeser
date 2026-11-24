package diagnosis

import (
	"strings"
	"testing"

	"github.com/caeser-org/ceaser/internal/domain"
)

func TestObservedFailurePRRelationship(t *testing.T) {
	tests := []struct {
		name   string
		report Report
		want   string
	}{
		{
			name: "no comparison",
			want: "did not compare the failure",
		},
		{
			name:   "cold path regression",
			report: Report{GoColdPath: &GoColdPath{Verdict: ClassRealRegression}},
			want:   "supporting attribution to the PR",
		},
		{
			name:   "base comparison",
			report: Report{Comparison: &Comparison{Verdict: ClassPreExisting}},
			want:   "not specific to this PR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := observedFailurePRRelationship(tt.report)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("observedFailurePRRelationship() = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestBuildFailureIncidentsUsesExplicitMechanismAndAttributionLanguage(t *testing.T) {
	report := &Report{
		Classification:  ClassTestFailureObserved,
		FailureEpisodes: []domain.FailureEpisode{{TestIdentifier: "TestPanic", FailureMessage: "slice bounds panic"}},
	}
	incidents := buildFailureIncidents(report)
	if len(incidents) != 1 {
		t.Fatalf("buildFailureIncidents() returned %d incidents, want 1", len(incidents))
	}
	incident := incidents[0]
	if !strings.Contains(incident.ProposedExplanation, "candidate locations") || !strings.Contains(incident.ProposedExplanation, "did not establish") {
		t.Errorf("mechanism assessment does not explain the evidence limit: %q", incident.ProposedExplanation)
	}
	if !strings.Contains(incident.PRRelationship, "did not compare") {
		t.Errorf("PR attribution does not explain the missing comparison: %q", incident.PRRelationship)
	}
}
