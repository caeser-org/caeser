package evalcorpus

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// injectedDir is the subdirectory of a case that preserves the exact
// regression-test overlays applied to the base revision, mirroring their
// repo-relative paths.
const injectedDir = "injected"

// VerifyFixture checks that every preserved artifact of a case matches its
// recorded digest. It is called before each evaluation so a fixture cannot be
// silently changed or approximated. Cases without injected tests are valid.
func VerifyFixture(dir string, m Metadata) error {
	if len(m.InjectedTests) == 0 {
		return nil
	}
	if strings.TrimSpace(m.Provenance) == "" {
		return errors.New("a case with injected tests must record provenance")
	}
	for _, test := range m.InjectedTests {
		if err := validateRelativePath(test.Path); err != nil {
			return fmt.Errorf("injected test %q: %w", test.Path, err)
		}
		path := filepath.Join(dir, injectedDir, filepath.FromSlash(test.Path))
		sum, err := fileSHA256(path)
		if err != nil {
			return fmt.Errorf("injected test %q: %w", test.Path, err)
		}
		if !strings.EqualFold(sum, test.SHA256) {
			return fmt.Errorf("injected test %q digest mismatch: fixture changed", test.Path)
		}
	}
	return nil
}

// ApplyInjectedTests copies the preserved regression tests into a prepared
// repository checkout, verifying each digest on the way in. It never derives a
// test from an upstream revision; only the preserved fixture is used.
func ApplyInjectedTests(dir, repoDir string, m Metadata) error {
	if err := VerifyFixture(dir, m); err != nil {
		return err
	}
	for _, test := range m.InjectedTests {
		if err := validateRelativePath(test.Path); err != nil {
			return fmt.Errorf("injected test %q: %w", test.Path, err)
		}
		src := filepath.Join(dir, injectedDir, filepath.FromSlash(test.Path))
		dst := filepath.Join(repoDir, filepath.FromSlash(test.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
		sum, err := fileSHA256(dst)
		if err != nil || !strings.EqualFold(sum, test.SHA256) {
			return fmt.Errorf("injected test %q was not applied intact", test.Path)
		}
	}
	return nil
}

// VerifyCorpus loads every case and verifies its fixture digests. It returns the
// number of verified cases.
func VerifyCorpus(root string) (int, error) {
	_, cases, err := Load(root)
	if err != nil {
		return 0, err
	}
	for _, c := range cases {
		if err := VerifyFixture(c.Dir, c.Metadata); err != nil {
			return 0, fmt.Errorf("case %s: %w", c.Metadata.ID, err)
		}
	}
	return len(cases), nil
}

func validateRelativePath(path string) error {
	path = filepath.ToSlash(path)
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return errors.New("must be a repo-relative path without ..")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
