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

func TestMerge3NonOverlapping(t *testing.T) {
	base := "header\nline1\nline2\nline3\nfooter\n"
	ours := "header-mod\nline1\nline2\nline3\nfooter\n"
	theirs := "header\nline1\nline2\nline3\nfooter-mod\n"

	merged, conflict := Merge3(base, ours, theirs, "feature")
	if conflict {
		t.Fatalf("expected non-overlapping changes to merge cleanly, got conflict: %s", merged)
	}
	expected := "header-mod\nline1\nline2\nline3\nfooter-mod\n"
	if merged != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, merged)
	}
}

func TestMerge3OverlappingConflict(t *testing.T) {
	base := "header\nline1\nline2\nline3\nfooter\n"
	ours := "header\nline1-ours\nline2\nline3\nfooter\n"
	theirs := "header\nline1-theirs\nline2\nline3\nfooter\n"

	merged, conflict := Merge3(base, ours, theirs, "feature")
	if !conflict {
		t.Fatalf("expected conflict on overlapping edit, got none")
	}
	if !strings.Contains(merged, "header\n") || !strings.Contains(merged, "footer\n") {
		t.Fatalf("expected common header/footer outside conflict: %s", merged)
	}
	if !strings.Contains(merged, "<<<<<<< HEAD\nline1-ours\n=======\nline1-theirs\n>>>>>>> feature\n") {
		t.Fatalf("expected exact conflict markers around line1: %s", merged)
	}
}

func TestMerge3IdenticalChanges(t *testing.T) {
	base := "line1\nline2\nline3\n"
	ours := "line1\nline2-both\nline3\n"
	theirs := "line1\nline2-both\nline3\n"

	merged, conflict := Merge3(base, ours, theirs, "feature")
	if conflict {
		t.Fatalf("expected identical changes to merge without conflict, got: %s", merged)
	}
	expected := "line1\nline2-both\nline3\n"
	if merged != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, merged)
	}
}

func TestMerge3InsertionsAndDeletions(t *testing.T) {
	base := "line1\nline2\nline3\n"
	ours := "insert-start\nline1\nline2\nline3\n"
	theirs := "line1\nline2\nline3\ninsert-end\n"

	merged, conflict := Merge3(base, ours, theirs, "feature")
	if conflict {
		t.Fatalf("expected clean merge of insertions at start and end: %s", merged)
	}
	expected := "insert-start\nline1\nline2\nline3\ninsert-end\n"
	if merged != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, merged)
	}

	base2 := "line1\nline2\nline3\n"
	ours2 := "line1\nline3\n"
	theirs2 := "line1\nline2-mod\nline3\n"
	merged2, conflict2 := Merge3(base2, ours2, theirs2, "feature")
	if !conflict2 {
		t.Fatalf("expected delete vs mod conflict, got none")
	}
	if !strings.Contains(merged2, "<<<<<<< HEAD\n=======\nline2-mod\n>>>>>>> feature") {
		t.Fatalf("expected delete vs mod conflict marker: %s", merged2)
	}
}

func TestMerge3EmptyBase(t *testing.T) {
	mergedSame, conflictSame := Merge3("", "alpha\nbeta\n", "alpha\nbeta\n", "feature")
	if conflictSame {
		t.Fatalf("expected same content from empty base to merge without conflict")
	}
	if mergedSame != "alpha\nbeta\n" {
		t.Fatalf("expected 'alpha\\nbeta\\n', got %q", mergedSame)
	}

	mergedDiff, conflictDiff := Merge3("", "alpha\n", "beta\n", "feature")
	if !conflictDiff {
		t.Fatalf("expected different content from empty base to conflict")
	}
	if !strings.Contains(mergedDiff, "<<<<<<< HEAD\nalpha\n=======\nbeta\n>>>>>>> feature") {
		t.Fatalf("expected conflict marker: %s", mergedDiff)
	}
}


