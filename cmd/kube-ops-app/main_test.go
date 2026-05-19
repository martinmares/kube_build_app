package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
)

func TestEnvListCommand(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "ops.yml")
	writeFile(t, configPath, `environments:
  - name: test
    namespace: ops-test
    root_path: /repo/environments
    branch: main
`)

	out, err := executeCommand("--config", configPath, "env", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "test\ttest\tops-test\tmain") {
		t.Fatalf("env list output = %q", out)
	}
}

func TestEnvRenderDigestCommandJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "envs", "test", "env.unsecured.json"), `{"environment":{"NAMESPACE":"ops-test","TSM_REGISTRY_URL":"registry.local","TSM_RELEASE_ID":"1"}}`)
	writeFile(t, filepath.Join(root, "envs", "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    image: "{{env:TSM_REGISTRY_URL}}/api:{{env:TSM_RELEASE_ID}}"
`)
	configPath := filepath.Join(root, "ops.yml")
	writeFile(t, configPath, `environments:
  - name: test
    namespace: ops-test
    root_path: `+filepath.ToSlash(filepath.Join(root, "envs"))+`
    branch: main
`)

	out, err := executeCommand("--config", configPath, "--output", "json", "env", "render-digest", "test")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Environment string `json:"environment"`
		Digest      string `json:"digest"`
		Files       int    `json:"files"`
		Build       struct {
			Deployments int `json:"deployments"`
		} `json:"build"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if payload.Environment != "test" || !strings.HasPrefix(payload.Digest, "sha256:") || payload.Files == 0 || payload.Build.Deployments != 1 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestEnvStatusCommandUnknownAndMarkApplied(t *testing.T) {
	root, configPath := writeOpsFixture(t)
	statePath := filepath.Join(root, "state.json")

	out, err := executeCommand("--config", configPath, "--state", statePath, "env", "status", "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "test\tUnknown\tdesired=sha256:") || !strings.Contains(out, "applied=-") {
		t.Fatalf("unknown status output = %q", out)
	}

	out, err = executeCommand("--config", configPath, "--state", statePath, "env", "mark-applied", "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "test\tmarked applied\tsha256:") {
		t.Fatalf("mark-applied output = %q", out)
	}

	out, err = executeCommand("--config", configPath, "--state", statePath, "env", "status", "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "test\tInSync\tdesired=sha256:") {
		t.Fatalf("in-sync status output = %q", out)
	}
}

func executeCommand(args ...string) (string, error) {
	cmd := newRootCommand(appinfo.For(appinfo.OpsAppName), &cliOptions{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func writeOpsFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "envs", "test", "env.unsecured.json"), `{"environment":{"NAMESPACE":"ops-test","TSM_REGISTRY_URL":"registry.local","TSM_RELEASE_ID":"1"}}`)
	writeFile(t, filepath.Join(root, "envs", "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    image: "{{env:TSM_REGISTRY_URL}}/api:{{env:TSM_RELEASE_ID}}"
`)
	configPath := filepath.Join(root, "ops.yml")
	writeFile(t, configPath, `environments:
  - name: test
    namespace: ops-test
    root_path: `+filepath.ToSlash(filepath.Join(root, "envs"))+`
    branch: main
`)
	return root, configPath
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
