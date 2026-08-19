package buildapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyResourcePoliciesReplacesContainerAndSidecarResources(t *testing.T) {
	root := t.TempDir()
	policyPath := filepath.Join(root, "test", "apps", "api.yml")
	writeResourcePolicyTestFile(t, policyPath, `
containers:
  api:
    cpu:
      from: 250m
      to: "2"
    memory:
      from: 512Mi
      to: 1Gi
  metrics:
    cpu:
      from: 10m
      to: 50m
    memory:
      from: 16Mi
      to: 64Mi
`)
	apps := []appModel{{
		Name: "api",
		Containers: []containerSpec{{
			Name:      "api",
			Resources: map[string]map[string]any{"cpu": {"from": "1m", "to": "2m"}},
		}},
		Sidecars: []containerSpec{{Name: "metrics"}},
	}}

	err := applyResourcePolicies(apps, []string{"/source/test/apps/api.yml"}, Options{
		Environment:        "test",
		ResourcePolicyRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resourceRequestValue(apps[0].Containers[0].Resources, "cpu"); got != "250m" {
		t.Fatalf("container cpu request = %q, want 250m", got)
	}
	if got := resourceLimitValue(apps[0].Containers[0].Resources, "memory"); got != "1Gi" {
		t.Fatalf("container memory limit = %q, want 1Gi", got)
	}
	if got := resourceLimitValue(apps[0].Sidecars[0].Resources, "cpu"); got != "50m" {
		t.Fatalf("sidecar cpu limit = %q, want 50m", got)
	}
	if _, exists := apps[0].Containers[0].Resources["cpu"]["requests"]; exists {
		t.Fatal("inline resources were merged instead of replaced")
	}
}

func TestApplyResourcePoliciesRequiresMatchingPolicyFile(t *testing.T) {
	err := applyResourcePolicies(
		[]appModel{{Name: "api", Containers: []containerSpec{{Name: "api"}}}},
		[]string{"/source/test/apps/api.yml"},
		Options{Environment: "test", ResourcePolicyRoot: t.TempDir()},
	)
	if err == nil || !strings.Contains(err.Error(), "resource policy file not found") {
		t.Fatalf("error = %v, want missing policy file", err)
	}
}

func TestApplyResourcePoliciesRejectsMissingAndUnknownContainers(t *testing.T) {
	for _, test := range []struct {
		name       string
		containers string
		want       string
	}{
		{
			name: "missing",
			containers: `
  other:
    cpu: {from: 10m, to: 20m}
    memory: {from: 16Mi, to: 32Mi}
`,
			want: `has no entry for container "api"`,
		},
		{
			name: "unknown",
			containers: `
  api:
    cpu: {from: 10m, to: 20m}
    memory: {from: 16Mi, to: 32Mi}
  obsolete:
    cpu: {from: 10m, to: 20m}
    memory: {from: 16Mi, to: 32Mi}
`,
			want: "contains unknown container(s): obsolete",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeResourcePolicyTestFile(t, filepath.Join(root, "test", "apps", "api.yml"), "containers:\n"+test.containers)
			err := applyResourcePolicies(
				[]appModel{{Name: "api", Containers: []containerSpec{{Name: "api"}}}},
				[]string{"/source/test/apps/api.yml"},
				Options{Environment: "test", ResourcePolicyRoot: root},
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestApplyResourcePoliciesRequiresCompleteCPUAndMemoryRanges(t *testing.T) {
	root := t.TempDir()
	writeResourcePolicyTestFile(t, filepath.Join(root, "test", "apps", "api.yml"), `
containers:
  api:
    cpu: {from: 100m}
    memory: {from: 128Mi, to: 256Mi}
`)
	err := applyResourcePolicies(
		[]appModel{{Name: "api", Containers: []containerSpec{{Name: "api"}}}},
		[]string{"/source/test/apps/api.yml"},
		Options{Environment: "test", ResourcePolicyRoot: root},
	)
	if err == nil || !strings.Contains(err.Error(), "cpu.to is required") {
		t.Fatalf("error = %v, want incomplete cpu range", err)
	}
}

func writeResourcePolicyTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
