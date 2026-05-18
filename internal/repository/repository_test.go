package repository

import (
	"os"
	"os/exec"
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
	expected := []string{"_defaults.yml", "assets.secured.json", "assets.unsecured.json", "ssl/cert.pem", "ui/nginx.conf"}
	if len(relativePaths) != len(expected) {
		t.Fatalf("relative paths = %#v, want %#v", relativePaths, expected)
	}
	for i := range expected {
		if relativePaths[i] != expected[i] {
			t.Fatalf("relative paths = %#v, want %#v", relativePaths, expected)
		}
	}
	if assets[0].Driver != "defaults" || assets[1].Driver != "special" || assets[3].Driver != "configmap" {
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
	if len(container.Envs) != 2 || container.Envs[1].Kind != "secret" || container.Envs[1].SecretName == nil || *container.Envs[1].SecretName != "app-secret" {
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
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.UpdateAppContainerResources("test", "api.yml", 0, ResourceUpdate{CPURequest: "200m", CPULimit: "600m", MemoryRequest: "256Mi", MemoryLimit: "768Mi"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Resources.CPURequest == nil || *result.Resources.CPURequest != "200m" {
		t.Fatalf("resources = %#v, want cpu request 200m", result.Resources)
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

	result, err := repo.UpdateAppContainerEnvs("test", "api.yml", 1, []VarItem{{Name: "MODE", Value: "new"}, {Name: "QUEUE", Value: "critical"}}, "")
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
	if !contains(detail.Content, "  - name: worker\n    envs:\n      - name: MODE\n        value: \"new\"\n      - name: QUEUE\n        value: \"critical\"\n    image: worker:1") {
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

	result, err := repo.UpdateAppContainerEnvs("test", "api.yml", 0, []VarItem{{Name: "PLAIN", Value: "new"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Envs) != 2 || result.Envs[1].Kind != "secret" {
		t.Fatalf("envs = %#v, want preserved secret", result.Envs)
	}
	detail, err := repo.AppDetail("test", "api.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(detail.Content, `value: "new"`) || !contains(detail.Content, "valueFrom:") || !contains(detail.Content, "name: api-secret") {
		t.Fatalf("valueFrom variable was not preserved:\n%s", detail.Content)
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
				Name:         "api-public",
				HTTPHostname: "api.example.test",
				HTTPPath:     "/",
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
	if !contains(detail.Content, "  - name: api\n    ports:\n      - name: http\n        port: 8080\n        metrics: true\n        expose_as:\n          - service_name: api\n            port: 80\n            external:\n              - name: api-public\n                http:\n                  - hostname: \"api.example.test\"\n                    path: \"/\"\n    image: api:1") {
		t.Fatalf("ports block not inserted after container name:\n%s", detail.Content)
	}
}

func TestUpdateAppContainerPortsRejectsUnsupportedAdvancedFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "test", "apps", "api.yml"), `name: api
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
                http:
                  - hostname: api.example.test
                    path: /
`)
	repo, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.UpdateAppContainerPorts("test", "api.yml", 0, []PortUpdate{{Name: "http", Port: "8080"}}, "")
	if err == nil || !contains(err.Error(), "annotations") {
		t.Fatalf("UpdateAppContainerPorts error = %v, want unsupported annotations error", err)
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
	if !contains(detail.Content, "  - name: api\n    probes:\n      preset: \"spring-actuator\"\n      port: \"8080\"\n      path: \"/actuator/health\"\n    image: api:1") {
		t.Fatalf("probes not inserted after container name:\n%s", detail.Content)
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
