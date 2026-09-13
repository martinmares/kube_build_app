package repository

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
    envs:
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
	writeFile(t, filepath.Join(root, "test", "apps", "_defaults.yml"), "arch: amd64\n")
	writeFile(t, filepath.Join(root, "test", "assets.secured.json"), "{\"assets\":{}}\n")
	writeFile(t, filepath.Join(root, "test", "assets.unsecured.json"), "{\"assets\":{}}\n")
	writeFile(t, filepath.Join(root, "test", "env.secured.json"), "{\"environment\":{}}\n")
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), "{\"environment\":{}}\n")
	writeFile(t, filepath.Join(root, "test", "shared.assets.yml"), "assets: []\n")
	writeFile(t, filepath.Join(root, "test", "replica-profiles.yml"), "profiles: []\n")

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
	expected := []string{"_defaults.yml", "assets.secured.json", "assets.unsecured.json", "env.secured.json", "env.unsecured.json", "replica-profiles.yml", "shared.assets.yml", "ssl/cert.pem", "ui/nginx.conf"}
	if len(relativePaths) != len(expected) {
		t.Fatalf("relative paths = %#v, want %#v", relativePaths, expected)
	}
	for i := range expected {
		if relativePaths[i] != expected[i] {
			t.Fatalf("relative paths = %#v, want %#v", relativePaths, expected)
		}
	}
	if assets[0].Driver != "defaults" || assets[1].Driver != "special" || assets[5].Driver != "metadata" || assets[7].Driver != "configmap" {
		t.Fatalf("unexpected drivers: %#v", assets)
	}
}

func TestGitStatusMarksDirtyAppsInNestedEnvironmentRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	project := t.TempDir()
	runGit(t, project, "init")
	envRoot := filepath.Join(project, "envs")
	writeFile(t, filepath.Join(envRoot, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")

	repo, err := New(envRoot)
	if err != nil {
		t.Fatal(err)
	}
	status := repo.GitStatus()
	if !status.Available || !status.Dirty {
		t.Fatalf("git status = %#v, want available dirty repo", status)
	}
	apps, err := repo.Apps("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || !apps[0].IsDirty {
		t.Fatalf("apps dirty state = %#v, want dirty api app", apps)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !detail.IsDirty {
		t.Fatalf("detail IsDirty = false, want true")
	}

	diff := repo.GitDiff("test/apps/api.yml")
	if !diff.Available {
		t.Fatalf("git diff unavailable: %#v", diff)
	}
	if !strings.Contains(diff.Content, "+++ b/test/apps/api.yml") || !strings.Contains(diff.Content, "+name: api") {
		t.Fatalf("unexpected diff content:\n%s", diff.Content)
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
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
    envs:
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
	if len(container.Envs) != 2 || container.Envs[1].Kind != "kubernetes_value_from" || container.Envs[1].Value == nil || !contains(*container.Envs[1].Value, "app-secret") {
		t.Fatalf("unexpected envs model: %#v", container.Envs)
	}
	if len(container.Ports) != 1 || len(container.Ports[0].ExposeAs) != 1 || !container.Ports[0].ExposeAs[0].IngressEnabled {
		t.Fatalf("unexpected ports model: %#v", container.Ports)
	}
	if container.Ports[0].Port == nil || *container.Ports[0].Port != "8080" {
		t.Fatalf("unexpected port model: %#v", container.Ports[0])
	}
}

func TestUpdateAppVarsReplacesOnlyVarsBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `vars:
  - name: APP_NAME
    value: api
name: "{{var:APP_NAME}}"
replicas: 1
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := repo.UpdateAppVars("test", "api.yml", []VarItem{{Name: "APP_NAME", Value: "worker"}, {Name: "PORT", Value: "8080"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(vars.Items) != 2 || vars.Items[0].Value != "worker" || vars.Items[1].Name != "PORT" {
		t.Fatalf("unexpected vars: %#v", vars.Items)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, `value: "worker"`) || !contains(detail.Content, `name: "{{var:APP_NAME}}"`) || !contains(detail.Content, "replicas: 1") {
		t.Fatalf("unexpected updated content:\n%s", detail.Content)
	}
}

func TestUpdateAppVarsRejectsStaleContentHash(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "vars:\n  - name: APP_NAME\n    value: api\nname: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := repo.AppVars("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "vars:\n  - name: APP_NAME\n    value: changed\nname: api\n")

	_, err = repo.UpdateAppVars("test", "api.yml", []VarItem{{Name: "APP_NAME", Value: "worker"}}, vars.ContentHash)
	if !IsConflictError(err) {
		t.Fatalf("UpdateAppVars error = %v, want conflict", err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "value: changed") {
		t.Fatalf("stale write changed content unexpectedly:\n%s", detail.Content)
	}
}

func TestUpdateAppVarsRejectsInvalidNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"BAD-NAME", "1BAD", "BAD.NAME"} {
		_, err := repo.UpdateAppVars("test", "api.yml", []VarItem{{Name: name, Value: "value"}}, "")
		if err == nil || !contains(err.Error(), "invalid variable name") {
			t.Fatalf("UpdateAppVars name %q error = %v, want invalid name", name, err)
		}
	}
}

func TestUpdateAppReplicasReplacesTopLevelReplicas(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\nreplicas: 1\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	replicas, err := repo.UpdateAppReplicas("test", "api.yml", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if replicas.Replicas == nil || *replicas.Replicas != 3 {
		t.Fatalf("replicas = %#v, want 3", replicas.Replicas)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "replicas: 3") || contains(detail.Content, "replicas: 1") {
		t.Fatalf("unexpected content:\n%s", detail.Content)
	}
}

func TestUpdateAppReplicasInsertsAfterVarsBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "vars:\n  - name: APP_NAME\n    value: api\nname: \"{{var:APP_NAME}}\"\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.UpdateAppReplicas("test", "api.yml", 2, ""); err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "vars:\n  - name: APP_NAME\n    value: api\nreplicas: 2\nname:") {
		t.Fatalf("replicas not inserted after vars block:\n%s", detail.Content)
	}
}

func TestUpdateAppReplicasRejectsStaleContentHash(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, "name: api\nreplicas: 1\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	replicas, err := repo.AppReplicas("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, appPath, "name: api\nreplicas: 4\n")

	_, err = repo.UpdateAppReplicas("test", "api.yml", 2, replicas.ContentHash)
	if !IsConflictError(err) {
		t.Fatalf("UpdateAppReplicas error = %v, want conflict", err)
	}
}

func TestUpdateAppAutoscalingWritesBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\nreplicas: 2\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppAutoscaling("test", "api.yml", AutoscalingUpdate{
		Enabled:                  true,
		MinReplicas:              "2",
		MaxReplicas:              "6",
		CPUAverageUtilization:    "75",
		MemoryAverageUtilization: "80",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Autoscaling.Enabled || result.Autoscaling.MaxReplicas == nil || *result.Autoscaling.MaxReplicas != 6 {
		t.Fatalf("autoscaling = %#v, want enabled max 6", result.Autoscaling)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "replicas: 2\nautoscaling:\n  enabled: true\n  min_replicas: 2\n  max_replicas: 6\n  cpu:\n    average_utilization: 75\n  memory:\n    average_utilization: 80\ncontainers:") {
		t.Fatalf("autoscaling block not inserted after replicas:\n%s", detail.Content)
	}
}

func TestUpdateAppAutoscalingRejectsInvalidRanges(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\nreplicas: 2\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		autoscaling AutoscalingUpdate
		want        string
	}{
		{
			name: "enabled requires positive min",
			autoscaling: AutoscalingUpdate{
				Enabled:                  true,
				MinReplicas:              "0",
				MaxReplicas:              "2",
				CPUAverageUtilization:    "75",
				MemoryAverageUtilization: "",
			},
			want: "min_replicas",
		},
		{
			name: "max must be at least min",
			autoscaling: AutoscalingUpdate{
				Enabled:                  true,
				MinReplicas:              "3",
				MaxReplicas:              "2",
				CPUAverageUtilization:    "75",
				MemoryAverageUtilization: "",
			},
			want: "max_replicas",
		},
		{
			name: "utilization has upper bound",
			autoscaling: AutoscalingUpdate{
				Enabled:                  true,
				MinReplicas:              "1",
				MaxReplicas:              "2",
				CPUAverageUtilization:    "101",
				MemoryAverageUtilization: "",
			},
			want: "cpu_average_utilization",
		},
		{
			name: "enabled requires metric",
			autoscaling: AutoscalingUpdate{
				Enabled:     true,
				MinReplicas: "1",
				MaxReplicas: "2",
			},
			want: "metric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.UpdateAppAutoscaling("test", "api.yml", tt.autoscaling, "")
			if err == nil || !contains(err.Error(), tt.want) {
				t.Fatalf("UpdateAppAutoscaling error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestUpdateAppContainerResourcesWritesRequestsLimits(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    resources:
      cpu:
        from: "100m"
        to: "500m"
      memory:
        from: "128Mi"
        to: "512Mi"
      ephemeral-storage:
        from: "64Mi"
        to: "1Gi"
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{
		CPURequest: "200m", CPULimit: "600m", MemoryRequest: "256Mi", MemoryLimit: "768Mi",
		EphemeralStorageRequest: "128Mi", EphemeralStorageLimit: "2Gi",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Resources.CPURequest == nil || *result.Resources.CPURequest != "200m" {
		t.Fatalf("resources = %#v, want cpu request 200m", result.Resources)
	}
	if result.Resources.EphemeralStorageRequest == nil || *result.Resources.EphemeralStorageRequest != "128Mi" {
		t.Fatalf("resources = %#v, want ephemeral storage request 128Mi", result.Resources)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`requests: "200m"`, `limits: "600m"`, `requests: "256Mi"`, `limits: "768Mi"`} {
		if !contains(detail.Content, expected) {
			t.Fatalf("content missing %q:\n%s", expected, detail.Content)
		}
	}
	if contains(detail.Content, "from:") || contains(detail.Content, "to:") {
		t.Fatalf("legacy from/to should be replaced:\n%s", detail.Content)
	}
	if !contains(detail.Content, "ephemeral-storage:\n        requests: \"128Mi\"\n        limits: \"2Gi\"") {
		t.Fatalf("ephemeral storage missing:\n%s", detail.Content)
	}
}

func TestUpdateAppContainerResourcesRejectsInvalidQuantities(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{CPURequest: "1000ABC"}, ""); err == nil || !contains(err.Error(), "cpu_request") {
		t.Fatalf("CPU quantity error = %v, want cpu_request validation", err)
	}
	if _, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{MemoryRequest: "1000ABC"}, ""); err == nil || !contains(err.Error(), "memory_request") {
		t.Fatalf("memory quantity error = %v, want memory_request validation", err)
	}
	if _, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{EphemeralStorageRequest: "1000ABC"}, ""); err == nil || !contains(err.Error(), "ephemeral_storage_request") {
		t.Fatalf("ephemeral storage quantity error = %v, want validation", err)
	}
	if _, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{EphemeralStorageRequest: "1{{var:SIZE}}"}, ""); err == nil || !contains(err.Error(), "ephemeral_storage_request") {
		t.Fatalf("partial template error = %v, want validation", err)
	}
	if _, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{CPURequest: "500m", MemoryRequest: "1000Mi"}, ""); err != nil {
		t.Fatalf("valid quantities rejected: %v", err)
	}
	if _, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{EphemeralStorageLimit: "{{var:EPHEMERAL_LIMIT}}"}, ""); err != nil {
		t.Fatalf("complete resource template rejected: %v", err)
	}
}

func TestUpdateAppContainerResourcesInsertsMissingBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n    image: api:1\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{CPURequest: "100m"}, ""); err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "  - name: api\n    resources:\n      cpu:\n        requests: \"100m\"\n    image: api:1") {
		t.Fatalf("resources not inserted after container name:\n%s", detail.Content)
	}
}

func TestUpdateAppContainerEnvsUpdatesSelectedContainer(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    envs:
      - name: API_ONLY
        value: "1"
  - name: worker
    envs:
      - name: MODE
        value: "old"
    image: worker:1
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppContainerEnvs("test", "api.yml", 1, []ContainerEnvUpdate{
		{SourceIndex: 0, Name: "MODE", Kind: "value", Value: "new"},
		{SourceIndex: -1, Name: "QUEUE", Kind: "value", Value: "critical"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Envs) != 2 || result.Envs[0].Value == nil || *result.Envs[0].Value != "new" {
		t.Fatalf("envs = %#v, want updated MODE", result.Envs)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "API_ONLY") {
		t.Fatalf("first container env was changed unexpectedly:\n%s", detail.Content)
	}
	if !contains(detail.Content, "  - name: worker\n    envs:\n      - name: MODE\n        value: new\n      - name: QUEUE\n        value: critical\n    image: worker:1") {
		t.Fatalf("worker envs not updated in place:\n%s", detail.Content)
	}
}

func TestUpdateAppContainerEnvsPreservesValueFromItems(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    envs:
      - name: PLAIN
        value: "old"
      - name: SECRET_TOKEN
        valueFrom:
          secretKeyRef:
            name: api-secret
            key: token
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppContainerEnvs("test", "api.yml", 0, []ContainerEnvUpdate{
		{SourceIndex: 0, Name: "PLAIN", Kind: "value", Value: "new"},
		{SourceIndex: 1, Name: "SECRET_TOKEN", Kind: "preserve"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Envs) != 2 || result.Envs[1].Kind != "kubernetes_value_from" {
		t.Fatalf("envs = %#v, want preserved raw valueFrom", result.Envs)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, `value: new`) || !contains(detail.Content, "valueFrom:") || !contains(detail.Content, "name: api-secret") {
		t.Fatalf("valueFrom variable was not preserved:\n%s", detail.Content)
	}
}

func TestAppModelPreservesMetamodelEnvReferencesAndEmptyValue(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    envs:
      - name: EMPTY
        value: ""
      - name: TOKEN_FILE
        workload_identity_token_ref_name: runtime-config
      - name: CA_FILE
        shared_asset_ref_name: test-ca
      - name: INHERITED
        remove: true
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.AppModel("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	envs := model.Containers[0].Envs
	if len(envs) != 4 || envs[0].Value == nil || *envs[0].Value != "" || !envs[0].IsValueEditable {
		t.Fatalf("empty value was not preserved: %#v", envs)
	}
	if envs[1].Kind != "workload_identity_token" || envs[1].WorkloadIdentityTokenRefName == nil || *envs[1].WorkloadIdentityTokenRefName != "runtime-config" || !envs[1].IsValueEditable {
		t.Fatalf("token reference was not preserved: %#v", envs[1])
	}
	if envs[2].Kind != "shared_asset" || envs[2].SharedAssetRefName == nil || *envs[2].SharedAssetRefName != "test-ca" || !envs[2].IsValueEditable {
		t.Fatalf("shared asset reference was not preserved: %#v", envs[2])
	}
	if envs[3].Kind != "remove" || !envs[3].Remove || !envs[3].IsValueEditable {
		t.Fatalf("remove env was not preserved: %#v", envs[3])
	}
}

func TestUpdateAppContainerEnvsSupportsBuilderSourcesAndPreservesRawEntries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "_defaults.yml"), `workload_identity:
  tokens:
    - name: runtime-config
      audience: config.example.test
`)
	writeFile(t, filepath.Join(root, "test", "shared.assets.yml"), `assets:
  - name: test-ca
    file: ca.pem
    to: /etc/ssl/test-ca.pem
`)
	appPath := filepath.Join(root, "test", "apps", "api.yml")
	writeFile(t, appPath, `name: api
containers:
  - name: api
    envs:
      - name: PLAIN
        value: "old"
      - name: SECRET
        secret_name: api-secret
        key: token
      - name: CPU_REQUEST
        resource_name: requests.cpu
        divisor: 1m
      - name: POD_NAME
        field_path: metadata.name
      - name: TOKEN_FILE
        workload_identity_token_ref_name: runtime-config
      - name: CA_FILE
        shared_asset_ref_name: test-ca
      - name: INHERITED
        remove: true
      - name: RAW_SECRET
        valueFrom:
          secretKeyRef:
            name: raw-secret
            key: password
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.AppModel("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	envs := model.Containers[0].Envs
	wantKinds := []string{"value", "secret", "resource", "field", "workload_identity_token", "shared_asset", "remove", "kubernetes_value_from"}
	if len(envs) != len(wantKinds) {
		t.Fatalf("envs = %#v", envs)
	}
	for index, kind := range wantKinds {
		if envs[index].Kind != kind {
			t.Fatalf("envs[%d].kind = %q, want %q", index, envs[index].Kind, kind)
		}
	}
	if !envs[1].IsValueEditable || envs[7].IsValueEditable {
		t.Fatalf("builder/raw editability mismatch: %#v", envs)
	}
	if !reflect.DeepEqual(model.WorkloadIdentityTokenRefNames, []string{"runtime-config"}) || !reflect.DeepEqual(model.SharedAssetRefNames, []string{"test-ca"}) {
		t.Fatalf("reference catalogs missing: tokens=%v assets=%v", model.WorkloadIdentityTokenRefNames, model.SharedAssetRefNames)
	}

	updates := []ContainerEnvUpdate{
		{SourceIndex: 0, Name: "PLAIN", Kind: "value", Value: "old"},
		{SourceIndex: 1, Name: "SECRET", Kind: "secret", SecretName: "api-secret", Key: "token"},
		{SourceIndex: 2, Name: "CPU_REQUEST", Kind: "resource", ResourceName: "requests.cpu", Divisor: "1m"},
		{SourceIndex: 3, Name: "POD_NAME", Kind: "field", FieldPath: "metadata.name"},
		{SourceIndex: 4, Name: "TOKEN_FILE", Kind: "workload_identity_token", WorkloadIdentityTokenRefName: "runtime-config"},
		{SourceIndex: 5, Name: "CA_FILE", Kind: "shared_asset", SharedAssetRefName: "test-ca"},
		{SourceIndex: 6, Name: "INHERITED", Kind: "remove"},
		{SourceIndex: 7, Name: "RAW_SECRET", Kind: "preserve"},
	}
	original, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateAppContainerEnvs("test", "api.yml", 0, updates, ""); err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, unchanged) {
		t.Fatalf("no-op env save changed source:\n%s", unchanged)
	}

	updates[1].Key = "credential"
	if _, err := repo.UpdateAppContainerEnvs("test", "api.yml", 0, updates, ""); err != nil {
		t.Fatal(err)
	}
	changed, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(changed)
	if !contains(text, "key: credential") || !contains(text, "valueFrom:") || !contains(text, "name: raw-secret") {
		t.Fatalf("builder update lost raw env entry:\n%s", text)
	}
	if strings.Index(text, "name: SECRET") > strings.Index(text, "name: RAW_SECRET") {
		t.Fatalf("environment entry order changed:\n%s", text)
	}
}

func TestUpdateAppContainerEnvsRejectsMissingPreservedEntryAndUnknownReference(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    envs:
      - name: RAW
        valueFrom:
          fieldRef: {fieldPath: metadata.name}
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateAppContainerEnvs("test", "api.yml", 0, nil, ""); err == nil || !contains(err.Error(), "preserve every read-only") {
		t.Fatalf("missing preserve error = %v", err)
	}
	if _, err := repo.UpdateAppContainerEnvs("test", "api.yml", 0, []ContainerEnvUpdate{
		{SourceIndex: 0, Name: "RAW", Kind: "preserve"},
		{SourceIndex: -1, Name: "TOKEN", Kind: "workload_identity_token", WorkloadIdentityTokenRefName: "missing"},
	}, ""); err == nil || !contains(err.Error(), "unknown workload identity token") {
		t.Fatalf("unknown token error = %v", err)
	}
}

func TestSimpleEditorsRejectAdvancedBlocksWithoutChangingSource(t *testing.T) {
	tests := []struct {
		name    string
		content string
		update  func(*Repository) error
		field   string
	}{
		{
			name:    "resources unknown gpu",
			content: "name: api\ncontainers:\n  - name: api\n    resources:\n      gpu:\n        requests: 1\n",
			update: func(repo *Repository) error {
				_, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{CPURequest: "100m"}, "")
				return err
			},
			field: "gpu",
		},
		{
			name:    "autoscaling raw",
			content: "name: api\nautoscaling:\n  enabled: true\n  min_replicas: 1\n  max_replicas: 2\n  raw:\n    behavior: {}\ncontainers:\n  - name: api\n",
			update: func(repo *Repository) error {
				_, err := repo.UpdateAppAutoscaling("test", "api.yml", AutoscalingUpdate{Enabled: true, MinReplicas: "1", MaxReplicas: "2", CPUAverageUtilization: "80"}, "")
				return err
			},
			field: "raw",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "test", "apps", "api.yml")
			writeFile(t, path, tt.content)
			repo, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			err = tt.update(repo)
			if err == nil || !contains(err.Error(), tt.field) {
				t.Fatalf("error = %v, want unsupported %s", err, tt.field)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != tt.content {
				t.Fatalf("source changed after rejected update:\n%s", content)
			}
		})
	}
}

func TestDefaultsContainerEnvsEditorRejectsAdvancedValuesWithoutChangingSource(t *testing.T) {
	root := t.TempDir()
	content := "container_envs:\n  - container_ref_name: \"*\"\n    envs:\n      - name: CONFIG_TOKEN\n        workload_identity_token_ref_name: runtime-config\n"
	path := filepath.Join(root, "test", "apps", "_defaults.yml")
	writeFile(t, path, content)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.Defaults("test")
	if err != nil {
		t.Fatal(err)
	}
	if got := model.ContainerEnvs[0].Envs[0]; got.Kind != "workload_identity_token" || got.WorkloadIdentityTokenRefName == nil {
		t.Fatalf("advanced defaults env not exposed read-only: %#v", got)
	}
	_, err = repo.UpdateDefaultsContainerEnvs("test", []ContainerEnvGroupUpdate{{
		ContainerRefName: "*", Envs: []VarItem{{Name: "NEW_VALUE", Value: "fixture"}},
	}}, model.ContentHash)
	if err == nil || !contains(err.Error(), "advanced environment value") {
		t.Fatalf("error = %v, want advanced environment value guard", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != content {
		t.Fatalf("source changed after rejected update:\n%s", after)
	}
}

func TestUpdateAppContainerRuntimeWritesJavaRuntime(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n    image: api:1\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppContainerRuntime("test", "api.yml", 0, JavaRuntimeUpdate{
		Xms:           "512m",
		Xmx:           "1536m",
		Opts:          []string{"-XX:+UseG1GC", "-Dspring.profiles.active=prod"},
		ExportEnvName: "APP_JAVA_OPTS",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Runtime.Java.Enabled || result.Runtime.Java.Xmx == nil || *result.Runtime.Java.Xmx != "1536m" {
		t.Fatalf("runtime = %#v, want java xmx 1536m", result.Runtime)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "  - name: api\n    runtime:\n      java:\n        xms: \"512m\"\n        xmx: \"1536m\"\n        opts:\n          - \"-XX:+UseG1GC\"\n          - \"-Dspring.profiles.active=prod\"\n        export:\n          env_name: APP_JAVA_OPTS\n    image: api:1") {
		t.Fatalf("runtime block not inserted after container name:\n%s", detail.Content)
	}
}

func TestUpdateAppContainerRuntimeRejectsInvalidFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.UpdateAppContainerRuntime("test", "api.yml", 0, JavaRuntimeUpdate{Xms: "1000ABC"}, ""); err == nil || !contains(err.Error(), "xms") {
		t.Fatalf("runtime xms error = %v, want xms validation", err)
	}
	if _, err := repo.UpdateAppContainerRuntime("test", "api.yml", 0, JavaRuntimeUpdate{ExportEnvName: "JAVA-OPTS"}, ""); err == nil || !contains(err.Error(), "export_env_name") {
		t.Fatalf("runtime export_env_name error = %v, want export env validation", err)
	}
}

func TestUpdateAppContainerPortsWritesServiceAndExternal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n    image: api:1\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppContainerPorts("test", "api.yml", 0, []PortUpdate{{
		Name:    "http",
		Port:    "8080",
		Metrics: true,
		ExposeAs: []ExposeUpdate{{
			ServiceName: "api",
			Port:        "80",
			Externals: []ExternalUpdate{{
				Name: "api-public",
				HTTP: []ExternalHostUpdate{{Hostname: "api.example.test", Path: "/"}},
			}},
		}},
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ports) != 1 || result.Ports[0].ExposeAs[0].ServiceName == nil || *result.Ports[0].ExposeAs[0].ServiceName != "api" {
		t.Fatalf("ports = %#v, want service_name api", result.Ports)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"ports:", "name: http", "port: 8080", "metrics: true", "service_name: api", "name: api-public", "hostname: api.example.test"} {
		if !contains(detail.Content, expected) {
			t.Fatalf("ports block missing %q:\n%s", expected, detail.Content)
		}
	}
}

func TestUpdateAppContainerPortsPreservesAdvancedFieldsAndNoOp(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "apps", "api.yml")
	original := `name: api
containers:
  - name: api
    ports:
      - name: http
        port: 8080
        expose_as:
          - service_name: api
            port: 80
            external:
              - name: api-public
                annotations:
                  nginx.ingress.kubernetes.io/rewrite-target: /
                labels:
                  exposure: public
                http:
                  - hostname: api.example.test
                    path: /
                    x-host-option: keep
                tls:
                  termination: edge
                  certificate: keep-secret-data
`
	writeFile(t, path, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	update := []PortUpdate{{SourceIndex: 0, Name: "http", Port: "8080", ExposeAs: []ExposeUpdate{{SourceIndex: 0, ServiceName: "api", Port: "80", Externals: []ExternalUpdate{{
		SourceIndex: 0, Name: "api-public", HTTP: []ExternalHostUpdate{{SourceIndex: 0, Hostname: "api.example.test", Path: "/"}},
		Annotations: []VarItem{{Name: "nginx.ingress.kubernetes.io/rewrite-target", Value: "/"}}, Labels: []VarItem{{Name: "exposure", Value: "public"}}, TLSTermination: "edge",
	}}}}}}
	if _, err = repo.UpdateAppContainerPorts("test", "api.yml", 0, update, ""); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("no-op ports update changed source:\n%s", content)
	}
	update[0].ExposeAs[0].Externals[0].Labels[0].Value = "external"
	if _, err = repo.UpdateAppContainerPorts("test", "api.yml", 0, update, ""); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"certificate: keep-secret-data", "x-host-option: keep", "exposure: external"} {
		if !contains(string(content), expected) {
			t.Fatalf("updated ports missing %q:\n%s", expected, content)
		}
	}
}

func TestUpdateAppContainerProbesWritesModernBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n    image: api:1\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppContainerProbes("test", "api.yml", 0, ProbeUpdate{Preset: "spring-actuator", Port: "8080", Path: "/actuator/health"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Probes.Preset == nil || *result.Probes.Preset != "spring-actuator" {
		t.Fatalf("probes = %#v, want spring-actuator preset", result.Probes)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "  - name: api\n    probes:\n      path: /actuator/health\n      port: 8080\n      preset: spring-actuator\n    image: api:1") {
		t.Fatalf("probes not inserted after container name:\n%s", detail.Content)
	}
}

func TestUpdateAppContainerProbesPreservesAdvancedAndUnknownFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "apps", "api.yml")
	original := `name: api
containers:
  - name: api
    probes:
      preset: spring-actuator
      path:
        live: /live
        ready: /ready
        extension: /keep
      http:
        port: 8080
        x-http-option: keep
      live:
        http: {path: /live-explicit, port: 8081, x-handler: keep}
        delay: 0
        period: 10
        timeout: 2
        success: 1
        failure: 5
        x-live-option: keep
      ready:
        command: ["/bin/check", "ready"]
        period: 3
      start:
        http: {path: /start, port: 8082}
        failure: 30
      x-probe-option: keep
    image: api:1
`
	writeFile(t, path, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	update := ProbeUpdate{
		Preset:     "spring-actuator",
		PathByType: map[string]string{"live": "/live", "ready": "/ready"},
		HTTP:       ProbeHTTPUpdate{Port: "8080"},
		Live:       ProbeHealthUpdate{HTTP: ProbeHTTPUpdate{Port: "8081", Path: "/live-explicit"}, Delay: "0", Period: "10", Timeout: "2", Success: "1", Failure: "5"},
		Ready:      ProbeHealthUpdate{Command: []string{"/bin/check", "ready"}, Period: "3"},
		Start:      ProbeHealthUpdate{HTTP: ProbeHTTPUpdate{Port: "8082", Path: "/start"}, Failure: "30"},
	}
	result, err := repo.UpdateAppContainerProbes("test", "api.yml", 0, update, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Probes.Live.HTTP.Port == nil || *result.Probes.Live.HTTP.Port != "8081" || len(result.Probes.Ready.Command) != 2 {
		t.Fatalf("advanced probes model = %#v", result.Probes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("no-op probe update changed source:\n%s", content)
	}

	update.Ready.Period = "4"
	if _, err := repo.UpdateAppContainerProbes("test", "api.yml", 0, update, ""); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"x-probe-option: keep", "x-http-option: keep", "x-handler: keep", "x-live-option: keep", "extension: /keep", "period: 4"} {
		if !contains(string(content), expected) {
			t.Fatalf("updated probes missing %q:\n%s", expected, content)
		}
	}
}

func TestUpdateAppContainerProbesRejectsInvalidPortRange(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, port := range []string{"0", "70000", "abc"} {
		_, err := repo.UpdateAppContainerProbes("test", "api.yml", 0, ProbeUpdate{Preset: "spring-actuator", Port: port}, "")
		if err == nil || !contains(err.Error(), "port") {
			t.Fatalf("UpdateAppContainerProbes port %q error = %v, want port validation", port, err)
		}
	}
	if _, err := repo.UpdateAppContainerProbes("test", "api.yml", 0, ProbeUpdate{Preset: "spring-actuator", Port: "8080"}, ""); err != nil {
		t.Fatalf("valid probe port rejected: %v", err)
	}
	if _, err := repo.UpdateAppContainerProbes("test", "api.yml", 0, ProbeUpdate{Live: ProbeHealthUpdate{Period: "0"}}, ""); err == nil || !contains(err.Error(), "live.period") {
		t.Fatalf("invalid live period error = %v", err)
	}
	if _, err := repo.UpdateAppContainerProbes("test", "api.yml", 0, ProbeUpdate{Start: ProbeHealthUpdate{Failure: "{{var:START_FAILURE}}"}}, ""); err != nil {
		t.Fatalf("probe timing template rejected: %v", err)
	}
}

func TestUpdateAppContainerProbesRejectsUnsafeKnownShapeWithoutChangingSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "apps", "api.yml")
	original := "name: api\ncontainers:\n  - name: api\n    probes:\n      http:\n        path:\n          live: /live\n          ready: /ready\n"
	writeFile(t, path, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.UpdateAppContainerProbes("test", "api.yml", 0, ProbeUpdate{HTTP: ProbeHTTPUpdate{Path: "/health"}}, "")
	if err == nil || !contains(err.Error(), "path mapping") {
		t.Fatalf("error = %v, want unsafe path mapping", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != original {
		t.Fatalf("source changed after rejected update:\n%s", content)
	}
}

func TestFixAppContainerLegacyProbeRenamesBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    probe:
      live:
        http:
          port: 8080
          path: /live
    image: api:1
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.FixAppContainerLegacyProbes("test", "api.yml", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Probes.Legacy {
		t.Fatalf("probes = %#v, want no legacy probes", result.Probes)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if contains(detail.Content, "    probe:") || !contains(detail.Content, "    probes:\n      live:\n        http:\n          port: 8080\n          path: /live") {
		t.Fatalf("legacy probe was not renamed:\n%s", detail.Content)
	}
}

func TestFixAppContainerLegacyHealthCopiesLiveReady(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
containers:
  - name: api
    health:
      http:
        port: 8080
        path:
          live: /live
          ready: /ready
    image: api:1
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.FixAppContainerLegacyProbes("test", "api.yml", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Probes.Enabled || result.Probes.Legacy {
		t.Fatalf("probes = %#v, want modern custom probes", result.Probes)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if contains(detail.Content, "    health:") {
		t.Fatalf("legacy health was not removed:\n%s", detail.Content)
	}
	for _, expected := range []string{
		"    probes:\n      live:\n        http:\n          port: 8080",
		"      ready:\n        http:\n          port: 8080",
		"          ready: /ready",
	} {
		if !contains(detail.Content, expected) {
			t.Fatalf("content missing %q:\n%s", expected, detail.Content)
		}
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
	writeFile(t, filepath.Join(root, "test", "apps", "_defaults.yml"), "arch: amd64\n")
	defaults, err := repo.AssetDetail("test", "_defaults.yml")
	if err != nil {
		t.Fatal(err)
	}
	if defaults.RelativePath != "_defaults.yml" || !contains(defaults.Content, "arch: amd64") {
		t.Fatalf("unexpected defaults detail: %#v", defaults)
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

func TestDefaultsReadsAndUpdatesVarsAndContainerEnvs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "_defaults.yml"), `vars:
  - name: APP_NAME
    value: api
arch: amd64
container_envs:
  - container_ref_name: "*"
    envs:
      - name: LOG_LEVEL
        value: INFO
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	defaults, err := repo.Defaults("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults.Vars) != 1 || defaults.Vars[0].Name != "APP_NAME" || len(defaults.ContainerEnvs) != 1 || defaults.ContainerEnvs[0].ContainerRefName != "*" {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}

	defaults, err = repo.UpdateDefaultsVars("test", []VarItem{{Name: "APP_NAME", Value: "worker"}, {Name: "PORT", Value: "8080"}}, defaults.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults.Vars) != 2 || defaults.Vars[1].Name != "PORT" {
		t.Fatalf("unexpected vars after update: %#v", defaults.Vars)
	}

	defaults, err = repo.UpdateDefaultsContainerEnvs("test", []ContainerEnvGroupUpdate{{ContainerRefName: "*", Envs: []VarItem{{Name: "LOG_LEVEL", Value: "DEBUG"}}}, {ContainerRefName: "api", Envs: []VarItem{{Name: "API_ONLY", Value: "true"}}}}, defaults.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults.ContainerEnvs) != 2 || defaults.ContainerEnvs[1].ContainerRefName != "api" {
		t.Fatalf("unexpected container env defaults after update: %#v", defaults.ContainerEnvs)
	}
	detail, err := repo.AssetDetail("test", "_defaults.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, "container_envs:\n  - container_ref_name: \"*\"\n    envs:\n      - name: LOG_LEVEL\n        value: \"DEBUG\"\n  - container_ref_name: \"api\"") {
		t.Fatalf("defaults content not updated as expected:\n%s", detail.Content)
	}
}

func TestUpdateSpecialEntriesUpdatesEnvUnsecuredJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{"A":"one","COUNT":1}}`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.SpecialEntries("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := repo.UpdateSpecialEntries("test", "env.unsecured.json", []SpecialEntry{{Key: "A", ValueType: "string", ValueText: "two"}, {Key: "COUNT", ValueType: "number", ValueText: "2"}, {Key: "FLAG", ValueType: "bool", ValueText: "true"}}, entries.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Entries) != 3 {
		t.Fatalf("entries len = %d, want 3: %#v", len(updated.Entries), updated.Entries)
	}
	detail, err := repo.AssetDetail("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, `"A":"two"`) || !contains(detail.Content, `"COUNT":2`) || !contains(detail.Content, `"FLAG":true`) {
		t.Fatalf("unexpected env.unsecured.json content:\n%s", detail.Content)
	}
}

func TestUpdateSpecialEntriesRejectsInvalidKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{"environment":{}}`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.SpecialEntries("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.UpdateSpecialEntries("test", "env.unsecured.json", []SpecialEntry{{Key: "BAD-NAME", ValueType: "string", ValueText: "value"}}, entries.ContentHash)
	if err == nil || !contains(err.Error(), "invalid entry key") {
		t.Fatalf("UpdateSpecialEntries error = %v, want invalid entry key", err)
	}
}

func TestSpecialEntriesPreservesEnvUnsecuredJSONOrder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "env.unsecured.json"), `{
  "_public_key": "keep",
  "environment": {
    "B": "two",
    "A": "one",
    "COUNT": 1
  },
  "other": true
}
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := repo.SpecialEntries("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, entry := range entries.Entries {
		keys = append(keys, entry.Key)
	}
	if strings.Join(keys, ",") != "B,A,COUNT" {
		t.Fatalf("entry order = %q, want B,A,COUNT", strings.Join(keys, ","))
	}
}

func TestUpdateSpecialEntriesPatchesEnvUnsecuredJSONWithoutReordering(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "env.unsecured.json")
	writeFile(t, path, `{
  "_public_key": "keep",
  "environment": {
    "B": "two",
    "A": "one",
    "COUNT": 1
  },
  "other": true
}
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.SpecialEntries("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.UpdateSpecialEntries("test", "env.unsecured.json", []SpecialEntry{
		{Key: "B", ValueType: "string", ValueText: "two"},
		{Key: "A", ValueType: "string", ValueText: "changed"},
		{Key: "COUNT", ValueType: "number", ValueText: "1"},
	}, entries.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "_public_key": "keep",
  "environment": {
    "B": "two",
    "A": "changed",
    "COUNT": 1
  },
  "other": true
}
`
	if string(gotBytes) != want {
		t.Fatalf("content changed unexpectedly:\n%s", string(gotBytes))
	}
}

func TestUpdateSpecialEntriesAppendsAndDeletesEnvUnsecuredJSONWithoutReordering(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "env.unsecured.json")
	writeFile(t, path, `{
  "environment": {
    "B": "two",
    "A": "one",
    "COUNT": 1
  },
  "other": true
}
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.SpecialEntries("test", "env.unsecured.json")
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.UpdateSpecialEntries("test", "env.unsecured.json", []SpecialEntry{
		{Key: "B", ValueType: "string", ValueText: "two"},
		{Key: "COUNT", ValueType: "number", ValueText: "1"},
		{Key: "NEW", ValueType: "bool", ValueText: "true"},
	}, entries.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "environment": {
    "B": "two",
    "COUNT": 1,
    "NEW": true
  },
  "other": true
}
`
	if string(gotBytes) != want {
		t.Fatalf("content changed unexpectedly:\n%s", string(gotBytes))
	}
}

func TestPatchEnvSecuredJSONKeepsExistingOrderForFutureEncryptedWrite(t *testing.T) {
	content := `{
  "_public_key": "keep",
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`

	got, err := replaceSpecialEntries(content, "environment", []SpecialEntry{
		{Key: "SECRET_B", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<b>]"},
		{Key: "SECRET_A", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<changed>]"},
		{Key: "SECRET_COUNT", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<count>]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "_public_key": "keep",
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<changed>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`
	if got != want {
		t.Fatalf("content changed unexpectedly:\n%s", got)
	}
}

func TestPatchEnvSecuredJSONAppendsAndDeletesForFutureEncryptedWrite(t *testing.T) {
	content := `{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`

	got, err := replaceSpecialEntries(content, "environment", []SpecialEntry{
		{Key: "SECRET_B", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<b>]"},
		{Key: "SECRET_COUNT", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<count>]"},
		{Key: "SECRET_NEW", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<new>]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]",
    "SECRET_NEW": "EncJson[@api=2.0:@box=<new>]"
  },
  "other": true
}
`
	if got != want {
		t.Fatalf("content changed unexpectedly:\n%s", got)
	}
}

func TestPatchEnvSecuredJSONInsertsNewEntryAtSubmittedPosition(t *testing.T) {
	content := `{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`

	got, err := replaceSpecialEntries(content, "environment", []SpecialEntry{
		{Key: "SECRET_B", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<b>]"},
		{Key: "SECRET_B_2", ValueType: "string", ValueText: "plain-new"},
		{Key: "SECRET_A", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<a>]"},
		{Key: "SECRET_COUNT", ValueType: "string", ValueText: "EncJson[@api=2.0:@box=<count>]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "environment": {
    "SECRET_B": "EncJson[@api=2.0:@box=<b>]",
    "SECRET_B_2": "plain-new",
    "SECRET_A": "EncJson[@api=2.0:@box=<a>]",
    "SECRET_COUNT": "EncJson[@api=2.0:@box=<count>]"
  },
  "other": true
}
`
	if got != want {
		t.Fatalf("content changed unexpectedly:\n%s", got)
	}
}

func TestUpdateAppReferencesPreservesOrderAndUnrelatedYAML(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), `container_profiles:
  - name: base
    defaults: {image: base}
  - name: java
    defaults: {image: java}
sidecar_definitions:
  - name: exporter
    image: exporter
runtime_asset_definitions:
  - name: config
    files: []
  - name: trust
    files: []
`)
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	writeFile(t, appPath, `name: api
# preserve this comment
replicas: 2
runtime_asset_ref_names:
  - config
containers:
  - name: api
    profile_ref_names:
      - base
    resources:
      cpu: {from: 100m, to: 500m}
sidecars:
  - name: local-gateway
    image: gateway:test
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppReferences("dev", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Containers) != 1 || current.Containers[0].Name != "api" || len(current.Sidecars) != 1 || current.Sidecars[0].Name != "local-gateway" {
		t.Fatalf("unexpected scopes: %#v", current)
	}
	if _, err := repo.UpdateAppReferences("dev", "api.yml", AppReferencesUpdate{RuntimeAssetRefNames: current.RuntimeAssetRefNames, Containers: current.Containers, Sidecars: current.Sidecars}, current.ContentHash, current.DefaultsHash); err != nil {
		t.Fatal(err)
	}
	noOpContent, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(noOpContent) != `name: api
# preserve this comment
replicas: 2
runtime_asset_ref_names:
  - config
containers:
  - name: api
    profile_ref_names:
      - base
    resources:
      cpu: {from: 100m, to: 500m}
sidecars:
  - name: local-gateway
    image: gateway:test
` {
		t.Fatalf("no-op reference save changed YAML:\n%s", noOpContent)
	}
	current, err = repo.AppReferences("dev", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	update := AppReferencesUpdate{
		SidecarRefNames:      []string{"exporter"},
		RuntimeAssetRefNames: []string{"trust", "config"},
		Containers:           []ContainerReferences{{Index: 0, Name: "api", ProfileRefNames: []string{"java", "base"}, RuntimeAssetRefNames: []string{"config"}}},
		Sidecars:             []ContainerReferences{{Index: 0, Name: "local-gateway", ProfileRefNames: []string{"base"}, RuntimeAssetRefNames: []string{"trust"}}},
	}
	updated, err := repo.UpdateAppReferences("dev", "api.yml", update, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(updated.RuntimeAssetRefNames, ",") != "trust,config" || strings.Join(updated.Containers[0].ProfileRefNames, ",") != "java,base" {
		t.Fatalf("reference order was not preserved: %#v", updated)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, preserved := range []string{"# preserve this comment", "replicas: 2", "cpu: {from: 100m, to: 500m}", "image: gateway:test"} {
		if !strings.Contains(string(content), preserved) {
			t.Fatalf("updated YAML lost %q:\n%s", preserved, content)
		}
	}
	if !strings.Contains(string(content), "      - java\n      - base") {
		t.Fatalf("ordered profiles missing:\n%s", content)
	}
}

func TestUpdateAppReferencesRejectsStaleDefaultsAndUnknownReference(t *testing.T) {
	root := t.TempDir()
	defaultsPath := filepath.Join(root, "dev", "apps", "_defaults.yml")
	writeFile(t, defaultsPath, "container_profiles:\n  - name: base\n")
	writeFile(t, filepath.Join(root, "dev", "apps", "api.yml"), "name: api\ncontainers:\n  - name: api\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppReferences("dev", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	update := AppReferencesUpdate{Containers: []ContainerReferences{{Index: 0, Name: "api", ProfileRefNames: []string{"base"}}}}
	writeFile(t, defaultsPath, "container_profiles:\n  - name: base\n  - name: changed\n")
	if _, err := repo.UpdateAppReferences("dev", "api.yml", update, current.ContentHash, current.DefaultsHash); !IsConflictError(err) {
		t.Fatalf("stale defaults error = %v, want conflict", err)
	}
	current, err = repo.AppReferences("dev", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	update.Containers[0].ProfileRefNames = []string{"missing"}
	if _, err := repo.UpdateAppReferences("dev", "api.yml", update, current.ContentHash, current.DefaultsHash); err == nil || !strings.Contains(err.Error(), "unknown definition") {
		t.Fatalf("unknown reference error = %v", err)
	}
}

func TestUpdateAppReferencesRequiresExplicitSharedPatchRemoval(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), "sidecar_definitions:\n  - name: exporter\n    image: exporter:1\n")
	appPath := filepath.Join(root, "dev", "apps", "api.yml")
	writeFile(t, appPath, `name: api
sidecar_ref_names:
  - exporter
sidecars:
  - name: gateway
    image: gateway:1
  - name: exporter
    # remove this patch with the reference
    envs:
      - name: MODE
        value: custom
containers:
  - name: api
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.AppReferences("dev", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	update := AppReferencesUpdate{Containers: current.Containers, Sidecars: current.Sidecars}
	if _, err := repo.UpdateAppReferences("dev", "api.yml", update, current.ContentHash, current.DefaultsHash); err == nil || !strings.Contains(err.Error(), "explicit removal") {
		t.Fatalf("missing patch removal error = %v", err)
	}
	update.RemoveSidecarPatches = []string{"exporter"}
	result, err := repo.UpdateAppReferences("dev", "api.yml", update, current.ContentHash, current.DefaultsHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SidecarRefNames) != 0 || len(result.Sidecars) != 1 || result.Sidecars[0].Name != "gateway" {
		t.Fatalf("unexpected references after removal: %#v", result)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "name: exporter") || strings.Contains(string(content), "remove this patch") || !strings.Contains(string(content), "name: gateway") {
		t.Fatalf("atomic reference/patch removal damaged YAML:\n%s", content)
	}
}

func TestUpdateDefaultsSidecarDefinitionImagePreservesOtherFields(t *testing.T) {
	root := t.TempDir()
	defaultsPath := filepath.Join(root, "dev", "apps", "_defaults.yml")
	original := `# catalog comment
sidecar_definitions:
  - name: exporter
    image: "{{env:REGISTRY_URL}}/exporter:{{var:VERSION}}"
    # keep startup
    startup:
      command: ["/exporter"]
    envs:
      - name: MODE
        value: java
  - name: proxy
    image: proxy:1
`
	writeFile(t, defaultsPath, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	current, err := repo.DefaultsSidecarDefinition("dev", "exporter")
	if err != nil {
		t.Fatal(err)
	}
	if current.Image != "{{env:REGISTRY_URL}}/exporter:{{var:VERSION}}" {
		t.Fatalf("raw image = %q", current.Image)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinition("dev", "exporter", DefaultsSidecarDefinitionUpdate{Image: current.Image}, current.ContentHash); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != original {
		t.Fatalf("no-op update changed source:\n%s", got)
	}

	updated, err := repo.UpdateDefaultsSidecarDefinition("dev", "exporter", DefaultsSidecarDefinitionUpdate{Image: "registry.example/exporter:2"}, current.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Image != "registry.example/exporter:2" {
		t.Fatalf("updated image = %q", updated.Image)
	}
	content, err = os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	for _, preserved := range []string{"# catalog comment", "# keep startup", `command: ["/exporter"]`, "value: java", "image: proxy:1"} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("updated source lost %q:\n%s", preserved, got)
		}
	}
	if !strings.Contains(got, `image: "registry.example/exporter:2"`) {
		t.Fatalf("updated source missing image:\n%s", got)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinition("dev", "exporter", DefaultsSidecarDefinitionUpdate{Image: "exporter:3"}, current.ContentHash); !IsConflictError(err) {
		t.Fatalf("stale update error = %v, want conflict", err)
	}
}

func TestUpdateDefaultsSidecarDefinitionStartupPreservesRawTemplatesAndOtherFields(t *testing.T) {
	root := t.TempDir()
	defaultsPath := filepath.Join(root, "dev", "apps", "_defaults.yml")
	original := `sidecar_definitions:
  - name: proxy
    image: "{{env:REGISTRY_URL}}/proxy:1"
    # startup comment
    startup:
      command: ["/bin/{{var:RUNNER}}"]
      arguments:
        - "--listen={{env:LISTEN}}"
    envs:
      - name: MODE
        value: strict
  - name: exporter
    image: exporter:1
`
	writeFile(t, defaultsPath, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	current, err := repo.DefaultsSidecarDefinitionStartup("dev", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	if current.Startup == nil || !current.Startup.CommandPresent || !current.Startup.ArgumentsPresent ||
		len(current.Startup.Command) != 1 || current.Startup.Command[0] != "/bin/{{var:RUNNER}}" ||
		len(current.Startup.Arguments) != 1 || current.Startup.Arguments[0] != "--listen={{env:LISTEN}}" {
		t.Fatalf("unexpected raw startup: %#v", current.Startup)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionStartup("dev", "proxy", DefaultsSidecarDefinitionStartupUpdate{Action: "set", Startup: *current.Startup}, current.ContentHash); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("no-op startup update changed source:\n%s", content)
	}

	updated, err := repo.UpdateDefaultsSidecarDefinitionStartup("dev", "proxy", DefaultsSidecarDefinitionStartupUpdate{
		Action: "set",
		Startup: SidecarStartupModel{
			CommandPresent: true, Command: []string{},
			ArgumentsPresent: true, Arguments: []string{"--config={{env:CONFIG_PATH}}"},
		},
	}, current.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Startup == nil || !updated.Startup.CommandPresent || len(updated.Startup.Command) != 0 || updated.Startup.Arguments[0] != "--config={{env:CONFIG_PATH}}" {
		t.Fatalf("unexpected updated startup: %#v", updated.Startup)
	}
	content, err = os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	for _, expected := range []string{`command: []`, `- "--config={{env:CONFIG_PATH}}"`, `image: "{{env:REGISTRY_URL}}/proxy:1"`, "value: strict", "image: exporter:1"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, got)
		}
	}

	removed, err := repo.UpdateDefaultsSidecarDefinitionStartup("dev", "proxy", DefaultsSidecarDefinitionStartupUpdate{Action: "remove"}, updated.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Startup != nil {
		t.Fatalf("startup after remove = %#v", removed.Startup)
	}
	content, err = os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "startup:") || !strings.Contains(string(content), "value: strict") {
		t.Fatalf("remove damaged source:\n%s", content)
	}
}

func TestDefaultsSidecarDefinitionStartupRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), `sidecar_definitions:
  - name: proxy
    startup:
      command: [proxy]
      working_directory: /app
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DefaultsSidecarDefinitionStartup("dev", "proxy"); err == nil || !strings.Contains(err.Error(), "working_directory") {
		t.Fatalf("unknown startup field error = %v", err)
	}
}

func TestUpdateDefaultsSidecarDefinitionResourcesPreservesTemplatesDialectAndOtherFields(t *testing.T) {
	root := t.TempDir()
	defaultsPath := filepath.Join(root, "dev", "apps", "_defaults.yml")
	original := `sidecar_definitions:
  - name: exporter
    image: exporter:1
    # resources comment
    resources:
      cpu: {from: "{{var:EXPORTER_CPU_REQUEST}}", to: 20m}
      memory:
        from: 16Mi
        to: "{{env:EXPORTER_MEMORY_LIMIT}}"
      ephemeral-storage:
        from: 32Mi
        to: "{{var:EXPORTER_EPHEMERAL_LIMIT}}"
    envs:
      - name: MODE
        value: strict
  - name: proxy
    image: proxy:1
`
	writeFile(t, defaultsPath, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	current, err := repo.DefaultsSidecarDefinitionResources("dev", "exporter")
	if err != nil {
		t.Fatal(err)
	}
	if current.Resources == nil || current.Resources.CPURequest == nil || *current.Resources.CPURequest != "{{var:EXPORTER_CPU_REQUEST}}" ||
		current.Resources.MemoryLimit == nil || *current.Resources.MemoryLimit != "{{env:EXPORTER_MEMORY_LIMIT}}" ||
		current.Resources.EphemeralStorageLimit == nil || *current.Resources.EphemeralStorageLimit != "{{var:EXPORTER_EPHEMERAL_LIMIT}}" {
		t.Fatalf("raw resources = %#v", current.Resources)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionResources("dev", "exporter", DefaultsSidecarDefinitionResourcesUpdate{
		Action: "set", Resources: resourceUpdateFromModel(*current.Resources),
	}, current.ContentHash); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("no-op resources update changed source:\n%s", content)
	}

	updated, err := repo.UpdateDefaultsSidecarDefinitionResources("dev", "exporter", DefaultsSidecarDefinitionResourcesUpdate{
		Action: "set", Resources: ResourceUpdate{
			CPURequest: "5m", CPULimit: "25m", MemoryRequest: "{{var:MEMORY_REQUEST}}", MemoryLimit: "64Mi",
			EphemeralStorageRequest: "64Mi", EphemeralStorageLimit: "256Mi",
		},
	}, current.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Resources == nil || updated.Resources.CPUFrom == nil || *updated.Resources.CPUFrom != "5m" || updated.Resources.MemoryFrom == nil ||
		updated.Resources.EphemeralStorageFrom == nil || *updated.Resources.EphemeralStorageFrom != "64Mi" {
		t.Fatalf("updated resources lost source dialect: %#v", updated.Resources)
	}
	content, err = os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	for _, expected := range []string{`from: "5m"`, `to: "25m"`, `from: "{{var:MEMORY_REQUEST}}"`, "value: strict", "image: proxy:1"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, got)
		}
	}

	removed, err := repo.UpdateDefaultsSidecarDefinitionResources("dev", "exporter", DefaultsSidecarDefinitionResourcesUpdate{Action: "remove"}, updated.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Resources != nil {
		t.Fatalf("resources after remove = %#v", removed.Resources)
	}
	content, err = os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "resources:") || !strings.Contains(string(content), "value: strict") {
		t.Fatalf("remove damaged source:\n%s", content)
	}
}

func TestDefaultsSidecarDefinitionResourcesSupportsEphemeralStorageAndRejectsInvalidValues(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dev", "apps", "_defaults.yml")
	writeFile(t, path, `sidecar_definitions:
  - name: exporter
    resources:
      cpu:
        requests: 5m
      ephemeral-storage:
        requests: 1Gi
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := repo.DefaultsSidecarDefinitionResources("dev", "exporter")
	if err != nil {
		t.Fatal(err)
	}
	if resources.Resources == nil || resources.Resources.EphemeralStorageRequest == nil || *resources.Resources.EphemeralStorageRequest != "1Gi" {
		t.Fatalf("ephemeral storage not parsed: %#v", resources.Resources)
	}

	writeFile(t, path, `sidecar_definitions:
  - name: exporter
    resources:
      cpu: {requests: 5m}
`)
	current, err := repo.DefaultsSidecarDefinitionResources("dev", "exporter")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionResources("dev", "exporter", DefaultsSidecarDefinitionResourcesUpdate{
		Action: "set", Resources: ResourceUpdate{CPURequest: "1000ABC"},
	}, current.ContentHash); err == nil || !strings.Contains(err.Error(), "cpu_request") {
		t.Fatalf("invalid resource error = %v", err)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionResources("dev", "exporter", DefaultsSidecarDefinitionResourcesUpdate{
		Action: "set", Resources: ResourceUpdate{EphemeralStorageLimit: "1000ABC"},
	}, current.ContentHash); err == nil || !strings.Contains(err.Error(), "ephemeral_storage_limit") {
		t.Fatalf("invalid ephemeral storage error = %v", err)
	}

	writeFile(t, path, `sidecar_definitions:
  - name: exporter
    resources:
      cpu: {requests: 5m, from: 2m}
`)
	if _, err := repo.DefaultsSidecarDefinitionResources("dev", "exporter"); err == nil || !strings.Contains(err.Error(), "both requests and from") {
		t.Fatalf("ambiguous resource dialect error = %v", err)
	}
}

func TestUpdateDefaultsSidecarDefinitionEnvsPreservesTypesOrderAndRawTemplates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dev", "apps", "_defaults.yml")
	original := `workload_identity:
  tokens:
    - name: runtime-config
      audience: config
sidecar_definitions:
  - name: proxy
    image: proxy:1
    # env comment
    envs:
      - name: MODE
        value: "{{var:PROXY_MODE}}"
      - name: PASSWORD
        secret_name: proxy-secret
        key: password
      - name: CPU_LIMIT
        resource_name: limits.cpu
        divisor: 1m
      - name: POD_NAME
        field_path: metadata.name
      - name: TOKEN_FILE
        workload_identity_token_ref_name: runtime-config
      - name: CA_FILE
        shared_asset_ref_name: internal-ca
      - name: REMOVE_DEFAULT
        remove: true
    resources:
      cpu: {from: 2m, to: 10m}
  - name: exporter
    image: exporter:1
`
	writeFile(t, path, original)
	writeFile(t, filepath.Join(root, "dev", "shared.assets.yml"), "assets:\n  - name: internal-ca\n    file: ca.pem\n    to: /ca.pem\n")
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	current, err := repo.DefaultsSidecarDefinitionEnvs("dev", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []string{"value", "secret", "resource", "field", "workload_identity_token", "shared_asset", "remove"}
	if len(current.Envs) != len(wantKinds) {
		t.Fatalf("env count = %d", len(current.Envs))
	}
	for index, kind := range wantKinds {
		if current.Envs[index].Kind != kind {
			t.Fatalf("envs[%d].kind = %q, want %q", index, current.Envs[index].Kind, kind)
		}
	}
	if current.Envs[0].Value != "{{var:PROXY_MODE}}" {
		t.Fatalf("raw value = %q", current.Envs[0].Value)
	}
	if strings.Join(current.WorkloadIdentityTokenRefNames, ",") != "runtime-config" || strings.Join(current.SharedAssetRefNames, ",") != "internal-ca" {
		t.Fatalf("reference catalogs = tokens %#v, assets %#v", current.WorkloadIdentityTokenRefNames, current.SharedAssetRefNames)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionEnvs("dev", "proxy", DefaultsSidecarDefinitionEnvsUpdate{Action: "set", Envs: current.Envs}, current.ContentHash); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("no-op env update changed source:\n%s", content)
	}

	updatedItems := append([]DefaultsSidecarEnvItem(nil), current.Envs...)
	updatedItems[0].Value = "strict"
	updatedItems[1], updatedItems[2] = updatedItems[2], updatedItems[1]
	updatedItems = append(updatedItems, DefaultsSidecarEnvItem{Name: "EMPTY_VALUE", Kind: "value"})
	updated, err := repo.UpdateDefaultsSidecarDefinitionEnvs("dev", "proxy", DefaultsSidecarDefinitionEnvsUpdate{Action: "set", Envs: updatedItems}, current.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Envs) != 8 || updated.Envs[1].Name != "CPU_LIMIT" || updated.Envs[7].Value != "" {
		t.Fatalf("updated envs = %#v", updated.Envs)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	for _, expected := range []string{`value: "strict"`, `resource_name: "limits.cpu"`, `secret_name: "proxy-secret"`, `value: ""`, "cpu: {from: 2m, to: 10m}", "image: exporter:1"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, got)
		}
	}
	if strings.Index(got, "name: CPU_LIMIT") > strings.Index(got, "name: PASSWORD") {
		t.Fatalf("updated source lost env order:\n%s", got)
	}

	removed, err := repo.UpdateDefaultsSidecarDefinitionEnvs("dev", "proxy", DefaultsSidecarDefinitionEnvsUpdate{Action: "remove"}, updated.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if removed.EnvsPresent || len(removed.Envs) != 0 {
		t.Fatalf("envs after remove = %#v", removed)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "envs:") || !strings.Contains(string(content), "cpu: {from: 2m, to: 10m}") {
		t.Fatalf("remove damaged source:\n%s", content)
	}
}

func TestDefaultsSidecarDefinitionEnvsRejectsUnsupportedOrAmbiguousEntries(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dev", "apps", "_defaults.yml")
	writeFile(t, path, `sidecar_definitions:
  - name: proxy
    envs:
      - name: MODE
        value: strict
        secret_name: proxy-secret
        key: mode
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DefaultsSidecarDefinitionEnvs("dev", "proxy"); err == nil || !strings.Contains(err.Error(), "multiple value sources") {
		t.Fatalf("ambiguous source error = %v", err)
	}

	writeFile(t, path, `sidecar_definitions:
  - name: proxy
    envs:
      - name: MODE
        value_from: external
`)
	if _, err := repo.DefaultsSidecarDefinitionEnvs("dev", "proxy"); err == nil || !strings.Contains(err.Error(), "value_from") {
		t.Fatalf("unsupported field error = %v", err)
	}

	writeFile(t, path, `sidecar_definitions:
  - name: proxy
    envs: []
`)
	current, err := repo.DefaultsSidecarDefinitionEnvs("dev", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.UpdateDefaultsSidecarDefinitionEnvs("dev", "proxy", DefaultsSidecarDefinitionEnvsUpdate{
		Action: "set", Envs: []DefaultsSidecarEnvItem{{Name: "SECRET", Kind: "secret", SecretName: "missing-key"}},
	}, current.ContentHash)
	if err == nil || !strings.Contains(err.Error(), "secret_name and key") {
		t.Fatalf("invalid secret error = %v", err)
	}

	_, err = repo.UpdateDefaultsSidecarDefinitionEnvs("dev", "proxy", DefaultsSidecarDefinitionEnvsUpdate{
		Action: "set", Envs: []DefaultsSidecarEnvItem{{Name: "TOKEN", Kind: "workload_identity_token", WorkloadIdentityTokenRefName: "missing-token"}},
	}, current.ContentHash)
	if err == nil || !strings.Contains(err.Error(), `unknown workload identity token "missing-token"`) {
		t.Fatalf("unknown token error = %v", err)
	}
	_, err = repo.UpdateDefaultsSidecarDefinitionEnvs("dev", "proxy", DefaultsSidecarDefinitionEnvsUpdate{
		Action: "set", Envs: []DefaultsSidecarEnvItem{{Name: "CA_FILE", Kind: "shared_asset", SharedAssetRefName: "missing-ca"}},
	}, current.ContentHash)
	if err == nil || !strings.Contains(err.Error(), `unknown shared asset "missing-ca"`) {
		t.Fatalf("unknown shared asset error = %v", err)
	}
}

func TestUpdateDefaultsSidecarDefinitionReferencesPreservesOrderAndOtherFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dev", "apps", "_defaults.yml")
	original := `container_profiles:
  - name: base
    defaults: {image_pull_policy: always}
  - name: hardened
    defaults: {security_context: {run_as_non_root: true}}
runtime_asset_definitions:
  - name: config
    files: []
  - name: trust
    files: []
sidecar_definitions:
  - name: proxy
    image: proxy:1
    profile_ref_names:
      - base
    runtime_asset_ref_names:
      - config
    envs:
      - name: MODE
        value: strict
  - name: exporter
    image: exporter:1
`
	writeFile(t, path, original)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.DefaultsSidecarDefinitionReferences("dev", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(current.ContainerProfiles, ",") != "base,hardened" || strings.Join(current.RuntimeAssetDefinitions, ",") != "config,trust" {
		t.Fatalf("catalogs = %#v / %#v", current.ContainerProfiles, current.RuntimeAssetDefinitions)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionReferences("dev", "proxy", DefaultsSidecarDefinitionReferencesUpdate{ProfileRefNames: current.ProfileRefNames, RuntimeAssetRefNames: current.RuntimeAssetRefNames}, current.ContentHash); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("no-op references update changed source:\n%s", content)
	}

	updated, err := repo.UpdateDefaultsSidecarDefinitionReferences("dev", "proxy", DefaultsSidecarDefinitionReferencesUpdate{
		ProfileRefNames: []string{"hardened", "base"}, RuntimeAssetRefNames: []string{"trust"},
	}, current.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(updated.ProfileRefNames, ",") != "hardened,base" || strings.Join(updated.RuntimeAssetRefNames, ",") != "trust" {
		t.Fatalf("updated references = %#v", updated)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	if strings.Index(got, "      - hardened") > strings.Index(got, "      - base") || !strings.Contains(got, "      - trust") || !strings.Contains(got, "value: strict") || !strings.Contains(got, "image: exporter:1") {
		t.Fatalf("updated references damaged source:\n%s", got)
	}

	cleared, err := repo.UpdateDefaultsSidecarDefinitionReferences("dev", "proxy", DefaultsSidecarDefinitionReferencesUpdate{}, updated.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.ProfileRefNames) != 0 || len(cleared.RuntimeAssetRefNames) != 0 {
		t.Fatalf("references after clear = %#v", cleared)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "profile_ref_names:") || strings.Contains(string(content), "runtime_asset_ref_names:") || !strings.Contains(string(content), "value: strict") {
		t.Fatalf("clear damaged source:\n%s", content)
	}
}

func TestDefaultsSidecarDefinitionReferencesRejectUnknownAndDuplicateNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "dev", "apps", "_defaults.yml"), `container_profiles:
  - name: base
    defaults: {}
sidecar_definitions:
  - name: proxy
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.DefaultsSidecarDefinitionReferences("dev", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionReferences("dev", "proxy", DefaultsSidecarDefinitionReferencesUpdate{ProfileRefNames: []string{"missing"}}, current.ContentHash); err == nil || !strings.Contains(err.Error(), "unknown definition") {
		t.Fatalf("unknown reference error = %v", err)
	}
	if _, err := repo.UpdateDefaultsSidecarDefinitionReferences("dev", "proxy", DefaultsSidecarDefinitionReferencesUpdate{ProfileRefNames: []string{"base", "base"}}, current.ContentHash); err == nil || !strings.Contains(err.Error(), "duplicate reference") {
		t.Fatalf("duplicate reference error = %v", err)
	}
}
