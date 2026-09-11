package repository_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/buildapp"
	"kube-env/internal/repository"
)

func TestDefaultsEditorOutputBuildsWithCurrentMetamodel(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "edit-metamodel", "environments"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "environments")
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := repo.Defaults("dev")
	if err != nil {
		t.Fatal(err)
	}
	defaults, err = repo.UpdateDefaultsContainerEnvs("dev", []repository.ContainerEnvGroupUpdate{
		{ContainerRefName: "*", Envs: []repository.VarItem{{Name: "DEFAULT_MODE", Value: "edited"}}},
		{ContainerRefName: "ui", Envs: []repository.VarItem{{Name: "UI_ONLY", Value: "true"}}},
	}, defaults.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "dev", "apps", "_defaults.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "container_ref_name: \"*\"") || strings.Contains(string(content), "  - name: \"*\"") {
		t.Fatalf("editor wrote obsolete container env selector:\n%s", content)
	}
	opts := buildapp.Options{Environment: "dev", Root: root, Target: t.TempDir()}
	if err := buildapp.Validate(opts); err != nil {
		t.Fatalf("builder rejected editor output: %v", err)
	}
	result, err := buildapp.Build(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deployments) != 2 {
		t.Fatalf("deployments = %d, want 2", len(result.Deployments))
	}
	_ = defaults
}
