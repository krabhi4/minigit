package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCherryPickClean(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-cp-clean-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(aPath, []byte("a initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c0, err := r.Commit("Initial commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("feature", c0); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("feature"); err != nil {
		t.Fatal(err)
	}

	bPath := filepath.Join(tempDir, "b.txt")
	if err := os.WriteFile(bPath, []byte("feature b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"b.txt"}); err != nil {
		t.Fatal(err)
	}
	featCommit, err := r.Commit("Add feature b", "Feature Dev <dev@feature.org>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}

	cPath := filepath.Join(tempDir, "c.txt")
	if err := os.WriteFile(cPath, []byte("c initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"c.txt"}); err != nil {
		t.Fatal(err)
	}
	cMain, err := r.Commit("Main commit", "")
	if err != nil {
		t.Fatal(err)
	}

	msg, err := r.CherryPick(featCommit)
	if err != nil {
		t.Fatalf("cherry-pick failed: %v", err)
	}
	if !strings.Contains(msg, "Add feature b") {
		t.Fatalf("unexpected cherry-pick output: %s", msg)
	}

	_, _, newHead, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}

	headCommit, err := r.LoadCommit(newHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(headCommit.ParentHashes) != 1 || headCommit.ParentHashes[0] != cMain {
		t.Fatalf("expected single parent %s, got %v", cMain, headCommit.ParentHashes)
	}
	if headCommit.Author != "Feature Dev <dev@feature.org>" {
		t.Fatalf("expected author preserved, got %s", headCommit.Author)
	}

	bContent, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatalf("b.txt missing after cherry-pick: %v", err)
	}
	if string(bContent) != "feature b\n" {
		t.Fatalf("b.txt content mismatch: %s", string(bContent))
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean {
		t.Fatalf("expected clean status after clean cherry-pick")
	}
}

func TestCherryPickConflictAndAbort(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-cp-conflict-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(aPath, []byte("line 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c0, err := r.Commit("Initial commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("feature", c0); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("feature"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("line 1\nfeature change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	featCommit, err := r.Commit("Feature update", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("line 1\nmain conflicting change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main update", ""); err != nil {
		t.Fatal(err)
	}

	_, cpErr := r.CherryPick(featCommit)
	if cpErr == nil {
		t.Fatalf("expected conflict on cherry-pick, got nil")
	}

	data, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<<<<<<< HEAD") {
		t.Fatalf("expected conflict markers in a.txt, got: %s", string(data))
	}

	cpHeadPath := filepath.Join(r.GitDir, "CHERRY_PICK_HEAD")
	if _, err := os.Stat(cpHeadPath); os.IsNotExist(err) {
		t.Fatalf("CHERRY_PICK_HEAD missing after conflict")
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsCherryPicking {
		t.Fatalf("expected IsCherryPicking to be true")
	}
	if len(st.Unmerged) != 1 {
		t.Fatalf("expected 1 unmerged file, got %d", len(st.Unmerged))
	}

	if err := r.CherryPickAbort(); err != nil {
		t.Fatalf("cherry-pick --abort failed: %v", err)
	}

	if _, err := os.Stat(cpHeadPath); !os.IsNotExist(err) {
		t.Fatalf("CHERRY_PICK_HEAD still present after abort")
	}

	abortedData, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(abortedData) != "line 1\nmain conflicting change\n" {
		t.Fatalf("a.txt not restored properly after abort: %s", string(abortedData))
	}

	stAfterAbort, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !stAfterAbort.Clean || stAfterAbort.IsCherryPicking {
		t.Fatalf("expected clean non-cherrypicking state after abort")
	}
}

func TestCherryPickConflictResolution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-cp-resolve-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(aPath, []byte("line 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c0, err := r.Commit("Initial commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("feature", c0); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("feature"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("line 1\nfeature change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	featCommit, err := r.Commit("Feature update commit message", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("line 1\nmain conflicting change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main update", ""); err != nil {
		t.Fatal(err)
	}

	_, _ = r.CherryPick(featCommit)

	if err := os.WriteFile(aPath, []byte("line 1\nresolved change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}

	mergeMsgData, err := os.ReadFile(filepath.Join(r.GitDir, "MERGE_MSG"))
	if err != nil {
		t.Fatal(err)
	}
	msgStr := strings.TrimSpace(string(mergeMsgData))

	commitHash, err := r.Commit(msgStr, "")
	if err != nil {
		t.Fatalf("commit after resolving cherry-pick failed: %v", err)
	}

	resolvedCommit, err := r.LoadCommit(commitHash)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resolvedCommit.Message, "Feature update commit message") {
		t.Fatalf("expected cherry-pick commit message preserved, got: %s", resolvedCommit.Message)
	}

	if _, err := os.Stat(filepath.Join(r.GitDir, "CHERRY_PICK_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("CHERRY_PICK_HEAD was not removed after commit")
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean || st.IsCherryPicking {
		t.Fatalf("expected clean state after cherry-pick resolution commit")
	}
}

func TestCherryPickEmpty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-cp-empty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(aPath, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c0, err := r.Commit("Initial commit", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.CherryPick(c0)
	if err == nil {
		t.Fatalf("expected empty cherry-pick error, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCherryPickMergeCommit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-cp-merge-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	f1 := filepath.Join(tempDir, "f1.txt")
	if err := os.WriteFile(f1, []byte("base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"f1.txt"}); err != nil {
		t.Fatal(err)
	}
	c0, err := r.Commit("Base commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("branch1", c0); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("branch1"); err != nil {
		t.Fatal(err)
	}
	f2 := filepath.Join(tempDir, "f2.txt")
	if err := os.WriteFile(f2, []byte("b1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"f2.txt"}); err != nil {
		t.Fatal(err)
	}
	_, err = r.Commit("Commit on branch1", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	f3 := filepath.Join(tempDir, "f3.txt")
	if err := os.WriteFile(f3, []byte("main3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"f3.txt"}); err != nil {
		t.Fatal(err)
	}
	_, err = r.Commit("Commit on main", "")
	if err != nil {
		t.Fatal(err)
	}

	mergeOut, err := r.Merge("branch1")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	_ = mergeOut

	_, _, mergeCommitHash, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("receiver", c0); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("receiver"); err != nil {
		t.Fatal(err)
	}

	_, err = r.CherryPick(mergeCommitHash)
	if err == nil {
		t.Fatalf("expected error cherry-picking merge commit without -m")
	}
	if !strings.Contains(err.Error(), "is a merge but no -m option was given") {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := r.CherryPick(mergeCommitHash, 1)
	if err != nil {
		t.Fatalf("cherry-pick with -m 1 failed: %v", err)
	}
	if msg == "" {
		t.Fatalf("expected cherry-pick output message")
	}

	data2, err := os.ReadFile(f2)
	if err != nil || string(data2) != "b1\n" {
		t.Fatalf("expected f2.txt from branch1 diff: %v", err)
	}
}

func TestCherryPickDirtyWorktreeProtection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-cp-dirty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fA := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(fA, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c0, err := r.Commit("Init", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("feat", c0); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("feat"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fA, []byte("feat edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	featCommit, err := r.Commit("feat commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fA, []byte("dirty worktree local work\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = r.CherryPick(featCommit)
	if err == nil {
		t.Fatalf("expected dirty worktree error on cherry-pick")
	}
	if !strings.Contains(err.Error(), "Your local changes to the following files would be overwritten") {
		t.Fatalf("unexpected error message: %v", err)
	}

	curData, _ := os.ReadFile(fA)
	if string(curData) != "dirty worktree local work\n" {
		t.Fatalf("local dirty work was overwritten: %s", string(curData))
	}
}

func TestCherryPickContinue(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-cp-continue-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fA := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(fA, []byte("base line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c0, err := r.Commit("Base commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.CreateBranch("branchA", c0); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("branchA"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fA, []byte("branchA line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	cpCommit, err := r.Commit("Feature branchA commit", "Original Dev <dev@original.org>")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fA, []byte("main conflicting line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Main conflicting commit", ""); err != nil {
		t.Fatal(err)
	}

	_, err = r.CherryPick(cpCommit)
	if err == nil {
		t.Fatalf("expected conflict on cherry-pick")
	}

	_, err = r.CherryPickContinue()
	if err == nil {
		t.Fatalf("expected CherryPickContinue to fail with unmerged files")
	}

	if err := os.WriteFile(fA, []byte("resolved line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}

	contMsg, err := r.CherryPickContinue()
	if err != nil {
		t.Fatalf("CherryPickContinue failed: %v", err)
	}
	if !strings.Contains(contMsg, "Feature branchA commit") {
		t.Fatalf("unexpected continue message: %s", contMsg)
	}

	_, _, headAfterCont, err := r.ResolveHEAD()
	if err != nil {
		t.Fatal(err)
	}
	contCommit, err := r.LoadCommit(headAfterCont)
	if err != nil {
		t.Fatal(err)
	}
	if contCommit.Author != "Original Dev <dev@original.org>" {
		t.Fatalf("expected original author preserved, got: %s", contCommit.Author)
	}
}
