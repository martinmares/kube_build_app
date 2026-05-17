package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentsDiscoversAndSortsEnvironmentDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "prod", "apps", "api.yml"), "name: api\n")
	writeFile(t, filepath.Join(root, "prod", "apps", "_defaults.yml"), "arch: amd64\n")
	writeFile(t, filepath.Join(root, "prod", "assets", "nginx.conf"), "server {}\n")
	writeFile(t, filepath.Join(root, "prod", "env.unsecured.json"), "{\"environment\":{}}\n")
	writeFile(t, filepath.Join(root, "test", "env.secured.json"), "{\"_public_key\":\"\",\"environment\":{}}\n")
	writeFile(t, filepath.Join(root, "ignored.txt"), "x\n")
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	environments, err := repo.Environments()
	if err != nil {
		t.Fatal(err)
	}

	if len(environments) != 2 {
		t.Fatalf("len(environments) = %d, want 2", len(environments))
	}
	if environments[0].Name != "prod" || environments[1].Name != "test" {
		t.Fatalf("environment order = %q, %q; want prod, test", environments[0].Name, environments[1].Name)
	}

	prod := environments[0]
	if !prod.HasAppsDir {
		t.Fatal("prod.HasAppsDir = false, want true")
	}
	if !prod.HasAssetsDir {
		t.Fatal("prod.HasAssetsDir = false, want true")
	}
	if !prod.HasEnvUnsecuredJSON {
		t.Fatal("prod.HasEnvUnsecuredJSON = false, want true")
	}
	if prod.AppFilesCount != 2 {
		t.Fatalf("prod.AppFilesCount = %d, want 2", prod.AppFilesCount)
	}
	if prod.AssetFilesCount != 1 {
		t.Fatalf("prod.AssetFilesCount = %d, want 1", prod.AssetFilesCount)
	}

	testEnv := environments[1]
	if !testEnv.HasEnvSecuredJSON {
		t.Fatal("test.HasEnvSecuredJSON = false, want true")
	}
}

func TestNewRejectsMissingRoot(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("New missing root error = nil, want error")
	}
}

func TestAppsListsRegularYamlAppsAndSkipsSpecialFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "_defaults.yml"), "arch: amd64\n")
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `vars:
  - name: APP_NAME
    value: api
name: "{{var:APP_NAME}}"
replicas: 2
containers:
  - name: api
    env_vars:
      - name: ONE
        value: "1"
    ports:
      - name: http
        port: 8080
  - name: sidecar
`)
	writeFile(t, filepath.Join(root, "test", "apps", "worker.yaml"), "name: \"worker\"\ncontainers:\n  - name: worker\n")
	writeFile(t, filepath.Join(root, "test", "apps", "notes.txt"), "ignore\n")

	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	apps, err := repo.Apps("test")
	if err != nil {
		t.Fatal(err)
	}

	if len(apps) != 2 {
		t.Fatalf("len(apps) = %d, want 2", len(apps))
	}
	if apps[0].FileName != "api.yml" || apps[1].FileName != "worker.yaml" {
		t.Fatalf("apps order = %#v", apps)
	}
	if apps[0].AppName != "api" {
		t.Fatalf("api app name = %q, want api", apps[0].AppName)
	}
	if apps[0].Replicas == nil || *apps[0].Replicas != 2 {
		t.Fatalf("api replicas = %#v, want 2", apps[0].Replicas)
	}
	if apps[0].ContainersCount != 2 {
		t.Fatalf("api containers count = %d, want 2", apps[0].ContainersCount)
	}
	if apps[1].AppName != "worker" {
		t.Fatalf("worker app name = %q, want worker", apps[1].AppName)
	}
}

func TestAssetsListsRecursiveAssetsAndSpecialRootAssets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "assets", "ui", "nginx.conf"), "server {}\n")
	writeFile(t, filepath.Join(root, "test", "assets", "ssl", "cert.pem"), "cert\n")
	writeFile(t, filepath.Join(root, "test", "assets.secured.json"), "{\"assets\":{}}\n")
	writeFile(t, filepath.Join(root, "test", "assets.unsecured.json"), "{\"assets\":{}}\n")

	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := repo.Assets("test")
	if err != nil {
		t.Fatal(err)
	}

	relativePaths := make([]string, 0, len(assets))
	for _, asset := range assets {
		relativePaths = append(relativePaths, asset.RelativePath)
	}
	expected := []string{"assets.secured.json", "assets.unsecured.json", "ssl/cert.pem", "ui/nginx.conf"}
	if len(relativePaths) != len(expected) {
		t.Fatalf("relative paths = %#v, want %#v", relativePaths, expected)
	}
	for i := range expected {
		if relativePaths[i] != expected[i] {
			t.Fatalf("relative paths = %#v, want %#v", relativePaths, expected)
		}
	}
	if assets[0].Driver != "special" || assets[2].Driver != "configmap" {
		t.Fatalf("unexpected drivers: %#v", assets)
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

func TestAppDetailRenderedVarsAndModel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `vars:
  - name: APP_NAME
    value: "api"
  - name: EXPOSE_PORT
    value: 8080
name: "{{var:APP_NAME}}"
replicas: 2
autoscaling:
  enabled: true
  min_replicas: 2
  max_replicas: 4
  cpu:
    average_utilization: 75
containers:
  - name: api
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    runtime:
      java:
        xms: 512m
        xmx: 1024m
        opts: ["-XX:+UseG1GC"]
    probes:
      preset: spring-actuator
      port: 8080
    mounts:
      - type: temp
        name: work
        mount_path: /work
    env_from:
      - config_map: api-env
    env_vars:
      - name: PLAIN
        value: hello
      - name: SECRET
        valueFrom:
          secretKeyRef:
            name: app-secret
            key: password
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
    ports:
      - name: http
        port: {{var:EXPOSE_PORT}}
        expose_as:
          - hostname: api
            port: 80
            external:
              - name: api-public
                http:
                  - hostname: api.example.test
                    path: /
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if detail.FileName != "api.yml" || detail.Summary.AppName != "api" {
		t.Fatalf("unexpected detail: %#v", detail)
	}

	rendered, err := repo.AppRendered("test", "api")
	if err != nil {
		t.Fatal(err)
	}
	if rendered.FileName != "api.yml" || !contains(rendered.Content, `name: "api"`) || contains(rendered.Content, "\nvars:") || strings.HasPrefix(rendered.Content, "vars:") {
		t.Fatalf("unexpected rendered content:\n%s", rendered.Content)
	}

	vars, err := repo.AppVars("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(vars.Items) != 2 || vars.Items[0].Name != "APP_NAME" || vars.Items[0].Value != "api" || vars.Items[1].Name != "EXPOSE_PORT" || vars.Items[1].Value != "8080" {
		t.Fatalf("unexpected vars: %#v", vars.Items)
	}

	model, err := repo.AppModel("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if model.AppName == nil || *model.AppName != "api" || len(model.Containers) != 1 {
		t.Fatalf("unexpected model: %#v", model)
	}
	if !model.Autoscaling.Enabled || model.Autoscaling.CPUAverageUtilization == nil || *model.Autoscaling.CPUAverageUtilization != 75 {
		t.Fatalf("unexpected autoscaling model: %#v", model.Autoscaling)
	}
	container := model.Containers[0]
	if !container.Runtime.Java.Enabled || container.Runtime.Java.Xmx == nil || *container.Runtime.Java.Xmx != "1024m" {
		t.Fatalf("unexpected runtime model: %#v", container.Runtime)
	}
	if !container.Probes.Enabled || container.Probes.Preset == nil || *container.Probes.Preset != "spring-actuator" {
		t.Fatalf("unexpected probes model: %#v", container.Probes)
	}
	if container.EnvFromCount != 1 || container.MountsCount != 1 {
		t.Fatalf("unexpected env_from/mounts count: env_from=%d mounts=%d", container.EnvFromCount, container.MountsCount)
	}
	if len(container.EnvVars) != 2 || container.EnvVars[1].Kind != "secret" || container.EnvVars[1].SecretName == nil || *container.EnvVars[1].SecretName != "app-secret" {
		t.Fatalf("unexpected env model: %#v", container.EnvVars)
	}
	if len(container.Ports) != 1 || len(container.Ports[0].ExposeAs) != 1 || !container.Ports[0].ExposeAs[0].IngressEnabled {
		t.Fatalf("unexpected ports model: %#v", container.Ports)
	}
	if container.Ports[0].Port == nil || *container.Ports[0].Port != "8080" {
		t.Fatalf("unexpected port model: %#v", container.Ports[0])
	}
}

func TestAssetDetailSupportsSpecialRootAndRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "assets", "ui", "nginx.conf"), "server {}\n")
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{}}`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := repo.AssetDetail("test", "ui/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	if asset.RelativePath != "ui/nginx.conf" || asset.Content != "server {}\n" {
		t.Fatalf("unexpected asset detail: %#v", asset)
	}

	special, err := repo.AssetDetail("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}
	if special.RelativePath != "env.unsecured.json" || !contains(special.Content, "environment") {
		t.Fatalf("unexpected special detail: %#v", special)
	}

	if _, err := repo.AssetDetail("test", "../env.unsecured.json"); err == nil {
		t.Fatal("path escape succeeded, want error")
	}
	if _, err := repo.AppDetail("test", "../api.yml"); err == nil {
		t.Fatal("app path escape succeeded, want error")
	}
}

func contains(value string, needle string) bool {
	return strings.Contains(value, needle)
}

func TestSpecialEntriesForEnvAndAssets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{"A":"one","B":2}}`)
	writeFile(t, filepath.Join(root, "test", "assets.unsecured.json"), `{"assets":{"ssl/cert.pem":{"content":"Q0VSVA==","kind":"text"},"legacy.bin":"AAE="}}`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	envEntries, err := repo.SpecialEntries("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}
	if !envEntries.Editable || len(envEntries.Entries) != 2 || envEntries.Entries[0].Key != "A" || envEntries.Entries[1].ValueType != "number" {
		t.Fatalf("unexpected env entries: %#v", envEntries)
	}

	assetEntries, err := repo.SpecialEntries("test", "assets.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(assetEntries.Entries) != 2 || assetEntries.Entries[1].Key != "ssl/cert.pem" || assetEntries.Entries[1].ValueType != "text" {
		t.Fatalf("unexpected asset entries: %#v", assetEntries)
	}
}

func TestSpecialEntriesForSecuredAreReadOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.secured.json"), `{"environment":{"SECRET":"EncJson[@api=2.0:@box=<x>]"}}`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := repo.SpecialEntries("test", "env.secured.json")
	if err != nil {
		t.Fatal(err)
	}
	if entries.Editable || entries.Warning == nil || len(entries.Entries) != 1 || entries.Entries[0].Key != "SECRET" {
		t.Fatalf("unexpected secured entries: %#v", entries)
	}
}
