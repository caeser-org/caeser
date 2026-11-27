package caeser_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFirstInvestigationFixture keeps docs/first-investigation.md accurate:
// the fixture must still fail the way the walkthrough describes.
func TestFirstInvestigationFixture(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not installed")
	}
	cmd := exec.Command(goBin, "test", "./...")
	cmd.Dir = filepath.Join("docs", "examples", "first-investigation")
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("fixture tests passed; the walkthrough needs TestStatusPaid to fail\n%s", out)
	}
	for _, want := range []string{
		"--- FAIL: TestStatusPaid",
		`Status() = "pending", want "ready"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("fixture output is missing %q\n%s", want, out)
		}
	}
}
