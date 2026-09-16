package webapp

import (
	"encoding/json"
	"fmt"
	"kube-env/internal/appinfo"
	"kube-env/internal/repository"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileFormAPI(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "dev", "apps", "_defaults.yml")
	writeFile(t, file, "container_profiles:\n  - name: java\n    defaults:\n      image: java:1\n      startup: {command: [/bin/sh]}\n")
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: false})
	path := "/api/v1/envs/dev/defaults/container-profiles/java"
	cur, err := repo.DefaultsContainerProfile("dev", "java")
	if err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"expected_hash":%q,"profile":{"defaults":{"image":"java:1","startup":{"command":["/bin/sh"],"arguments":["/app/start.sh"]},"probes":{"start":{"failure":40}}}}}`, cur.ContentHash)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(payload)))
	if res.Code != http.StatusOK {
		t.Fatalf("save %d: %s", res.Code, res.Body.String())
	}
	var saved repository.DefaultsContainerProfile
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ContentHash == cur.ContentHash || saved.Defaults["probes"] == nil {
		t.Fatal("full draft was not saved")
	}
	before, _ := os.ReadFile(file)
	for _, tc := range []struct {
		server *Server
		body   string
		status int
	}{
		{s, payload, http.StatusConflict},
		{NewServer(appinfo.For(appinfo.EditAppName), repo, Options{ReadOnly: true}), payload, http.StatusForbidden},
		{s, fmt.Sprintf(`{"expected_hash":%q,"profile":{"defaults":{"image":"java:2","probes":{"start":{"failure":0}}}}}`, saved.ContentHash), http.StatusBadRequest},
	} {
		res := httptest.NewRecorder()
		tc.server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(tc.body)))
		if res.Code != tc.status {
			t.Fatalf("status %d expected %d: %s", res.Code, tc.status, res.Body.String())
		}
		after, _ := os.ReadFile(file)
		if string(after) != string(before) {
			t.Fatal("failed request modified file")
		}
	}
}
