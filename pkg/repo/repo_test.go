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

func TestRepoMergeChunkLevelNonOverlapping(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-chunk-merge-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	docPath := filepath.Join(tempDir, "document.txt")
	initialContent := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"
	if err := os.WriteFile(docPath, []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"document.txt"}); err != nil {
		t.Fatal(err)
	}
	c1, err := r.Commit("Initial commit", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("feature-edit", c1); err != nil {
		t.Fatal(err)
	}

	mainContent := "line 1 modified by main\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"
	if err := os.WriteFile(docPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"document.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main edit top", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("feature-edit"); err != nil {
		t.Fatal(err)
	}
	featureContent := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10 modified by feature\n"
	if err := os.WriteFile(docPath, []byte(featureContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"document.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Feature edit bottom", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	mergeMsg, err := r.Merge("feature-edit")
	if err != nil {
		t.Fatalf("expected non-overlapping edits to merge cleanly, got error: %v", err)
	}
	if !strings.Contains(mergeMsg, "Merged feature-edit into main") {
		t.Fatalf("unexpected merge message: %s", mergeMsg)
	}

	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	mergedStr := string(data)
	if !strings.Contains(mergedStr, "line 1 modified by main") {
		t.Fatalf("merged document missing main edit: %s", mergedStr)
	}
	if !strings.Contains(mergedStr, "line 10 modified by feature") {
		t.Fatalf("merged document missing feature edit: %s", mergedStr)
	}
	if !strings.Contains(mergedStr, "line 5\n") {
		t.Fatalf("merged document missing intermediate lines: %s", mergedStr)
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean {
		t.Fatalf("expected clean working tree after successful merge: %+v", st)
	}
}

func TestRepoMergeMultiStageConflictsAndStatus(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-stage-conflict-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "shared.txt")
	baseContent := "line 1\nline 2\nline 3\nline 4\n"
	if err := os.WriteFile(fPath, []byte(baseContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	baseHash, err := r.Commit("Base commit", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("branch-conflict", baseHash); err != nil {
		t.Fatal(err)
	}

	mainContent := "line 1\nline 2\nline 3 modified by main\nline 4\n"
	if err := os.WriteFile(fPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main modification", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("branch-conflict"); err != nil {
		t.Fatal(err)
	}
	branchContent := "line 1\nline 2\nline 3 modified by branch\nline 4\n"
	if err := os.WriteFile(fPath, []byte(branchContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Branch modification", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}

	_, err = r.Merge("branch-conflict")
	if err == nil {
		t.Fatalf("expected merge conflict, got nil error")
	}

	idx, err := r.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if !idx.HasConflicts() {
		t.Fatalf("expected index to have conflicts")
	}

	stages := make(map[int]bool)
	for _, entry := range idx.Entries {
		if entry.Path == "shared.txt" {
			stages[entry.Stage()] = true
		}
	}
	if !stages[1] || !stages[2] || !stages[3] {
		t.Fatalf("expected stages 1, 2, and 3 for shared.txt, got %v", stages)
	}
	if stages[0] {
		t.Fatalf("stage 0 must not exist for conflicted path")
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Clean {
		t.Fatalf("status should not be clean")
	}
	if len(st.Unmerged) != 1 {
		t.Fatalf("expected 1 unmerged file, got %d", len(st.Unmerged))
	}
	if st.Unmerged[0].Path != "shared.txt" || st.Unmerged[0].Status != StatusBothModified {
		t.Fatalf("unexpected unmerged file status: %+v", st.Unmerged[0])
	}
	if len(st.Staged) != 0 {
		t.Fatalf("unmerged path must not appear in staged files: %+v", st.Staged)
	}

	_, err = r.Commit("Attempt commit with conflicts", "Tester <t@t.com>")
	if err == nil {
		t.Fatalf("commit should fail when index has unmerged entries")
	}

	resolvedContent := "line 1\nline 2\nline 3 resolved\nline 4\n"
	if err := os.WriteFile(fPath, []byte(resolvedContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}

	idxAfter, err := r.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if idxAfter.HasConflicts() {
		t.Fatalf("expected no conflicts after adding resolved file")
	}
	var sharedEntries []int
	for _, entry := range idxAfter.Entries {
		if entry.Path == "shared.txt" {
			sharedEntries = append(sharedEntries, entry.Stage())
		}
	}
	if len(sharedEntries) != 1 || sharedEntries[0] != 0 {
		t.Fatalf("expected only stage 0 after resolution, got stages: %v", sharedEntries)
	}

	stAfter, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(stAfter.Unmerged) != 0 {
		t.Fatalf("expected 0 unmerged files after resolution, got %d", len(stAfter.Unmerged))
	}
	if len(stAfter.Staged) != 1 || stAfter.Staged[0].Path != "shared.txt" || stAfter.Staged[0].Status != StatusModified {
		t.Fatalf("expected shared.txt to be staged modified, got: %+v", stAfter.Staged)
	}

	resolveHash, err := r.Commit("Resolve conflict", "Tester <t@t.com>")
	if err != nil {
		t.Fatalf("expected commit after conflict resolution to succeed: %v", err)
	}
	if len(resolveHash) != 40 {
		t.Fatalf("expected valid commit hash, got %s", resolveHash)
	}

	finalStatus, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !finalStatus.Clean {
		t.Fatalf("expected clean status after commit: %+v", finalStatus)
	}
}

