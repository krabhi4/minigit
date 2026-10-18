package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLine(t *testing.T) {
	pat, err := ParseLine("*.log", "")
	if err != nil || pat == nil {
		t.Fatalf("unexpected error parsing *.log: %v", err)
	}
	if pat.Negate || pat.DirOnly || pat.Anchored {
		t.Fatalf("unexpected flags for *.log: %+v", pat)
	}

	pat, err = ParseLine("!important.log", "")
	if err != nil || pat == nil || !pat.Negate {
		t.Fatalf("expected negate flag for !important.log: %+v", pat)
	}

	pat, err = ParseLine("build/", "")
	if err != nil || pat == nil || !pat.DirOnly {
		t.Fatalf("expected dirOnly flag for build/: %+v", pat)
	}

	pat, err = ParseLine("/root_only.txt", "")
	if err != nil || pat == nil || !pat.Anchored {
		t.Fatalf("expected anchored flag for /root_only.txt: %+v", pat)
	}

	pat, err = ParseLine("doc/*.txt", "")
	if err != nil || pat == nil || !pat.Anchored {
		t.Fatalf("expected anchored flag for doc/*.txt: %+v", pat)
	}

	pat, err = ParseLine("# comment", "")
	if err != nil || pat != nil {
		t.Fatalf("comment should return nil: %+v", pat)
	}

	pat, err = ParseLine("   ", "")
	if err != nil || pat != nil {
		t.Fatalf("blank line should return nil: %+v", pat)
	}
}

func TestMatcherRootPatterns(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-ignore-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	gitIgnoreContent := "*.log\n!keep.log\nbuild/\n/root.txt\na/**/b\ntemp\n"
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(gitIgnoreContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadMatcher(tempDir)
	if err != nil {
		t.Fatalf("LoadMatcher failed: %v", err)
	}

	if !m.IsIgnored("test.log", false) {
		t.Fatalf("expected test.log to be ignored")
	}
	if !m.IsIgnored("sub/dir/test.log", false) {
		t.Fatalf("expected sub/dir/test.log to be ignored")
	}
	if m.IsIgnored("keep.log", false) {
		t.Fatalf("expected keep.log NOT to be ignored")
	}
	if m.IsIgnored("sub/keep.log", false) {
		t.Fatalf("expected sub/keep.log NOT to be ignored")
	}

	if !m.IsIgnored("build", true) {
		t.Fatalf("expected build directory to be ignored")
	}
	if !m.IsIgnored("build/output.bin", false) {
		t.Fatalf("expected file in build directory to be ignored")
	}
	if !m.IsIgnored("build/nested/file.txt", false) {
		t.Fatalf("expected file in nested build directory to be ignored")
	}

	if !m.IsIgnored("root.txt", false) {
		t.Fatalf("expected root.txt to be ignored")
	}
	if m.IsIgnored("sub/root.txt", false) {
		t.Fatalf("expected sub/root.txt NOT to be ignored")
	}

	if !m.IsIgnored("a/b", false) {
		t.Fatalf("expected a/b to be ignored")
	}
	if !m.IsIgnored("a/x/b", false) {
		t.Fatalf("expected a/x/b to be ignored")
	}
	if !m.IsIgnored("a/x/y/b", false) {
		t.Fatalf("expected a/x/y/b to be ignored")
	}
	if m.IsIgnored("a/x/c", false) {
		t.Fatalf("expected a/x/c NOT to be ignored")
	}

	if !m.IsIgnored("temp", false) {
		t.Fatalf("expected temp file to be ignored")
	}
	if !m.IsIgnored("sub/temp", false) {
		t.Fatalf("expected sub/temp to be ignored")
	}
	if !m.IsIgnored("temp/file.txt", false) {
		t.Fatalf("expected temp/file.txt to be ignored")
	}
}

func TestMatcherNestedGitignore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-nested-ignore-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.txt\n!root_keep.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(tempDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(subDir, ".gitignore"), []byte("!special.txt\n/nested_only.dat\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadMatcher(tempDir)
	if err != nil {
		t.Fatalf("LoadMatcher failed: %v", err)
	}

	if !m.IsIgnored("normal.txt", false) {
		t.Fatalf("expected normal.txt to be ignored by root .gitignore")
	}
	if m.IsIgnored("root_keep.txt", false) {
		t.Fatalf("expected root_keep.txt NOT to be ignored")
	}

	if !m.IsIgnored("sub/normal.txt", false) {
		t.Fatalf("expected sub/normal.txt to be ignored by root .gitignore")
	}
	if m.IsIgnored("sub/special.txt", false) {
		t.Fatalf("expected sub/special.txt NOT to be ignored (overridden by sub/.gitignore)")
	}

	if !m.IsIgnored("sub/nested_only.dat", false) {
		t.Fatalf("expected sub/nested_only.dat to be ignored")
	}
	if m.IsIgnored("sub/deep/nested_only.dat", false) {
		t.Fatalf("expected sub/deep/nested_only.dat NOT to be ignored (anchored to sub)")
	}
	if m.IsIgnored("nested_only.dat", false) {
		t.Fatalf("expected nested_only.dat at root NOT to be ignored")
	}
}
