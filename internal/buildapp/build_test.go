package buildapp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildGeneratesBasicDeployment(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
vars:
  - name: APP_NAME
    value: "api"
name: {{var:APP_NAME}}
replicas: 2
containers:
  - name: "{{var:APP_NAME}}"
    image: "{{TSM_REGISTRY_URL}}/{{var:APP_NAME}}:{{TSM_RELEASE_ID}}"
    startup:
      command:
        - /bin/sh
      arguments:
        - -c
        - echo ok
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
    ports:
      - name: http
        port: 8080
        expose_as:
          - hostname: api
            port: 80
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deployments) != 1 {
		t.Fatalf("len(deployments) = %d, want 1", len(result.Deployments))
	}

	deploymentPath := filepath.Join(target, "deployments", "api-deployment.yml")
	content, err := os.ReadFile(deploymentPath)
	if err != nil {
		t.Fatal(err)
	}
	var deployment map[string]any
	if err := yaml.Unmarshal(content, &deployment); err != nil {
		t.Fatal(err)
	}

	if got := digString(deployment, "metadata", "name"); got != "api" {
		t.Fatalf("metadata.name = %q, want api", got)
	}
	if got := digString(deployment, "metadata", "namespace"); got != "nac-test" {
		t.Fatalf("metadata.namespace = %q, want nac-test", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "image"); got != "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}" {
		t.Fatalf("container image = %q, want unresolved apply-env placeholders", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "ports", "0", "containerPort"); got != 8080 {
		t.Fatalf("container port = %d, want 8080", got)
	}

	service := loadYAML(t, filepath.Join(target, "services", "api-service.yml"))
	if got := digString(service, "metadata", "name"); got != "api" {
		t.Fatalf("service metadata.name = %q, want api", got)
	}
	if got := digInt(service, "spec", "ports", "0", "port"); got != 80 {
		t.Fatalf("service port = %d, want 80", got)
	}
	if got := digInt(service, "spec", "ports", "0", "targetPort"); got != 8080 {
		t.Fatalf("service targetPort = %d, want 8080", got)
	}
}

func TestBuildSupportsEnvFileAndVarsSourcePrecedence(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "json-ns",
			"TSM_REGISTRY_URL": "json-registry",
			"TSM_RELEASE_ID":   "json-release",
			"SOURCE_TEST":      "json-value",
		},
	})
	writeFile(t, filepath.Join(envDir, ".env"), "SOURCE_TEST=dot-value\nTSM_RELEASE_ID=dot-release\n")
	writeFile(t, filepath.Join(envDir, "explicit.env"), "SOURCE_TEST=explicit-value\nNAMESPACE=explicit-ns\nTSM_REGISTRY_URL=explicit-registry\nTSM_RELEASE_ID=explicit-release\n")
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{env:TSM_REGISTRY_URL}}/{{env:SOURCE_TEST}}:{{env:TSM_RELEASE_ID}}"
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	t.Setenv("SOURCE_TEST", "env-value")
	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "image"); got != "json-registry/env-value:json-release" {
		t.Fatalf("default image = %q", got)
	}

	target2 := filepath.Join(t.TempDir(), "target")
	_, err = Build(Options{Environment: "test", Root: root, Target: target2, VarsSources: []string{"json,dot-env"}})
	if err != nil {
		t.Fatal(err)
	}
	deployment = loadYAML(t, filepath.Join(target2, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "image"); got != "json-registry/dot-value:dot-release" {
		t.Fatalf("dot-env image = %q", got)
	}

	target3 := filepath.Join(t.TempDir(), "target")
	_, err = Build(Options{Environment: "test", Root: root, Target: target3, EnvFile: filepath.Join(envDir, "explicit.env")})
	if err != nil {
		t.Fatal(err)
	}
	deployment = loadYAML(t, filepath.Join(target3, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "metadata", "namespace"); got != "explicit-ns" {
		t.Fatalf("explicit namespace = %q", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "image"); got != "explicit-registry/explicit-value:explicit-release" {
		t.Fatalf("explicit image = %q", got)
	}
}

func TestLoadEnvVarsDecryptSecuredUsesEncjsonAPISelection(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE": "json-ns",
		},
	})
	writeFile(t, filepath.Join(envDir, "env.secured.json"), `{
  "_public_key": "dummy",
  "environment": {
    "SECRET": "EncJson[@api=2.0:@box=<encrypted>]"
  }
}`)
	rustBin, rustArgs := writeFakeEncjson(t, "rust", `{"_public_key":"decrypted-key","environment":{"SECRET":"decrypted-secret"}}`)
	legacyBin, _ := writeFakeEncjson(t, "legacy", `{"_public_key":"legacy-key","environment":{"SECRET":"legacy-secret"}}`)
	keydir := filepath.Join(t.TempDir(), "keys")
	t.Setenv("ENCJSON_PATH", rustBin)
	t.Setenv("ENCJSON_LEGACY_PATH", legacyBin)
	t.Setenv("ENCJSON_KEYDIR", keydir)

	vars, err := loadEnvVars(envDir, Options{DecryptSecured: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := vars["SECRET"]; got != "decrypted-secret" {
		t.Fatalf("SECRET = %q, want decrypted-secret", got)
	}
	if got := vars["_public_key"]; got != "decrypted-key" {
		t.Fatalf("_public_key = %q, want decrypted-key", got)
	}
	argsContent, err := os.ReadFile(rustArgs)
	if err != nil {
		t.Fatal(err)
	}
	args := string(argsContent)
	for _, expected := range []string{"decrypt", "-k", keydir, "-f", filepath.Join(envDir, "env.secured.json")} {
		if !strings.Contains(args, expected) {
			t.Fatalf("fake encjson args missing %q: %s", expected, args)
		}
	}
}

func TestLoadEnvVarsDecryptSecuredUsesLegacyEncjsonForAPI1(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE": "json-ns",
		},
	})
	writeFile(t, filepath.Join(envDir, "env.secured.json"), `{
  "_public_key": "dummy",
  "environment": {
    "SECRET": "EncJson[@api=1.0:@box=<encrypted>]"
  }
}`)
	rustBin, _ := writeFakeEncjson(t, "rust", `{"_public_key":"rust-key","environment":{"SECRET":"rust-secret"}}`)
	legacyBin, legacyArgs := writeFakeEncjson(t, "legacy", `{"_public_key":"legacy-key","environment":{"SECRET":"legacy-secret"}}`)
	t.Setenv("ENCJSON_PATH", rustBin)
	t.Setenv("ENCJSON_LEGACY_PATH", legacyBin)
	t.Setenv("ENCJSON_KEYDIR", filepath.Join(t.TempDir(), "keys"))

	vars, err := loadEnvVars(envDir, Options{DecryptSecured: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := vars["SECRET"]; got != "legacy-secret" {
		t.Fatalf("SECRET = %q, want legacy-secret", got)
	}
	if _, err := os.Stat(legacyArgs); err != nil {
		t.Fatalf("legacy encjson was not executed: %v", err)
	}
}

func TestLoadEnvVarsDecryptSecuredOmitsKeydirWhenUnset(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE": "json-ns",
		},
	})
	writeFile(t, filepath.Join(envDir, "env.secured.json"), `{
  "_public_key": "dummy",
  "environment": {
    "SECRET": "EncJson[@api=2.0:@box=<encrypted>]"
  }
}`)
	rustBin, rustArgs := writeFakeEncjson(t, "rust", `{"_public_key":"decrypted-key","environment":{"SECRET":"decrypted-secret"}}`)
	t.Setenv("ENCJSON_PATH", rustBin)
	t.Setenv("ENCJSON_KEYDIR", "")

	vars, err := loadEnvVars(envDir, Options{DecryptSecured: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := vars["SECRET"]; got != "decrypted-secret" {
		t.Fatalf("SECRET = %q, want decrypted-secret", got)
	}
	argsContent, err := os.ReadFile(rustArgs)
	if err != nil {
		t.Fatal(err)
	}
	args := string(argsContent)
	if strings.Contains(args, "-k") {
		t.Fatalf("fake encjson args unexpectedly contain -k: %s", args)
	}
	for _, expected := range []string{"decrypt", "-f", filepath.Join(envDir, "env.secured.json")} {
		if !strings.Contains(args, expected) {
			t.Fatalf("fake encjson args missing %q: %s", expected, args)
		}
	}
}

func TestBuildGeneratesServiceWithMetricsAndHeadless(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    ports:
      - name: http
        port: 8080
        metrics: true
        metricsPathFor: api
        expose_as:
          - hostname: api-headless
            port: 80
            type: headless
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 1 {
		t.Fatalf("len(services) = %d, want 1", len(result.Services))
	}

	service := loadYAML(t, filepath.Join(target, "services", "api-headless-service.yml"))
	if got := digString(service, "spec", "clusterIP"); got != "None" {
		t.Fatalf("clusterIP = %q, want None", got)
	}
	if got := digString(service, "metadata", "labels", "metrics"); got != "true" {
		t.Fatalf("metrics label = %q, want true", got)
	}
	if got := digString(service, "metadata", "labels", "metricsPathFor"); got != "api-headless" {
		t.Fatalf("metricsPathFor label = %q, want api-headless", got)
	}
	if got := digInt(service, "spec", "ports", "1", "port"); got != 9090 {
		t.Fatalf("metrics port = %d, want 9090", got)
	}
	if got := digInt(service, "spec", "ports", "1", "targetPort"); got != 8080 {
		t.Fatalf("metrics targetPort = %d, want 8080", got)
	}
}

func TestBuildGeneratesExternalIngressAndRoute(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    ports:
      - name: http
        port: 8080
        expose_as:
          - hostname: api
            port: 80
            external:
              - name: api-public
                http:
                  - hostname: api.example.test
                    path: /
                https:
                  - hostname: api.example.test
                    secret_name: api-tls
              - name: api-route
                as_route: true
                http:
                  - hostname: api-route.example.test
                tls:
                  termination: edge
                  insecureEdgeTerminationPolicy: Redirect
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Externals) != 2 {
		t.Fatalf("len(externals) = %d, want 2", len(result.Externals))
	}

	ingress := loadYAML(t, filepath.Join(target, "services", "external", "api-public-ingress.yml"))
	if got := digString(ingress, "kind"); got != "Ingress" {
		t.Fatalf("ingress kind = %q", got)
	}
	if got := digString(ingress, "spec", "rules", "0", "host"); got != "api.example.test" {
		t.Fatalf("ingress host = %q", got)
	}
	if got := digInt(ingress, "spec", "rules", "0", "http", "paths", "0", "backend", "service", "port", "number"); got != 80 {
		t.Fatalf("ingress backend port = %d, want 80", got)
	}

	route := loadYAML(t, filepath.Join(target, "services", "external", "api-route-route.yml"))
	if got := digString(route, "kind"); got != "Route" {
		t.Fatalf("route kind = %q", got)
	}
	if got := digString(route, "spec", "port", "targetPort"); got != "http-80" {
		t.Fatalf("route targetPort = %q, want http-80", got)
	}
	if got := digString(route, "spec", "tls", "termination"); got != "edge" {
		t.Fatalf("route tls termination = %q, want edge", got)
	}
}

func TestBuildGeneratesLegacyHealthProbes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    health:
      http:
        path:
          live: /live
          ready: /ready
        port: 8080
      delay: 10
      period: 5
      timeout: 3
      success: 1
      failure: 2
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "livenessProbe", "httpGet", "path"); got != "/live" {
		t.Fatalf("liveness path = %q, want /live", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "readinessProbe", "httpGet", "path"); got != "/ready" {
		t.Fatalf("readiness path = %q, want /ready", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "livenessProbe", "initialDelaySeconds"); got != 10 {
		t.Fatalf("initialDelaySeconds = %d, want 10", got)
	}
}

func TestBuildGeneratesExplicitProbeBlocks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    probe:
      live:
        command: ["/bin/check-live"]
        delay: 11
      ready:
        http:
          path: /ready
          port: 8080
        period: 7
      start:
        http:
          path: /start
          port: 8080
        failure: 30
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "livenessProbe", "exec", "command", "0"); got != "/bin/check-live" {
		t.Fatalf("liveness command = %q, want /bin/check-live", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "livenessProbe", "initialDelaySeconds"); got != 11 {
		t.Fatalf("liveness delay = %d, want 11", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "readinessProbe", "httpGet", "path"); got != "/ready" {
		t.Fatalf("readiness path = %q, want /ready", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "startupProbe", "httpGet", "path"); got != "/start" {
		t.Fatalf("startup path = %q, want /start", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "startupProbe", "failureThreshold"); got != 30 {
		t.Fatalf("startup failureThreshold = %d, want 30", got)
	}
}

func TestBuildGeneratesModernProbesPreset(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
vars:
  - name: EXPOSE_PORT
    value: 8080
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    probes:
      preset: spring-actuator
      port: "{{var:EXPOSE_PORT}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "livenessProbe", "httpGet", "path"); got != "/actuator/health/liveness" {
		t.Fatalf("liveness path = %q, want /actuator/health/liveness", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "readinessProbe", "httpGet", "path"); got != "/actuator/health/readiness" {
		t.Fatalf("readiness path = %q, want /actuator/health/readiness", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "startupProbe", "httpGet", "path"); got != "/actuator/health" {
		t.Fatalf("startup path = %q, want /actuator/health", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "livenessProbe", "httpGet", "port"); got != 8080 {
		t.Fatalf("liveness port = %d, want 8080", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "startupProbe", "failureThreshold"); got != 30 {
		t.Fatalf("startup failureThreshold = %d, want 30", got)
	}
}

func TestBuildGeneratesModernProbesOverrides(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    probes:
      http:
        path: /healthz
        port: 8080
      ready:
        period: 3
        success: 1
      start:
        command: ["/bin/check-start"]
        failure: 40
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "livenessProbe", "httpGet", "path"); got != "/healthz" {
		t.Fatalf("liveness path = %q, want /healthz", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "readinessProbe", "periodSeconds"); got != 3 {
		t.Fatalf("readiness period = %d, want 3", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "startupProbe", "exec", "command", "0"); got != "/bin/check-start" {
		t.Fatalf("startup command = %q, want /bin/check-start", got)
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "startupProbe", "failureThreshold"); got != 40 {
		t.Fatalf("startup failureThreshold = %d, want 40", got)
	}
}

func TestBuildGeneratesSchedulingFieldsAndRawContainerPassthrough(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
arch: amd64
node_selector:
  workload: tsm
tolerations:
  - key: dedicated
    operator: Equal
    value: tsm
    effect: NoSchedule
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    raw:
      securityContext:
        allowPrivilegeEscalation: false
      stdin: true
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "nodeSelector", "kubernetes.io/arch"); got != "amd64" {
		t.Fatalf("arch nodeSelector = %q, want amd64", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "nodeSelector", "workload"); got != "tsm" {
		t.Fatalf("workload nodeSelector = %q, want tsm", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "tolerations", "0", "key"); got != "dedicated" {
		t.Fatalf("toleration key = %q, want dedicated", got)
	}
	if got := digBool(deployment, "spec", "template", "spec", "containers", "0", "securityContext", "allowPrivilegeEscalation"); got {
		t.Fatalf("allowPrivilegeEscalation = true, want false")
	}
	if got := digBool(deployment, "spec", "template", "spec", "containers", "0", "stdin"); !got {
		t.Fatalf("stdin = false, want true")
	}
}

func TestBuildGeneratesPodAndContainerSecurityContext(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
security_context:
  runAsNonRoot: true
  fsGroup: 2000
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    security_context:
      allowPrivilegeEscalation: false
      runAsUser: 1000
      capabilities:
        drop: ["ALL"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digBool(deployment, "spec", "template", "spec", "securityContext", "runAsNonRoot"); !got {
		t.Fatalf("pod runAsNonRoot = false, want true")
	}
	if got := digInt(deployment, "spec", "template", "spec", "securityContext", "fsGroup"); got != 2000 {
		t.Fatalf("pod fsGroup = %d, want 2000", got)
	}
	if got := digBool(deployment, "spec", "template", "spec", "containers", "0", "securityContext", "allowPrivilegeEscalation"); got {
		t.Fatalf("container allowPrivilegeEscalation = true, want false")
	}
	if got := digInt(deployment, "spec", "template", "spec", "containers", "0", "securityContext", "runAsUser"); got != 1000 {
		t.Fatalf("container runAsUser = %d, want 1000", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "securityContext", "capabilities", "drop", "0"); got != "ALL" {
		t.Fatalf("container dropped capability = %q, want ALL", got)
	}
}

func TestBuildGeneratesLifecyclePreStopAndTerminationGrace(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
termination_grace_period: 45
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    lifecycle:
      pre_stop:
        command: ["/bin/sh", "-c", "sleep 10"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digInt(deployment, "spec", "template", "spec", "terminationGracePeriodSeconds"); got != 45 {
		t.Fatalf("terminationGracePeriodSeconds = %d, want 45", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "lifecycle", "preStop", "exec", "command", "2"); got != "sleep 10" {
		t.Fatalf("preStop command[2] = %q, want sleep 10", got)
	}
}

func TestBuildGeneratesServiceAccountAndEnvFrom(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
service_account: tsm-api
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    env_from:
      - config_map: api-config
      - secret: api-secret
        prefix: SECRET_
      - secretRef:
          name: optional-secret
          optional: true
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "serviceAccountName"); got != "tsm-api" {
		t.Fatalf("serviceAccountName = %q, want tsm-api", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "envFrom", "0", "configMapRef", "name"); got != "api-config" {
		t.Fatalf("envFrom[0].configMapRef.name = %q, want api-config", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "envFrom", "1", "secretRef", "name"); got != "api-secret" {
		t.Fatalf("envFrom[1].secretRef.name = %q, want api-secret", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "envFrom", "1", "prefix"); got != "SECRET_" {
		t.Fatalf("envFrom[1].prefix = %q, want SECRET_", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "envFrom", "2", "secretRef", "name"); got != "optional-secret" {
		t.Fatalf("envFrom[2].secretRef.name = %q, want optional-secret", got)
	}
	if got := digBool(deployment, "spec", "template", "spec", "containers", "0", "envFrom", "2", "secretRef", "optional"); !got {
		t.Fatalf("envFrom[2].secretRef.optional = false, want true")
	}
}

func TestBuildGeneratesGenericInitContainers(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
init_containers:
  - name: migrate
    image: "{{TSM_REGISTRY_URL}}/api-migrate:{{TSM_RELEASE_ID}}"
    command: ["/bin/sh", "-c"]
    arguments: ["./migrate.sh"]
    env_vars:
      - name: LOG_LEVEL
        value: INFO
    env_from:
      - config_map: api-config
    mounts:
      - type: empty_dir
        name: work
        mount_path: /work
    security_context:
      runAsNonRoot: true
    resources:
      cpu:
        from: "50m"
        to: "100m"
      memory:
        from: "64Mi"
        to: "128Mi"
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    mounts:
      - type: empty_dir
        name: work
        mount_path: /work
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "initContainers", "0", "name"); got != "migrate" {
		t.Fatalf("init container name = %q, want migrate", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "initContainers", "0", "image"); got != "{{TSM_REGISTRY_URL}}/api-migrate:{{TSM_RELEASE_ID}}" {
		t.Fatalf("init container image = %q, want runtime placeholder image", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "initContainers", "0", "args", "0"); got != "./migrate.sh" {
		t.Fatalf("init container args[0] = %q, want ./migrate.sh", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "initContainers", "0", "env", "0", "name"); got != "LOG_LEVEL" {
		t.Fatalf("init container env[0].name = %q, want LOG_LEVEL", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "initContainers", "0", "envFrom", "0", "configMapRef", "name"); got != "api-config" {
		t.Fatalf("init container envFrom[0].configMapRef.name = %q, want api-config", got)
	}
	if got := digBool(deployment, "spec", "template", "spec", "initContainers", "0", "securityContext", "runAsNonRoot"); !got {
		t.Fatalf("init container runAsNonRoot = false, want true")
	}
	if got := digString(deployment, "spec", "template", "spec", "initContainers", "0", "volumeMounts", "0", "mountPath"); got != "/work" {
		t.Fatalf("init container volumeMount mountPath = %q, want /work", got)
	}
	if emptyDir := digAny(deployment, "spec", "template", "spec", "volumes", "0", "emptyDir"); emptyDir == nil {
		t.Fatal("expected emptyDir volume for init/container shared mount")
	}
}

func TestBuildAcceptsRequestsLimitsResourceAliases(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 2
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    resources:
      cpu:
        requests: "250m"
        limits: "750m"
      memory:
        requests: "384Mi"
        limits: "768Mi"
      ephemeral-storage:
        requests: "1Gi"
        limits: "2Gi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "resources", "requests", "cpu"); got != "250m" {
		t.Fatalf("cpu request = %q, want 250m", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "resources", "limits", "memory"); got != "768Mi" {
		t.Fatalf("memory limit = %q, want 768Mi", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "resources", "requests", "ephemeral-storage"); got != "1Gi" {
		t.Fatalf("ephemeral-storage request = %q, want 1Gi", got)
	}

	summary, err := ResourceSummary(Options{Environment: "test", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Totals.CPURequestCores != 0.5 {
		t.Fatalf("cpu request total = %v, want 0.5", summary.Totals.CPURequestCores)
	}
	if summary.Totals.MemoryLimitMiB != 1536 {
		t.Fatalf("memory limit total = %v, want 1536", summary.Totals.MemoryLimitMiB)
	}
}

func TestBuildAppliesDeploymentAndPodRaw(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
deployment_raw:
  spec:
    revisionHistoryLimit: 2
pod_raw:
  dnsPolicy: ClusterFirst
  enableServiceLinks: false
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digInt(deployment, "spec", "revisionHistoryLimit"); got != 2 {
		t.Fatalf("revisionHistoryLimit = %d, want 2", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "dnsPolicy"); got != "ClusterFirst" {
		t.Fatalf("dnsPolicy = %q, want ClusterFirst", got)
	}
	if got := digBool(deployment, "spec", "template", "spec", "enableServiceLinks"); got {
		t.Fatal("enableServiceLinks = true, want false")
	}
	if got := digInt(deployment, "spec", "replicas"); got != 1 {
		t.Fatalf("replicas = %d, want 1", got)
	}
}

func TestBuildGeneratesModernSchedulingFields(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 2
scheduling:
  arch: amd64
  node_selector:
    workload: tsm
  tolerations:
    - key: dedicated
      operator: Equal
      value: tsm
      effect: NoSchedule
  spread:
    by: hostname
    max_skew: 1
    when_unsatisfiable: ScheduleAnyway
  anti_affinity:
    self: preferred
    topology: kubernetes.io/hostname
  affinity:
    nodeAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 10
          preference:
            matchExpressions:
              - key: disk
                operator: In
                values: [ssd]
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "nodeSelector", "kubernetes.io/arch"); got != "amd64" {
		t.Fatalf("arch nodeSelector = %q, want amd64", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "nodeSelector", "workload"); got != "tsm" {
		t.Fatalf("workload nodeSelector = %q, want tsm", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "topologySpreadConstraints", "0", "topologyKey"); got != "kubernetes.io/hostname" {
		t.Fatalf("topologyKey = %q, want kubernetes.io/hostname", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "affinity", "podAntiAffinity", "preferredDuringSchedulingIgnoredDuringExecution", "0", "podAffinityTerm", "topologyKey"); got != "kubernetes.io/hostname" {
		t.Fatalf("podAntiAffinity topologyKey = %q, want kubernetes.io/hostname", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "affinity", "nodeAffinity", "preferredDuringSchedulingIgnoredDuringExecution", "0", "preference", "matchExpressions", "0", "key"); got != "disk" {
		t.Fatalf("nodeAffinity match key = %q, want disk", got)
	}
}

func TestBuildGeneratesRegistryDNSAndAnnotations(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
annotations:
  argocd.argoproj.io/sync-wave: "2"
pod_annotations:
  checksum/config: abc123
registry:
  - secret_name: regcred
dns:
  - ip: 10.0.0.10
    hostnames:
      - legacy.local
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "metadata", "annotations", "argocd.argoproj.io/sync-wave"); got != "2" {
		t.Fatalf("sync-wave annotation = %q, want 2", got)
	}
	if got := digString(deployment, "spec", "template", "metadata", "annotations", "checksum/config"); got != "abc123" {
		t.Fatalf("pod checksum annotation = %q, want abc123", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "imagePullSecrets", "0", "name"); got != "regcred" {
		t.Fatalf("imagePullSecret = %q, want regcred", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "hostAliases", "0", "ip"); got != "10.0.0.10" {
		t.Fatalf("hostAlias ip = %q, want 10.0.0.10", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "hostAliases", "0", "hostnames", "0"); got != "legacy.local" {
		t.Fatalf("hostAlias hostname = %q, want legacy.local", got)
	}
}

func TestBuildGeneratesToolsInitContainersAndCgroupDefaults(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "_defaults.yml"), `
tools:
  - name: util-encjson-rs
    image: toolbox:1
    expose_bin: /usr/bin/encjson-rs
    as: /app/tools/encjson
`)
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    enable_cgroup_exporter: true
    env_vars:
      - name: CGROUP_EXPORTER_LISTEN
        value: "127.0.0.1:9393"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "initContainers", "0", "name"); got != "util-encjson-rs" {
		t.Fatalf("tool initContainer name = %q", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts", "0", "mountPath"); got != "/app/tools" {
		t.Fatalf("tools mountPath = %q", got)
	}
	env := digSlice(deployment, "spec", "template", "spec", "containers", "0", "env")
	if got := envValue(env, "CGROUP_EXPORTER_LISTEN"); got != "127.0.0.1:9393" {
		t.Fatalf("CGROUP_EXPORTER_LISTEN = %q", got)
	}
	if got := envFieldPath(env, "CGROUP_EXPORTER_NODE_NAME"); got != "spec.nodeName" {
		t.Fatalf("CGROUP_EXPORTER_NODE_NAME fieldPath = %q", got)
	}
}

func TestValidateRejectsStartupAndSimpleInitTogether(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "invalid.yml"), `
name: invalid
replicas: 1
containers:
  - name: invalid
    image: "{{TSM_REGISTRY_URL}}/invalid:{{TSM_RELEASE_ID}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    simple_init:
      enabled: true
      exec:
        command: ["/app/start.sh"]
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	err := Validate(Options{Environment: "test", Root: root})
	if err == nil {
		t.Fatal("expected simple_init/startup validation error")
	}
	if !strings.Contains(err.Error(), "XOR violation") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestBuildGeneratesRolloutChecksumAnnotations(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{"environment": map[string]any{"NAMESPACE": "nac-test", "TSM_REGISTRY_URL": "registry.local/tsm", "TSM_RELEASE_ID": "1.0.0"}})
	writeFile(t, filepath.Join(envDir, "env.secured.json"), `{"_public_key":"dummy","environment":{"SECRET":"abc"}}`)
	writeFile(t, filepath.Join(envDir, "assets", "config.tpl"), "url={{EPAP_URL}}\n")
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
rollout_on:
  checksums:
    config:
      files:
        - env.unsecured.json
        - env.secured.json
    mtls:
      files:
        - assets/config.tpl
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "metadata", "annotations", "checksum/config"); got != checksumForFiles(t, envDir, "env.secured.json", "env.unsecured.json") {
		t.Fatalf("checksum/config = %q", got)
	}
	if got := digString(deployment, "spec", "template", "metadata", "annotations", "checksum/mtls"); got != checksumForFiles(t, envDir, "assets/config.tpl") {
		t.Fatalf("checksum/mtls = %q", got)
	}
}

func TestBuildRolloutChecksumMissingFileFails(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{"environment": map[string]any{"NAMESPACE": "nac-test", "TSM_REGISTRY_URL": "registry.local/tsm", "TSM_RELEASE_ID": "1.0.0"}})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
rollout_on:
  checksums:
    config:
      files: [missing.file]
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    startup: {command: ["/bin/sh"], arguments: ["-c", "echo ok"]}
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)
	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err == nil {
		t.Fatal("expected missing rollout checksum file error")
	}
}

func TestBuildRolloutChecksumConflictingPodAnnotationFails(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{"environment": map[string]any{"NAMESPACE": "nac-test", "TSM_REGISTRY_URL": "registry.local/tsm", "TSM_RELEASE_ID": "1.0.0"}})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
pod_annotations:
  checksum/config: manual
rollout_on:
  checksums:
    config:
      files: [env.unsecured.json]
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    startup: {command: ["/bin/sh"], arguments: ["-c", "echo ok"]}
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)
	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err == nil {
		t.Fatal("expected conflicting pod annotation error")
	}
}

func TestBuildSkipsIgnoredAppsAndCanDisableServiceGeneration(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "ignored.yml"), `
name: ignored
ignore: true
replicas: 1
containers:
  - name: ignored
    image: "{{TSM_REGISTRY_URL}}/ignored:{{TSM_RELEASE_ID}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
disable_create_service: true
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    ports:
      - name: http
        port: 8080
        expose_as:
          - hostname: api
            port: 80
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deployments) != 1 {
		t.Fatalf("len(deployments) = %d, want 1", len(result.Deployments))
	}
	if _, err := os.Stat(filepath.Join(target, "deployments", "ignored-deployment.yml")); !os.IsNotExist(err) {
		t.Fatalf("ignored deployment exists or stat error differs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "services", "api-service.yml")); !os.IsNotExist(err) {
		t.Fatalf("disabled service exists or stat error differs: %v", err)
	}
}

func TestBuildGeneratesStrategiesStatefulSetAndBudget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeMinimalApp(t, envDir, "recreate.yml", `
name: recreate
strategy: recreate
min_available: 1
`)
	writeMinimalApp(t, envDir, "one-by-one.yml", `
name: one-by-one
strategy: one-by-one
max_unavailable: 1
`)
	writeMinimalApp(t, envDir, "stateful.yml", `
name: stateful
kind: StatefulSet
subdomain_name: stateful-headless
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Budgets) != 2 {
		t.Fatalf("len(budgets) = %d, want 2", len(result.Budgets))
	}

	recreate := loadYAML(t, filepath.Join(target, "deployments", "recreate-deployment.yml"))
	if got := digString(recreate, "spec", "strategy", "type"); got != "Recreate" {
		t.Fatalf("recreate strategy type = %q, want Recreate", got)
	}
	recreateBudget := loadYAML(t, filepath.Join(target, "deployments", "recreate-budget.yml"))
	if got := digInt(recreateBudget, "spec", "minAvailable"); got != 1 {
		t.Fatalf("minAvailable = %d, want 1", got)
	}

	oneByOne := loadYAML(t, filepath.Join(target, "deployments", "one-by-one-deployment.yml"))
	if got := digInt(oneByOne, "spec", "strategy", "rollingUpdate", "maxSurge"); got != 0 {
		t.Fatalf("one-by-one maxSurge = %d, want 0", got)
	}
	if got := digInt(oneByOne, "spec", "strategy", "rollingUpdate", "maxUnavailable"); got != 1 {
		t.Fatalf("one-by-one maxUnavailable = %d, want 1", got)
	}
	oneByOneBudget := loadYAML(t, filepath.Join(target, "deployments", "one-by-one-budget.yml"))
	if got := digInt(oneByOneBudget, "spec", "maxUnavailable"); got != 1 {
		t.Fatalf("budget maxUnavailable = %d, want 1", got)
	}

	stateful := loadYAML(t, filepath.Join(target, "deployments", "stateful-deployment.yml"))
	if got := digString(stateful, "kind"); got != "StatefulSet" {
		t.Fatalf("kind = %q, want StatefulSet", got)
	}
	if got := digString(stateful, "spec", "serviceName"); got != "stateful-headless" {
		t.Fatalf("serviceName = %q, want stateful-headless", got)
	}
	if got := digString(stateful, "spec", "updateStrategy", "type"); got != "RollingUpdate" {
		t.Fatalf("updateStrategy.type = %q, want RollingUpdate", got)
	}
	if strategy := digAny(stateful, "spec", "strategy"); strategy != nil {
		t.Fatalf("stateful strategy exists: %#v", strategy)
	}
}

func TestBuildAppliesReplicaProfile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "replica-profiles.yml"), `
defaults:
  profile: maintenance
profiles:
  maintenance:
    all: 1
    apps:
      api: 2
  legacy:
    all: 3
    worker: 4
`)
	writeMinimalApp(t, envDir, "api.yml", `
name: api
`)
	writeMinimalApp(t, envDir, "worker.yml", `
name: worker
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	api := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digInt(api, "spec", "replicas"); got != 2 {
		t.Fatalf("api replicas = %d, want 2", got)
	}
	worker := loadYAML(t, filepath.Join(target, "deployments", "worker-deployment.yml"))
	if got := digInt(worker, "spec", "replicas"); got != 1 {
		t.Fatalf("worker replicas = %d, want 1", got)
	}
}

func TestBuildAppliesLegacyReplicaProfileFromOption(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "replica-profiles.yml"), `
profiles:
  legacy:
    all: 3
    worker: 4
`)
	writeMinimalApp(t, envDir, "api.yml", `
name: api
`)
	writeMinimalApp(t, envDir, "worker.yml", `
name: worker
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target, Profile: "legacy"})
	if err != nil {
		t.Fatal(err)
	}

	api := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digInt(api, "spec", "replicas"); got != 3 {
		t.Fatalf("api replicas = %d, want 3", got)
	}
	worker := loadYAML(t, filepath.Join(target, "deployments", "worker-deployment.yml"))
	if got := digInt(worker, "spec", "replicas"); got != 4 {
		t.Fatalf("worker replicas = %d, want 4", got)
	}
}

func TestBuildAppliesReleaseManifestAndScaleDown(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	manifestPath := filepath.Join(root, "release.yml")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, manifestPath, `
images:
  - app_name: api
    image: registry.release/api
    tag: "2.0.0"
  - app_name: worker
    container_name: worker
    image: registry.release/worker
    digest: sha256:abcdef
`)
	writeMinimalApp(t, envDir, "api.yml", `
name: api
`)
	writeFile(t, filepath.Join(envDir, "apps", "worker.yml"), `
name: worker
replicas: 3
containers:
  - name: worker
    image: "{{TSM_REGISTRY_URL}}/worker:{{TSM_RELEASE_ID}}"
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target, ReleaseManifest: manifestPath, Down: []string{"worker"}})
	if err != nil {
		t.Fatal(err)
	}

	api := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(api, "spec", "template", "spec", "containers", "0", "image"); got != "registry.release/api:2.0.0" {
		t.Fatalf("api image = %q", got)
	}
	worker := loadYAML(t, filepath.Join(target, "deployments", "worker-deployment.yml"))
	if got := digString(worker, "spec", "template", "spec", "containers", "0", "image"); got != "registry.release/worker@sha256:abcdef" {
		t.Fatalf("worker image = %q", got)
	}
	if got := digInt(worker, "spec", "replicas"); got != 0 {
		t.Fatalf("worker replicas = %d, want 0", got)
	}
}

func TestInventoryContainsProfilesAndMTLSPaths(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "assets", "inventory.cfg"), "value=1\n")
	writeFile(t, filepath.Join(envDir, "replica-profiles.yml"), `
defaults:
  profile: normal
profiles:
  normal:
    apps:
      tsm-deco: 1
`)
	writeFile(t, filepath.Join(envDir, "apps", "tsm-deco.yml"), `
name: tsm-deco
rollout_on:
  checksums:
    config:
      files:
        - env.unsecured.json
        - assets/inventory.cfg
replicas: 2
containers:
  - name: tsm-deco
    image: "{{TSM_REGISTRY_URL}}/tsm-deco:{{TSM_RELEASE_ID}}"
    mtls:
      enabled: true
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	payload, err := Inventory(Options{Environment: "test", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := digString(payload, "profiles", "defaults", "profile"); got != "normal" {
		t.Fatalf("profile default = %q, want normal", got)
	}
	items := payload["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	item := items[0]
	if got := item["replicas"]; got != 2 {
		t.Fatalf("inventory replicas = %#v, want 2", got)
	}
	mtlsPaths := item["mtls_paths"].(map[string]any)
	if got := mtlsPaths["secured_json"]; got != "test/mtls/tsm-deco/tsm-deco.secured.json" {
		t.Fatalf("secured_json = %#v", got)
	}
	checksums := item["rollout_checksums"].(map[string]string)
	if got := checksums["checksum/config"]; got != checksumForFiles(t, envDir, "assets/inventory.cfg", "env.unsecured.json") {
		t.Fatalf("checksum/config = %q", got)
	}
}

func TestResourceSummarySupportsTextAndStructuredData(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE": "summary-test",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 3
containers:
  - name: api
    image: api:latest
    resources:
      cpu:
        from: "250m"
        to: "1"
      memory:
        from: "128Mi"
        to: "512Mi"
`)

	summary, err := ResourceSummary(Options{Environment: "test", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Environment != "test" {
		t.Fatalf("environment = %q, want test", summary.Environment)
	}
	if summary.Totals.Apps != 1 || summary.Totals.Containers != 1 || summary.Totals.Replicas != 3 {
		t.Fatalf("unexpected totals: %#v", summary.Totals)
	}
	if summary.Totals.CPURequestCores != 0.75 {
		t.Fatalf("cpu request total = %v, want 0.75", summary.Totals.CPURequestCores)
	}
	if summary.Totals.CPULimitCores != 3 {
		t.Fatalf("cpu limit total = %v, want 3", summary.Totals.CPULimitCores)
	}
	if summary.Totals.MemoryRequestMiB != 384 {
		t.Fatalf("memory request total = %v, want 384", summary.Totals.MemoryRequestMiB)
	}
	if summary.Totals.MemoryLimitMiB != 1536 {
		t.Fatalf("memory limit total = %v, want 1536", summary.Totals.MemoryLimitMiB)
	}
	text := FormatResourceSummaryText(summary)
	for _, expected := range []string{
		"Resource summary for environment: test",
		"+-----+-----+-----------+---------+---------+---------+---------+------------+------------+------------+------------+",
		"| api |   3 | api       | 250m    | 1       | 128Mi   | 512Mi   |      0.750 |      3.000 |    384.0Mi |   1536.0Mi |",
		"cpu req:    0.750 cores",
		"mem lim:    1536.0Mi",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("summary text missing %q:\n%s", expected, text)
		}
	}
}

func TestBuildGeneratesAssetConfigMapVolumeAndMount(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
			"DYNAMIC_VALUE":    "resolved",
		},
	})
	rawContent := "value={{DYNAMIC_VALUE}}\n"
	writeFile(t, filepath.Join(envDir, "assets", "app.conf"), rawContent)
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    assets:
      - file: assets/app.conf
        to: /app/app.conf
        transform: false
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(result.Assets))
	}

	expectedDigest := assetCRC("assets/app.conf", "/app/app.conf", rawContent)
	expectedName := "api-asset-" + expectedDigest

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "volumes", "0", "name"); got != expectedName {
		t.Fatalf("volume name = %q, want %s", got, expectedName)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts", "0", "mountPath"); got != "/app/app.conf" {
		t.Fatalf("mountPath = %q, want /app/app.conf", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts", "0", "subPath"); got != "app.conf" {
		t.Fatalf("subPath = %q, want app.conf", got)
	}

	configMap := loadYAML(t, filepath.Join(target, "assets", expectedName+".yml"))
	if got := digString(configMap, "metadata", "name"); got != expectedName {
		t.Fatalf("configMap name = %q, want %s", got, expectedName)
	}
	if got := digString(configMap, "data", "app.conf"); got != rawContent {
		t.Fatalf("asset content = %q, want raw content", got)
	}
}

func TestBuildGeneratesSharedAssetsAndCanDisableThemPerApp(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
			"SHARED_VALUE":     "resolved",
		},
	})
	writeFile(t, filepath.Join(envDir, "assets", "shared.conf.tpl"), "shared={{SHARED_VALUE}}\n")
	writeFile(t, filepath.Join(envDir, "shared.assets.yml"), `
assets:
  - file: assets/shared.conf.tpl
    to: /app/shared.conf
    transform: true
`)
	writeMinimalApp(t, envDir, "api.yml", `
name: api
`)
	writeMinimalApp(t, envDir, "isolated.yml", `
name: isolated
disable_shared_assets: true
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(result.Assets))
	}

	content := "shared=resolved\n"
	expectedName := "shared-asset-" + assetCRC("assets/shared.conf.tpl", "/app/shared.conf", content)
	shared := loadYAML(t, filepath.Join(target, "assets", "shared", expectedName+".yml"))
	if got := digString(shared, "data", "shared.conf"); got != content {
		t.Fatalf("shared asset content = %q, want %q", got, content)
	}

	api := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(api, "spec", "template", "spec", "volumes", "0", "name"); got != expectedName {
		t.Fatalf("shared volume name = %q, want %s", got, expectedName)
	}
	if got := digString(api, "spec", "template", "spec", "containers", "0", "volumeMounts", "0", "mountPath"); got != "/app/shared.conf" {
		t.Fatalf("shared mountPath = %q, want /app/shared.conf", got)
	}

	isolated := loadYAML(t, filepath.Join(target, "deployments", "isolated-deployment.yml"))
	if mounts := digSlice(isolated, "spec", "template", "spec", "containers", "0", "volumeMounts"); len(mounts) != 0 {
		t.Fatalf("isolated app has shared mounts: %#v", mounts)
	}
	if volumes := digSlice(isolated, "spec", "template", "spec", "volumes"); len(volumes) != 0 {
		t.Fatalf("isolated app has shared volumes: %#v", volumes)
	}
}

func TestBuildGeneratesTransformedAndBinaryAssets(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
			"DYNAMIC_VALUE":    "resolved",
		},
	})
	writeFile(t, filepath.Join(envDir, "assets", "rendered.conf.tpl"), "value={{DYNAMIC_VALUE}}\n")
	writeFile(t, filepath.Join(envDir, "assets", "cert.bin"), "\x00\x01abc")
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    assets:
      - file: assets/rendered.conf.tpl
        to: /app/rendered.conf
        transform: true
      - file: assets/cert.bin
        to: /app/cert.bin
        binary: true
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	renderedContent := "value=resolved\n"
	renderedName := "api-asset-" + assetCRC("assets/rendered.conf.tpl", "/app/rendered.conf", renderedContent)
	renderedConfigMap := loadYAML(t, filepath.Join(target, "assets", renderedName+".yml"))
	if got := digString(renderedConfigMap, "data", "rendered.conf"); got != renderedContent {
		t.Fatalf("rendered asset content = %q, want %q", got, renderedContent)
	}

	binaryContent := "\x00\x01abc"
	binaryName := "api-asset-" + assetCRC("assets/cert.bin", "/app/cert.bin", binaryContent)
	binaryConfigMap := loadYAML(t, filepath.Join(target, "assets", binaryName+".yml"))
	if got := digString(binaryConfigMap, "binaryData", "cert.bin"); got != base64.StdEncoding.EncodeToString([]byte(binaryContent)) {
		t.Fatalf("binary asset content = %q", got)
	}
}

func TestBuildGeneratesVolumeOnlyAssetTypes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    assets:
      - temp: true
        to: /tmp/cache
      - pvc: true
        name: data-claim
        to: /data
      - nfs-server: nfs.local
        path: /export/data
        to: /nfs
      - host-path: /var/lib/host-data
        to: /host
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Assets) != 0 {
		t.Fatalf("len(configmap assets) = %d, want 0", len(result.Assets))
	}
	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	volumes := digSlice(deployment, "spec", "template", "spec", "volumes")
	if len(volumes) != 4 {
		t.Fatalf("len(volumes) = %d, want 4: %#v", len(volumes), volumes)
	}
	if got := digString(deployment, "spec", "template", "spec", "volumes", "1", "persistentVolumeClaim", "claimName"); got != "data-claim" {
		t.Fatalf("pvc claimName = %q", got)
	}
	if emptyDir := digAny(deployment, "spec", "template", "spec", "volumes", "0", "emptyDir"); emptyDir == nil {
		t.Fatalf("temp volume missing emptyDir: %#v", volumes[0])
	}
	if got := digString(deployment, "spec", "template", "spec", "volumes", "2", "nfs", "server"); got != "nfs.local" {
		t.Fatalf("nfs server = %q", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "volumes", "3", "hostPath", "path"); got != "/var/lib/host-data" {
		t.Fatalf("hostPath path = %q", got)
	}
	if subPath := digAny(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts", "0", "subPath"); subPath != nil {
		t.Fatalf("temp mount has subPath: %#v", subPath)
	}
}

func TestBuildGeneratesModernMounts(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "assets", "app.conf"), "value=1\n")
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    mounts:
      - type: config
        file: assets/app.conf
        mount_path: /app/app.conf
      - type: empty_dir
        name: cache
        mount_path: /tmp/cache
      - type: pvc
        name: data
        claim_name: data-claim
        mount_path: /data
      - type: nfs
        name: nfs-data
        server: nfs.local
        path: /export/data
        mount_path: /nfs
      - type: host_path
        name: host-data
        path: /var/lib/host-data
        mount_path: /host
      - type: raw
        volume:
          name: projected-config
          projected:
            sources: []
        mount:
          name: projected-config
          mountPath: /app/projected
          readOnly: true
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Assets) != 1 {
		t.Fatalf("len(configmap assets) = %d, want 1", len(result.Assets))
	}
	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts", "0", "mountPath"); got != "/app/app.conf" {
		t.Fatalf("config mountPath = %q", got)
	}
	if emptyDir := digAny(deployment, "spec", "template", "spec", "volumes", "1", "emptyDir"); emptyDir == nil {
		t.Fatalf("empty_dir volume missing emptyDir")
	}
	if got := digString(deployment, "spec", "template", "spec", "volumes", "2", "persistentVolumeClaim", "claimName"); got != "data-claim" {
		t.Fatalf("pvc claimName = %q", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "volumes", "3", "nfs", "server"); got != "nfs.local" {
		t.Fatalf("nfs server = %q", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "volumes", "4", "hostPath", "path"); got != "/var/lib/host-data" {
		t.Fatalf("hostPath path = %q", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "volumes", "5", "name"); got != "projected-config" {
		t.Fatalf("raw volume name = %q", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts", "5", "mountPath"); got != "/app/projected" {
		t.Fatalf("raw mountPath = %q", got)
	}
	if got := digBool(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts", "5", "readOnly"); !got {
		t.Fatalf("raw mount readOnly = false, want true")
	}
}

func TestBuildHelmEscapesAssets(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "assets", "values.yaml"), "plain={{ HELM_VALUE }} wrapped={{`{{ ALREADY }}`}}\n")
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    assets:
      - file: assets/values.yaml
        to: /app/values.yaml
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target, HelmEscapeAssets: true})
	if err != nil {
		t.Fatal(err)
	}
	expected := "plain={{`{{ HELM_VALUE }}`}} wrapped={{`{{ ALREADY }}`}}\n"
	name := "api-asset-" + assetCRC("assets/values.yaml", "/app/values.yaml", expected)
	configMap := loadYAML(t, filepath.Join(target, "assets", name+".yml"))
	if got := digString(configMap, "data", "values.yaml"); got != expected {
		t.Fatalf("helm escaped content = %q, want %q", got, expected)
	}
}

func TestBuildAddsMTLSAssets(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeJSON(t, filepath.Join(envDir, "mtls", "api", "api.secured.json"), map[string]any{
		"_public_key": "dummy",
		"environment": map[string]any{
			"tls.crt": "dummy",
		},
	})
	writeJSON(t, filepath.Join(envDir, "mtls", "api", "api.secured.schema.json"), map[string]any{
		"*": map[string]any{
			"encoding": "base64",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    mtls:
      enabled: true
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	result, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Assets) != 2 {
		t.Fatalf("len(assets) = %d, want 2", len(result.Assets))
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	mounts := digSlice(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts")
	mountPaths := map[string]bool{}
	for _, item := range mounts {
		mount, _ := item.(map[string]any)
		path, _ := mount["mountPath"].(string)
		mountPaths[path] = true
	}
	if !mountPaths["/app/mtls.enc/mtls.secured.json"] {
		t.Fatalf("missing mtls.secured.json mount: %#v", mountPaths)
	}
	if !mountPaths["/app/mtls.enc/mtls.secured.schema.json"] {
		t.Fatalf("missing mtls.secured.schema.json mount: %#v", mountPaths)
	}
}

func TestBuildAddsMTLSAssetsWithCustomMountDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeJSON(t, filepath.Join(envDir, "mtls", "api", "api.secured.json"), map[string]any{"environment": map[string]any{}})
	writeJSON(t, filepath.Join(envDir, "mtls", "api", "api.secured.schema.json"), map[string]any{})
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    mtls:
      enabled: true
      mount_dir: /custom/mtls
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	mounts := digSlice(deployment, "spec", "template", "spec", "containers", "0", "volumeMounts")
	mountPaths := map[string]bool{}
	for _, item := range mounts {
		mount, _ := item.(map[string]any)
		path, _ := mount["mountPath"].(string)
		mountPaths[path] = true
	}
	if !mountPaths["/custom/mtls/mtls.secured.json"] {
		t.Fatalf("missing custom mtls.secured.json mount: %#v", mountPaths)
	}
	if !mountPaths["/custom/mtls/mtls.secured.schema.json"] {
		t.Fatalf("missing custom mtls.secured.schema.json mount: %#v", mountPaths)
	}
}

func TestBuildMergesDefaultsVarsWithOverrideAndRemove(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "_defaults.yml"), `
replicas: 3
vars:
  - name: APP_NAME
    value: "from-defaults"
  - name: REMOVED
    value: "should-not-appear"
labels:
  inherited: "true"
containers:
  - name: ignored-default-container
`)
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
vars:
  - name: APP_NAME
    value: "api"
  - name: REMOVED
    remove: true
name: {{var:APP_NAME}}
containers:
  - name: "{{var:APP_NAME}}"
    image: "{{TSM_REGISTRY_URL}}/{{var:APP_NAME}}:{{TSM_RELEASE_ID}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	if got := digString(deployment, "metadata", "name"); got != "api" {
		t.Fatalf("metadata.name = %q, want api", got)
	}
	if got := digString(deployment, "metadata", "labels", "inherited"); got != "true" {
		t.Fatalf("metadata.labels.inherited = %q, want true", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "image"); got != "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}" {
		t.Fatalf("container image = %q, want unresolved apply-env placeholders", got)
	}
	if got := digString(deployment, "spec", "template", "spec", "containers", "0", "args", "1"); got != "echo ok" {
		t.Fatalf("container arg = %q, want echo ok", got)
	}
}

func TestBuildAppliesContainerEnvVarDefaultsOverrideAndRemove(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	envDir := filepath.Join(root, "test")
	writeJSON(t, filepath.Join(envDir, "env.unsecured.json"), map[string]any{
		"environment": map[string]any{
			"NAMESPACE":        "nac-test",
			"TSM_REGISTRY_URL": "registry.local/tsm",
			"TSM_RELEASE_ID":   "1.0.0",
		},
	})
	writeFile(t, filepath.Join(envDir, "apps", "_defaults.yml"), `
container_env_vars:
  - name: "*"
    env_vars:
      - name: GLOBAL_FLAG
        value: "true"
      - name: SHARED_SECRET
        secret_name: shared-secret
        key: shared-key
  - name: "api"
    env_vars:
      - name: SERVICE_ONLY
        value: "service-default"
`)
	writeFile(t, filepath.Join(envDir, "apps", "api.yml"), `
name: api
replicas: 1
containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    env_vars:
      - name: GLOBAL_FLAG
        value: "false"
      - name: SHARED_SECRET
        remove: true
      - name: LOCAL_ONLY
        field_path: spec.nodeName
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu:
        from: "100m"
        to: "200m"
      memory:
        from: "128Mi"
        to: "256Mi"
`)

	_, err := Build(Options{Environment: "test", Root: root, Target: target})
	if err != nil {
		t.Fatal(err)
	}

	deployment := loadYAML(t, filepath.Join(target, "deployments", "api-deployment.yml"))
	env := digSlice(deployment, "spec", "template", "spec", "containers", "0", "env")
	if len(env) != 3 {
		t.Fatalf("len(env) = %d, want 3: %#v", len(env), env)
	}
	if got := envValue(env, "GLOBAL_FLAG"); got != "false" {
		t.Fatalf("GLOBAL_FLAG = %q, want false", got)
	}
	if got := envValue(env, "SERVICE_ONLY"); got != "service-default" {
		t.Fatalf("SERVICE_ONLY = %q, want service-default", got)
	}
	if got := envFieldPath(env, "LOCAL_ONLY"); got != "spec.nodeName" {
		t.Fatalf("LOCAL_ONLY fieldPath = %q, want spec.nodeName", got)
	}
	if got := envValue(env, "SHARED_SECRET"); got != "" {
		t.Fatalf("SHARED_SECRET unexpectedly exists: %q", got)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(content))
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

func writeMinimalApp(t *testing.T, envDir string, fileName string, header string) {
	t.Helper()
	writeFile(t, filepath.Join(envDir, "apps", fileName), header+`
replicas: 1
containers:
  - name: app
    image: "{{TSM_REGISTRY_URL}}/app:{{TSM_RELEASE_ID}}"
    startup:
      command: ["/bin/sh"]
      arguments: ["-c", "echo ok"]
    resources:
      cpu: {from: "100m", to: "200m"}
      memory: {from: "128Mi", to: "256Mi"}
`)
}

func loadYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := yaml.Unmarshal(content, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func digString(value any, path ...string) string {
	current := value
	for _, part := range path {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil {
				return ""
			}
			if len(typed) <= index {
				return ""
			}
			current = typed[index]
		default:
			return ""
		}
	}
	if out, ok := current.(string); ok {
		return out
	}
	return ""
}

func digInt(value any, path ...string) int {
	current := digAny(value, path...)
	switch typed := current.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func digBool(value any, path ...string) bool {
	current := digAny(value, path...)
	out, _ := current.(bool)
	return out
}

func digSlice(value any, path ...string) []any {
	current := digAny(value, path...)
	if out, ok := current.([]any); ok {
		return out
	}
	return nil
}

func digAny(value any, path ...string) any {
	current := value
	for _, part := range path {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil {
				return nil
			}
			if index < 0 || len(typed) <= index {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}
	return current
}

func envValue(items []any, name string) string {
	item := envItem(items, name)
	if item == nil {
		return ""
	}
	value, _ := item["value"].(string)
	return value
}

func envFieldPath(items []any, name string) string {
	item := envItem(items, name)
	if item == nil {
		return ""
	}
	valueFrom, _ := item["valueFrom"].(map[string]any)
	fieldRef, _ := valueFrom["fieldRef"].(map[string]any)
	fieldPath, _ := fieldRef["fieldPath"].(string)
	return fieldPath
}

func envItem(items []any, name string) map[string]any {
	for _, item := range items {
		env, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if env["name"] == name {
			return env
		}
	}
	return nil
}

func assetCRC(relativeFile string, targetPath string, content string) string {
	return fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(relativeFile+targetPath+content)))
}

func checksumForFiles(t *testing.T, envDir string, relativePaths ...string) string {
	t.Helper()
	sort.Strings(relativePaths)
	hash := sha256.New()
	for _, relative := range relativePaths {
		content, err := os.ReadFile(filepath.Join(envDir, relative))
		if err != nil {
			t.Fatal(err)
		}
		hash.Write([]byte(relative))
		hash.Write([]byte{0})
		hash.Write(content)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func writeFakeEncjson(t *testing.T, name string, output string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, name+".args")
	binPath := filepath.Join(dir, name)
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" > %q
cat <<'JSON'
%s
JSON
`, argsPath, output)
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binPath, argsPath
}
