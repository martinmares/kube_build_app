package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
)

func TestCobraLegacySummaryFlag(t *testing.T) {
	root := writeCLIEnv(t)
	out := runCLI(t, "-s", "-e", "test", "-R", root)
	for _, expected := range []string{
		"Resource summary for environment: test",
		"| api |   2 | api",
		"Totals",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("output missing %q:\n%s", expected, out)
		}
	}
}

func TestCobraBuildReportsDefaultOutputDir(t *testing.T) {
	root := writeCLIEnv(t)
	out := runCLI(t, "build", "-e", "test", "-R", root)
	for _, expected := range []string{
		"Generated 1 deployment(s)",
		"Output: " + filepath.Join(root, "test", "target"),
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("build output missing %q:\n%s", expected, out)
		}
	}
}

func TestCobraBuildRejectsSummaryFlag(t *testing.T) {
	root := writeCLIEnv(t)
	out, err := runCLIError("build", "-e", "test", "-R", root, "-s")
	if err == nil {
		t.Fatalf("build -s succeeded, want error:\n%s", out)
	}
	if !strings.Contains(err.Error(), "unknown shorthand flag: 's'") {
		t.Fatalf("unexpected error: %v\n%s", err, out)
	}
}

func TestCobraNoArgsPrintsHelp(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommand(appinfo.For(appinfo.BuildAppName), &cliOptions{})
	cmd.SetArgs(nil)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	oldArgs := os.Args
	os.Args = []string{"kube-build-app"}
	defer func() { os.Args = oldArgs }()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute without args failed: %v\n%s", err, out.String())
	}
	for _, expected := range []string{
		"Build Kubernetes manifests from kube environment repositories",
		"Available Commands:",
		"completion",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("help output missing %q:\n%s", expected, out.String())
		}
	}
}

func TestCobraSubcommands(t *testing.T) {
	root := writeCLIEnv(t)
	if got := strings.TrimSpace(runCLI(t, "list", "-e", "test", "-R", root)); got != "api" {
		t.Fatalf("list output = %q, want api", got)
	}
	if got := strings.TrimSpace(runCLI(t, "validate", "-e", "test", "-R", root)); got != "Validation OK" {
		t.Fatalf("validate output = %q, want Validation OK", got)
	}
	jsonOut := runCLI(t, "summary", "--summary-format", "json", "-e", "test", "-R", root)
	if !strings.Contains(jsonOut, `"environment": "test"`) || !strings.Contains(jsonOut, `"total_cpu_request_cores": 0.2`) {
		t.Fatalf("unexpected summary json:\n%s", jsonOut)
	}
}

func TestCobraCompletionCommand(t *testing.T) {
	out := runCLI(t, "completion", "bash")
	if !strings.Contains(out, "bash completion for kube-build-app") {
		t.Fatalf("unexpected completion output:\n%s", out)
	}
}

func TestCobraBuildVerboseTextAndJSON(t *testing.T) {
	root := writeCLIEnv(t)
	text := runCLI(t, "build", "-e", "test", "-R", root, "--verbose")
	for _, expected := range []string{
		"App api, with 1 container/s",
		"container api has 0 asset/s",
		"deployment api ->",
		"Generated 1 deployment(s)",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("verbose text missing %q:\n%s", expected, text)
		}
	}
	jsonOut := runCLI(t, "build", "-e", "test", "-R", root, "--verbose", "--log-format", "json")
	for _, expected := range []string{
		`"type": "app"`,
		`"app": "api"`,
		`"type": "deployment"`,
		"Generated 1 deployment(s)",
	} {
		if !strings.Contains(jsonOut, expected) {
			t.Fatalf("verbose json missing %q:\n%s", expected, jsonOut)
		}
	}
}

func TestCobraBuildVerboseColorModes(t *testing.T) {
	root := writeCLIEnv(t)
	colored := runCLI(t, "build", "-e", "test", "-R", root, "--verbose", "--color", "always")
	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("expected ANSI color output:\n%s", colored)
	}
	plain := runCLI(t, "build", "-e", "test", "-R", root, "--verbose", "--color", "never")
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("expected plain output, got ANSI color:\n%s", plain)
	}
	jsonOut := runCLI(t, "build", "-e", "test", "-R", root, "--verbose", "--log-format", "json", "--color", "always")
	if strings.Contains(jsonOut, "\x1b[") {
		t.Fatalf("json log must not be colorized:\n%s", jsonOut)
	}
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runCLIError(args...)
	if err != nil {
		t.Fatalf("execute %v failed: %v\n%s", args, err, out)
	}
	return out
}

func runCLIError(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := newRootCommand(appinfo.For(appinfo.BuildAppName), &cliOptions{})
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func writeCLIEnv(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "environments")
	envDir := filepath.Join(root, "test")
	if err := os.MkdirAll(filepath.Join(envDir, "apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "env.unsecured.json"), []byte(`{"environment":{"NAMESPACE":"cli-test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := `
name: api
replicas: 2
containers:
  - name: api
    image: api:latest
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "64Mi", to: "128Mi"}
`
	if err := os.WriteFile(filepath.Join(envDir, "apps", "api.yml"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
