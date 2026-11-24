package evalcorpus

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestVerifyFixtureAndApply(t *testing.T) {
	dir := t.TempDir()
	test := "pkg/a_test.go"
	content := "package pkg\n\nfunc TestBug(t *testing.T) {}\n"
	writeFile(t, filepath.Join(dir, injectedDir, test), content)
	m := Metadata{
		ID:            "case-x",
		Provenance:    "controlled harness run 123",
		InjectedTests: []InjectedTest{{Path: test, SHA256: digest(content)}},
	}
	if err := VerifyFixture(dir, m); err != nil {
		t.Fatalf("verify: %v", err)
	}
	repo := t.TempDir()
	if err := ApplyInjectedTests(dir, repo, m); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(repo, test))
	if err != nil || string(got) != content {
		t.Fatalf("applied test mismatch: %v", err)
	}
}

func TestVerifyFixtureRejectsTamperedFile(t *testing.T) {
	dir := t.TempDir()
	test := "pkg/a_test.go"
	writeFile(t, filepath.Join(dir, injectedDir, test), "original")
	m := Metadata{ID: "case-x", Provenance: "p", InjectedTests: []InjectedTest{{Path: test, SHA256: digest("original")}}}
	// Tamper the preserved copy.
	writeFile(t, filepath.Join(dir, injectedDir, test), "changed")
	if err := VerifyFixture(dir, m); err == nil {
		t.Fatal("tampered fixture was accepted")
	}
}

func TestVerifyFixtureRequiresProvenance(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, injectedDir, "a.go"), "x")
	m := Metadata{ID: "case-x", InjectedTests: []InjectedTest{{Path: "a.go", SHA256: digest("x")}}}
	if err := VerifyFixture(dir, m); err == nil {
		t.Fatal("missing provenance accepted")
	}
}

func TestVerifyFixtureRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	m := Metadata{ID: "case-x", Provenance: "p", InjectedTests: []InjectedTest{{Path: "../escape.go", SHA256: digest("x")}}}
	if err := VerifyFixture(dir, m); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestVerifyFixtureNoopWithoutInjectedTests(t *testing.T) {
	if err := VerifyFixture(t.TempDir(), Metadata{ID: "case-y"}); err != nil {
		t.Fatalf("case without injected tests should verify: %v", err)
	}
}

// Load must verify fixtures so a changed injection never silently affects an
// evaluation.
func TestLoadVerifiesFixtureDigest(t *testing.T) {
	root := t.TempDir()
	test := "pkg/a_test.go"
	log := "--- FAIL: TestBug\nFAIL\n"
	writeFile(t, filepath.Join(root, "manifest.yaml"), "version: \"1\"\ncases:\n  - id: case-x\n    tier: full\n    category: test_failure_observed\n")
	caseDir := filepath.Join(root, "cases", "case-x")
	writeFile(t, filepath.Join(caseDir, "failure.log"), log)
	writeFile(t, filepath.Join(caseDir, injectedDir, test), "package pkg")
	meta := strings.Join([]string{
		"id: case-x",
		"tier: full",
		"category: test_failure_observed",
		"repo: https://example.com/x",
		"run_url: https://example.com/run",
		"head_sha: " + strings.Repeat("a", 40),
		"base_sha: " + strings.Repeat("b", 40),
		"command: [go, test, ./...]",
		"log_sha256: " + digest(log),
		"license_note: MIT",
		"provenance: controlled harness",
		"injected_tests:",
		"  - path: " + test,
		"    sha256: " + digest("package pkg"),
	}, "\n")
	writeFile(t, filepath.Join(caseDir, "metadata.yaml"), meta)
	writeFile(t, filepath.Join(caseDir, "expected.yaml"), strings.Join([]string{
		"primary_failure: TestBug",
		"category: test_failure_observed",
		"expected_outcome: answerable",
		"culprit_file: pkg/a.go",
		"culprit_function: Bug",
		"evidence_source: reviewed",
		"annotator: test",
	}, "\n"))

	if _, _, err := Load(root); err != nil {
		t.Fatalf("load with valid fixture: %v", err)
	}
	// Tamper the injected test; Load must now fail.
	writeFile(t, filepath.Join(caseDir, injectedDir, test), "package pkg // changed")
	if _, _, err := Load(root); err == nil {
		t.Fatal("Load accepted a tampered injected test")
	}
}
