package buildapp

import (
	"path/filepath"
	"testing"
)

func TestBuildNamespaceOptionOverridesEnvironmentNamespace(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{"NAMESPACE": "namespace-from-env"},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
labels:
  generated-namespace: "{{env:NAMESPACE}}"
containers:
  - name: api
    image: api:latest
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	_, err := Build(Options{
		Environment: "test",
		Namespace:   "namespace-from-cli",
		Root:        root,
		Target:      target,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "metadata", "namespace"); got != "namespace-from-cli" {
		t.Fatalf("metadata.namespace = %q, want namespace-from-cli", got)
	}
	if got := digString(deployment, "metadata", "labels", "generated-namespace"); got != "namespace-from-cli" {
		t.Fatalf("expanded namespace label = %q, want namespace-from-cli", got)
	}
}

func TestLoadBuildVarsNamespaceOptionOverridesExplicitEnvFile(t *testing.T) {
	envDir := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "build.env")
	writeFile(t, envFile, "NAMESPACE=namespace-from-dotenv\n")

	vars, err := loadBuildVars(envDir, Options{
		EnvFile:   envFile,
		Namespace: "namespace-from-cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := vars["NAMESPACE"]; got != "namespace-from-cli" {
		t.Fatalf("NAMESPACE = %q, want namespace-from-cli", got)
	}
}
