package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

func TestGitStatusEndpoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/git/status", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), `"available":`) {
		t.Fatalf("git status response missing available flag: %s", response.Body.String())
	}
}

func TestGitDiffEndpoint(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/git/diff/test/apps/api.yml", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"available":true`) || !strings.Contains(response.Body.String(), `+++ b/test/apps/api.yml`) {
		t.Fatalf("unexpected diff response:\n%s", response.Body.String())
	}
}

func TestGitRestoreEndpointRequiresWriteMode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: changed\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}

	readOnlyServer := NewServer(appinfo.For(appinfo.EditAppName), repo)
	readOnlyReq := httptest.NewRequest(http.MethodPost, "/api/v1/git/restore/test/apps/api.yml", strings.NewReader(`{}`))
	readOnlyRes := httptest.NewRecorder()
	readOnlyServer.Handler().ServeHTTP(readOnlyRes, readOnlyReq)
	if readOnlyRes.Code != http.StatusForbidden {
		t.Fatalf("read-only status = %d, want 403: %s", readOnlyRes.Code, readOnlyRes.Body.String())
	}

	writeServer := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/git/restore/test/apps/api.yml", strings.NewReader(`{}`))
	writeRes := httptest.NewRecorder()
	writeServer.Handler().ServeHTTP(writeRes, writeReq)
	if writeRes.Code != http.StatusOK {
		t.Fatalf("write status = %d, want 200: %s", writeRes.Code, writeRes.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "test", "apps", "api.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "name: api\n" {
		t.Fatalf("content = %q, want restored original", content)
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
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
    vars:
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

func TestAppVarsUpdateEndpointRequiresWriteMode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "vars:\n  - name: APP_NAME\n    value: api\nname: \"{{var:APP_NAME}}\"\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}

	readOnlyServer := NewServer(appinfo.For(appinfo.EditAppName), repo)
	readOnlyReq := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/vars", strings.NewReader(`{"items":[{"name":"APP_NAME","value":"worker"}]}`))
	readOnlyRes := httptest.NewRecorder()
	readOnlyServer.Handler().ServeHTTP(readOnlyRes, readOnlyReq)
	if readOnlyRes.Code != http.StatusForbidden {
		t.Fatalf("read-only status = %d, want 403: %s", readOnlyRes.Code, readOnlyRes.Body.String())
	}

	writeServer := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	writeReq := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/vars", strings.NewReader(`{"items":[{"name":"APP_NAME","value":"worker"}]}`))
	writeRes := httptest.NewRecorder()
	writeServer.Handler().ServeHTTP(writeRes, writeReq)
	if writeRes.Code != http.StatusOK {
		t.Fatalf("write status = %d, want 200: %s", writeRes.Code, writeRes.Body.String())
	}
	if !strings.Contains(writeRes.Body.String(), `"value":"worker"`) {
		t.Fatalf("unexpected write response:\n%s", writeRes.Body.String())
	}
}

func TestAppVarsUpdateEndpointRejectsStaleHash(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "vars:\n  - name: APP_NAME\n    value: api\nname: \"{{var:APP_NAME}}\"\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := repo.AppVars("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, appPath, "vars:\n  - name: APP_NAME\n    value: changed\nname: \"{{var:APP_NAME}}\"\n")

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + vars.ContentHash + `","items":[{"name":"APP_NAME","value":"worker"}]}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/vars", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", response.Code, response.Body.String())
	}
}

func TestAppReplicasUpdateEndpoint(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\nreplicas: 1\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + detail.ContentHash + `","replicas":3}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/replicas", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"replicas":3`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestAppContainerResourcesUpdateEndpoint(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\ncontainers:\n  - name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + detail.ContentHash + `","resources":{"cpu_request":"100m","memory_limit":"512Mi"}}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/resources", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"cpu_request":"100m"`) || !strings.Contains(response.Body.String(), `"memory_limit":"512Mi"`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestAppContainerVarsUpdateEndpoint(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\ncontainers:\n  - name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + detail.ContentHash + `","items":[{"name":"MODE","value":"api"},{"name":"PORT","value":"8080"}]}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/vars", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"name":"MODE"`) || !strings.Contains(response.Body.String(), `"value":"8080"`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestAppAutoscalingUpdateEndpoint(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\ncontainers:\n  - name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + detail.ContentHash + `","autoscaling":{"enabled":true,"min_replicas":"2","max_replicas":"6","cpu_average_utilization":"75"}}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/autoscaling", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"enabled":true`) || !strings.Contains(response.Body.String(), `"max_replicas":6`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestAppContainerRuntimeUpdateEndpoint(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\ncontainers:\n  - name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + detail.ContentHash + `","runtime":{"xms":"512m","xmx":"1536m","opts":["-XX:+UseG1GC"],"export_env_name":"APP_JAVA_OPTS"}}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/runtime/java", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"xmx":"1536m"`) || !strings.Contains(response.Body.String(), `"export_env_name":"APP_JAVA_OPTS"`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestAppContainerProbesUpdateEndpoint(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\ncontainers:\n  - name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + detail.ContentHash + `","probes":{"preset":"spring-actuator","port":"8080","path":"/healthz"}}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/probes", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"preset":"spring-actuator"`) || !strings.Contains(response.Body.String(), `"port":"8080"`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestAppContainerLegacyProbesFixEndpoint(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\ncontainers:\n  - name: api\n    probe:\n      ready:\n        http:\n          port: 8080\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + detail.ContentHash + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/envs/test/apps/api.yml/containers/0/probes/fix-legacy", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"legacy":true`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
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
		{"/api/v1/envs/test/preview", `"files"`},
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
