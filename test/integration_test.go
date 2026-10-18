package test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	cmd := exec.Command("go", "build", "-o", "../minigit", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build minigit: %v\nOutput: %s\n", err, string(out))
		os.Exit(1)
	}
	os.Exit(m.Run())
}

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

func TestChunkLevelThreeWayMergeIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-chunk-integ-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	serviceFile := filepath.Join(tempDir, "service.go")
	initialCode := "package main\n\nfunc start() {\n\tprintln(\"init\")\n}\n\nfunc worker() {\n\tprintln(\"work\")\n}\n\nfunc stop() {\n\tprintln(\"cleanup\")\n}\n"
	if err := os.WriteFile(serviceFile, []byte(initialCode), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "service.go")
	runCmd(t, tempDir, binPath, "commit", "-m", "Initial service")

	runCmd(t, tempDir, binPath, "branch", "side-branch")

	mainCode := "package main\n\nfunc start() {\n\tprintln(\"started\")\n\tprintln(\"init\")\n}\n\nfunc worker() {\n\tprintln(\"work\")\n}\n\nfunc stop() {\n\tprintln(\"cleanup\")\n}\n"
	if err := os.WriteFile(serviceFile, []byte(mainCode), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "service.go")
	runCmd(t, tempDir, binPath, "commit", "-m", "Modify start on main")

	runCmd(t, tempDir, binPath, "checkout", "side-branch")
	sideCode := "package main\n\nfunc start() {\n\tprintln(\"init\")\n}\n\nfunc worker() {\n\tprintln(\"work\")\n}\n\nfunc stop() {\n\tprintln(\"stopped\")\n\tprintln(\"cleanup\")\n}\n"
	if err := os.WriteFile(serviceFile, []byte(sideCode), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "service.go")
	runCmd(t, tempDir, binPath, "commit", "-m", "Modify stop on side-branch")

	runCmd(t, tempDir, binPath, "checkout", "main")
	mergeOut := runCmd(t, tempDir, binPath, "merge", "side-branch")
	if !strings.Contains(mergeOut, "Merged side-branch into main") {
		t.Fatalf("expected clean chunk merge: %s", mergeOut)
	}

	data, err := os.ReadFile(serviceFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "println(\"started\")") || !strings.Contains(content, "println(\"stopped\")") {
		t.Fatalf("expected both edits in merged file: %s", content)
	}
	if strings.Contains(content, "<<<<<<<") || strings.Contains(content, ">>>>>>>") {
		t.Fatalf("unexpected conflict markers in merged file: %s", content)
	}

	fsckOut := runCmd(t, tempDir, "git", "fsck", "--full")
	if strings.Contains(fsckOut, "error") || strings.Contains(fsckOut, "fatal") || strings.Contains(fsckOut, "corrupt") {
		t.Fatalf("git fsck reported corruption after chunk merge: %s", fsckOut)
	}

	statusOut := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(statusOut, "nothing to commit, working tree clean") {
		t.Fatalf("expected clean status after merge: %s", statusOut)
	}
}

func TestMultiStageConflictAndStatusIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-multistage-integ-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	sharedFile := filepath.Join(tempDir, "shared.txt")
	baseText := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	if err := os.WriteFile(sharedFile, []byte(baseText), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Base commit")

	runCmd(t, tempDir, binPath, "branch", "other-branch")

	mainText := "line 1\nline 2\nline 3 from main\nline 4\nline 5\n"
	if err := os.WriteFile(sharedFile, []byte(mainText), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Main edit")

	runCmd(t, tempDir, binPath, "checkout", "other-branch")
	otherText := "line 1\nline 2\nline 3 from other\nline 4\nline 5\n"
	if err := os.WriteFile(sharedFile, []byte(otherText), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Other edit")

	runCmd(t, tempDir, binPath, "checkout", "main")

	mergeCmd := exec.Command(binPath, "merge", "other-branch")
	mergeCmd.Dir = tempDir
	mergeOut, err := mergeCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected merge conflict, got success: %s", string(mergeOut))
	}

	data, err := os.ReadFile(sharedFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "line 1\nline 2\n<<<<<<< HEAD\nline 3 from main\n=======\nline 3 from other\n>>>>>>> other-branch\nline 4\nline 5\n") {
		t.Fatalf("unexpected content with conflict: %s", content)
	}

	lsFilesOut := runCmd(t, tempDir, "git", "ls-files", "--stage")
	if !strings.Contains(lsFilesOut, " 1\tshared.txt") {
		t.Fatalf("missing stage 1 in git ls-files: %s", lsFilesOut)
	}
	if !strings.Contains(lsFilesOut, " 2\tshared.txt") {
		t.Fatalf("missing stage 2 in git ls-files: %s", lsFilesOut)
	}
	if !strings.Contains(lsFilesOut, " 3\tshared.txt") {
		t.Fatalf("missing stage 3 in git ls-files: %s", lsFilesOut)
	}
	if strings.Contains(lsFilesOut, " 0\tshared.txt") {
		t.Fatalf("stage 0 should not exist in conflict: %s", lsFilesOut)
	}

	statusOut := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(statusOut, "You have unmerged paths.") {
		t.Fatalf("status missing unmerged paths notice: %s", statusOut)
	}
	if !strings.Contains(statusOut, "both modified:   shared.txt") {
		t.Fatalf("status missing 'both modified: shared.txt': %s", statusOut)
	}

	commitCmd := exec.Command(binPath, "commit", "-m", "Premature commit")
	commitCmd.Dir = tempDir
	commitOut, err := commitCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("commit should fail with unresolved conflict: %s", string(commitOut))
	}

	resolvedText := "line 1\nline 2\nline 3 merged\nline 4\nline 5\n"
	if err := os.WriteFile(sharedFile, []byte(resolvedText), 0644); err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "add", "shared.txt")

	lsFilesResolved := runCmd(t, tempDir, "git", "ls-files", "--stage")
	if strings.Contains(lsFilesResolved, " 1\tshared.txt") || strings.Contains(lsFilesResolved, " 2\tshared.txt") || strings.Contains(lsFilesResolved, " 3\tshared.txt") {
		t.Fatalf("stages 1,2,3 should be cleared after add: %s", lsFilesResolved)
	}
	if !strings.Contains(lsFilesResolved, " 0\tshared.txt") {
		t.Fatalf("stage 0 should be present after add: %s", lsFilesResolved)
	}

	statusResolved := runCmd(t, tempDir, binPath, "status")
	if strings.Contains(statusResolved, "Unmerged paths") {
		t.Fatalf("status should not show unmerged paths after resolution: %s", statusResolved)
	}
	if !strings.Contains(statusResolved, "modified:   shared.txt") {
		t.Fatalf("status should show modified shared.txt staged: %s", statusResolved)
	}

	runCmd(t, tempDir, binPath, "commit", "-m", "Resolved merge conflict")

	parentsOut := runCmd(t, tempDir, "git", "rev-parse", "HEAD^@")
	parentHashes := strings.Fields(parentsOut)
	if len(parentHashes) != 2 {
		t.Fatalf("expected 2 parents on merge commit from git rev-parse HEAD^@, got %v", parentHashes)
	}

	finalStatus := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(finalStatus, "nothing to commit, working tree clean") {
		t.Fatalf("expected clean status after merge commit: %s", finalStatus)
	}

	fsckFinal := runCmd(t, tempDir, "git", "fsck", "--full")
	if strings.Contains(fsckFinal, "error") || strings.Contains(fsckFinal, "fatal") || strings.Contains(fsckFinal, "corrupt") {
		t.Fatalf("git fsck reported corruption after merge resolution: %s", fsckFinal)
	}
}

func TestMergeAbortIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-integ-abort-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	fPath := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(fPath, []byte("base line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "file.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Base commit")

	runCmd(t, tempDir, binPath, "branch", "feature-x")

	if err := os.WriteFile(fPath, []byte("main line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "file.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Main edit")

	runCmd(t, tempDir, binPath, "checkout", "feature-x")
	if err := os.WriteFile(fPath, []byte("feature line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	extraPath := filepath.Join(tempDir, "extra.txt")
	if err := os.WriteFile(extraPath, []byte("extra data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "file.txt", "extra.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Feature edit")

	runCmd(t, tempDir, binPath, "checkout", "main")

	mergeCmd := exec.Command(binPath, "merge", "feature-x")
	mergeCmd.Dir = tempDir
	mergeOut, err := mergeCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected merge conflict, got success: %s", string(mergeOut))
	}

	statusBefore := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(statusBefore, "You have unmerged paths.") {
		t.Fatalf("expected unmerged paths notice in status: %s", statusBefore)
	}

	abortOut := runCmd(t, tempDir, binPath, "merge", "--abort")
	_ = abortOut

	statusAfter := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(statusAfter, "nothing to commit, working tree clean") {
		t.Fatalf("expected clean status after merge --abort: %s", statusAfter)
	}

	data, err := os.ReadFile(fPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "main line\n" {
		t.Fatalf("expected file.txt reverted to main line, got: %s", string(data))
	}

	if _, err := os.Stat(extraPath); !os.IsNotExist(err) {
		t.Fatalf("expected extra.txt removed after abort")
	}

	fsckOut := runCmd(t, tempDir, "git", "fsck", "--full")
	if strings.Contains(fsckOut, "error") || strings.Contains(fsckOut, "fatal") || strings.Contains(fsckOut, "corrupt") {
		t.Fatalf("git fsck reported corruption after abort: %s", fsckOut)
	}
}

func TestDiffStagedUnmergedIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-integ-diff-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	fPath := filepath.Join(tempDir, "shared.txt")
	if err := os.WriteFile(fPath, []byte("base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Base")

	runCmd(t, tempDir, binPath, "branch", "branch-y")

	if err := os.WriteFile(fPath, []byte("main version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Main edit")

	runCmd(t, tempDir, binPath, "checkout", "branch-y")
	if err := os.WriteFile(fPath, []byte("branch version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "shared.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Branch edit")

	runCmd(t, tempDir, binPath, "checkout", "main")

	mergeCmd := exec.Command(binPath, "merge", "branch-y")
	mergeCmd.Dir = tempDir
	_, _ = mergeCmd.CombinedOutput()

	diffOut := runCmd(t, tempDir, binPath, "diff", "--staged")
	if strings.Contains(diffOut, "+++ /dev/null") || strings.Contains(diffOut, "--- a/shared.txt") {
		t.Fatalf("diff --staged should not report unmerged shared.txt as deleted: %s", diffOut)
	}
}

func TestCloneAndIgnoreIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-clone-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	runCmd(t, srcDir, binPath, "init")

	if err := os.WriteFile(filepath.Join(srcDir, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# Cloned Project\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "debug.log"), []byte("ignore me\n"), 0644); err != nil {
		t.Fatal(err)
	}

	stOut := runCmd(t, srcDir, binPath, "status")
	if strings.Contains(stOut, "debug.log") {
		t.Fatalf("debug.log should be ignored, got status:\n%s", stOut)
	}

	runCmd(t, srcDir, binPath, "add", ".")
	runCmd(t, srcDir, binPath, "commit", "-m", "Initial commit in src")

	runCmd(t, srcDir, binPath, "add", "-f", "debug.log")
	stOut = runCmd(t, srcDir, binPath, "status")
	if !strings.Contains(stOut, "debug.log") {
		t.Fatalf("force add failed to stage ignored file:\n%s", stOut)
	}
	runCmd(t, srcDir, binPath, "commit", "-m", "Force add log")

	dstDir := filepath.Join(tempDir, "dst")
	cloneOut := runCmd(t, tempDir, binPath, "clone", "src", "dst")
	if !strings.Contains(cloneOut, "Cloning into") {
		t.Fatalf("clone output missing confirmation: %s", cloneOut)
	}

	clonedReadme := filepath.Join(dstDir, "README.md")
	data, err := os.ReadFile(clonedReadme)
	if err != nil || string(data) != "# Cloned Project\n" {
		t.Fatalf("cloned README content incorrect: %v, data: %s", err, string(data))
	}

	logOut := runCmd(t, dstDir, binPath, "log", "--oneline")
	if !strings.Contains(logOut, "Force add log") || !strings.Contains(logOut, "Initial commit in src") {
		t.Fatalf("cloned repository missing commit history: %s", logOut)
	}
}

func TestBinaryDiffAndMergeIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-bin-integ-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	binFile := filepath.Join(tempDir, "data.bin")
	baseContent := []byte("\x00\x01\x02\x03\x04base_binary")
	if err := os.WriteFile(binFile, baseContent, 0644); err != nil {
		t.Fatal(err)
	}

	diffInitial := runCmd(t, tempDir, binPath, "diff")
	_ = diffInitial

	runCmd(t, tempDir, binPath, "add", "data.bin")

	diffStagedNew := runCmd(t, tempDir, binPath, "diff", "--staged")
	if !strings.Contains(diffStagedNew, "Binary files") || !strings.Contains(diffStagedNew, "differ") {
		t.Fatalf("expected binary diff for newly added binary file: %s", diffStagedNew)
	}

	runCmd(t, tempDir, binPath, "commit", "-m", "Base binary commit")

	mainContent := []byte("\x00\x01\x02\x03\x04main_binary")
	if err := os.WriteFile(binFile, mainContent, 0644); err != nil {
		t.Fatal(err)
	}

	diffUnstaged := runCmd(t, tempDir, binPath, "diff")
	if !strings.Contains(diffUnstaged, "Binary files a/data.bin and b/data.bin differ") {
		t.Fatalf("expected unstaged binary diff: %s", diffUnstaged)
	}

	runCmd(t, tempDir, binPath, "branch", "feat-bin")

	runCmd(t, tempDir, binPath, "add", "data.bin")
	runCmd(t, tempDir, binPath, "commit", "-m", "Main edit binary")

	runCmd(t, tempDir, binPath, "checkout", "feat-bin")
	featContent := []byte("\x00\x01\x02\x03\x04feat_binary")
	if err := os.WriteFile(binFile, featContent, 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "data.bin")
	runCmd(t, tempDir, binPath, "commit", "-m", "Feat edit binary")

	runCmd(t, tempDir, binPath, "checkout", "main")

	mergeCmd := exec.Command(binPath, "merge", "feat-bin")
	mergeCmd.Dir = tempDir
	mergeOut, err := mergeCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected merge conflict for binary files: %s", string(mergeOut))
	}

	statusOut := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(statusOut, "both modified:   data.bin") {
		t.Fatalf("expected status to show both modified for data.bin: %s", statusOut)
	}

	curContent, err := os.ReadFile(binFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(curContent, mainContent) {
		t.Fatalf("binary file should not contain text conflict markers")
	}

	lsOut := runCmd(t, tempDir, "git", "ls-files", "--stage")
	if !strings.Contains(lsOut, " 1\tdata.bin") || !strings.Contains(lsOut, " 2\tdata.bin") || !strings.Contains(lsOut, " 3\tdata.bin") {
		t.Fatalf("expected stages 1, 2, 3 in index for binary conflict: %s", lsOut)
	}

	runCmd(t, tempDir, binPath, "merge", "--abort")
	cleanStatus := runCmd(t, tempDir, binPath, "status")
	if !strings.Contains(cleanStatus, "nothing to commit, working tree clean") {
		t.Fatalf("expected clean status after merge abort: %s", cleanStatus)
	}
}

func TestDiffRenameDetectionIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-rename-integ-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	f1 := filepath.Join(tempDir, "old_name.txt")
	content := "alpha line\nbeta line\ngamma line\ndelta line\n"
	if err := os.WriteFile(f1, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "old_name.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Add old_name")

	runCmd(t, tempDir, binPath, "rm", "old_name.txt")
	f2 := filepath.Join(tempDir, "new_name.txt")
	if err := os.WriteFile(f2, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "new_name.txt")

	stagedDiff := runCmd(t, tempDir, binPath, "diff", "--staged")
	if !strings.Contains(stagedDiff, "similarity index 100%") {
		t.Fatalf("missing similarity index 100%%: %s", stagedDiff)
	}
	if !strings.Contains(stagedDiff, "rename from old_name.txt") || !strings.Contains(stagedDiff, "rename to new_name.txt") {
		t.Fatalf("missing rename from/to headers: %s", stagedDiff)
	}

	runCmd(t, tempDir, binPath, "commit", "-m", "Renamed file")

	partOld := filepath.Join(tempDir, "module_v1.txt")
	partContentOld := "func A() {}\nfunc B() {}\nfunc C() {}\nfunc D() {}\n"
	if err := os.WriteFile(partOld, []byte(partContentOld), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "module_v1.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Add module v1")

	runCmd(t, tempDir, binPath, "rm", "module_v1.txt")
	partNew := filepath.Join(tempDir, "module_v2.txt")
	partContentNew := "func A() {}\nfunc B() {}\nfunc C() {}\nfunc D_modified() {}\n"
	if err := os.WriteFile(partNew, []byte(partContentNew), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "module_v2.txt")

	partDiff := runCmd(t, tempDir, binPath, "diff", "--staged")
	if !strings.Contains(partDiff, "similarity index 75%") {
		t.Fatalf("expected similarity index 75%%: %s", partDiff)
	}
	if !strings.Contains(partDiff, "rename from module_v1.txt") || !strings.Contains(partDiff, "rename to module_v2.txt") {
		t.Fatalf("missing partial rename headers: %s", partDiff)
	}
	if !strings.Contains(partDiff, "--- a/module_v1.txt") || !strings.Contains(partDiff, "+++ b/module_v2.txt") {
		t.Fatalf("missing unified diff header in partial rename: %s", partDiff)
	}
}

func TestNestedGitignoreIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-ignore-integ-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, tempDir, binPath, "init")

	rootIgnore := "*.log\n/root_only.txt\nbuild/\ntemp\n!keep.log\n"
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(rootIgnore), 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(tempDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	subIgnore := "sub_ignored.txt\n!override.log\n/sub_root.dat\n"
	if err := os.WriteFile(filepath.Join(subDir, ".gitignore"), []byte(subIgnore), 0644); err != nil {
		t.Fatal(err)
	}

	deepDir := filepath.Join(subDir, "deep")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatal(err)
	}

	testFiles := map[string]bool{
		".gitignore":            false,
		"keep.log":              false,
		"normal.txt":            false,
		"test.log":              true,
		"root_only.txt":         true,
		"sub/.gitignore":        false,
		"sub/normal.txt":        false,
		"sub/override.log":      false,
		"sub/root_only.txt":     false,
		"sub/test.log":          true,
		"sub/sub_ignored.txt":   true,
		"sub/sub_root.dat":      true,
		"sub/deep/sub_root.dat": false,
		"temp":                  true,
		"sub/temp":              true,
		"build/out.bin":         true,
		"build/nested/deep.bin": true,
	}

	for p := range testFiles {
		if p == ".gitignore" || p == "sub/.gitignore" {
			continue
		}
		full := filepath.Join(tempDir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	statusOut := runCmd(t, tempDir, binPath, "status")
	for p, shouldIgnore := range testFiles {
		if shouldIgnore {
			if strings.Contains(statusOut, "\t"+p+"\n") || strings.Contains(statusOut, " "+p+"\n") {
				t.Fatalf("file %s should be ignored, but appeared in status: %s", p, statusOut)
			}
		} else {
			if !strings.Contains(statusOut, p) {
				t.Fatalf("file %s should NOT be ignored, but missing from status: %s", p, statusOut)
			}
		}
	}

	runCmd(t, tempDir, binPath, "add", ".")
	stagedStatus := runCmd(t, tempDir, binPath, "status")
	for p, shouldIgnore := range testFiles {
		if shouldIgnore {
			if strings.Contains(stagedStatus, "new file:   "+p+"\n") {
				t.Fatalf("ignored file %s was staged by add .: %s", p, stagedStatus)
			}
		} else {
			if !strings.Contains(stagedStatus, "new file:   "+p+"\n") {
				t.Fatalf("non-ignored file %s was NOT staged by add .: %s", p, stagedStatus)
			}
		}
	}

	gitStatus := runCmd(t, tempDir, "git", "status", "--porcelain")
	porcelainFiles := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(gitStatus), "\n") {
		if len(line) > 3 {
			porcelainFiles[strings.TrimSpace(line[3:])] = true
		}
	}
	for p, shouldIgnore := range testFiles {
		if shouldIgnore {
			if porcelainFiles[p] {
				t.Fatalf("standard git disagreed; ignored file %s in porcelain: %s", p, gitStatus)
			}
		} else {
			if !porcelainFiles[p] {
				t.Fatalf("standard git disagreed; non-ignored file %s not in porcelain: %s", p, gitStatus)
			}
		}
	}
}

func TestCrossCompatibilityCloneStandardGit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-clone-compat-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	gitSrc := filepath.Join(tempDir, "git-src")
	if err := os.MkdirAll(gitSrc, 0755); err != nil {
		t.Fatal(err)
	}
	runCmd(t, gitSrc, "git", "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(gitSrc, "file1.txt"), []byte("standard git commit 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, gitSrc, "git", "add", "file1.txt")
	runCmd(t, gitSrc, "git", "-c", "user.name=Author", "-c", "user.email=a@a.com", "commit", "-m", "First git commit")
	runCmd(t, gitSrc, "git", "tag", "v1.0.0")

	runCmd(t, gitSrc, "git", "branch", "git-branch")
	runCmd(t, gitSrc, "git", "checkout", "git-branch")
	if err := os.WriteFile(filepath.Join(gitSrc, "branch_file.txt"), []byte("branch data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, gitSrc, "git", "add", "branch_file.txt")
	runCmd(t, gitSrc, "git", "-c", "user.name=Author", "-c", "user.email=a@a.com", "commit", "-m", "Branch git commit")

	runCmd(t, gitSrc, "git", "checkout", "main")
	runCmd(t, gitSrc, "git", "gc")

	minigitDst := filepath.Join(tempDir, "minigit-dst")
	runCmd(t, tempDir, binPath, "clone", "git-src", "minigit-dst")

	fsckOut := runCmd(t, minigitDst, "git", "fsck", "--full")
	if strings.Contains(fsckOut, "error") || strings.Contains(fsckOut, "fatal") || strings.Contains(fsckOut, "corrupt") {
		t.Fatalf("git fsck failed on minigit clone: %s", fsckOut)
	}

	minigitStatus := runCmd(t, minigitDst, binPath, "status")
	if !strings.Contains(minigitStatus, "nothing to commit, working tree clean") {
		t.Fatalf("expected clean working tree in minigit clone: %s", minigitStatus)
	}

	minigitTags := runCmd(t, minigitDst, binPath, "tag")
	if !strings.Contains(minigitTags, "v1.0.0") {
		t.Fatalf("minigit clone missing tag v1.0.0: %s", minigitTags)
	}

	minigitBranches := runCmd(t, minigitDst, binPath, "branch")
	if !strings.Contains(minigitBranches, "git-branch") {
		t.Fatalf("minigit clone missing branch git-branch: %s", minigitBranches)
	}

	minigitSrc := filepath.Join(tempDir, "minigit-src")
	runCmd(t, tempDir, binPath, "init", "minigit-src")
	if err := os.WriteFile(filepath.Join(minigitSrc, "hello.txt"), []byte("from minigit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, minigitSrc, binPath, "add", "hello.txt")
	runCmd(t, minigitSrc, binPath, "commit", "-m", "Commit from minigit for cloning")

	gitDst := filepath.Join(tempDir, "git-dst")
	runCmd(t, tempDir, "git", "clone", "minigit-src", "git-dst")

	gitCloneFsck := runCmd(t, gitDst, "git", "fsck", "--full")
	if strings.Contains(gitCloneFsck, "error") || strings.Contains(gitCloneFsck, "fatal") || strings.Contains(gitCloneFsck, "corrupt") {
		t.Fatalf("git fsck failed on git clone of minigit repo: %s", gitCloneFsck)
	}

	clonedData, err := os.ReadFile(filepath.Join(gitDst, "hello.txt"))
	if err != nil || string(clonedData) != "from minigit\n" {
		t.Fatalf("standard git clone could not checkout files from minigit repo: %v, data: %s", err, string(clonedData))
	}
}

func TestRevisionResolutionAndCLIDiff(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "minigit-integ-rev-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath, err := filepath.Abs("../minigit")
	if err != nil {
		t.Fatal(err)
	}

	verOut := runCmd(t, tempDir, binPath, "version")
	if !strings.Contains(verOut, "minigit version") {
		t.Fatalf("expected version output, got: %s", verOut)
	}

	runCmd(t, tempDir, binPath, "init")
	f1 := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(f1, []byte("first line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "sample.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "First commit")

	runCmd(t, tempDir, binPath, "branch", "feat", "HEAD")
	runCmd(t, tempDir, binPath, "tag", "v1.0", "HEAD")

	if err := os.WriteFile(f1, []byte("second line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, tempDir, binPath, "add", "sample.txt")
	runCmd(t, tempDir, binPath, "commit", "-m", "Second commit")

	logMain := runCmd(t, tempDir, binPath, "log", "main")
	if !strings.Contains(logMain, "First commit") || !strings.Contains(logMain, "Second commit") {
		t.Fatalf("unexpected log from main: %s", logMain)
	}

	logHead := runCmd(t, tempDir, binPath, "log", "HEAD", "--oneline", "-n", "1")
	if !strings.Contains(logHead, "Second commit") {
		t.Fatalf("unexpected log from HEAD: %s", logHead)
	}

	diffCommits := runCmd(t, tempDir, binPath, "diff", "HEAD~1", "HEAD")
	if !strings.Contains(diffCommits, "-first line") || !strings.Contains(diffCommits, "+second line") {
		t.Fatalf("unexpected diff between commits: %s", diffCommits)
	}

	diffBranches := runCmd(t, tempDir, binPath, "diff", "feat", "main")
	if !strings.Contains(diffBranches, "-first line") || !strings.Contains(diffBranches, "+second line") {
		t.Fatalf("unexpected diff between branches: %s", diffBranches)
	}

	if err := os.WriteFile(f1, []byte("worktree modification\n"), 0644); err != nil {
		t.Fatal(err)
	}
	diffHead := runCmd(t, tempDir, binPath, "diff", "HEAD")
	if !strings.Contains(diffHead, "-second line") || !strings.Contains(diffHead, "+worktree modification") {
		t.Fatalf("unexpected diff against HEAD: %s", diffHead)
	}

	catHead := runCmd(t, tempDir, binPath, "cat-file", "-t", "HEAD")
	if strings.TrimSpace(catHead) != "commit" {
		t.Fatalf("expected cat-file -t HEAD to be commit, got: %s", catHead)
	}

	runCmd(t, tempDir, binPath, "checkout", "v1.0")
	checkoutData, err := os.ReadFile(f1)
	if err != nil || string(checkoutData) != "first line\n" {
		t.Fatalf("expected checked out file at v1.0 to have first line, got: %s", string(checkoutData))
	}
}
