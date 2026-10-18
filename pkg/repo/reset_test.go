package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetSoft(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-reset-soft-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(aPath, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c1, err := r.Commit("First commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bPath := filepath.Join(tempDir, "b.txt")
	if err := os.WriteFile(bPath, []byte("b1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt", "b.txt"}); err != nil {
		t.Fatal(err)
	}
	_, err = r.Commit("Second commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reset(ResetSoft, "HEAD~1"); err != nil {
		t.Fatalf("reset --soft failed: %v", err)
	}

	_, _, curHead, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}
	if curHead != c1 {
		t.Fatalf("expected HEAD at %s, got %s", c1, curHead)
	}

	contentA, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contentA) != "v2\n" {
		t.Fatalf("worktree a.txt changed during soft reset: %s", string(contentA))
	}

	contentB, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contentB) != "b1\n" {
		t.Fatalf("worktree b.txt changed during soft reset: %s", string(contentB))
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 2 {
		t.Fatalf("expected 2 staged files after soft reset, got %d", len(st.Staged))
	}
	if len(st.Unstaged) != 0 {
		t.Fatalf("expected 0 unstaged files after soft reset, got %d", len(st.Unstaged))
	}
}

func TestResetMixed(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-reset-mixed-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(aPath, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c1, err := r.Commit("First commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	_, err = r.Commit("Second commit", "")
	if err != nil {
		t.Fatal(err)
	}

	out, err := r.Reset(ResetMixed, "HEAD~1")
	if err != nil {
		t.Fatalf("reset --mixed failed: %v", err)
	}
	if out == "" {
		t.Fatalf("expected unstaged changes output on mixed reset")
	}

	_, _, curHead, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}
	if curHead != c1 {
		t.Fatalf("expected HEAD at %s, got %s", c1, curHead)
	}

	contentA, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contentA) != "v2\n" {
		t.Fatalf("worktree a.txt changed during mixed reset: %s", string(contentA))
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 0 {
		t.Fatalf("expected 0 staged files after mixed reset, got %d", len(st.Staged))
	}
	if len(st.Unstaged) != 1 {
		t.Fatalf("expected 1 unstaged file after mixed reset, got %d", len(st.Unstaged))
	}

	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	stStaged, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(stStaged.Staged) != 1 {
		t.Fatalf("expected 1 staged file before reset HEAD")
	}

	if _, err := r.Reset(ResetMixed, "HEAD"); err != nil {
		t.Fatalf("reset mixed HEAD failed: %v", err)
	}
	stUnstaged, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(stUnstaged.Staged) != 0 {
		t.Fatalf("expected staged changes cleared by reset HEAD, got %d", len(stUnstaged.Staged))
	}
	if len(stUnstaged.Unstaged) != 1 {
		t.Fatalf("expected 1 unstaged change remaining")
	}
}

func TestResetHard(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-reset-hard-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	bPath := filepath.Join(tempDir, "b.txt")
	if err := os.WriteFile(aPath, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("b1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt", "b.txt"}); err != nil {
		t.Fatal(err)
	}
	c1, err := r.Commit("First commit", "")
	if err != nil {
		t.Fatal(err)
	}

	cPath := filepath.Join(tempDir, "c.txt")
	if err := os.WriteFile(aPath, []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cPath, []byte("c1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt", "c.txt"}); err != nil {
		t.Fatal(err)
	}
	_, err = r.Commit("Second commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("dirty a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	uPath := filepath.Join(tempDir, "untracked.txt")
	if err := os.WriteFile(uPath, []byte("keep me\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := r.Reset(ResetHard, "HEAD~1")
	if err != nil {
		t.Fatalf("reset --hard failed: %v", err)
	}
	if !strings.Contains(out, "HEAD is now at") {
		t.Fatalf("unexpected hard reset output: %s", out)
	}

	_, _, curHead, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}
	if curHead != c1 {
		t.Fatalf("expected HEAD at %s, got %s", c1, curHead)
	}

	contentA, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contentA) != "v1\n" {
		t.Fatalf("expected a.txt reverted to v1, got: %s", string(contentA))
	}

	if _, err := os.Stat(cPath); !os.IsNotExist(err) {
		t.Fatalf("expected c.txt to be removed after hard reset")
	}

	contentU, err := os.ReadFile(uPath)
	if err != nil {
		t.Fatalf("untracked.txt was deleted by hard reset: %v", err)
	}
	if string(contentU) != "keep me\n" {
		t.Fatalf("untracked.txt modified: %s", string(contentU))
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 0 || len(st.Unstaged) != 0 {
		t.Fatalf("expected clean tracked files after hard reset")
	}
	if len(st.Untracked) != 1 || st.Untracked[0] != "untracked.txt" {
		t.Fatalf("expected untracked.txt to remain untracked")
	}
}

func TestResetUnbornBranch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-reset-unborn-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(fPath, []byte("content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"file.txt"}); err != nil {
		t.Fatal(err)
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 1 {
		t.Fatalf("expected 1 staged file on unborn branch")
	}

	if _, err := r.Reset(ResetSoft, "HEAD"); err != nil {
		t.Fatalf("soft reset on unborn branch failed: %v", err)
	}

	if _, err := r.Reset(ResetMixed, "HEAD"); err != nil {
		t.Fatalf("mixed reset on unborn branch failed: %v", err)
	}

	stMixed, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(stMixed.Staged) != 0 {
		t.Fatalf("expected staged files cleared after mixed reset on unborn branch")
	}
	if len(stMixed.Untracked) != 1 {
		t.Fatalf("expected file to be untracked after mixed reset on unborn branch")
	}

	if err := r.Add([]string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reset(ResetHard, "HEAD"); err != nil {
		t.Fatalf("hard reset on unborn branch failed: %v", err)
	}

	if _, err := os.Stat(fPath); !os.IsNotExist(err) {
		t.Fatalf("file should have been removed by hard reset on unborn branch")
	}
}

func TestResetPaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-reset-paths-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	f1 := filepath.Join(tempDir, "f1.txt")
	f2 := filepath.Join(tempDir, "f2.txt")
	if err := os.WriteFile(f1, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"f1.txt", "f2.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Commit 1", ""); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(f1, []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"f1.txt", "f2.txt"}); err != nil {
		t.Fatal(err)
	}

	stPre, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(stPre.Staged) != 2 {
		t.Fatalf("expected 2 staged files")
	}

	if err := r.ResetPaths("HEAD", []string{"f1.txt"}); err != nil {
		t.Fatalf("ResetPaths failed: %v", err)
	}

	stPost, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(stPost.Staged) != 1 || stPost.Staged[0].Path != "f2.txt" {
		t.Fatalf("expected only f2.txt staged, got: %+v", stPost.Staged)
	}
	if len(stPost.Unstaged) != 1 || stPost.Unstaged[0].Path != "f1.txt" {
		t.Fatalf("expected f1.txt unstaged, got: %+v", stPost.Unstaged)
	}
}
