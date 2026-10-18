package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %v: %v\nOutput: %s", name, args, err, string(out))
	}
	return string(out)
}

func TestMinigitFullLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-lifecycle-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	if _, err := os.Stat(filepath.Join(tempDir, ".git")); err != nil {
		t.Fatalf(".git directory was not created")
	}

	docPath := filepath.Join(tempDir, "hello.txt")
	if err := os.WriteFile(docPath, []byte("Hello Minigit\n"), 0644); err != nil {
		t.Fatal(err)
	}

	stOut := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(stOut, "hello.txt") || !strings.Contains(stOut, "Untracked files") {
		t.Fatalf("status did not show untracked hello.txt: %s", stOut)
	}

	runCmd(t, tempDir, binPath, "add", "hello.txt")

	stOut = runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(stOut, "Changes to be committed") || !strings.Contains(stOut, "new file:   hello.txt") {
		t.Fatalf("status did not show staged hello.txt: %s", stOut)
	}

	commitOut := runCmd(t, tempDir, binPath, "commit", "-m", "First commit from minigit")
	if !strings.Contains(commitOut, "First commit from minigit") {
		t.Fatalf("commit output unexpected: %s", commitOut)
	}

	logOut := runCmd(t, tempDir, binPath, "log")
	if !strings.Contains(logOut, "First commit from minigit") {
		t.Fatalf("log missing commit message: %s", logOut)
	}

	runCmd(t, tempDir, binPath, "branch", "feature-branch")
	branchOut := runCmd(t, tempDir, binPath, "branch")
	if !strings.Contains(branchOut, "* main") || !strings.Contains(branchOut, "feature-branch") {
		t.Fatalf("branch listing unexpected: %s", branchOut)
	}

	runCmd(t, tempDir, binPath, "checkout", "feature-branch")
	branchOut = runCmd(t, tempDir, binPath, "branch")
	if !strings.Contains(branchOut, "* feature-branch") {
		t.Fatalf("expected feature-branch to be active: %s", branchOut)
	}

	featDoc := filepath.Join(tempDir, "feature.md")
	if err := os.WriteFile(featDoc, []byte("# Feature Documentation\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "feature.md")
	runCmd(t, tempDir, binPath, "commit", "-m", "Add feature doc")

	runCmd(t, tempDir, binPath, "tag", "v0.1.0")
	tagOut := runCmd(t, tempDir, binPath, "tag")
	if !strings.Contains(tagOut, "v0.1.0") {
		t.Fatalf("tag listing missing v0.1.0: %s", tagOut)
	}

	runCmd(t, tempDir, binPath, "checkout", "main")
	if _, err := os.Stat(featDoc); !os.IsNotExist(err) {
		t.Fatalf("expected feature.md to disappear on main branch")
	}

	runCmd(t, tempDir, binPath, "checkout", "feature-branch")
	if _, err := os.Stat(featDoc); err != nil {
		t.Fatalf("expected feature.md to exist on feature-branch")
	}
}

func TestGitFsckAndCrossCompatibility(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-git-compat-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "app.py"), []byte("print('hello')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# My App\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "add", ".")
	runCmd(t, tempDir, binPath, "commit", "-m", "Commit from minigit")

	fsckOut := runCmd(t, tempDir, "git", "fsck", "--full")
	if strings.Contains(fsckOut, "error") || strings.Contains(fsckOut, "fatal") || strings.Contains(fsckOut, "corrupt") {
		t.Fatalf("git fsck reported corruption: %s", fsckOut)
	}

	gitLogOut := runCmd(t, tempDir, "git", "log", "--oneline")
	if !strings.Contains(gitLogOut, "Commit from minigit") {
		t.Fatalf("git log could not read minigit commit: %s", gitLogOut)
	}

	gitStatusOut := runCmd(t, tempDir, "git", "status")
	if !strings.Contains(gitStatusOut, "nothing to commit") {
		t.Fatalf("git status disagreed with minigit commit state: %s", gitStatusOut)
	}

	if err := os.WriteFile(filepath.Join(tempDir, "git_created.txt"), []byte("Created by real git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, "git", "add", "git_created.txt")
	runCmd(t, tempDir, "git", "-c", "user.name=GitAuthor", "-c", "user.email=author@git.com", "commit", "-m", "Commit from real git")

	minigitLogOut := runCmd(t, tempDir, binPath, "log", "--oneline")
	if !strings.Contains(minigitLogOut, "Commit from real git") || !strings.Contains(minigitLogOut, "Commit from minigit") {
		t.Fatalf("minigit log failed to read commit history written by real git: %s", minigitLogOut)
	}

	minigitStatusOut := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(minigitStatusOut, "nothing to commit, working tree clean") {
		t.Fatalf("minigit status showed dirty repo after git commit: %s", minigitStatusOut)
	}
}

func TestGitGcAndPackfileCompatibility(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-gc-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")
	testFile := filepath.Join(tempDir, "data.txt")
	if err := os.WriteFile(testFile, []byte("Packfile test data\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "add", "data.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Packed commit test")
	runCmd(t, tempDir, binPath, "tag", "v1.0-packed")
	runCmd(t, tempDir, binPath, "branch", "packed-branch")

	runCmd(t, tempDir, "git", "gc")

	logOut := runCmd(t, tempDir, binPath, "log")
	if !strings.Contains(logOut, "Packed commit test") {
		t.Fatalf("minigit log failed on gc-packed repo: %s", logOut)
	}

	statusOut := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(statusOut, "nothing to commit, working tree clean") {
		t.Fatalf("minigit status failed on gc-packed repo: %s", statusOut)
	}

	branchOut := runCmd(t, tempDir, binPath, "branch")
	if !strings.Contains(branchOut, "packed-branch") {
		t.Fatalf("minigit branch failed to list packed branch: %s", branchOut)
	}

	tagOut := runCmd(t, tempDir, binPath, "tag")
	if !strings.Contains(tagOut, "v1.0-packed") {
		t.Fatalf("minigit tag failed to list packed tag: %s", tagOut)
	}

	runCmd(t, tempDir, binPath, "checkout", "packed-branch")
	branchOut = runCmd(t, tempDir, binPath, "branch")
	if !strings.Contains(branchOut, "* packed-branch") {
		t.Fatalf("expected packed-branch active: %s", branchOut)
	}
}

func TestSubdirectoryExecution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-subdir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	subDir := filepath.Join(tempDir, "deep", "nested", "folder")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	subFile := filepath.Join(subDir, "inner.txt")
	if err := os.WriteFile(subFile, []byte("nested content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runCmd(t, subDir, binPath, "add", "inner.txt")
	statusOut := runCmd(t, subDir, binPath, "status")
	if !strings.Contains(statusOut, "deep/nested/folder/inner.txt") {
		t.Fatalf("expected staged deep/nested/folder/inner.txt, got %s", statusOut)
	}

	runCmd(t, subDir, binPath, "commit", "-m", "Commit from nested directory")
	logOut := runCmd(t, subDir, binPath, "log", "--oneline")
	if !strings.Contains(logOut, "Commit from nested directory") {
		t.Fatalf("expected log to show nested commit: %s", logOut)
	}

	runCmd(t, subDir, binPath, "rm", "inner.txt")
	if _, err := os.Stat(subFile); !os.IsNotExist(err) {
		t.Fatalf("expected inner.txt to be removed by rm")
	}
}

func TestSymlinksAndEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-symlink-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	targetFile := filepath.Join(tempDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target file content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(tempDir, "link_to_target.txt")
	if err := os.Symlink("target.txt", symlinkPath); err != nil {
		t.Fatal(err)
	}

	danglingPath := filepath.Join(tempDir, "broken_link.txt")
	if err := os.Symlink("non_existent_dest", danglingPath); err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "add", ".")
	runCmd(t, tempDir, binPath, "commit", "-m", "Add symlinks")

	lsOut := runCmd(t, tempDir, "git", "ls-files", "--stage")
	if !strings.Contains(lsOut, "120000") {
		t.Fatalf("expected git ls-files to have mode 120000 for symlinks: %s", lsOut)
	}

	runCmd(t, tempDir, binPath, "branch", "other")
	runCmd(t, tempDir, binPath, "checkout", "other")

	fi, err := os.Lstat(symlinkPath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected link_to_target.txt to be a symlink on checked out branch")
	}

	dest, err := os.Readlink(symlinkPath)
	if err != nil || dest != "target.txt" {
		t.Fatalf("expected symlink destination 'target.txt', got %s", dest)
	}
}

func TestDiffDeletedAndAddedFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-diff-deleted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")
	f1 := filepath.Join(tempDir, "to_delete.txt")
	if err := os.WriteFile(f1, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "to_delete.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Add to_delete")

	_ = os.Remove(f1)
	unstagedDiff := runCmd(t, tempDir, binPath, "diff")
	if !strings.Contains(unstagedDiff, "--- a/to_delete.txt") || !strings.Contains(unstagedDiff, "+++ /dev/null") {
		t.Fatalf("expected unstaged diff to show file deleted: %s", unstagedDiff)
	}

	runCmd(t, tempDir, binPath, "rm", "to_delete.txt")
	stagedDiff := runCmd(t, tempDir, binPath, "diff", "--staged")
	if !strings.Contains(stagedDiff, "--- a/to_delete.txt") || !strings.Contains(stagedDiff, "+++ /dev/null") {
		t.Fatalf("expected staged diff to show file deleted: %s", stagedDiff)
	}
}

func TestThreeWayMergeAndConflicts(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-merge-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")
	baseDoc := filepath.Join(tempDir, "shared.txt")
	if err := os.WriteFile(baseDoc, []byte("base line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Initial commit")

	runCmd(t, tempDir, binPath, "branch", "branch-a")
	runCmd(t, tempDir, binPath, "checkout", "branch-a")
	if err := os.WriteFile(baseDoc, []byte("changed in branch-a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Branch A change")

	runCmd(t, tempDir, binPath, "checkout", "main")
	ffMergeOut := runCmd(t, tempDir, binPath, "merge", "branch-a")
	if !strings.Contains(ffMergeOut, "Fast-forward") {
		t.Fatalf("expected fast-forward merge: %s", ffMergeOut)
	}

	runCmd(t, tempDir, binPath, "branch", "branch-b")
	runCmd(t, tempDir, binPath, "checkout", "branch-b")
	bDoc := filepath.Join(tempDir, "file_b.txt")
	if err := os.WriteFile(bDoc, []byte("file b content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "file_b.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Branch B independent change")

	runCmd(t, tempDir, binPath, "checkout", "main")
	aDoc := filepath.Join(tempDir, "file_a.txt")
	if err := os.WriteFile(aDoc, []byte("file a content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "file_a.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Main independent change")

	cleanMergeOut := runCmd(t, tempDir, binPath, "merge", "branch-b")
	if !strings.Contains(cleanMergeOut, "Merged branch-b into main") {
		t.Fatalf("expected clean 3-way merge: %s", cleanMergeOut)
	}

	logOut := runCmd(t, tempDir, binPath, "log")
	if !strings.Contains(logOut, "Merge branch 'branch-b'") {
		t.Fatalf("expected merge commit in history: %s", logOut)
	}

	conflictBranch := "conflict-branch"
	runCmd(t, tempDir, binPath, "branch", conflictBranch)
	runCmd(t, tempDir, binPath, "checkout", conflictBranch)
	if err := os.WriteFile(baseDoc, []byte("conflict from conflict-branch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Conflict commit")

	runCmd(t, tempDir, binPath, "checkout", "main")
	if err := os.WriteFile(baseDoc, []byte("conflict from main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Main conflict commit")

	cmd := exec.Command(binPath, "merge", conflictBranch)
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected merge to fail with conflict, but succeeded: %s", string(out))
	}
	if !strings.Contains(string(out), "Automatic merge failed") {
		t.Fatalf("unexpected conflict error output: %s", string(out))
	}

	data, err := os.ReadFile(baseDoc)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "<<<<<<< HEAD") || !strings.Contains(content, ">>>>>>> conflict-branch") {
		t.Fatalf("expected conflict markers in shared.txt: %s", content)
	}
}
