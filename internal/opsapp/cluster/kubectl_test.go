package cluster

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestInspectNamespaceUsesKubectlJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake kubectl is Unix-only")
	}
	binDir := t.TempDir()
	kubectlPath := filepath.Join(binDir, "kubectl")
	if err := os.WriteFile(kubectlPath, []byte(fakeKubectlScript), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	got, err := InspectNamespace(context.Background(), "kube-ops-test", Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Namespace != "kube-ops-test" {
		t.Fatalf("summary = %#v", got)
	}
	if got.DeploymentCount != 1 || got.ReadyDeployments != 1 || got.PodCount != 2 || got.ReadyPods != 1 || got.ServiceCount != 1 {
		t.Fatalf("summary counts = %#v", got)
	}
	if got.Deployments[0].Name != "api" || got.Deployments[0].Ready != 2 {
		t.Fatalf("deployments = %#v", got.Deployments)
	}
	if got.Services[0].Ports != "80:8080/TCP" {
		t.Fatalf("services = %#v", got.Services)
	}
}

func TestInspectNamespaceReturnsUnavailableSummaryOnKubectlError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake kubectl is Unix-only")
	}
	binDir := t.TempDir()
	kubectlPath := filepath.Join(binDir, "kubectl")
	if err := os.WriteFile(kubectlPath, []byte("#!/bin/sh\necho boom >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := InspectNamespace(context.Background(), "missing", Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if got.Available || got.Error == "" {
		t.Fatalf("summary = %#v, want unavailable with error", got)
	}
}

const fakeKubectlScript = `#!/bin/sh
resource=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "get" ]; then
    resource="$arg"
    break
  fi
  prev="$arg"
done
case "$resource" in
  deployments)
    cat <<'JSON'
{"items":[{"metadata":{"name":"api"},"spec":{"replicas":2},"status":{"readyReplicas":2,"availableReplicas":2,"updatedReplicas":2}}]}
JSON
    ;;
  pods)
    cat <<'JSON'
{"items":[{"metadata":{"name":"api-1"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}},{"metadata":{"name":"api-2"},"status":{"phase":"Pending","containerStatuses":[{"ready":false,"restartCount":1}]}}]}
JSON
    ;;
  services)
    cat <<'JSON'
{"items":[{"metadata":{"name":"api"},"spec":{"type":"ClusterIP","clusterIP":"10.0.0.1","ports":[{"port":80,"targetPort":8080,"protocol":"TCP"}]}}]}
JSON
    ;;
  *)
    echo "unexpected resource $resource" >&2
    exit 2
    ;;
esac
`
