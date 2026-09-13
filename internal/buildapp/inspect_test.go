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
	if inspection.Defaults == nil || inspection.SharedAssets == nil {
		t.Fatalf("catalog sources missing: defaults=%#v shared=%#v", inspection.Defaults, inspection.SharedAssets)
	}
	if !hasReferenceUsage(inspection.Usage, "container_profile", "java-service", "java-api") ||
		!hasReferenceUsage(inspection.Usage, "sidecar_definition", "process-exporter", "") ||
		!hasReferenceUsage(inspection.Usage, "runtime_asset_definition", "java-config", "*") {
		t.Fatalf("reference usage missing: %#v", inspection.Usage)
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
	if !hasInspectOrigin(imageField.Origins, "container_profile", "java-service") || hasInspectOrigin(imageField.Origins, "release_manifest", "") {
		t.Fatalf("profile image provenance is incorrect: %#v", imageField.Origins)
	}
	if sourceField := java.Source.Fields["containers[0].profile_ref_names"]; !sourceField.Present || sourceField.Line == 0 {
		t.Fatalf("source field metadata missing: %#v", java.Source.Fields)
	}
	defaultEnv := inspectedEnvByName(t, container.EnvEntries, "DEFAULT_MODE")
	localEnv := inspectedEnvByName(t, container.EnvEntries, "JAVA_ARGS")
	if !hasInspectOrigin(defaultEnv.Origins, "container_env_defaults", "*") || !localEnv.Capabilities.CanReset || !hasInspectOrigin(localEnv.Origins, "local", "JAVA_ARGS") {
		t.Fatalf("env entry provenance missing: %#v", container.EnvEntries)
	}
	if localEnv.Effective["value"] != "-Xms256m -Xmx512m" {
		t.Fatalf("JAVA_ARGS changed from an env string: %#v", localEnv.Effective)
	}
	if _, converted := container.Runtime["java"]; converted {
		t.Fatalf("JAVA_ARGS was converted to runtime.java: %#v", container.Runtime)
	}
	if len(java.Effective.RuntimeAssets) != 1 {
		t.Fatalf("runtime assets = %#v, want selected java-config", java.Effective.RuntimeAssets)
	}
	processExporter := effectiveContainerByName(t, java.Effective.Sidecars, "process-exporter")
	if !hasInspectOrigin(processExporter.Origins, "sidecar_definition", "process-exporter") || hasInspectOrigin(processExporter.Fields["image"].Origins, "local", "") {
		t.Fatalf("shared sidecar provenance mapped to local effective index: %#v", processExporter)
	}
	processEnv := inspectedEnvByName(t, processExporter.EnvEntries, "TARGET_PROCESS")
	if !hasInspectOrigin(processEnv.Origins, "sidecar_definition", "process-exporter") || hasInspectOrigin(processEnv.Origins, "local", "TARGET_PROCESS") {
		t.Fatalf("shared sidecar env provenance is incorrect: %#v", processEnv)
	}
	if !processEnv.Capabilities.CanOverride || processEnv.Capabilities.CanReset || processEnv.WriteTarget == nil || processEnv.WriteTarget.NameAssertion != "process-exporter" || processEnv.WriteTarget.SourceIndex != -1 || processEnv.WriteTarget.ExpectedDependencyHash != inspection.Defaults.ContentHash {
		t.Fatalf("shared sidecar env write target is unsafe: %#v", processEnv)
	}
	processResources := processExporter.Fields["resources"]
	if !processResources.Capabilities.CanOverride || processResources.Capabilities.CanReset || processResources.WriteTarget == nil || processResources.WriteTarget.NameAssertion != "process-exporter" || processResources.WriteTarget.SourceIndex != -1 || processResources.WriteTarget.ExpectedDependencyHash != inspection.Defaults.ContentHash {
		t.Fatalf("shared sidecar resources write target is unsafe: %#v", processResources)
	}
	processStartup := processExporter.Fields["startup"]
	if !processStartup.Capabilities.CanOverride || processStartup.Capabilities.CanReset || processStartup.WriteTarget == nil || processStartup.WriteTarget.NameAssertion != "process-exporter" || processStartup.WriteTarget.SourceIndex != -1 || processStartup.WriteTarget.ExpectedDependencyHash != inspection.Defaults.ContentHash {
		t.Fatalf("shared sidecar startup write target is unsafe: %#v", processStartup)
	}
	if !hasInspectOrigin(java.Origins, "container_profile", "java-service") || !hasInspectOrigin(java.Origins, "sidecar_definition", "process-exporter") {
		t.Fatalf("composition origins missing: %#v", java.Origins)
	}
	if !hasResolvedReferenceUsage(inspection.Usage, "workload_identity_token", "runtime-config", "java-api") || !hasResolvedReferenceUsage(inspection.Usage, "shared_asset", "test-ca", "java-api") {
		t.Fatalf("transitive runtime asset usage missing: %#v", inspection.Usage)
	}
}

func TestInspectMarksOnlyImagesResolvedFromReleaseManifest(t *testing.T) {
	root := editMetamodelFixtureRoot(t)
	inspection, err := Inspect(Options{
		Environment:     "dev",
		Root:            root,
		ReleaseManifest: filepath.Join(root, "dev", "_release.yml"),
		ImagePolicy:     "strict",
		ImageReference:  "tag",
	})
	if err != nil {
		t.Fatal(err)
	}
	java := inspectedAppByName(t, inspection, "java-api.yml")
	container := java.Effective.Containers[0]
	if container.Image != "registry.release.example.test/java-api:2026.09.11" {
		t.Fatalf("release image = %q", container.Image)
	}
	imageOrigins := container.Fields["image"].Origins
	if len(imageOrigins) == 0 || imageOrigins[len(imageOrigins)-1].Kind != "release_manifest" {
		t.Fatalf("release image is not the winning provenance: %#v", imageOrigins)
	}

	processExporter := effectiveContainerByName(t, java.Effective.Sidecars, "process-exporter")
	if hasInspectOrigin(processExporter.Fields["image"].Origins, "release_manifest", "") || !hasInspectOrigin(processExporter.Fields["image"].Origins, "sidecar_definition", "process-exporter") {
		t.Fatalf("non-overridden sidecar image provenance is incorrect: %#v", processExporter.Fields["image"].Origins)
	}
}

func TestExpandReferenceUsageFollowsSidecarDefinitionReferences(t *testing.T) {
	defaults := SourceDocument{Model: map[string]any{
		"sidecar_definitions": []any{map[string]any{
			"name":                    "process-exporter",
			"profile_ref_names":       []any{"exporter-profile"},
			"runtime_asset_ref_names": []any{"exporter-config"},
		}},
		"container_profiles": []any{map[string]any{
			"name": "exporter-profile",
			"defaults": map[string]any{"envs": []any{map[string]any{
				"name": "TOKEN", "workload_identity_token_ref_name": "runtime-token",
			}}},
		}},
		"runtime_asset_definitions": []any{map[string]any{
			"name":   "exporter-config",
			"source": map[string]any{"ca_shared_asset_ref_name": "cluster-ca"},
		}},
	}}
	base := []ReferenceUsage{{
		Kind: "sidecar_definition", Name: "process-exporter", App: "api", AppFile: "api.yml", Document: "apps/api.yml",
	}}

	usage := expandReferenceUsage(base, defaults)
	for _, expected := range []struct{ kind, name string }{
		{"container_profile", "exporter-profile"},
		{"runtime_asset_definition", "exporter-config"},
		{"workload_identity_token", "runtime-token"},
		{"shared_asset", "cluster-ca"},
	} {
		if !hasReferenceUsage(usage, expected.kind, expected.name, "process-exporter") {
			t.Fatalf("missing transitive %s %q for shared sidecar: %#v", expected.kind, expected.name, usage)
		}
	}
}

func TestEffectiveInspectionAppExposesAdvancedConfiguration(t *testing.T) {
	terminationGrace := 45
	app := appModel{
		Name: "api", Strategy: "recreate", SubdomainName: "internal",
		SecurityContext:  map[string]any{"run_as_non_root": true},
		TerminationGrace: &terminationGrace,
		PodInfo:          podInfoSpec{Enabled: true, MountPath: "/etc/podinfo"},
		DownwardAPI: downwardAPISpec{Mounts: []downwardAPIMountSpec{{
			Name: "runtime-info", MountPath: "/etc/runtime-info",
			Items: []downwardAPIItemSpec{{Path: "namespace", FieldPath: "metadata.namespace"}},
		}}},
		DeploymentRaw: map[string]any{"revisionHistoryLimit": 3},
		PodRaw:        map[string]any{"hostNetwork": false},
		RolloutOn: rolloutOnSpec{Checksums: map[string]rolloutChecksumSpec{
			"configuration": {Files: []string{"assets/app.conf"}},
		}},
		Scheduling: schedulingSpec{Arch: "amd64", NodeSelector: map[string]any{"pool": "apps"}},
	}

	effective := effectiveInspectionApp(app, SourceDocument{}, SourceDocument{}, Options{}, nil)
	if effective.Strategy != "recreate" || effective.SubdomainName != "internal" || effective.TerminationGracePeriod == nil || *effective.TerminationGracePeriod != 45 {
		t.Fatalf("scalar advanced fields missing: %#v", effective)
	}
	if effective.SecurityContext["run_as_non_root"] != true || effective.PodInfo["mount_path"] != "/etc/podinfo" {
		t.Fatalf("pod advanced fields missing: %#v", effective)
	}
	if effective.Scheduling["arch"] != "amd64" || effective.DeploymentRaw["revisionHistoryLimit"] != 3 || len(effective.DownwardAPI) == 0 || len(effective.RolloutOn) == 0 {
		t.Fatalf("structured advanced fields missing: %#v", effective)
	}
}

func TestInspectDoesNotOfferResourcesOverrideWithExternalPolicy(t *testing.T) {
	root := editMetamodelFixtureRoot(t)
	policyRoot, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "edit-metamodel", "resources"))
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := Inspect(Options{Environment: "dev", Root: root, ResourcePolicyRoot: policyRoot})
	if err != nil {
		t.Fatal(err)
	}
	java := inspectedAppByName(t, inspection, "java-api.yml")
	processExporter := effectiveContainerByName(t, java.Effective.Sidecars, "process-exporter")
	resources := processExporter.Fields["resources"]
	if resources.Capabilities.CanOverride || resources.WriteTarget != nil || !strings.Contains(resources.Capabilities.Reason, "authoritative") {
		t.Fatalf("external resource policy exposed an ineffective writer: %#v", resources)
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
	if inspection.Defaults == nil || inspection.SharedAssets == nil || len(inspection.Usage) == 0 {
		t.Fatalf("source catalogs unavailable after effective failure: %#v", inspection)
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

func hasReferenceUsage(usage []ReferenceUsage, kind, name, container string) bool {
	for _, item := range usage {
		if item.Kind == kind && item.Name == name && item.Container == container {
			return true
		}
	}
	return false
}

func effectiveContainerByName(t *testing.T, containers []EffectiveContainer, name string) EffectiveContainer {
	t.Helper()
	for _, container := range containers {
		if container.Name == name {
			return container
		}
	}
	t.Fatalf("effective container %s not found", name)
	return EffectiveContainer{}
}

func hasResolvedReferenceUsage(usage []ReferenceUsage, kind, name, app string) bool {
	for _, item := range usage {
		if item.Kind == kind && item.Name == name && item.App == app {
			return true
		}
	}
	return false
}
