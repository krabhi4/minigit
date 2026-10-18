package diff

import (
	"strings"
	"testing"
)

func TestMyersDiffIdentical(t *testing.T) {
	a := []string{"one", "two", "three"}
	b := []string{"one", "two", "three"}
	edits := MyersDiff(a, b)
	if len(edits) != 3 {
		t.Fatalf("expected 3 edits, got %d", len(edits))
	}
	for _, e := range edits {
		if e.Op != OpEqual {
			t.Fatalf("expected all equal, got %v", e.Op)
		}
	}
	diff := FormatUnifiedDiff("f", "f", edits, 3)
	if diff != "" {
		t.Fatalf("expected empty diff for identical inputs, got %q", diff)
	}
}

func TestMyersDiffEmpty(t *testing.T) {
	edits := MyersDiff(nil, nil)
	if edits != nil {
		t.Fatalf("expected nil edits, got %v", edits)
	}

	a := []string{"hello"}
	editsA := MyersDiff(a, nil)
	if len(editsA) != 1 || editsA[0].Op != OpDelete {
		t.Fatalf("expected single delete, got %v", editsA)
	}

	b := []string{"world"}
	editsB := MyersDiff(nil, b)
	if len(editsB) != 1 || editsB[0].Op != OpInsert {
		t.Fatalf("expected single insert, got %v", editsB)
	}
}

func TestDiffStringsUnified(t *testing.T) {
	oldContent := "alpha\nbeta\ngamma\ndelta\n"
	newContent := "alpha\nbeta\nmodified gamma\ndelta\nepsilon\n"

	diffText := DiffStrings("sample.txt", "sample.txt", oldContent, newContent)
	if !strings.Contains(diffText, "--- a/sample.txt") {
		t.Fatalf("missing old header")
	}
	if !strings.Contains(diffText, "+++ b/sample.txt") {
		t.Fatalf("missing new header")
	}
	if !strings.Contains(diffText, "-gamma") {
		t.Fatalf("missing deleted line: %s", diffText)
	}
	if !strings.Contains(diffText, "+modified gamma") {
		t.Fatalf("missing inserted line: %s", diffText)
	}
	if !strings.Contains(diffText, "+epsilon") {
		t.Fatalf("missing inserted line epsilon: %s", diffText)
	}
}

func TestDiffStringsNewFileAndDeletedFile(t *testing.T) {
	newDiff := DiffStrings("/dev/null", "new.txt", "", "hello\nworld\n")
	if !strings.Contains(newDiff, "--- /dev/null") {
		t.Fatalf("expected /dev/null header, got %s", newDiff)
	}
	if !strings.Contains(newDiff, "+++ b/new.txt") {
		t.Fatalf("expected b/new.txt header, got %s", newDiff)
	}
	if !strings.Contains(newDiff, "@@ -0,0 +1,2 @@") {
		t.Fatalf("expected hunk @@ -0,0 +1,2 @@, got %s", newDiff)
	}

	delDiff := DiffStrings("old.txt", "/dev/null", "single line\n", "")
	if !strings.Contains(delDiff, "--- a/old.txt") {
		t.Fatalf("expected a/old.txt header, got %s", delDiff)
	}
	if !strings.Contains(delDiff, "+++ /dev/null") {
		t.Fatalf("expected /dev/null header, got %s", delDiff)
	}
	if !strings.Contains(delDiff, "@@ -1 +0,0 @@") {
		t.Fatalf("expected hunk @@ -1 +0,0 @@, got %s", delDiff)
	}
}

func TestDiffStringsTrailingNewlines(t *testing.T) {
	oldContent := "foo\n\n\n"
	newContent := "foo\n"
	diffText := DiffStrings("t.txt", "t.txt", oldContent, newContent)
	if !strings.Contains(diffText, "-") {
		t.Fatalf("expected deleted empty lines to be detected, got %q", diffText)
	}
}

