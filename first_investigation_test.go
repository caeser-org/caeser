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
	dir := filepath.Join("docs", "examples", "first-investigation")

	source, err := os.ReadFile(filepath.Join(dir, "order.go"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	lines := strings.Split(string(source), "\n")
	for line, want := range map[int]string{
		14: "func Status(o Order) string {",
		15: "if o.Paid && o.Shipped {",
		16: `return "ready"`,
	} {
		if line > len(lines) || !strings.Contains(lines[line-1], want) {
			t.Errorf("order.go:%d should contain %q; update docs/first-investigation.md if the fixture moved", line, want)
		}
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not installed")
	}
	cmd := exec.Command(goBin, "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("fixture tests passed; the walkthrough needs TestStatusPaid to fail\n%s", out)
	}
	for _, want := range []string{
		"--- FAIL: TestStatusPaid",
		`order_test.go:8: Status() = "pending", want "ready"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("fixture output is missing %q\n%s", want, out)
		}
	}
}
