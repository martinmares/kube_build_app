package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoriesDetectsAddedModifiedDeleted(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	writeFile(t, filepath.Join(oldRoot, "same.yml"), "same\n")
	writeFile(t, filepath.Join(newRoot, "same.yml"), "same\n")
	writeFile(t, filepath.Join(oldRoot, "modified.yml"), "old\n")
	writeFile(t, filepath.Join(newRoot, "modified.yml"), "new\n")
	writeFile(t, filepath.Join(oldRoot, "deleted.yml"), "gone\n")
	writeFile(t, filepath.Join(newRoot, "added.yml"), "fresh\n")

	result, err := Directories(oldRoot, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || len(result.Files) != 3 {
		t.Fatalf("result = %#v, want 3 changed files", result)
	}
	statuses := map[string]string{}
	for _, file := range result.Files {
		statuses[file.Path] = file.Status
		if len(file.Unified) == 0 {
			t.Fatalf("file %s has empty unified diff", file.Path)
		}
	}
	if statuses["added.yml"] != "added" || statuses["deleted.yml"] != "deleted" || statuses["modified.yml"] != "modified" {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func TestDirectoriesUsesLineDiffForModifiedFiles(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	writeFile(t, filepath.Join(oldRoot, "app.yml"), "line 1\nline 2\nline 3\nline 4\nline 5\n")
	writeFile(t, filepath.Join(newRoot, "app.yml"), "line 1\nline 2 changed\nline 3\nline 4\nline 5\n")

	result, err := Directories(oldRoot, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("files = %#v, want one modified file", result.Files)
	}
	unified := result.Files[0].Unified
	joined := strings.Join(unified, "\n")
	if !strings.Contains(joined, "-line 2") || !strings.Contains(joined, "+line 2 changed") {
		t.Fatalf("unified diff = %q, want changed lines", joined)
	}
	if strings.Count(joined, "-line ") > 1 || strings.Count(joined, "+line ") > 1 {
		t.Fatalf("unified diff = %q, appears to include whole file replacement", joined)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
