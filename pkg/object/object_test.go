package object

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHashAndWriteBlob(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-obj-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	gitDir := filepath.Join(tempDir, ".git")
	content := []byte("hello world\n")

	hash, fullData := HashObject(TypeBlob, content)
	expectedHash := "3b18e512dba79e4c8300dd08aeb37f8e728b8dad"
	if hash != expectedHash {
		t.Fatalf("expected hash %s, got %s", expectedHash, hash)
	}
	if string(fullData[:13]) != "blob 12\x00hello" {
		t.Fatalf("unexpected header in fullData: %q", fullData[:13])
	}

	writtenHash, err := WriteObject(gitDir, TypeBlob, content)
	if err != nil {
		t.Fatal(err)
	}
	if writtenHash != expectedHash {
		t.Fatalf("expected written hash %s, got %s", expectedHash, writtenHash)
	}

	objType, readContent, fullHash, err := ReadObject(gitDir, hash[:7])
	if err != nil {
		t.Fatal(err)
	}
	if objType != TypeBlob {
		t.Fatalf("expected type blob, got %s", objType)
	}
	if string(readContent) != string(content) {
		t.Fatalf("expected content %q, got %q", string(content), string(readContent))
	}
	if fullHash != expectedHash {
		t.Fatalf("expected full hash %s, got %s", expectedHash, fullHash)
	}
}

func TestTreeSerializeAndParse(t *testing.T) {
	tree := &Tree{
		Entries: []TreeEntry{
			{Mode: "100644", Name: "a.txt", Hash: "83f9fcffd0078f98366ccea622e98843de7f2b22"},
			{Mode: "40000", Name: "a", Hash: "04756934bd18bee46b7978441ff47dfd695e6344"},
			{Mode: "100644", Name: "a-dash.txt", Hash: "a2544f7ec3007899167de1fef481a5a0fd63fa41"},
		},
	}

	serialized := tree.Serialize()

	if tree.Entries[0].Name != "a-dash.txt" {
		t.Fatalf("expected first entry a-dash.txt, got %s", tree.Entries[0].Name)
	}
	if tree.Entries[1].Name != "a.txt" {
		t.Fatalf("expected second entry a.txt, got %s", tree.Entries[1].Name)
	}
	if tree.Entries[2].Name != "a" {
		t.Fatalf("expected third entry a, got %s", tree.Entries[2].Name)
	}

	parsed, err := ParseTree(serialized)
	if err != nil {
		t.Fatal(err)
	}

	if len(parsed.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(parsed.Entries))
	}
	for i, entry := range parsed.Entries {
		if entry.Name != tree.Entries[i].Name {
			t.Fatalf("entry %d name mismatch: %s vs %s", i, entry.Name, tree.Entries[i].Name)
		}
		if entry.Mode != tree.Entries[i].Mode {
			t.Fatalf("entry %d mode mismatch: %s vs %s", i, entry.Mode, tree.Entries[i].Mode)
		}
		if entry.Hash != tree.Entries[i].Hash {
			t.Fatalf("entry %d hash mismatch: %s vs %s", i, entry.Hash, tree.Entries[i].Hash)
		}
	}
}

func TestCommitSerializeAndParse(t *testing.T) {
	now := time.Unix(1700000000, 0)
	c := &Commit{
		TreeHash:      "52f13e2940cb1c6dfc116781fb7912cef05e1670",
		ParentHashes:  []string{"3b18e512dba79e4c8300dd08aeb37f8e728b8dad"},
		Author:        "Dev User <dev@example.com>",
		AuthorTime:    now,
		Committer:     "Dev User <dev@example.com>",
		CommitterTime: now,
		Message:       "Implement something awesome\n\nDetailed explanation",
	}

	serialized := c.Serialize()
	parsed, err := ParseCommit(serialized)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.TreeHash != c.TreeHash {
		t.Fatalf("expected tree %s, got %s", c.TreeHash, parsed.TreeHash)
	}
	if len(parsed.ParentHashes) != 1 || parsed.ParentHashes[0] != c.ParentHashes[0] {
		t.Fatalf("expected parents %v, got %v", c.ParentHashes, parsed.ParentHashes)
	}
	if parsed.Author != c.Author {
		t.Fatalf("expected author %s, got %s", c.Author, parsed.Author)
	}
	if parsed.AuthorTime.Unix() != now.Unix() {
		t.Fatalf("expected author time %d, got %d", now.Unix(), parsed.AuthorTime.Unix())
	}
	if parsed.Message != c.Message+"\n" {
		t.Fatalf("expected message %q, got %q", c.Message+"\n", parsed.Message)
	}
}
