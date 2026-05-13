package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
