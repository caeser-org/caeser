package caeser_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// copyableSummaryFilter returns the jq program the publish job uses for the
// copyable PR comment summary.
func copyableSummaryFilter(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(".github/workflows/ci-diagnose.yml")
	if err != nil {
		t.Fatalf("read ci-diagnose.yml: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse ci-diagnose.yml: %v", err)
	}
	publish := mapKey(mapKey(doc.Content[0], "jobs"), "publish")
	list := mapKey(publish, "steps")
	if list == nil {
		t.Fatal("publish job has no steps")
	}
	for _, step := range list.Content {
		if name := mapKey(step, "name"); name == nil || name.Value != "Prepare bounded diagnosis comment" {
			continue
		}
		filter := mapKey(mapKey(step, "env"), "COPYABLE_SUMMARY_FILTER")
		if filter == nil || filter.Value == "" {
			t.Fatal("comment step has no COPYABLE_SUMMARY_FILTER")
		}
		if run := mapKey(step, "run"); run == nil || !strings.Contains(run.Value, `jq -r "$COPYABLE_SUMMARY_FILTER"`) || !strings.Contains(run.Value, "<summary>Copyable summary</summary>") {
			t.Fatal("comment step does not render the copyable summary")
		}
		return filter.Value
	}
	t.Fatal("publish job has no Prepare bounded diagnosis comment step")
	return ""
}

func renderCopyableSummary(t *testing.T, filter, report string) string {
	t.Helper()
	cmd := exec.Command("jq", "-r", filter)
	cmd.Stdin = strings.NewReader(report)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jq: %v\n%s", err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

func TestCopyableSummary(t *testing.T) {
	filter := copyableSummaryFilter(t)
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed")
	}

	t.Run("with implementation lead", func(t *testing.T) {
		report := `{
			"environment": {"head_commit": "0123abcd"},
			"normalized_evidence": {"executions": [{"tests": [
				{"name": "TestParse/empty", "state": "failed", "file": "parse_test.go", "line": 42,
				 "message": "got <b>nil</b>\n  ping @octocat"}
			]}]},
			"likely_function": {"primary": {"function": "pkg.Parse", "file": "parse.go", "line": 17,
				"evidence_ids": ["E1", "E2", "E3", "E4"]}},
			"causal_explanation": {"checks": [{"target": "pkg.Parse return value"}]},
			"root_cause": {"next_actions": ["rerun the test"]}
		}`
		got := renderCopyableSummary(t, filter, report)
		want := strings.Join([]string{
			"Failed check: TestParse/empty (parse_test.go:42)",
			"Failure: got &lt;b&gt;nil&lt;/b&gt; ping ＠octocat",
			"Revision: 0123abcd",
			"Start here: pkg.Parse at parse.go:17",
			"Evidence: E1, E2, E3",
			"Next check: Inspect pkg.Parse return value",
		}, "\n")
		if got != want {
			t.Fatalf("summary mismatch\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("without implementation lead", func(t *testing.T) {
		report := `{
			"environment": {"head_commit": "0123abcd"},
			"failure_episodes": [{"test_identifier": "TestServe", "failure_message": "timeout after 30s"}],
			"root_cause": {"evidence_ids": ["E9"], "next_actions": ["run the smallest failing test in a network-disabled sandbox"]}
		}`
		got := renderCopyableSummary(t, filter, report)
		want := strings.Join([]string{
			"Failed check: TestServe",
			"Failure: timeout after 30s",
			"Revision: 0123abcd",
			"Evidence: E9",
			"Next check: run the smallest failing test in a network-disabled sandbox",
		}, "\n")
		if got != want {
			t.Fatalf("summary mismatch\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("empty report", func(t *testing.T) {
		if got := renderCopyableSummary(t, filter, `{}`); got != "" {
			t.Fatalf("expected no summary for an empty report, got %q", got)
		}
	})
}
