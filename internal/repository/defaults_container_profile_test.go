package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileFormAtomicSave(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dev", "apps", "_defaults.yml")
	source := `# catalog
container_profiles:
  - name: java
    defaults:
      image: "<from release manifest>"
      startup:
        command: [/bin/sh]
        arguments: [/app/start.sh]
      ports:
        - name: http
          port: {{env:PORT}}
          expose_as:
            - service_name: "{{var:APP_NAME}}"
              port: 80
      probes:
        http: {path: /health, port: "{{env:PORT}}"}
        start: {failure: 30, period: 10}
      custom: {keep: true}
  # next profile
  - name: worker
    defaults:
      image: worker:1
vars:
  - name: KEEP
    value: yes
`
	writeFile(t, path, source)
	repo, _ := New(root)
	current, err := repo.DefaultsContainerProfile("dev", "java")
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the JSON number representation used by HTTP requests.
	data, _ := json.Marshal(current.Defaults)
	var draft map[string]any
	_ = json.Unmarshal(data, &draft)
	draft["startup"].(map[string]any)["arguments"] = []any{"/app/new-start.sh"}
	draft["probes"].(map[string]any)["start"].(map[string]any)["failure"] = float64(40)
	draft["resources"] = map[string]any{"cpu": map[string]any{"from": "1000ABC"}}
	_, err = repo.UpdateDefaultsContainerProfile("dev", "java", DefaultsContainerProfileUpdate{Defaults: draft}, current.ContentHash)
	if err == nil {
		t.Fatal("invalid resource accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != source {
		t.Fatal("validation failure wrote partial changes")
	}
	draft["resources"] = map[string]any{"cpu": map[string]any{"from": "100m", "to": "500m"}}
	updated, err := repo.UpdateDefaultsContainerProfile("dev", "java", DefaultsContainerProfileUpdate{Defaults: draft}, current.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	for _, keep := range []string{"port: {{env:PORT}}", `service_name: "{{var:APP_NAME}}"`, "custom: {keep: true}", "# next profile\n  - name: worker\n    defaults:\n      image: worker:1\nvars:\n  - name: KEEP\n    value: yes\n"} {
		if !strings.Contains(string(got), keep) {
			t.Fatalf("lost untouched source %q:\n%s", keep, got)
		}
	}
	if !profileEqual(updated.Defaults, draft) {
		t.Fatalf("saved model mismatch: %#v", updated.Defaults)
	}
	_, err = repo.UpdateDefaultsContainerProfile("dev", "java", DefaultsContainerProfileUpdate{Defaults: draft}, updated.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	noop, _ := os.ReadFile(path)
	if string(noop) != string(got) {
		t.Fatal("no-op reformatted source")
	}
	_, err = repo.UpdateDefaultsContainerProfile("dev", "java", DefaultsContainerProfileUpdate{Defaults: draft}, current.ContentHash)
	if !IsConflictError(err) {
		t.Fatalf("stale hash: %v", err)
	}
}

func TestProfileWriterSourceVariants(t *testing.T) {
	for _, body := range []string{
		"container_profiles:\n  - name: java\n    defaults:\n      image: |\n        old-image\n      startup: {command: [/bin/sh]}\n",
		"container_profiles:\n    - name: java\n      defaults:\n          image: old-image\n          startup: {command: [/bin/sh]}\n",
		"container_profiles:\r\n  - name: java\r\n    defaults:\r\n      image: old-image\r\n      startup: {command: [/bin/sh]}\r\n",
	} {
		t.Run(body[:20], func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "dev", "apps", "_defaults.yml")
			writeFile(t, path, body)
			repo, _ := New(root)
			cur, err := repo.DefaultsContainerProfile("dev", "java")
			if err != nil {
				t.Fatal(err)
			}
			_, err = repo.UpdateDefaultsContainerProfile("dev", "java", DefaultsContainerProfileUpdate{Image: "new:1"}, cur.ContentHash)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(path)
			if !strings.Contains(string(got), "startup: {command: [/bin/sh]}") {
				t.Fatal("untouched flow block changed")
			}
		})
	}
	for _, body := range []string{
		"container_profiles: [{name: java, defaults: {image: old}}]\n",
		"container_profiles:\n  - name: java\n    defaults: {image: old}\n",
		"base: &image old\ncontainer_profiles:\n  - name: java\n    defaults:\n      image: *image\n",
	} {
		root := t.TempDir()
		path := filepath.Join(root, "dev", "apps", "_defaults.yml")
		writeFile(t, path, body)
		repo, _ := New(root)
		cur, err := repo.DefaultsContainerProfile("dev", "java")
		if err != nil {
			t.Fatal(err)
		}
		_, err = repo.UpdateDefaultsContainerProfile("dev", "java", DefaultsContainerProfileUpdate{Image: "new:1"}, cur.ContentHash)
		if err == nil {
			t.Fatal("unsafe source accepted")
		}
		got, _ := os.ReadFile(path)
		if string(got) != body {
			t.Fatal("unsafe source changed")
		}
	}
}

func TestProfilePortTemplates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dev", "apps", "_defaults.yml")
	writeFile(t, path, "container_profiles:\n  - name: java\n    defaults:\n      image: java:1\n")
	repo, _ := New(root)
	current, err := repo.DefaultsContainerProfile("dev", "java")
	if err != nil {
		t.Fatal(err)
	}
	current.Defaults["ports"] = []any{map[string]any{"name": "http", "port": "{{env:PORT}}", "expose_as": []any{map[string]any{"hostname": "{{var:APP_NAME}}", "port": "{{var:DEFAULT_SERVICE_PORT}}"}}}}
	updated, err := repo.UpdateDefaultsContainerProfile("dev", "java", DefaultsContainerProfileUpdate{Defaults: current.Defaults}, current.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if !profileEqual(updated.Defaults, current.Defaults) {
		t.Fatal("port templates changed")
	}
	for _, invalid := range []string{"", "0", "65536", "1000ABC", "prefix{{env:PORT}}"} {
		if err := validateProfilePort("service port", invalid); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}
