package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
	"kube-env/internal/repository"
)

func TestHealthz(t *testing.T) {
	server := NewServer(appinfo.For(appinfo.EditAppName), nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if payload["status"] != "UP" {
		t.Fatalf("status payload = %q, want UP", payload["status"])
	}
}

func TestInfo(t *testing.T) {
	info := appinfo.For(appinfo.EditAppName)
	server := NewServer(info, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload appinfo.Info
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if payload.Name != appinfo.EditAppName {
		t.Fatalf("name = %q, want %q", payload.Name, appinfo.EditAppName)
	}
}

func TestEnvironmentsEndpoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/envs", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload struct {
		Root  string                   `json:"root"`
		Items []repository.Environment `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if payload.Root != repo.Root() {
		t.Fatalf("root = %q, want %q", payload.Root, repo.Root())
	}
	if len(payload.Items) != 1 || payload.Items[0].Name != "test" {
		t.Fatalf("items = %#v, want one test environment", payload.Items)
	}
}

func TestAppsEndpoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/apps", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload struct {
		Env   string           `json:"env"`
		Items []repository.App `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if payload.Env != "test" {
		t.Fatalf("env = %q, want test", payload.Env)
	}
	if len(payload.Items) != 1 || payload.Items[0].AppName != "api" {
		t.Fatalf("items = %#v, want api app", payload.Items)
	}
}

func TestAssetsEndpoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "assets", "ui", "nginx.conf"), "server {}\n")
	writeFile(t, filepath.Join(root, "test", "assets.secured.json"), "{\"assets\":{}}\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/assets", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload struct {
		Env   string             `json:"env"`
		Items []repository.Asset `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if payload.Env != "test" {
		t.Fatalf("env = %q, want test", payload.Env)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %#v", len(payload.Items), payload.Items)
	}
	if payload.Items[0].RelativePath != "assets.secured.json" || payload.Items[1].RelativePath != "ui/nginx.conf" {
		t.Fatalf("items = %#v, want special asset and nginx.conf", payload.Items)
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

func TestAppReadEndpoints(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `vars:
  - name: APP_NAME
    value: api
name: "{{var:APP_NAME}}"
replicas: 1
containers:
  - name: api
    env_vars:
      - name: MODE
        value: test
`)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)

	for _, item := range []struct {
		path     string
		expected string
	}{
		{"/api/v1/envs/test/apps/api.yml", `"file_name":"api.yml"`},
		{"/api/v1/envs/test/apps/api.yml/rendered", `"file_name":"api.yml"`},
		{"/api/v1/envs/test/apps/api.yml/vars", `"name":"APP_NAME"`},
		{"/api/v1/envs/test/apps/api.yml/model", `"containers"`},
	} {
		request := httptest.NewRequest(http.MethodGet, item.path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200: %s", item.path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), item.expected) {
			t.Fatalf("%s response missing %q:\n%s", item.path, item.expected, response.Body.String())
		}
	}
}

func TestAssetContentEndpoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "assets", "ui", "nginx.conf"), "server {}\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/assets/content/ui/nginx.conf", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"relative_path":"ui/nginx.conf"`) || !strings.Contains(response.Body.String(), "server {}") {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestSpecialEntriesAndPreflightEndpoints(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{"A":"one"}}`)
	writeFile(t, filepath.Join(root, "test", "env.secured.json"), `{"environment":{"SECRET":"EncJson[@api=2.0:@box=<x>]"}}`)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)

	entriesReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/assets/special/env.unsecured.json/entries", nil)
	entriesRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(entriesRes, entriesReq)
	if entriesRes.Code != http.StatusOK {
		t.Fatalf("entries status = %d, want 200: %s", entriesRes.Code, entriesRes.Body.String())
	}
	if !strings.Contains(entriesRes.Body.String(), `"key":"A"`) || !strings.Contains(entriesRes.Body.String(), `"editable":true`) {
		t.Fatalf("unexpected entries response:\n%s", entriesRes.Body.String())
	}

	preflightReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/assets/special/env.secured.json/preflight", nil)
	preflightRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(preflightRes, preflightReq)
	if preflightRes.Code != http.StatusOK {
		t.Fatalf("preflight status = %d, want 200: %s", preflightRes.Code, preflightRes.Body.String())
	}
	if !strings.Contains(preflightRes.Body.String(), `"mode":"modern"`) || !strings.Contains(preflightRes.Body.String(), `"ok":false`) {
		t.Fatalf("unexpected preflight response:\n%s", preflightRes.Body.String())
	}
}

func TestIndexAndStaticAssets(t *testing.T) {
	server := NewServer(appinfo.For(appinfo.EditAppName), nil)
	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(indexRes, indexReq)
	if indexRes.Code != http.StatusOK {
		t.Fatalf("index status = %d, want 200", indexRes.Code)
	}
	if !strings.Contains(indexRes.Body.String(), "kube-edit-app") || !strings.Contains(indexRes.Body.String(), "/static/ui.js") {
		t.Fatalf("unexpected index response:\n%s", indexRes.Body.String())
	}

	staticReq := httptest.NewRequest(http.MethodGet, "/static/ui.js", nil)
	staticRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(staticRes, staticReq)
	if staticRes.Code != http.StatusOK {
		t.Fatalf("static status = %d, want 200", staticRes.Code)
	}
	if !strings.Contains(staticRes.Body.String(), "const state") {
		t.Fatalf("unexpected static response:\n%s", staticRes.Body.String())
	}
}

func TestBuildReadOnlyEndpoints(t *testing.T) {
	root := t.TempDir()
	writeBuildFixture(t, root)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)

	for _, item := range []struct {
		path     string
		expected string
	}{
		{"/api/v1/envs/test/validate", `"ok":true`},
		{"/api/v1/envs/test/summary", `"environment":"test"`},
		{"/api/v1/envs/test/inventory", `"env":"test"`},
	} {
		request := httptest.NewRequest(http.MethodPost, item.path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200: %s", item.path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), item.expected) {
			t.Fatalf("%s response missing %q:\n%s", item.path, item.expected, response.Body.String())
		}
	}
}

func writeBuildFixture(t *testing.T, root string) {
	t.Helper()
	envDir := filepath.Join(root, "test")
	writeFile(t, filepath.Join(envDir, "env.unsecured.json"), `{"environment":{"NAMESPACE":"nac-test","TSM_REGISTRY_URL":"registry.local/tsm","TSM_RELEASE_ID":"1.0.0"}}`)
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 2
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)
}
