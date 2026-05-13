package repository

import (
	"os"
	"path/filepath"
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
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\nreplicas: 2\ncontainers:\n  - name: api\n  - name: sidecar\n")
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
