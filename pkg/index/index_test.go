package index

import (
	"crypto/sha1"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexReadWriteAndGitCompatibility(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-index-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(gitDir, "index")

	idx := &Index{}

	shaFoo := sha1.Sum([]byte("blob 4\x00foo\n"))
	shaBar := sha1.Sum([]byte("blob 4\x00bar\n"))

	idx.AddOrUpdate(IndexEntry{
		Mode:     0100644,
		FileSize: 4,
		Sha1:     shaFoo,
		Path:     "foo.txt",
	})
	idx.AddOrUpdate(IndexEntry{
		Mode:     0100644,
		FileSize: 4,
		Sha1:     shaBar,
		Path:     "subdir/bar.txt",
	})

	if err := idx.Write(indexPath); err != nil {
		t.Fatal(err)
	}

	readBack, err := ReadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(readBack.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(readBack.Entries))
	}
	if readBack.Entries[0].Path != "foo.txt" {
		t.Fatalf("expected foo.txt first, got %s", readBack.Entries[0].Path)
	}
	if readBack.Entries[1].Path != "subdir/bar.txt" {
		t.Fatalf("expected subdir/bar.txt second, got %s", readBack.Entries[1].Path)
	}
	if readBack.Entries[0].Sha1 != shaFoo {
		t.Fatalf("sha mismatch for foo.txt")
	}

	cmd := exec.Command("git", "init", tempDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, string(out))
	}

	if err := idx.Write(indexPath); err != nil {
		t.Fatal(err)
	}

	lsCmd := exec.Command("git", "ls-files", "--stage")
	lsCmd.Dir = tempDir
	out, err := lsCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files failed: %v: %s", err, string(out))
	}

	outStr := string(out)
	if !strings.Contains(outStr, "foo.txt") || !strings.Contains(outStr, "subdir/bar.txt") {
		t.Fatalf("unexpected git ls-files output: %s", outStr)
	}
}

func TestBuildTree(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-tree-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		t.Fatal(err)
	}

	idx := &Index{}
	shaA := sha1.Sum([]byte("blob 2\x00a\n"))
	shaB := sha1.Sum([]byte("blob 2\x00b\n"))

	idx.AddOrUpdate(IndexEntry{
		Mode: 0100644,
		Sha1: shaA,
		Path: "file.txt",
	})
	idx.AddOrUpdate(IndexEntry{
		Mode: 0100644,
		Sha1: shaB,
		Path: "nested/dir/inner.txt",
	})

	rootTreeHash, err := idx.BuildTree(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootTreeHash) != 40 {
		t.Fatalf("expected 40 char tree hash, got %s", rootTreeHash)
	}
}

func TestIndexStageSortingAndRemoval(t *testing.T) {
	idx := &Index{}
	sha := sha1.Sum([]byte("blob 1\x00x"))
	idx.AddOrUpdate(IndexEntry{Path: "conflict.txt", Flags: 2 << 12, Sha1: sha})
	idx.AddOrUpdate(IndexEntry{Path: "conflict.txt", Flags: 1 << 12, Sha1: sha})
	idx.AddOrUpdate(IndexEntry{Path: "conflict.txt", Flags: 3 << 12, Sha1: sha})

	if len(idx.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(idx.Entries))
	}
	if (idx.Entries[0].Flags>>12)&3 != 1 || (idx.Entries[1].Flags>>12)&3 != 2 || (idx.Entries[2].Flags>>12)&3 != 3 {
		t.Fatalf("expected stages 1, 2, 3 sorted, got %d, %d, %d",
			(idx.Entries[0].Flags>>12)&3, (idx.Entries[1].Flags>>12)&3, (idx.Entries[2].Flags>>12)&3)
	}

	idx.AddOrUpdate(IndexEntry{Path: "conflict.txt", Flags: 0, Sha1: sha})
	if len(idx.Entries) != 1 {
		t.Fatalf("expected 1 entry after stage 0 resolution, got %d", len(idx.Entries))
	}
	if (idx.Entries[0].Flags>>12)&3 != 0 {
		t.Fatalf("expected stage 0, got %d", (idx.Entries[0].Flags>>12)&3)
	}
}

func TestBuildTreeWithUnmergedEntries(t *testing.T) {
	idx := &Index{}
	sha := sha1.Sum([]byte("blob 1\x00x"))
	idx.AddOrUpdate(IndexEntry{Path: "conflicted.txt", Flags: 2 << 12, Sha1: sha})
	if !idx.HasConflicts() {
		t.Fatalf("expected HasConflicts() to be true")
	}

	_, err := idx.BuildTree("/tmp")
	if err == nil {
		t.Fatalf("expected BuildTree to fail with unmerged entries")
	}
	if !strings.Contains(err.Error(), "unmerged") {
		t.Fatalf("expected error mentioning unmerged, got: %v", err)
	}

	idx.AddOrUpdate(IndexEntry{Path: "conflicted.txt", Flags: 0, Sha1: sha})
	if idx.HasConflicts() {
		t.Fatalf("expected HasConflicts() to be false after stage 0 add")
	}
}

