package render

import (
	"os"
	"path/filepath"
	"testing"

	"kube-env/internal/opsapp/config"
)

func TestRenderDigestBuildsEnvironment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{"NAMESPACE":"ops-test","TSM_REGISTRY_URL":"registry.local","TSM_RELEASE_ID":"1"}}`)
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    image: "{{env:TSM_REGISTRY_URL}}/api:{{env:TSM_RELEASE_ID}}"
`)

	result, err := RenderDigest(config.EnvironmentConfig{Name: "test", EnvName: "test", Namespace: "ops-test", RootPath: root, TargetRevision: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Environment != "test" || result.EnvName != "test" || result.Namespace != "ops-test" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Digest == "" || result.Files == 0 || result.Build.Deployments != 1 {
		t.Fatalf("result = %#v, want digest and one deployment", result)
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
