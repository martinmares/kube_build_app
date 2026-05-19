package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeFile(t, path, `environments:
  - name: test
    namespace: tsm-test
    root_path: /repo/environments
    branch: main
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.HTTP.Listen != ":8080" {
		t.Fatalf("listen = %q, want :8080", cfg.Server.HTTP.Listen)
	}
	if len(cfg.Environments) != 1 {
		t.Fatalf("env count = %d, want 1", len(cfg.Environments))
	}
	env := cfg.Environments[0]
	if env.EnvName != "test" || env.TargetRevision != "main" {
		t.Fatalf("env defaults = %#v, want env_name test and target_revision main", env)
	}
}

func TestLoadFileRejectsDuplicateEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeFile(t, path, `environments:
  - name: test
  - name: test
`)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile error = nil, want duplicate environment error")
	}
}

func TestLoadFileRejectsMissingEnvironments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeFile(t, path, `server:
  http:
    listen: ":8185"
`)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile error = nil, want missing environments error")
	}
}

func TestLoadFileRejectsNestedServerEnvironments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeFile(t, path, `server:
  http:
    listen: ":8185"
  environments:
    - name: test
      namespace: kube-ops-test
      root_path: test
`)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile error = nil, want nested environments error")
	}
}

func TestLoadFileRejectsMissingNamespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeFile(t, path, `environments:
  - name: test
    root_path: test
`)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile error = nil, want missing namespace error")
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
