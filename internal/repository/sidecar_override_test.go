package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedSidecarEnvOverrideAndResetAreMinimal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), `sidecar_definitions:
  - name: cgroup-runtime-exporter
    image: registry/exporter:1
    startup:
      command: ["/exporter"]
    envs:
      - name: CGROUP_EXPORTER_TARGET_PID_REGEXP
        value: java
    resources:
      cpu: {from: 2m, to: 10m}
`)
	appPath := filepath.Join(root, "dev", "apps", "ui.yml")
	original := `name: ui
sidecar_ref_names:
  - cgroup-runtime-exporter
containers:
  - name: ui
    image: nginx
`
	writeFile(t, appPath, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppSidecarEnvOverride("dev", "ui.yml", "cgroup-runtime-exporter", "CGROUP_EXPORTER_TARGET_PID_REGEXP")
	if err != nil {
		t.Fatal(err)
	}
	if !current.SharedDefinition || current.LocalPatch || current.Local != nil {
		t.Fatalf("unexpected initial override state: %#v", current)
	}
	updated, err := repo.UpdateAppSidecarEnvOverride("dev", "ui.yml", "cgroup-runtime-exporter", "CGROUP_EXPORTER_TARGET_PID_REGEXP", SidecarEnvOverrideUpdate{Action: "set", Value: `(^|/)nginx(\s|$)`}, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Local == nil || updated.Local.Value == nil || *updated.Local.Value != `(^|/)nginx(\s|$)` {
		t.Fatalf("unexpected local override: %#v", updated)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "  - name: cgroup-runtime-exporter\n    envs:\n      - name: CGROUP_EXPORTER_TARGET_PID_REGEXP") {
		t.Fatalf("minimal sidecar patch missing:\n%s", content)
	}
	for _, inherited := range []string{"registry/exporter:1", "command:", "resources:"} {
		if strings.Contains(string(content), inherited) {
			t.Fatalf("inherited field %q was materialized:\n%s", inherited, content)
		}
	}
	reset, err := repo.UpdateAppSidecarEnvOverride("dev", "ui.yml", "cgroup-runtime-exporter", "CGROUP_EXPORTER_TARGET_PID_REGEXP", SidecarEnvOverrideUpdate{Action: "reset"}, updated.ContentHash, updated.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if reset.LocalPatch || reset.Local != nil {
		t.Fatalf("override was not reset: %#v", reset)
	}
	content, err = os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("reset did not restore original source:\n%s", content)
	}
}

func TestAppOnlySidecarEnvResetKeepsSidecar(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), "sidecar_definitions: []\n")
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	writeFile(t, appPath, `name: api
sidecars:
  - name: gateway
    image: gateway:1
    envs:
      - name: MODE
        value: strict
containers:
  - name: api
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppSidecarEnvOverride("dev", "api.yml", "gateway", "MODE")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdateAppSidecarEnvOverride("dev", "api.yml", "gateway", "MODE", SidecarEnvOverrideUpdate{Action: "reset"}, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LocalPatch != true || updated.Local != nil {
		t.Fatalf("app-only sidecar state = %#v", updated)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "  - name: gateway\n    image: gateway:1") || strings.Contains(string(content), "name: MODE") {
		t.Fatalf("app-only sidecar was damaged:\n%s", content)
	}
}

func TestSharedSidecarEnvResetKeepsOtherLocalPatchFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), `sidecar_definitions:
  - name: exporter
    image: exporter:1
    envs:
      - name: MODE
        value: java
`)
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	writeFile(t, appPath, `name: api
sidecar_ref_names:
  - exporter
sidecars:
  - name: exporter
    resources:
      cpu: {from: 5m, to: 20m}
    envs:
      - name: MODE
        value: nginx
containers:
  - name: api
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppSidecarEnvOverride("dev", "api.yml", "exporter", "MODE")
	if err != nil {
		t.Fatal(err)
	}
	reset, err := repo.UpdateAppSidecarEnvOverride("dev", "api.yml", "exporter", "MODE", SidecarEnvOverrideUpdate{Action: "reset"}, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if !reset.LocalPatch || reset.Local != nil {
		t.Fatalf("unexpected reset state: %#v", reset)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "  - name: exporter\n    resources:\n") || strings.Contains(string(content), "name: MODE") {
		t.Fatalf("reset damaged the remaining local patch:\n%s", content)
	}
}

func TestSharedSidecarEnvOverrideRejectsStaleDefaults(t *testing.T) {
	root := t.TempDir()
	defaultsPath := filepath.Join(root, "dev", "apps", "_defaults.yml")
	writeFile(t, defaultsPath, "sidecar_definitions:\n  - name: exporter\n    image: exporter:1\n")
	writeFile(t, filepath.Join(root, "dev", "apps", "api.yml"), "name: api\nsidecar_ref_names:\n  - exporter\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppSidecarEnvOverride("dev", "api.yml", "exporter", "MODE")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, defaultsPath, "sidecar_definitions:\n  - name: exporter\n    image: exporter:2\n")
	_, err = repo.UpdateAppSidecarEnvOverride("dev", "api.yml", "exporter", "MODE", SidecarEnvOverrideUpdate{Action: "set", Value: "new"}, current.ContentHash, current.DefaultsHash)
	if !IsConflictError(err) {
		t.Fatalf("stale defaults error = %v, want conflict", err)
	}
}
