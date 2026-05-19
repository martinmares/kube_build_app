package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutResolvesCommitAndCreatesWorktree(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	workDir := filepath.Join(t.TempDir(), "work")

	result, err := Checkout(CheckoutOptions{Repo: repo, Revision: "HEAD", WorkDir: workDir, Name: "test/env"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Repo != repo || result.Revision != "HEAD" || len(result.ResolvedCommit) != 40 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.HasPrefix(result.WorktreePath, workDir) {
		t.Fatalf("worktree path %q does not use work dir %q", result.WorktreePath, workDir)
	}
	content, err := os.ReadFile(filepath.Join(result.WorktreePath, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("checked out content = %q", content)
	}
}

func TestCheckoutUsesSafeFallbackRevision(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	result, err := Checkout(CheckoutOptions{Repo: repo, WorkDir: filepath.Join(t.TempDir(), "work"), Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != "HEAD" || !strings.HasSuffix(result.WorktreePath, string(filepath.Separator)+"repo") {
		t.Fatalf("result = %#v", result)
	}
}

func TestIsLocalRepoPath(t *testing.T) {
	repo := initGitRepo(t)
	if !isLocalRepoPath(repo) {
		t.Fatalf("isLocalRepoPath(%q) = false, want true", repo)
	}
	for _, remote := range []string{"https://example.invalid/repo.git", "ssh://git@example.invalid/repo.git", "git@example.invalid:repo.git"} {
		if isLocalRepoPath(remote) {
			t.Fatalf("isLocalRepoPath(%q) = true, want false", remote)
		}
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
