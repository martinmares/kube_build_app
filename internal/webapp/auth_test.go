package webapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
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

func authReq(method, path, groups, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Auth-User", "mares")
	req.Header.Set("X-Auth-Groups", groups)
	return req
}
