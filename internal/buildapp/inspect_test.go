package buildapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectUsesBuilderCompositionAndPreservesSourceTemplates(t *testing.T) {
	root := editMetamodelFixtureRoot(t)
	inspection, err := Inspect(Options{Environment: "dev", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.EffectiveError != "" || inspection.Namespace != "edit-metamodel-dev" || len(inspection.Apps) != 2 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	java := inspectedAppByName(t, inspection, "java-api.yml")
	if java.Source.Model["name"] != "{{var:APP_NAME}}" || !strings.Contains(java.Source.RawContent, "{{env:REGISTRY_URL}}") {
		t.Fatalf("source templates were not preserved: %#v", java.Source)
	}
	if java.Effective == nil || len(java.Effective.Containers) != 1 || len(java.Effective.Sidecars) != 2 {
		t.Fatalf("composed containers missing: %#v", java.Effective)
	}
	container := java.Effective.Containers[0]
	if java.Effective.Kind != "Deployment" || container.Image != "registry.example.test/apps/java-api:2026.09.11" || len(container.Ports) != 1 || len(container.Probes) == 0 {
		t.Fatalf("profile values missing from effective container: %#v", container)
	}
	imageField := container.Fields["image"]
	if imageField.WriteTarget == nil || imageField.WriteTarget.ExpectedHash != java.Source.ContentHash || !imageField.Capabilities.CanOverride {
		t.Fatalf("safe image write metadata missing: %#v", imageField)
	}
	if sourceField := java.Source.Fields["containers[0].profile_ref_names"]; !sourceField.Present || sourceField.Line == 0 {
		t.Fatalf("source field metadata missing: %#v", java.Source.Fields)
	}
	defaultEnv := inspectedEnvByName(t, container.EnvEntries, "DEFAULT_MODE")
	localEnv := inspectedEnvByName(t, container.EnvEntries, "JAVA_ARGS")
	if !hasInspectOrigin(defaultEnv.Origins, "container_env_defaults", "*") || !localEnv.Capabilities.CanReset || !hasInspectOrigin(localEnv.Origins, "local", "JAVA_ARGS") {
		t.Fatalf("env entry provenance missing: %#v", container.EnvEntries)
	}
	if len(java.Effective.RuntimeAssets) != 1 {
		t.Fatalf("runtime assets = %#v, want selected java-config", java.Effective.RuntimeAssets)
	}
	if !hasInspectOrigin(java.Origins, "container_profile", "java-service") || !hasInspectOrigin(java.Origins, "sidecar_definition", "process-exporter") {
		t.Fatalf("composition origins missing: %#v", java.Origins)
	}
}

func TestInspectKeepsSourceWhenBuildVariablesFail(t *testing.T) {
	root := editMetamodelFixtureRoot(t)
	inspection, err := Inspect(Options{Environment: "dev", Root: root, EnvFile: filepath.Join(t.TempDir(), "missing.env")})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.EffectiveError == "" || len(inspection.Apps) != 2 {
		t.Fatalf("expected effective error with source apps: %#v", inspection)
	}
	for _, app := range inspection.Apps {
		if app.Source.RawContent == "" || app.Source.Model == nil || app.Effective != nil {
			t.Fatalf("source unavailable after effective failure: %#v", app)
		}
	}
}

func TestDescribeBuildContextDoesNotExposeHeaderValues(t *testing.T) {
	context, err := DescribeBuildContext(Options{
		EnvURL:         "https://config.example.test/render",
		EnvURLHeaders:  []string{"Authorization: Bearer secret", "X-Tenant: fixture"},
		EnvURLInsecure: true,
		Namespace:      "override",
		ImagePolicy:    "strict",
		ImageReference: "tag",
		YAMLIndent:     4,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "Bearer") || context.EnvURLHeaderCount != 2 || !context.EnvURLConfigured {
		t.Fatalf("unsafe context description: %s", encoded)
	}
}

func TestParseTemplateYAMLPreservesQuotedAndTypedPlaceholders(t *testing.T) {
	model, err := parseTemplateYAML([]byte("name: \"api-{{var:SUFFIX}}\"\nport: {{env:PORT}}\nlegacy: {{LEGACY}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if model["name"] != "api-{{var:SUFFIX}}" || model["port"] != "{{env:PORT}}" || model["legacy"] != "{{LEGACY}}" {
		t.Fatalf("templates changed: %#v", model)
	}
}

func editMetamodelFixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "edit-metamodel", "environments"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func inspectedAppByName(t *testing.T, inspection Inspection, name string) InspectedApp {
	t.Helper()
	for _, app := range inspection.Apps {
		if app.FileName == name {
			return app
		}
	}
	t.Fatalf("app %s not found", name)
	return InspectedApp{}
}

func hasInspectOrigin(origins []InspectOrigin, kind, definition string) bool {
	for _, origin := range origins {
		if origin.Kind == kind && origin.DefinitionName == definition {
			return true
		}
	}
	return false
}

func inspectedEnvByName(t *testing.T, entries []InspectEnvEntry, name string) InspectEnvEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("env %s not found", name)
	return InspectEnvEntry{}
}
