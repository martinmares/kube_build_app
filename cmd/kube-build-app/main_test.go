package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestCobraBuildImageOverride(t *testing.T) {
	root := writeCLIEnv(t)
	target := filepath.Join(t.TempDir(), "target")
	runCLI(t, "build", "-e", "test", "-R", root, "-t", target, "--image", "api/api=registry.cli/api:2")
	content, err := os.ReadFile(filepath.Join(target, "deployments", "api-deployment.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "image: registry.cli/api:2") {
		t.Fatalf("deployment image override missing:\n%s", content)
	}
}

func TestCobraBuildYAMLIndent(t *testing.T) {
	root := writeCLIEnv(t)
	target := filepath.Join(t.TempDir(), "target")
	runCLI(t, "build", "-e", "test", "-R", root, "-t", target, "--yaml-indent", "4")
	content, err := os.ReadFile(filepath.Join(target, "deployments", "api-deployment.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "\n    template:\n") {
		t.Fatalf("deployment does not use four-space YAML indentation:\n%s", content)
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

func TestCobraImportDeploymentWithExplicitReport(t *testing.T) {
	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "deployment.yml")
	output := filepath.Join(tempDir, "environments", "test", "apps", "api.yml")
	report := filepath.Join(tempDir, "api.import-report.json")
	deployment := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: source
spec:
  replicas: 2
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: registry.local/api:1
`
	if err := os.WriteFile(source, []byte(deployment), 0o644); err != nil {
		t.Fatal(err)
	}

	commandOutput := runCLI(t, "import", "--file", source, "--output", output, "--report", report)
	if !strings.Contains(commandOutput, "Imported Deployment api -> "+output) ||
		!strings.Contains(commandOutput, "Report: "+report) {
		t.Fatalf("unexpected import output:\n%s", commandOutput)
	}
	appContent, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"name: api", "replicas: 2", "image: registry.local/api:1"} {
		if !strings.Contains(string(appContent), expected) {
			t.Fatalf("app output missing %q:\n%s", expected, appContent)
		}
	}
	reportContent, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"source": "` + source + `"`,
		`"related_objects_read": false`,
		`"secret_values_read": false`,
	} {
		if !strings.Contains(string(reportContent), expected) {
			t.Fatalf("report missing %q:\n%s", expected, reportContent)
		}
	}

	root := filepath.Join(tempDir, "environments")
	envFile := filepath.Join(root, "test", "env.unsecured.json")
	if err := os.WriteFile(envFile, []byte(`{"environment":{"NAMESPACE":"import-test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runCLI(t, "validate", "-e", "test", "-R", root)); got != "Validation OK" {
		t.Fatalf("imported model does not validate: %s", got)
	}
	target := filepath.Join(tempDir, "deploy")
	runCLI(t, "build", "-e", "test", "-R", root, "-t", target)
	rendered, err := os.ReadFile(filepath.Join(target, "deployments", "api-deployment.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "image: registry.local/api:1") {
		t.Fatalf("imported model did not render expected image:\n%s", rendered)
	}
}

func TestCobraImportDoesNotCreateImplicitReportAndDoesNotOverwrite(t *testing.T) {
	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "deployment.yml")
	output := filepath.Join(tempDir, "api.yml")
	deployment := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: api:1
`
	if err := os.WriteFile(source, []byte(deployment), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "import", "-f", source, "-o", output)
	if _, err := os.Stat(filepath.Join(tempDir, "api.import-report.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("implicit report exists or stat failed: %v", err)
	}
	_, err := runCLIError("import", "-f", source, "-o", output)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected overwrite result: %v", err)
	}
}

func TestCobraClusterImportReadsMatchingServices(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake kubectl shell script is Unix-specific")
	}

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeKubectl := filepath.Join(binDir, "kubectl")
	script := `#!/bin/sh
case "$*" in
  *"get deployment tsm-gateway -o yaml"*)
    cat <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tsm-gateway
  namespace: sandbox
spec:
  selector:
    matchLabels:
      app: tsm-gateway
  template:
    metadata:
      labels:
        app: tsm-gateway
    spec:
      containers:
        - name: tsm-gateway
          image: registry.local/tsm-gateway:2.4
          ports:
            - containerPort: 8080
          resources:
            requests:
              cpu: 100m
              memory: 250Mi
            limits:
              cpu: "4"
              memory: 1100Mi
YAML
    ;;
  *"get services -o yaml"*)
    cat <<'YAML'
apiVersion: v1
kind: ServiceList
items:
  - apiVersion: v1
    kind: Service
    metadata:
      name: tsm-gateway
    spec:
      selector:
        app: tsm-gateway
      ports:
        - name: http
          port: 80
          targetPort: 8080
YAML
    ;;
  *)
    echo "unexpected kubectl arguments: $*" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(fakeKubectl, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KUBECONFIG", "")

	output := filepath.Join(tempDir, "tsm-gateway.yml")
	report := filepath.Join(tempDir, "tsm-gateway.import-report.json")
	runCLI(t,
		"import",
		"--namespace", "sandbox",
		"--deployment", "tsm-gateway",
		"--output", output,
		"--report", report,
	)

	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	model := string(content)
	for _, expected := range []string{
		"from: 100m",
		`to: "4"`,
		"- name: http",
		"service_name: tsm-gateway",
		"port: 80",
	} {
		if !strings.Contains(model, expected) {
			t.Fatalf("cluster import missing %q:\n%s", expected, model)
		}
	}
	if strings.Contains(model, "requests:") || strings.Contains(model, "limits:") {
		t.Fatalf("cluster import emitted compatibility resource aliases:\n%s", model)
	}

	reportContent, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"services": 1`, `"related_objects_read": true`} {
		if !strings.Contains(string(reportContent), expected) {
			t.Fatalf("cluster import report missing %q:\n%s", expected, reportContent)
		}
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

func TestSkeletonEnvAndAppAdd(t *testing.T) {
	root := filepath.Join(t.TempDir(), "environments")
	out := runCLI(t, "skeleton", "env", "--root", root, "--env", "dev", "--namespace", "app-dev", "--registry-url", "registry.local/app", "--release-id", "1.0.0")
	if !strings.Contains(out, "Created environment skeleton:") {
		t.Fatalf("unexpected skeleton output:\n%s", out)
	}
	envPath := filepath.Join(root, "dev", "env.unsecured.json")
	envContent, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"NAMESPACE": "app-dev"`, `"REGISTRY_URL": "registry.local/app"`, `"RELEASE_ID": "1.0.0"`} {
		if !strings.Contains(string(envContent), expected) {
			t.Fatalf("env skeleton missing %q:\n%s", expected, envContent)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "dev", "apps", "_defaults.yml")); err != nil {
		t.Fatal(err)
	}

	out = runCLI(t, "app", "add", "api", "-e", "dev", "--root", root)
	if !strings.Contains(out, "Created app model:") {
		t.Fatalf("unexpected app add output:\n%s", out)
	}
	appContent, err := os.ReadFile(filepath.Join(root, "dev", "apps", "api.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"name: api",
		`image: "{{REGISTRY_URL}}/api:{{RELEASE_ID}}"`,
		`requests: "100m"`,
		`limits: "512Mi"`,
	} {
		if !strings.Contains(string(appContent), expected) {
			t.Fatalf("app skeleton missing %q:\n%s", expected, appContent)
		}
	}
}

func TestAppAddImageOverrideAndNoClobber(t *testing.T) {
	root := filepath.Join(t.TempDir(), "environments")
	runCLI(t, "skeleton", "env", "--root", root, "--env", "dev")
	runCLI(t, "app", "add", "worker", "-e", "dev", "--root", root, "--image", "custom/worker:1")
	content, err := os.ReadFile(filepath.Join(root, "dev", "apps", "worker.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `image: "custom/worker:1"`) {
		t.Fatalf("app image override missing:\n%s", content)
	}
	out, err := runCLIError("app", "add", "worker", "-e", "dev", "--root", root)
	if err == nil {
		t.Fatalf("app add overwrote existing file, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected no-clobber error: %v\n%s", err, out)
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
