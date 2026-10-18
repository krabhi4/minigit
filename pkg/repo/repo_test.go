package repo

import (
	"bytes"
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

	resolveCommit, err := r.LoadCommit(resolveHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolveCommit.ParentHashes) != 2 {
		t.Fatalf("expected 2 parent hashes for merge commit, got %v", resolveCommit.ParentHashes)
	}
	if _, err := os.Stat(filepath.Join(r.GitDir, "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("expected MERGE_HEAD to be removed after commit")
	}

	finalStatus, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !finalStatus.Clean {
		t.Fatalf("expected clean status after commit: %+v", finalStatus)
	}
}

func TestRepoMergeAbort(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-abort-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "shared.txt")
	if err := os.WriteFile(fPath, []byte("line 1\nline 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	baseHash, err := r.Commit("Base", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("side", baseHash); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fPath, []byte("line 1 main\nline 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main edit", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("side"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fPath, []byte("line 1 side\nline 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(tempDir, "side_only.txt")
	if err := os.WriteFile(newFile, []byte("only in side\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt", "side_only.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Side edit", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}

	_, err = r.Merge("side")
	if err == nil {
		t.Fatalf("expected merge conflict")
	}

	if _, err := os.Stat(filepath.Join(r.GitDir, "MERGE_HEAD")); err != nil {
		t.Fatalf("expected MERGE_HEAD to exist during conflict: %v", err)
	}

	diffStaged, err := r.Diff(true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diffStaged, "shared.txt") {
		t.Fatalf("diff staged should not report unmerged file: %s", diffStaged)
	}

	if err := r.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(r.GitDir, "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("expected MERGE_HEAD to be deleted after abort")
	}

	sharedData, err := os.ReadFile(fPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sharedData) != "line 1 main\nline 2\n" {
		t.Fatalf("expected shared.txt restored to HEAD content, got: %s", string(sharedData))
	}

	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Fatalf("expected side_only.txt removed after abort")
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean {
		t.Fatalf("expected clean status after abort: %+v", st)
	}

	err = r.MergeAbort()
	if err == nil || !strings.Contains(err.Error(), "There is no merge to abort") {
		t.Fatalf("expected error on redundant abort, got: %v", err)
	}
}

func TestRepoMergeSameTreeResolution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-sametree-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "shared.txt")
	if err := os.WriteFile(fPath, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	baseHash, err := r.Commit("Base", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("branch-a", baseHash); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fPath, []byte("main version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main version", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("branch-a"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fPath, []byte("branch version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Branch version", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	_, err = r.Merge("branch-a")
	if err == nil {
		t.Fatalf("expected conflict")
	}

	if err := os.WriteFile(fPath, []byte("main version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"shared.txt"}); err != nil {
		t.Fatal(err)
	}

	commitHash, err := r.Commit("Keep ours", "Tester <t@t.com>")
	if err != nil {
		t.Fatalf("expected commit to succeed even when tree equals HEAD, got: %v", err)
	}

	c, err := r.LoadCommit(commitHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ParentHashes) != 2 {
		t.Fatalf("expected 2 parents for merge commit, got %v", c.ParentHashes)
	}
}

func TestRepoModifyDeleteConflict(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-moddel-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "mod_del.txt")
	if err := os.WriteFile(fPath, []byte("base data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"mod_del.txt"}); err != nil {
		t.Fatal(err)
	}
	baseHash, err := r.Commit("Base", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("side", baseHash); err != nil {
		t.Fatal(err)
	}

	if err := r.Remove([]string{"mod_del.txt"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Delete on main", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("side"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fPath, []byte("modified in side\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"mod_del.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Modify on side", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	_, err = r.Merge("side")
	if err == nil {
		t.Fatalf("expected conflict on modify/delete")
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Unmerged) != 1 || st.Unmerged[0].Status != StatusDeletedByUs {
		t.Fatalf("expected unmerged status deleted by us, got: %+v", st.Unmerged)
	}

	content, err := os.ReadFile(fPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "modified in side\n" {
		t.Fatalf("expected raw modified content in tree, got: %q", string(content))
	}
}

func TestRepoClone(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-clone-src-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	srcRepo, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	f1 := filepath.Join(tempDir, "hello.txt")
	if err := os.WriteFile(f1, []byte("hello clone\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := srcRepo.Add([]string{"hello.txt"}); err != nil {
		t.Fatal(err)
	}
	commitHash, err := srcRepo.Commit("commit for clone", "Tester <test@example.com>")
	if err != nil {
		t.Fatal(err)
	}

	dstDir, err := os.MkdirTemp("", "minigit-clone-dst-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dstDir)
	targetDir := filepath.Join(dstDir, "cloned_repo")

	clonedRepo, err := Clone(tempDir, targetDir)
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	branch, _, headHash, err := clonedRepo.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" || headHash != commitHash {
		t.Fatalf("mismatched cloned HEAD: branch=%s, hash=%s, expected=%s", branch, headHash, commitHash)
	}

	clonedFile := filepath.Join(targetDir, "hello.txt")
	data, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatalf("failed to read cloned file: %v", err)
	}
	if string(data) != "hello clone\n" {
		t.Fatalf("unexpected cloned file content: %q", string(data))
	}
}

func TestRepoIgnore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-repo-ignore-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.log\nsecret/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "app.log"), []byte("log entry\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "secret"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "secret", "key.txt"), []byte("secret key\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "main.txt"), []byte("valid file\n"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range st.Untracked {
		if strings.HasSuffix(u, ".log") || strings.HasPrefix(u, "secret/") {
			t.Fatalf("ignored file appeared in untracked: %s", u)
		}
	}

	err = r.Add([]string{"app.log"})
	if err == nil {
		t.Fatalf("expected error adding ignored file without force")
	}

	err = r.Add([]string{"app.log"}, true)
	if err != nil {
		t.Fatalf("force add of ignored file should succeed: %v", err)
	}

	err = r.Add([]string{"."})
	if err != nil {
		t.Fatal(err)
	}

	st, err = r.Status()
	if err != nil {
		t.Fatal(err)
	}
	var stagedPaths []string
	for _, s := range st.Staged {
		stagedPaths = append(stagedPaths, s.Path)
	}
	for _, p := range stagedPaths {
		if strings.HasPrefix(p, "secret/") {
			t.Fatalf("secret directory file was unexpectedly staged: %s", p)
		}
	}
}

func TestRepoDiffTreesRename(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-repo-difftrees-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"file1.txt"}); err != nil {
		t.Fatal(err)
	}
	c1, err := r.Commit("First", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Remove([]string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file2.txt"), []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"file2.txt"}); err != nil {
		t.Fatal(err)
	}
	c2, err := r.Commit("Second", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	commit1, _ := r.LoadCommit(c1)
	commit2, _ := r.LoadCommit(c2)
	tree1, _ := r.FlattenTree(commit1.TreeHash, "")
	tree2, _ := r.FlattenTree(commit2.TreeHash, "")

	diffOut, err := r.DiffTrees(tree1, tree2)
	if err != nil {
		t.Fatalf("DiffTrees failed: %v", err)
	}
	if !strings.Contains(diffOut, "similarity index 100%") {
		t.Fatalf("expected rename detection in diff, got: %s", diffOut)
	}
	if !strings.Contains(diffOut, "rename from file1.txt") || !strings.Contains(diffOut, "rename to file2.txt") {
		t.Fatalf("expected rename from/to headers, got: %s", diffOut)
	}
}

func TestRepoBinaryMergeConflict(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-repo-binmerge-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	binFile := filepath.Join(tempDir, "image.png")
	baseBytes := []byte("PNG\x00\x00\x00\rIHDRbase")
	if err := os.WriteFile(binFile, baseBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"image.png"}); err != nil {
		t.Fatal(err)
	}
	cBase, err := r.Commit("Base image", "Tester <t@t.com>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("feature", cBase); err != nil {
		t.Fatal(err)
	}

	mainBytes := []byte("PNG\x00\x00\x00\rIHDRmain_version")
	if err := os.WriteFile(binFile, mainBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"image.png"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main edit image", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("feature"); err != nil {
		t.Fatal(err)
	}
	featBytes := []byte("PNG\x00\x00\x00\rIHDRfeat_version")
	if err := os.WriteFile(binFile, featBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"image.png"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Feature edit image", "Tester <t@t.com>"); err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}

	_, err = r.Merge("feature")
	if err == nil {
		t.Fatalf("expected binary merge conflict")
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Unmerged) != 1 || st.Unmerged[0].Path != "image.png" {
		t.Fatalf("expected unmerged status for image.png, got: %+v", st.Unmerged)
	}

	treeContent, err := os.ReadFile(binFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(treeContent, mainBytes) {
		t.Fatalf("expected working tree to retain main binary content without markers")
	}

	if err := r.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort failed: %v", err)
	}

	stAfter, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !stAfter.Clean {
		t.Fatalf("expected clean status after merge abort, got: %+v", stAfter)
	}
}
