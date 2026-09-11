package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
	"kube-env/internal/buildapp"
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

func TestGitCommitEndpointRequiresWriteModeAndCommitsSelected(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	writeFile(t, filepath.Join(root, "test", "apps", "worker.yml"), "name: worker\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: changed-api\n")
	writeFile(t, filepath.Join(root, "test", "apps", "worker.yml"), "name: changed-worker\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}

	readOnlyServer := NewServer(appinfo.For(appinfo.EditAppName), repo)
	readOnlyReq := httptest.NewRequest(http.MethodPost, "/api/v1/git/commit", strings.NewReader(`{"paths":["test/apps/api.yml"],"message":"accept api"}`))
	readOnlyRes := httptest.NewRecorder()
	readOnlyServer.Handler().ServeHTTP(readOnlyRes, readOnlyReq)
	if readOnlyRes.Code != http.StatusForbidden {
		t.Fatalf("read-only status = %d, want 403: %s", readOnlyRes.Code, readOnlyRes.Body.String())
	}

	writeServer := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/git/commit", strings.NewReader(`{"paths":["test/apps/api.yml"],"message":"accept api"}`))
	writeRes := httptest.NewRecorder()
	writeServer.Handler().ServeHTTP(writeRes, writeReq)
	if writeRes.Code != http.StatusOK {
		t.Fatalf("write status = %d, want 200: %s", writeRes.Code, writeRes.Body.String())
	}
	if !strings.Contains(writeRes.Body.String(), `"committed":true`) {
		t.Fatalf("unexpected commit response:\n%s", writeRes.Body.String())
	}
	status := repo.GitStatus()
	if len(status.Files) != 1 || status.Files[0].Path != "test/apps/worker.yml" {
		t.Fatalf("status files after selected commit = %#v, want only worker dirty", status.Files)
	}
}

func TestMutatingEndpointsRequireWriteMode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	writeFile(t, filepath.Join(root, "test", "apps", "_defaults.yml"), "vars: []\n")
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{}}`)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)

	for _, item := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/git/restore/test/apps/api.yml", `{}`},
		{http.MethodPost, "/api/v1/git/commit", `{"paths":["test/apps/api.yml"],"message":"test"}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/vars", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/defaults/vars", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/defaults/container-envs", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/replicas", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/autoscaling", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/resources", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/envs", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/runtime/java", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/ports", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/probes", `{}`},
		{http.MethodPost, "/api/v1/envs/test/apps/api.yml/containers/0/probes/fix-legacy", `{}`},
		{http.MethodPatch, "/api/v1/envs/test/assets/special/env.unsecured.json/entries", `{}`},
	} {
		t.Run(item.method+" "+item.path, func(t *testing.T) {
			request := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", response.Code, response.Body.String())
			}
		})
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
    envs:
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

func TestAppContainerEnvsUpdateEndpoint(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/envs", strings.NewReader(body))
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

func TestAppContainerPortsUpdateEndpoint(t *testing.T) {
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
	body := `{"expected_hash":"` + detail.ContentHash + `","ports":[{"name":"http","port":"8080","expose_as":[{"service_name":"api","port":"80","externals":[{"name":"api-public","http_hostname":"api.example.test","http_path":"/"}]}]}]}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/apps/api.yml/containers/0/ports", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"service_name":"api"`) || !strings.Contains(response.Body.String(), `"http_hostname":"api.example.test"`) {
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

func TestInspectionAndBuildContextEndpointsUseConfiguredOptions(t *testing.T) {
	root := editMetamodelWebFixtureRoot(t)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{BuildOptions: buildapp.Options{
		Namespace:      "web-override",
		ImagePolicy:    "fallback",
		ImageReference: "auto",
		YAMLIndent:     4,
	}})

	contextReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/dev/build-context", nil)
	contextRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(contextRes, contextReq)
	if contextRes.Code != http.StatusOK || !strings.Contains(contextRes.Body.String(), `"namespace_override":"web-override"`) || !strings.Contains(contextRes.Body.String(), `"yaml_indent":4`) {
		t.Fatalf("unexpected build context response (%d): %s", contextRes.Code, contextRes.Body.String())
	}

	inspectReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/dev/inspect", nil)
	inspectRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(inspectRes, inspectReq)
	if inspectRes.Code != http.StatusOK || !strings.Contains(inspectRes.Body.String(), `"namespace":"web-override"`) || !strings.Contains(inspectRes.Body.String(), `"sidecars"`) || !strings.Contains(inspectRes.Body.String(), `"container_profile"`) {
		t.Fatalf("unexpected inspection response (%d): %s", inspectRes.Code, inspectRes.Body.String())
	}
}

func TestDefaultsContainerEnvsAPIProducesBuildableStableMetamodel(t *testing.T) {
	root := copyEditMetamodelWebFixture(t)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})

	get := httptest.NewRequest(http.MethodGet, "/api/v1/envs/dev/defaults", nil)
	getResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("defaults status = %d: %s", getResponse.Code, getResponse.Body.String())
	}
	var defaults repository.DefaultsModel
	if err := json.Unmarshal(getResponse.Body.Bytes(), &defaults); err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"expected_hash":%q,"groups":[{"container_ref_name":"*","envs":[{"name":"DEFAULT_MODE","value":"api-edited"}]}]}`, defaults.ContentHash)
	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/dev/defaults/container-envs", strings.NewReader(payload))
	patchResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(patchResponse, patch)
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("defaults patch status = %d: %s", patchResponse.Code, patchResponse.Body.String())
	}
	if err := json.Unmarshal(patchResponse.Body.Bytes(), &defaults); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "dev", "apps", "_defaults.yml")
	beforeNoop, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	payload = fmt.Sprintf(`{"expected_hash":%q,"groups":[{"container_ref_name":"*","envs":[{"name":"DEFAULT_MODE","value":"api-edited"}]}]}`, defaults.ContentHash)
	patch = httptest.NewRequest(http.MethodPatch, "/api/v1/envs/dev/defaults/container-envs", strings.NewReader(payload))
	patchResponse = httptest.NewRecorder()
	server.Handler().ServeHTTP(patchResponse, patch)
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("no-op defaults patch status = %d: %s", patchResponse.Code, patchResponse.Body.String())
	}
	afterNoop, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeNoop, afterNoop) {
		t.Fatalf("no-op defaults update changed source:\nbefore:\n%s\nafter:\n%s", beforeNoop, afterNoop)
	}
	if err := buildapp.Validate(buildapp.Options{Environment: "dev", Root: root}); err != nil {
		t.Fatalf("builder rejected API output: %v", err)
	}
}

func TestWebPreviewMatchesDirectBuilderWithSameContext(t *testing.T) {
	root := editMetamodelWebFixtureRoot(t)
	directTarget := t.TempDir()
	resourcePolicyRoot := filepath.Join(filepath.Dir(root), "resources")
	opts := buildapp.Options{
		Environment: "dev", Root: root, Target: directTarget, Namespace: "preview-namespace",
		ResourcePolicyRoot: resourcePolicyRoot, ReleaseManifest: filepath.Join(root, "dev", "_release.yml"),
		SyncProfile: "kube-deploy-sync", YAMLIndent: 4,
	}
	result, err := buildapp.Build(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deployments) == 0 {
		t.Fatal("direct build returned no deployments")
	}
	javaManifest, err := os.ReadFile(result.Deployments[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"namespace: preview-namespace", "image: registry.release.example.test/java-api:2026.09.11", "150m", "800Mi", "kube-build-app.io/sync-hash"} {
		if !strings.Contains(string(javaManifest), expected) {
			t.Fatalf("direct manifest missing %q:\n%s", expected, javaManifest)
		}
	}
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{BuildOptions: opts})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/envs/dev/preview", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d: %s", response.Code, response.Body.String())
	}
	var preview buildPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	for _, file := range preview.Files {
		direct, err := os.ReadFile(filepath.Join(directTarget, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		contentReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/dev/preview/"+preview.ID+"/content/"+file.Path, nil)
		contentRes := httptest.NewRecorder()
		server.Handler().ServeHTTP(contentRes, contentReq)
		if contentRes.Code != http.StatusOK {
			t.Fatalf("preview content %s status = %d: %s", file.Path, contentRes.Code, contentRes.Body.String())
		}
		var content buildPreviewContent
		if err := json.Unmarshal(contentRes.Body.Bytes(), &content); err != nil {
			t.Fatal(err)
		}
		if content.Content != string(direct) {
			t.Fatalf("web preview differs from direct build for %s", file.Path)
		}
	}
}

func TestClusterStatusEndpointDisabledByDefault(t *testing.T) {
	root := t.TempDir()
	writeBuildFixture(t, root)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/cluster", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestClusterStatusEndpointUsesNamespaceFromEnv(t *testing.T) {
	root := t.TempDir()
	writeBuildFixture(t, root)
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "kubectl"), []byte(fakeClusterKubectlScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ClusterStatus: true})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/cluster", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{`"enabled":true`, `"namespace":"nac-test"`, `"deployment_count":1`, `"ready_pods":1`, `"service_count":1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response missing %s:\n%s", expected, body)
		}
	}
}

func TestBuildPreviewContentEndpoint(t *testing.T) {
	root := t.TempDir()
	writeBuildFixture(t, root)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo)

	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/envs/test/preview", nil)
	previewRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(previewRes, previewReq)
	if previewRes.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200: %s", previewRes.Code, previewRes.Body.String())
	}
	var preview buildPreview
	if err := json.Unmarshal(previewRes.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.ID == "" || len(preview.Files) == 0 {
		t.Fatalf("preview missing id/files: %#v", preview)
	}

	contentPath := "/api/v1/envs/test/preview/" + preview.ID + "/content/" + preview.Files[0].Path
	contentReq := httptest.NewRequest(http.MethodGet, contentPath, nil)
	contentRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(contentRes, contentReq)
	if contentRes.Code != http.StatusOK {
		t.Fatalf("content status = %d, want 200: %s", contentRes.Code, contentRes.Body.String())
	}
	var content buildPreviewContent
	if err := json.Unmarshal(contentRes.Body.Bytes(), &content); err != nil {
		t.Fatal(err)
	}
	if content.Path == "" || content.Content == "" || content.Binary {
		t.Fatalf("unexpected preview content: %#v", content)
	}
}

func TestSecuredSpecialPreflightAndDecryptedEntries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.secured.json"), `{"environment":{"SECRET":"EncJson[@api=2.0:@box=<x>]"}}`)
	fakeEncjson := filepath.Join(root, "fake-encjson")
	writeFile(t, fakeEncjson, "#!/bin/sh\nprintf '%s\\n' '{\"environment\":{\"SECRET\":\"plain\",\"COUNT\":2}}'\n")
	if err := os.Chmod(fakeEncjson, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{EncjsonPath: fakeEncjson})

	preflightReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/assets/special/env.secured.json/preflight", nil)
	preflightRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(preflightRes, preflightReq)
	if preflightRes.Code != http.StatusOK {
		t.Fatalf("preflight status = %d, want 200: %s", preflightRes.Code, preflightRes.Body.String())
	}
	if !strings.Contains(preflightRes.Body.String(), `"decrypt_ok":true`) {
		t.Fatalf("preflight did not decrypt:\n%s", preflightRes.Body.String())
	}

	entriesReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/assets/special/env.secured.json/decrypted-entries", nil)
	entriesRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(entriesRes, entriesReq)
	if entriesRes.Code != http.StatusOK {
		t.Fatalf("decrypted entries status = %d, want 200: %s", entriesRes.Code, entriesRes.Body.String())
	}
	if !strings.Contains(entriesRes.Body.String(), `"key":"SECRET"`) || !strings.Contains(entriesRes.Body.String(), `"value_text":"plain"`) {
		t.Fatalf("unexpected decrypted entries:\n%s", entriesRes.Body.String())
	}
}

func TestSecuredSpecialDecryptedEntriesPreserveEnvironmentOrder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.secured.json"), `{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  }
}
`)
	fakeEncjson := filepath.Join(root, "fake-encjson")
	writeFile(t, fakeEncjson, "#!/bin/sh\ncat <<'JSON'\n{\"environment\":{\"SECRET_B\":\"plain-b\",\"SECRET_A\":\"plain-a\",\"SECRET_COUNT\":2}}\nJSON\n")
	if err := os.Chmod(fakeEncjson, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{EncjsonPath: fakeEncjson})

	entriesReq := httptest.NewRequest(http.MethodGet, "/api/v1/envs/test/assets/special/env.secured.json/decrypted-entries", nil)
	entriesRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(entriesRes, entriesReq)
	if entriesRes.Code != http.StatusOK {
		t.Fatalf("decrypted entries status = %d, want 200: %s", entriesRes.Code, entriesRes.Body.String())
	}
	body := entriesRes.Body.String()
	bIdx := strings.Index(body, `"key":"SECRET_B"`)
	aIdx := strings.Index(body, `"key":"SECRET_A"`)
	countIdx := strings.Index(body, `"key":"SECRET_COUNT"`)
	if bIdx < 0 || aIdx < 0 || countIdx < 0 || !(bIdx < aIdx && aIdx < countIdx) {
		t.Fatalf("decrypted entries order not preserved:\n%s", body)
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

func editMetamodelWebFixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "edit-metamodel", "environments"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func copyEditMetamodelWebFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "environments")
	if err := os.CopyFS(root, os.DirFS(editMetamodelWebFixtureRoot(t))); err != nil {
		t.Fatal(err)
	}
	return root
}

const fakeClusterKubectlScript = `#!/bin/sh
resource=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "get" ]; then
    resource="$arg"
    break
  fi
  prev="$arg"
done
case "$resource" in
  deployments)
    cat <<'JSON'
{"items":[{"metadata":{"name":"api"},"spec":{"replicas":1},"status":{"readyReplicas":1,"availableReplicas":1,"updatedReplicas":1}}]}
JSON
    ;;
  pods)
    cat <<'JSON'
{"items":[{"metadata":{"name":"api-1"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}}]}
JSON
    ;;
  services)
    cat <<'JSON'
{"items":[{"metadata":{"name":"api"},"spec":{"type":"ClusterIP","clusterIP":"10.0.0.1","ports":[{"port":80,"targetPort":8080,"protocol":"TCP"}]}}]}
JSON
    ;;
  *)
    echo "unexpected resource $resource" >&2
    exit 2
    ;;
esac
`

func TestDefaultsUpdateEndpoints(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "_defaults.yml"), "vars:\n  - name: APP_NAME\n    value: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := repo.Defaults("test")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})

	varsBody := `{"expected_hash":"` + defaults.ContentHash + `","items":[{"name":"APP_NAME","value":"worker"}]}`
	varsReq := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/defaults/vars", strings.NewReader(varsBody))
	varsRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(varsRes, varsReq)
	if varsRes.Code != http.StatusOK {
		t.Fatalf("vars status = %d, want 200: %s", varsRes.Code, varsRes.Body.String())
	}
	if !strings.Contains(varsRes.Body.String(), `"value":"worker"`) {
		t.Fatalf("unexpected vars response:\n%s", varsRes.Body.String())
	}

	defaults, err = repo.Defaults("test")
	if err != nil {
		t.Fatal(err)
	}
	envsBody := `{"expected_hash":"` + defaults.ContentHash + `","groups":[{"container_ref_name":"*","envs":[{"name":"LOG_LEVEL","value":"INFO"}]}]}`
	envsReq := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/defaults/container-envs", strings.NewReader(envsBody))
	envsRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(envsRes, envsReq)
	if envsRes.Code != http.StatusOK {
		t.Fatalf("container envs status = %d, want 200: %s", envsRes.Code, envsRes.Body.String())
	}
	if !strings.Contains(envsRes.Body.String(), `"container_envs"`) || !strings.Contains(envsRes.Body.String(), `"LOG_LEVEL"`) {
		t.Fatalf("unexpected container envs response:\n%s", envsRes.Body.String())
	}
}

func TestSpecialEntriesUpdateEndpoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{"A":"one"}}`)
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.SpecialEntries("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	body := `{"expected_hash":"` + entries.ContentHash + `","entries":[{"key":"A","value_type":"string","value_text":"two"},{"key":"FLAG","value_type":"bool","value_text":"true"}]}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/assets/special/env.unsecured.json/entries", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"key":"FLAG"`) || !strings.Contains(response.Body.String(), `"value_type":"bool"`) {
		t.Fatalf("unexpected response:\n%s", response.Body.String())
	}
}

func TestSecuredSpecialEntriesUpdateEndpointPatchesEncryptedValuesOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "env.secured.json")
	writeFile(t, path, `{
  "_public_key": "keep",
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`)
	fakeEncjson := filepath.Join(root, "fake-encjson")
	writeFile(t, fakeEncjson, `#!/bin/sh
cmd="$1"
shift
file=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-f" ]; then
    file="$2"
    shift 2
  else
    shift
  fi
done
if [ "$cmd" = "decrypt" ]; then
  cat <<'JSON'
{
  "_public_key": "keep",
  "environment": {
    "SECRET_B": "plain-b",
    "SECRET_A": "plain-a",
    "SECRET_COUNT": 2
  },
  "other": true
}
JSON
  exit 0
fi
grep -q '"SECRET_A": "changed"' "$file" || exit 9
cat <<'JSON'
{
  "_public_key": "keep",
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<changed>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
JSON
`)
	if err := os.Chmod(fakeEncjson, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.SpecialEntries("test", "env.secured.json")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false, EncjsonPath: fakeEncjson})
	body := `{"expected_hash":"` + entries.ContentHash + `","entries":[{"key":"SECRET_B","value_type":"string","value_text":"plain-b"},{"key":"SECRET_A","value_type":"string","value_text":"changed"},{"key":"SECRET_COUNT","value_type":"number","value_text":"2"}]}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/assets/special/env.secured.json/entries", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "_public_key": "keep",
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<changed>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`
	if string(gotBytes) != want {
		t.Fatalf("content changed unexpectedly:\n%s", string(gotBytes))
	}
}

func TestSecuredSpecialEntriesUpdateEndpointEncryptsOnlyNewPlainEntryAtSubmittedPosition(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "env.secured.json")
	writeFile(t, path, `{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`)
	fakeEncjson := filepath.Join(root, "fake-encjson")
	writeFile(t, fakeEncjson, `#!/bin/sh
cmd="$1"
shift
file=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-f" ]; then
    file="$2"
    shift 2
  else
    shift
  fi
done
if [ "$cmd" = "decrypt" ]; then
  cat <<'JSON'
{
  "environment": {
    "SECRET_B": "plain-b",
    "SECRET_A": "plain-a",
    "SECRET_COUNT": 2
  },
  "other": true
}
JSON
  exit 0
fi
grep -Fq '"SECRET_B": "EncJson[@api=2.0:@box=<b>]"' "$file" || exit 9
grep -Fq '"SECRET_A": "EncJson[@api=2.0:@box=<a>]"' "$file" || exit 9
grep -Fq '"SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"' "$file" || exit 9
grep -Fq '"SECRET_B_2": "plain-new"' "$file" || exit 9
cat <<'JSON'
{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_B_2": "EncJson[@api=2.0:@box=<new>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
JSON
`)
	if err := os.Chmod(fakeEncjson, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.SpecialEntries("test", "env.secured.json")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false, EncjsonPath: fakeEncjson})
	body := `{"expected_hash":"` + entries.ContentHash + `","entries":[{"key":"SECRET_B","value_type":"string","value_text":"plain-b"},{"key":"SECRET_B_2","value_type":"string","value_text":"plain-new"},{"key":"SECRET_A","value_type":"string","value_text":"plain-a"},{"key":"SECRET_COUNT","value_type":"number","value_text":"2"}]}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/envs/test/assets/special/env.secured.json/entries", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_B_2": "EncJson[@api=2.0:@box=<new>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`
	if string(gotBytes) != want {
		t.Fatalf("content changed unexpectedly:\n%s", string(gotBytes))
	}
}
