package appinfo

import "testing"

func TestForReturnsSelectedAppNameAndBuildInfo(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalDate := Date
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	})

	Version = "1.2.3"
	Commit = "abc123"
	Date = "2026-05-12"

	info := For(BuildAppName)
	if info.Name != BuildAppName {
		t.Fatalf("Name = %q, want %q", info.Name, BuildAppName)
	}
	if info.Version != "1.2.3" {
		t.Fatalf("Version = %q, want %q", info.Version, "1.2.3")
	}
	if info.Commit != "abc123" {
		t.Fatalf("Commit = %q, want %q", info.Commit, "abc123")
	}
	if info.Date != "2026-05-12" {
		t.Fatalf("Date = %q, want %q", info.Date, "2026-05-12")
	}
}
