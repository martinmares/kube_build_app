package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
	"kube-env/internal/opsapp/config"
)

func TestServerInfoAndEnvironmentList(t *testing.T) {
	srv := New(Options{Info: appinfo.Info{Name: "kube-ops-app", Version: "test"}, Config: testConfig(t)})

	infoResp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(infoResp, httptest.NewRequest(http.MethodGet, "/api/v1/info", nil))
	if infoResp.Code != http.StatusOK {
		t.Fatalf("info status = %d, body = %s", infoResp.Code, infoResp.Body.String())
	}
	if !strings.Contains(infoResp.Body.String(), "kube-ops-app") {
		t.Fatalf("info body = %s", infoResp.Body.String())
	}

	envsResp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(envsResp, httptest.NewRequest(http.MethodGet, "/api/v1/envs", nil))
	if envsResp.Code != http.StatusOK {
		t.Fatalf("envs status = %d, body = %s", envsResp.Code, envsResp.Body.String())
	}
	var payload struct {
		Environments []config.EnvironmentConfig `json:"environments"`
	}
	if err := json.Unmarshal(envsResp.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Environments) != 1 || payload.Environments[0].Name != "test" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestServerStatus(t *testing.T) {
	srv := New(Options{Info: appinfo.Info{Name: "kube-ops-app", Version: "test"}, Config: testConfig(t)})
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/status", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Unknown") || !strings.Contains(resp.Body.String(), "sha256:") {
		t.Fatalf("status body = %s", resp.Body.String())
	}
}

func TestServerMissingEnvironment(t *testing.T) {
	srv := New(Options{Info: appinfo.Info{Name: "kube-ops-app", Version: "test"}, Config: testConfig(t)})
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/envs/missing/status", nil))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
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
	return config.Config{Environments: []config.EnvironmentConfig{{Name: "test", EnvName: "test", Namespace: "ops-test", RootPath: root, TargetRevision: "main"}}}
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
