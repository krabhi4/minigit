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
