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

func TestSharedSidecarResourcesOverrideAndResetAreMinimal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), `sidecar_definitions:
  - name: exporter
    image: exporter:1
    startup:
      command: ["/exporter"]
    resources:
      cpu: {from: 2m, to: 10m}
      memory: {from: 8Mi, to: 32Mi}
`)
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	original := "name: api\nsidecar_ref_names:\n  - exporter\ncontainers:\n  - name: api\n"
	writeFile(t, appPath, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppSidecarResourcesOverride("dev", "api.yml", "exporter")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdateAppSidecarResourcesOverride("dev", "api.yml", "exporter", SidecarResourcesOverrideUpdate{
		Action: "set", Resources: ResourceUpdate{
			CPURequest: "5m", CPULimit: "20m", MemoryRequest: "16Mi", MemoryLimit: "64Mi",
			EphemeralStorageRequest: "32Mi", EphemeralStorageLimit: "128Mi",
		},
	}, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Local == nil || updated.Local.CPURequest == nil || *updated.Local.CPURequest != "5m" {
		t.Fatalf("unexpected resources override: %#v", updated)
	}
	if updated.Local.EphemeralStorageLimit == nil || *updated.Local.EphemeralStorageLimit != "128Mi" {
		t.Fatalf("ephemeral storage override missing: %#v", updated)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "  - name: exporter\n    resources:\n") || strings.Contains(string(content), "image:") || strings.Contains(string(content), "startup:") {
		t.Fatalf("resources override materialized inherited fields:\n%s", content)
	}
	if !strings.Contains(string(content), "ephemeral-storage:\n        requests: \"32Mi\"\n        limits: \"128Mi\"") {
		t.Fatalf("ephemeral storage override missing from source:\n%s", content)
	}
	reset, err := repo.UpdateAppSidecarResourcesOverride("dev", "api.yml", "exporter", SidecarResourcesOverrideUpdate{Action: "reset"}, updated.ContentHash, updated.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if reset.LocalPatch || reset.Local != nil {
		t.Fatalf("resources override was not reset: %#v", reset)
	}
	content, err = os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("resources reset did not restore original source:\n%s", content)
	}
}

func TestAppSidecarResourcesOverrideRejectsUnknownResourceWithoutChangingSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), "vars: []\n")
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	original := `name: api
sidecars:
  - name: exporter
    resources:
      gpu:
        requests: "1"
containers:
  - name: api
`
	writeFile(t, appPath, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.AppSidecarResourcesOverride("dev", "api.yml", "exporter"); err == nil || !strings.Contains(err.Error(), "gpu") {
		t.Fatalf("unknown resource error = %v", err)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("failed read changed source:\n%s", content)
	}
}

func TestSharedSidecarResourcesResetKeepsEnvPatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), "sidecar_definitions:\n  - name: exporter\n    image: exporter:1\n")
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	writeFile(t, appPath, `name: api
sidecar_ref_names:
  - exporter
sidecars:
  - name: exporter
    resources:
      cpu: {requests: 5m}
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
	current, err := repo.AppSidecarResourcesOverride("dev", "api.yml", "exporter")
	if err != nil {
		t.Fatal(err)
	}
	reset, err := repo.UpdateAppSidecarResourcesOverride("dev", "api.yml", "exporter", SidecarResourcesOverrideUpdate{Action: "reset"}, current.ContentHash, current.DefaultsHash)
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
	if strings.Contains(string(content), "resources:") || !strings.Contains(string(content), "name: MODE") {
		t.Fatalf("resources reset damaged env patch:\n%s", content)
	}
}

func TestSharedSidecarStartupOverrideAndResetAreMinimal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), `sidecar_definitions:
  - name: proxy
    image: proxy:1
    startup:
      command: ["proxy"]
      arguments: ["--listen", "8080"]
    resources:
      cpu: {from: 2m, to: 10m}
`)
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	original := "name: api\nsidecar_ref_names:\n  - proxy\ncontainers:\n  - name: api\n"
	writeFile(t, appPath, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppSidecarStartupOverride("dev", "api.yml", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdateAppSidecarStartupOverride("dev", "api.yml", "proxy", SidecarStartupOverrideUpdate{
		Action: "set", Startup: SidecarStartupModel{ArgumentsPresent: true, Arguments: []string{"--listen", "9090"}},
	}, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Local == nil || updated.Local.CommandPresent || !updated.Local.ArgumentsPresent || len(updated.Local.Arguments) != 2 {
		t.Fatalf("unexpected startup override: %#v", updated)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "    startup:\n      arguments:\n        - \"--listen\"") || strings.Contains(string(content), "command:") || strings.Contains(string(content), "resources:") || strings.Contains(string(content), "image:") {
		t.Fatalf("startup override materialized inherited fields:\n%s", content)
	}
	reset, err := repo.UpdateAppSidecarStartupOverride("dev", "api.yml", "proxy", SidecarStartupOverrideUpdate{Action: "reset"}, updated.ContentHash, updated.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if reset.LocalPatch || reset.Local != nil {
		t.Fatalf("startup override was not reset: %#v", reset)
	}
	content, err = os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("startup reset did not restore original source:\n%s", content)
	}
}

func TestSharedSidecarStartupSupportsExplicitEmptyList(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), "sidecar_definitions:\n  - name: proxy\n    startup:\n      arguments: [default]\n")
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	writeFile(t, appPath, "name: api\nsidecar_ref_names:\n  - proxy\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppSidecarStartupOverride("dev", "api.yml", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdateAppSidecarStartupOverride("dev", "api.yml", "proxy", SidecarStartupOverrideUpdate{
		Action: "set", Startup: SidecarStartupModel{ArgumentsPresent: true, Arguments: []string{}},
	}, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Local == nil || !updated.Local.ArgumentsPresent || len(updated.Local.Arguments) != 0 {
		t.Fatalf("explicit empty list was lost: %#v", updated)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "      arguments: []") {
		t.Fatalf("explicit empty list missing:\n%s", content)
	}
}
