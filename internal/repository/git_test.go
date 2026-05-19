package repository

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGitStatusFilesKeepsFirstPathCharacter(t *testing.T) {
	files := parseGitStatusFiles("M test/apps/api.yml\n M test/apps/worker.yml\n?? test/assets.secured.json\n", "")
	if len(files) != 3 {
		t.Fatalf("files len = %d, want 3: %#v", len(files), files)
	}
	want := map[string]string{
		"test/apps/api.yml":        "M",
		"test/apps/worker.yml":     "M",
		"test/assets.secured.json": "??",
	}
	for _, file := range files {
		if want[file.Path] != file.Code {
			t.Fatalf("unexpected parsed file: %#v, all files: %#v", file, files)
		}
		delete(want, file.Path)
	}
	if len(want) > 0 {
		t.Fatalf("missing parsed files: %#v, all files: %#v", want, files)
	}
}

func TestGitRestoreRestoresTrackedFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: changed\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.GitRestore("test/apps/api.yml", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Restored || result.Status.Dirty {
		t.Fatalf("restore result = %#v, want restored clean status", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "test", "apps", "api.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "name: api\n" {
		t.Fatalf("content = %q, want original", content)
	}
}

func TestGitRestoreUntrackedRequiresExplicitDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	path := filepath.Join(root, "test", "assets.secured.json")
	writeFile(t, path, "{}\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.GitRestore("test/assets.secured.json", false); err == nil || !strings.Contains(err.Error(), "delete_untracked=true") {
		t.Fatalf("restore untracked without delete error = %v, want explicit delete error", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("untracked file missing before explicit delete: %v", err)
	}
	result, err := repo.GitRestore("test/assets.secured.json", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Restored {
		t.Fatalf("restore result = %#v, want restored=true", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("untracked file still exists or unexpected stat error: %v", err)
	}
}

func TestGitCommitSelectedCommitsOnlySelectedPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	writeFile(t, filepath.Join(root, "test", "apps", "worker.yml"), "name: worker\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: changed-api\n")
	writeFile(t, filepath.Join(root, "test", "apps", "worker.yml"), "name: changed-worker\n")
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), "{\"environment\":{}}\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.GitCommitSelected([]string{"test/apps/api.yml", "test/env.unsecured.json"}, "accept selected")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || result.Commit == "" {
		t.Fatalf("commit result = %#v, want committed with hash", result)
	}
	status := repo.GitStatus()
	if len(status.Files) != 1 || status.Files[0].Path != "test/apps/worker.yml" {
		t.Fatalf("status files after selected commit = %#v, want only worker dirty", status.Files)
	}
	show := gitShow(t, root, "--name-only", "--format=", "HEAD")
	if !strings.Contains(show, "test/apps/api.yml") || !strings.Contains(show, "test/env.unsecured.json") || strings.Contains(show, "test/apps/worker.yml") {
		t.Fatalf("selected commit files unexpected:\n%s", show)
	}
}

func TestGitCommitSelectedRejectsCleanPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.GitCommitSelected([]string{"test/apps/api.yml"}, "should fail"); err == nil || !strings.Contains(err.Error(), "not dirty") {
		t.Fatalf("commit clean path error = %v, want not dirty", err)
	}
}

func gitShow(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "show"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}
