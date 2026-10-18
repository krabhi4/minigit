package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStashPushAndCommitStructure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-stash-struct-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	aPath := filepath.Join(tempDir, "a.txt")
	if err := os.WriteFile(aPath, []byte("initial a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	c0Hash, err := r.Commit("Initial commit", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(aPath, []byte("staged a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}

	bPath := filepath.Join(tempDir, "b.txt")
	if err := os.WriteFile(bPath, []byte("unstaged b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"b.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("unstaged b modified\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := r.StashPush("work in progress feature")
	if err != nil {
		t.Fatalf("stash push failed: %v", err)
	}
	if !strings.Contains(out, "work in progress feature") {
		t.Fatalf("unexpected stash push message: %s", out)
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean {
		t.Fatalf("expected clean status after stash, got staged=%d unstaged=%d", len(st.Staged), len(st.Unstaged))
	}

	curA, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(curA) != "initial a\n" {
		t.Fatalf("expected a.txt reverted to initial, got: %s", string(curA))
	}

	stashHash, err := r.ResolveRevision("stash")
	if err != nil {
		t.Fatalf("failed to resolve stash revision: %v", err)
	}

	stashCommit, err := r.LoadCommit(stashHash)
	if err != nil {
		t.Fatalf("failed to load stash commit: %v", err)
	}

	if len(stashCommit.ParentHashes) != 2 {
		t.Fatalf("stash commit must have 2 parents, got %d", len(stashCommit.ParentHashes))
	}
	if stashCommit.ParentHashes[0] != c0Hash {
		t.Fatalf("parent 0 must be HEAD (%s), got %s", c0Hash, stashCommit.ParentHashes[0])
	}

	indexCommitHash := stashCommit.ParentHashes[1]
	indexCommit, err := r.LoadCommit(indexCommitHash)
	if err != nil {
		t.Fatalf("failed to load index commit: %v", err)
	}

	if len(indexCommit.ParentHashes) != 1 || indexCommit.ParentHashes[0] != c0Hash {
		t.Fatalf("index commit parent must be HEAD (%s), got %v", c0Hash, indexCommit.ParentHashes)
	}

	indexFiles, err := r.FlattenTree(indexCommit.TreeHash, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := indexFiles["a.txt"]; !ok {
		t.Fatalf("a.txt missing from index commit tree")
	}

	stashFiles, err := r.FlattenTree(stashCommit.TreeHash, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stashFiles["b.txt"]; !ok {
		t.Fatalf("b.txt missing from working tree stash commit")
	}

	revParent0, err := r.ResolveRevision("stash^")
	if err != nil {
		t.Fatal(err)
	}
	if revParent0 != c0Hash {
		t.Fatalf("stash^ did not resolve to c0Hash")
	}

	revParent1, err := r.ResolveRevision("stash^2")
	if err != nil {
		t.Fatal(err)
	}
	if revParent1 != indexCommitHash {
		t.Fatalf("stash^2 did not resolve to indexCommitHash")
	}

	revStash0, err := r.ResolveRevision("stash@{0}")
	if err != nil {
		t.Fatal(err)
	}
	if revStash0 != stashHash {
		t.Fatalf("stash@{0} did not resolve to stashHash")
	}
}

func TestStashMultipleListAndDrop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-stash-multi-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(fPath, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Initial commit", ""); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fPath, []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StashPush("first-stash"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fPath, []byte("v3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StashPush("second-stash"); err != nil {
		t.Fatal(err)
	}

	list, err := r.StashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 stashes, got %d", len(list))
	}
	if !strings.Contains(list[0], "stash@{0}: On main: second-stash") {
		t.Fatalf("expected stash@{0} second-stash, got: %s", list[0])
	}
	if !strings.Contains(list[1], "stash@{1}: On main: first-stash") {
		t.Fatalf("expected stash@{1} first-stash, got: %s", list[1])
	}

	hash0, err := r.ResolveRevision("stash@{0}")
	if err != nil {
		t.Fatal(err)
	}
	hash1, err := r.ResolveRevision("stash@{1}")
	if err != nil {
		t.Fatal(err)
	}
	if hash0 == hash1 {
		t.Fatalf("expected distinct hashes for stash@{0} and stash@{1}")
	}

	dropMsg, err := r.StashDrop("stash@{0}")
	if err != nil {
		t.Fatalf("stash drop failed: %v", err)
	}
	if !strings.Contains(dropMsg, "Dropped stash@{0}") {
		t.Fatalf("unexpected drop message: %s", dropMsg)
	}

	listAfterDrop, err := r.StashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(listAfterDrop) != 1 {
		t.Fatalf("expected 1 stash after drop, got %d", len(listAfterDrop))
	}
	if !strings.Contains(listAfterDrop[0], "stash@{0}: On main: first-stash") {
		t.Fatalf("expected stash@{0} first-stash after drop, got: %s", listAfterDrop[0])
	}

	if err := r.StashClear(); err != nil {
		t.Fatal(err)
	}
	listAfterClear, err := r.StashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(listAfterClear) != 0 {
		t.Fatalf("expected 0 stashes after clear, got %d", len(listAfterClear))
	}
}

func TestStashPopCleanAndConflict(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-stash-pop-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	docPath := filepath.Join(tempDir, "doc.txt")
	if err := os.WriteFile(docPath, []byte("base line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"doc.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Initial commit", ""); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(docPath, []byte("base line\nstashed line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"doc.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StashPush("feature addition"); err != nil {
		t.Fatal(err)
	}

	popMsg, err := r.StashPop("stash@{0}")
	if err != nil {
		t.Fatalf("stash pop clean failed: %v", err)
	}
	if !strings.Contains(popMsg, "Dropped stash@{0}") {
		t.Fatalf("unexpected pop message: %s", popMsg)
	}

	restoredContent, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restoredContent) != "base line\nstashed line\n" {
		t.Fatalf("doc.txt content not restored properly: %s", string(restoredContent))
	}

	list, err := r.StashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected stash dropped after pop, but still exists")
	}

	if _, err := r.StashPush("second feature"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(docPath, []byte("conflicting head change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"doc.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Conflicting commit", ""); err != nil {
		t.Fatal(err)
	}

	_, popConflictErr := r.StashPop("stash@{0}")
	if popConflictErr == nil {
		t.Fatalf("expected conflict on stash pop, got nil")
	}

	conflictData, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conflictData), "<<<<<<< HEAD") {
		t.Fatalf("expected conflict markers in doc.txt, got: %s", string(conflictData))
	}

	listAfterConflict, err := r.StashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(listAfterConflict) != 1 {
		t.Fatalf("stash entry must be kept on conflict, but list length is %d", len(listAfterConflict))
	}
}

func TestStashBrokenAndModifiedSymlink(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-stash-symlink-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "base.txt")
	if err := os.WriteFile(fPath, []byte("base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"base.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Initial commit", ""); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(tempDir, "symlink_test")
	if err := os.Symlink("target1_nonexistent", linkPath); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"symlink_test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Add broken symlink", ""); err != nil {
		t.Fatal(err)
	}

	_ = os.Remove(linkPath)
	if err := os.Symlink("target2_modified_nonexistent", linkPath); err != nil {
		t.Fatal(err)
	}

	if _, err := r.StashPush("stash modified broken symlink"); err != nil {
		t.Fatalf("stash push failed on broken symlink: %v", err)
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean {
		t.Fatalf("expected clean status after stash push, got: %+v", st)
	}

	linkTgt, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if linkTgt != "target1_nonexistent" {
		t.Fatalf("expected symlink reverted to target1_nonexistent, got: %s", linkTgt)
	}

	if _, err := r.StashPop("refs/stash@{0}"); err != nil {
		t.Fatalf("stash pop failed: %v", err)
	}

	poppedTgt, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if poppedTgt != "target2_modified_nonexistent" {
		t.Fatalf("expected popped symlink to be target2_modified_nonexistent, got: %s", poppedTgt)
	}
}

func TestStashPopDirtyWorktreeConflict(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-stash-dirty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	r, err := Init(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	fPath := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(fPath, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Init", ""); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fPath, []byte("v2 stashed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StashPush("stashed edit"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fPath, []byte("v3 dirty local work\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = r.StashPop("stash@{0}")
	if err == nil {
		t.Fatalf("expected error popping stash onto dirty local worktree, got nil")
	}
	if !strings.Contains(err.Error(), "Your local changes to the following files would be overwritten") {
		t.Fatalf("unexpected error message: %v", err)
	}

	curData, _ := os.ReadFile(fPath)
	if string(curData) != "v3 dirty local work\n" {
		t.Fatalf("user's local work was overwritten: %s", string(curData))
	}

	list, err := r.StashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("stash must be preserved on dirty conflict, list length is %d", len(list))
	}
}

func TestStashPopUnstagedPreservationAndIndexRestore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-stash-index-*")
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
	if err := os.WriteFile(f1, []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"f1.txt", "f2.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("Init", ""); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(f1, []byte("1 modified unstaged\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StashPush("unstaged stash"); err != nil {
		t.Fatal(err)
	}

	if _, err := r.StashPop("stash@{0}"); err != nil {
		t.Fatal(err)
	}

	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 0 {
		t.Fatalf("expected 0 staged files after default stash pop, got %d", len(st.Staged))
	}
	if len(st.Unstaged) != 1 || st.Unstaged[0].Path != "f1.txt" {
		t.Fatalf("expected f1.txt to be unstaged after default stash pop, got: %+v", st.Unstaged)
	}

	if err := os.WriteFile(f1, []byte("1 staged edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add([]string{"f1.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("2 unstaged edit\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := r.StashPush("mixed staged and unstaged"); err != nil {
		t.Fatal(err)
	}

	if _, err := r.StashPop("stash@{0}", true); err != nil {
		t.Fatalf("stash pop with restoreIndex failed: %v", err)
	}

	stMixed, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(stMixed.Staged) != 1 || stMixed.Staged[0].Path != "f1.txt" {
		t.Fatalf("expected f1.txt to be staged with --index, got: %+v", stMixed.Staged)
	}
	if len(stMixed.Unstaged) != 1 || stMixed.Unstaged[0].Path != "f2.txt" {
		t.Fatalf("expected f2.txt to be unstaged with --index, got: %+v", stMixed.Unstaged)
	}
}
