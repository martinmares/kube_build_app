package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/opsapp/config"
)

func TestPrepareLocalAbsolutizesRootPath(t *testing.T) {
	root := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	prepared, err := Prepare(config.EnvironmentConfig{Name: "test", RootPath: "envs"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Git != nil || !filepath.IsAbs(prepared.Environment.RootPath) || !strings.HasSuffix(prepared.Environment.RootPath, string(filepath.Separator)+"envs") {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestPrepareFromGitChecksOutRevisionAndResolvesRootPath(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	workDir := filepath.Join(t.TempDir(), "work")

	prepared, err := Prepare(config.EnvironmentConfig{Name: "test", Repo: repo, RootPath: "envs", TargetRevision: "HEAD"}, Options{FromGit: true, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Git == nil || len(prepared.Git.ResolvedCommit) != 40 {
		t.Fatalf("prepared git = %#v", prepared.Git)
	}
	if prepared.Environment.RootPath != filepath.Join(prepared.Git.WorktreePath, "envs") {
		t.Fatalf("root path = %q, want checkout envs path", prepared.Environment.RootPath)
	}
}

func TestPrepareFromGitRejectsAbsoluteRootPath(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	_, err := Prepare(config.EnvironmentConfig{Name: "test", Repo: repo, RootPath: t.TempDir(), TargetRevision: "HEAD"}, Options{FromGit: true, WorkDir: filepath.Join(t.TempDir(), "work")})
	if err == nil || !strings.Contains(err.Error(), "root_path must be relative") {
		t.Fatalf("error = %v, want relative root_path error", err)
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
	if err := os.MkdirAll(filepath.Join(repo, "envs"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "envs", "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "envs/README.md")
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
