package state

import (
	"path/filepath"
	"testing"
)

func TestStoreLoadMissingReturnsEmpty(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "missing.json"))
	file, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Environments) != 0 {
		t.Fatalf("environments = %#v, want empty", file.Environments)
	}
}

func TestStoreSaveAndLoadEnvironment(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	want := EnvironmentState{AppliedRevision: "main", AppliedCommit: "abc123", AppliedDigest: "sha256:deadbeef", SnapshotPath: "/tmp/snapshots/test", AppliedBy: "test"}
	if err := store.SaveEnvironment("test", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Environment("test")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("environment state not found")
	}
	if got.AppliedRevision != want.AppliedRevision || got.AppliedCommit != want.AppliedCommit || got.AppliedDigest != want.AppliedDigest || got.SnapshotPath != want.SnapshotPath || got.AppliedBy != want.AppliedBy {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
}

func TestStoreSnapshotDir(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	path, err := store.SnapshotDir("test")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "test" || filepath.Base(filepath.Dir(path)) != "snapshots" {
		t.Fatalf("snapshot dir = %q, want snapshots/test", path)
	}
}
