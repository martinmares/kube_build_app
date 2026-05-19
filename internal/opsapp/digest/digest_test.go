package digest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryDigestIsStable(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeFile(t, filepath.Join(rootA, "b.yml"), "b")
	writeFile(t, filepath.Join(rootA, "nested", "a.yml"), "a")
	writeFile(t, filepath.Join(rootB, "nested", "a.yml"), "a")
	writeFile(t, filepath.Join(rootB, "b.yml"), "b")

	a, err := Directory(rootA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Directory(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest {
		t.Fatalf("digest A = %s, digest B = %s", a.Digest, b.Digest)
	}
	if a.Files != 2 || a.Bytes != 2 {
		t.Fatalf("result = %#v, want 2 files and 2 bytes", a)
	}
	if len(a.Paths) != 2 || a.Paths[0] != "b.yml" || a.Paths[1] != "nested/a.yml" {
		t.Fatalf("paths = %#v, want sorted slash paths", a.Paths)
	}
}

func TestDirectoryDigestChangesWithPath(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeFile(t, filepath.Join(rootA, "a.yml"), "same")
	writeFile(t, filepath.Join(rootB, "b.yml"), "same")

	a, err := Directory(rootA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Directory(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest == b.Digest {
		t.Fatalf("digest did not include path: %s", a.Digest)
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
