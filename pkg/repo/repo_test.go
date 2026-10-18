package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoWorkflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-repo-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	branch, isSymbolic, hash, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}
	if !isSymbolic || branch != "main" || hash != "" {
		t.Fatalf("unexpected head: branch=%s, isSymbolic=%v, hash=%s", branch, isSymbolic, hash)
	}

	f1 := filepath.Join(tempDir, "file1.txt")
	if err := os.WriteFile(f1, []byte("version 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Untracked) != 1 || st.Untracked[0] != "file1.txt" {
		t.Fatalf("expected 1 untracked file, got %v", st.Untracked)
	}

	if err := r.Add([]string{"file1.txt"}); err != nil {
		t.Fatal(err)
	}

	st, err = r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 1 || st.Staged[0].Path != "file1.txt" || st.Staged[0].Status != StatusAdded {
		t.Fatalf("expected 1 staged added, got %v", st.Staged)
	}

	c1Hash, err := r.Commit("Initial commit", "Tester <test@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	if len(c1Hash) != 40 {
		t.Fatalf("expected 40 char hash, got %s", c1Hash)
	}

	st, err = r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean {
		t.Fatalf("expected clean repo after commit, got staged=%v unstaged=%v untracked=%v", st.Staged, st.Unstaged, st.Untracked)
	}

	if err := r.CreateBranch("feature", c1Hash); err != nil {
		t.Fatal(err)
	}

	branches, curBranch, err := r.ListBranches()
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 || curBranch != "main" {
		t.Fatalf("expected 2 branches with current main, got %v, %s", branches, curBranch)
	}

	if err := r.Checkout("feature"); err != nil {
		t.Fatal(err)
	}
	_, _, curHash, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}
	if curHash != c1Hash {
		t.Fatalf("expected feature at %s, got %s", c1Hash, curHash)
	}

	f2 := filepath.Join(tempDir, "feature.txt")
	if err := os.WriteFile(f2, []byte("feature work\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"feature.txt"}); err != nil {
		t.Fatal(err)
	}
	c2Hash, err := r.Commit("Add feature", "Tester <test@example.com>")
	if err != nil {
		t.Fatal(err)
	}

	logs, err := r.Log("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 commits in log, got %d", len(logs))
	}
	if logs[0].Hash != c2Hash || logs[1].Hash != c1Hash {
		t.Fatalf("unexpected log order: %v", logs)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f2); !os.IsNotExist(err) {
		t.Fatalf("expected feature.txt to be removed when switching back to main")
	}

	if err := os.WriteFile(f1, []byte("version 1 modified\n"), 0644); err != nil {
		t.Fatal(err)
	}
	d, err := r.Diff(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "-version 1") || !strings.Contains(d, "+version 1 modified") {
		t.Fatalf("unexpected unstaged diff: %s", d)
	}
}
