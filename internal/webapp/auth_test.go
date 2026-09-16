package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
	"kube-env/internal/repository"
)

func TestParseTrustedProxyGroupsAdmin(t *testing.T) {
	p := parseTrustedProxyGroups("kube-edit-app:role:admin", "kube-edit-app")
	if !p.IsAdmin || !p.CanReadEnv("dev") || !p.CanWriteEnv("dev") {
		t.Fatal("admin should read and write every env")
	}
}

func TestParseTrustedProxyGroupsEnvReader(t *testing.T) {
	p := parseTrustedProxyGroups("kube-edit-app:env:dev:reader", "kube-edit-app")
	if !p.CanReadEnv("dev") {
		t.Fatal("reader should read own env")
	}
	if p.CanWriteEnv("dev") {
		t.Fatal("reader must not write own env")
	}
	if p.CanReadEnv("test") {
		t.Fatal("reader must not read other env")
	}
}

func TestParseTrustedProxyGroupsEnvWriter(t *testing.T) {
	p := parseTrustedProxyGroups("kube-edit-app:env:dev:writer", "kube-edit-app")
	if !p.CanReadEnv("dev") || !p.CanWriteEnv("dev") {
		t.Fatal("writer should read and write own env")
	}
	if p.CanWriteEnv("test") {
		t.Fatal("writer must not write other env")
	}
}

func TestTrustedProxyAuthRequiresIdentity(t *testing.T) {
	server := NewServer(appinfo.For(appinfo.EditAppName), nil, Options{
		TrustedProxyAuth: TrustedProxyAuthOptions{Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}

func TestTrustedProxyAuthAllowsHealthzWithoutIdentity(t *testing.T) {
	server := NewServer(appinfo.For(appinfo.EditAppName), nil, Options{
		TrustedProxyAuth: TrustedProxyAuthOptions{Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
}

func TestTrustedProxyAuthEnvReaderCanReadNotWrite(t *testing.T) {
	server := NewServer(appinfo.For(appinfo.EditAppName), nil, Options{
		TrustedProxyAuth: TrustedProxyAuthOptions{Enabled: true},
	})

	readReq := authReq(http.MethodGet, "/api/v1/envs/dev/apps", "kube-edit-app:env:dev:reader", "")
	readRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(readRes, readReq)
	if readRes.Code == http.StatusForbidden || readRes.Code == http.StatusUnauthorized {
		t.Fatalf("reader GET status = %d, want auth pass-through", readRes.Code)
	}

	writeReq := authReq(http.MethodPatch, "/api/v1/envs/dev/defaults/vars", "kube-edit-app:env:dev:reader", `{}`)
	writeRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(writeRes, writeReq)
	if writeRes.Code != http.StatusForbidden {
		t.Fatalf("reader PATCH status = %d, want 403", writeRes.Code)
	}
}

func TestTrustedProxyAuthEnvWriterCanWriteOwnEnv(t *testing.T) {
	server := NewServer(appinfo.For(appinfo.EditAppName), nil, Options{
		TrustedProxyAuth: TrustedProxyAuthOptions{Enabled: true},
	})
	req := authReq(http.MethodPatch, "/api/v1/envs/dev/defaults/vars", "kube-edit-app:env:dev:writer", `{}`)
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	if res.Code == http.StatusForbidden || res.Code == http.StatusUnauthorized {
		t.Fatalf("writer PATCH status = %d, want auth pass-through", res.Code)
	}
}

func TestTrustedProxyAuthGitMutationsRequireAdmin(t *testing.T) {
	server := NewServer(appinfo.For(appinfo.EditAppName), nil, Options{
		TrustedProxyAuth: TrustedProxyAuthOptions{Enabled: true},
	})
	req := authReq(http.MethodPost, "/api/v1/git/commit", "kube-edit-app:env:dev:writer", `{}`)
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("git commit status = %d, want 403", res.Code)
	}
}

func TestTrustedProxyAuthCoversMetamodelEndpointRoles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{
		TrustedProxyAuth: TrustedProxyAuthOptions{Enabled: true},
	})
	cases := []struct {
		method string
		path   string
		write  bool
	}{
		{http.MethodGet, "/api/v1/envs/dev/apps/api.yml/model", false},
		{http.MethodGet, "/api/v1/envs/dev/apps/api.yml/references", false},
		{http.MethodPatch, "/api/v1/envs/dev/apps/api.yml/references", true},
		{http.MethodGet, "/api/v1/envs/dev/apps/api.yml/sidecars/exporter/envs/MODE/override", false},
		{http.MethodPatch, "/api/v1/envs/dev/apps/api.yml/sidecars/exporter/envs/MODE/override", true},
		{http.MethodGet, "/api/v1/envs/dev/apps/api.yml/sidecars/exporter/resources/override", false},
		{http.MethodPatch, "/api/v1/envs/dev/apps/api.yml/sidecars/exporter/resources/override", true},
		{http.MethodGet, "/api/v1/envs/dev/apps/api.yml/sidecars/exporter/startup/override", false},
		{http.MethodPatch, "/api/v1/envs/dev/apps/api.yml/sidecars/exporter/startup/override", true},
		{http.MethodGet, "/api/v1/envs/dev/defaults", false},
		{http.MethodPatch, "/api/v1/envs/dev/defaults/container-envs", true},
		{http.MethodGet, "/api/v1/envs/dev/defaults/container-profiles/java-service", false},
		{http.MethodPatch, "/api/v1/envs/dev/defaults/container-profiles/java-service", true},
		{http.MethodGet, "/api/v1/envs/dev/defaults/sidecar-definitions/exporter", false},
		{http.MethodPatch, "/api/v1/envs/dev/defaults/sidecar-definitions/exporter", true},
		{http.MethodPatch, "/api/v1/envs/dev/defaults/sidecar-definitions/exporter/startup", true},
		{http.MethodPatch, "/api/v1/envs/dev/defaults/sidecar-definitions/exporter/resources", true},
		{http.MethodPatch, "/api/v1/envs/dev/defaults/sidecar-definitions/exporter/envs", true},
		{http.MethodPatch, "/api/v1/envs/dev/defaults/sidecar-definitions/exporter/references", true},
		{http.MethodGet, "/api/v1/envs/dev/inspect", false},
		{http.MethodGet, "/api/v1/envs/dev/build-context", false},
	}

	for _, item := range cases {
		name := item.method + " " + item.path
		t.Run(name, func(t *testing.T) {
			assertAuthResult(t, server, item.method, item.path, "kube-edit-app:env:dev:reader", !item.write)
			assertAuthResult(t, server, item.method, item.path, "kube-edit-app:env:dev:writer", true)
			assertAuthResult(t, server, item.method, strings.Replace(item.path, "/envs/dev/", "/envs/test/", 1), "kube-edit-app:env:dev:writer", false)
			assertAuthResult(t, server, item.method, item.path, "kube-edit-app:role:admin", true)
		})
	}
}

func TestTrustedProxyAuthFiltersEnvironmentList(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "api.yml"), "name: api\n")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{
		TrustedProxyAuth: TrustedProxyAuthOptions{Enabled: true},
	})
	request := authReq(http.MethodGet, "/api/v1/envs", "kube-edit-app:env:dev:reader", "")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Name != "dev" {
		t.Fatalf("visible environments = %#v, want only dev", payload.Items)
	}
}

func assertAuthResult(t *testing.T, server *Server, method, path, groups string, allowed bool) {
	t.Helper()
	request := authReq(method, path, groups, `{}`)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	denied := response.Code == http.StatusUnauthorized || response.Code == http.StatusForbidden
	if allowed && denied {
		t.Fatalf("status = %d, want request to pass authorization", response.Code)
	}
	if !allowed && !denied {
		t.Fatalf("status = %d, want authorization denial", response.Code)
	}
}

func authReq(method, path, groups, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Auth-User", "mares")
	req.Header.Set("X-Auth-Groups", groups)
	return req
}
