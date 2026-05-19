package status

import (
	"os"
	"path/filepath"
	"testing"

	"kube-env/internal/opsapp/config"
	"kube-env/internal/opsapp/render"
	"kube-env/internal/opsapp/state"
)

func TestComputeUnknownWithoutAppliedState(t *testing.T) {
	cfg := testConfig(t)
	got, err := Compute(cfg, state.NewStore(""), "test")
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncStatus != SyncUnknown || got.DesiredDigest == "" || got.Reason == "" {
		t.Fatalf("status = %#v, want unknown with desired digest", got)
	}
}

func TestComputeInSyncAndOutOfSync(t *testing.T) {
	cfg := testConfig(t)
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	desired, err := render.RenderDigestByName(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEnvironment("test", state.EnvironmentState{AppliedRevision: "main", AppliedDigest: desired.Digest}); err != nil {
		t.Fatal(err)
	}
	got, err := Compute(cfg, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncStatus != SyncInSync {
		t.Fatalf("sync status = %s, want %s", got.SyncStatus, SyncInSync)
	}

	if err := store.SaveEnvironment("test", state.EnvironmentState{AppliedRevision: "old", AppliedDigest: "sha256:old"}); err != nil {
		t.Fatal(err)
	}
	got, err = Compute(cfg, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncStatus != SyncOutOfSync {
		t.Fatalf("sync status = %s, want %s", got.SyncStatus, SyncOutOfSync)
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{"NAMESPACE":"ops-test","TSM_REGISTRY_URL":"registry.local","TSM_RELEASE_ID":"1"}}`)
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    image: "{{env:TSM_REGISTRY_URL}}/api:{{env:TSM_RELEASE_ID}}"
`)
	cfg := config.Config{Environments: []config.EnvironmentConfig{{Name: "test", EnvName: "test", Namespace: "ops-test", RootPath: root, TargetRevision: "main"}}}
	return cfg
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
