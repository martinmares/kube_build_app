package importapp

import (
	"strings"
	"testing"
)

func TestImportDeploymentMapsStructuredFieldsAndPreservesRawFields(t *testing.T) {
	result, err := ImportDeployment([]byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: order-api
  namespace: source-ns
  labels:
    team: orders
  annotations:
    deployment.kubernetes.io/revision: "7"
    owner: l2
spec:
  replicas: 2
  selector:
    matchLabels:
      workload: order-api
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        workload: order-api
      annotations:
        checksum/config: abc
    spec:
      serviceAccountName: order-api
      automountServiceAccountToken: false
      shareProcessNamespace: true
      imagePullSecrets:
        - name: registry-auth
      nodeSelector:
        workload: orders
      tolerations:
        - key: dedicated
          operator: Equal
          value: orders
          effect: NoSchedule
      volumes:
        - name: config
          configMap:
            name: order-api-config
      initContainers:
        - name: migrate
          image: registry.local/migrate:1
          command: ["/bin/migrate"]
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
      containers:
        - name: order-api
          image: registry.local/order-api:1
          command: ["/app/start"]
          args: ["--http"]
          env:
            - name: EMPTY
              value: ""
            - name: PASSWORD
              valueFrom:
                secretKeyRef:
                  name: order-api
                  key: password
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: CPU_LIMIT
              valueFrom:
                resourceFieldRef:
                  resource: limits.cpu
          envFrom:
            - configMapRef:
                name: common-env
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
          livenessProbe:
            httpGet:
              path: /health/live
              port: 8080
            periodSeconds: 10
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
          volumeMounts:
            - name: config
              mountPath: /app/config
        - name: metrics-helper
          image: registry.local/metrics-helper:1
`), "fixture.yml")
	if err != nil {
		t.Fatal(err)
	}

	if result.App.Name != "order-api" || result.App.Replicas != 2 {
		t.Fatalf("unexpected app identity: %#v", result.App)
	}
	if result.App.Strategy != "recreate" {
		t.Fatalf("strategy = %q, want recreate", result.App.Strategy)
	}
	if len(result.App.Containers) != 2 {
		t.Fatalf("containers = %d, want 2", len(result.App.Containers))
	}
	if len(result.App.InitContainers) != 1 {
		t.Fatalf("init containers = %d, want 1", len(result.App.InitContainers))
	}
	if result.App.Pod == nil || !result.App.Pod.ShareProcessNamespace {
		t.Fatal("share_process_namespace was not imported")
	}
	if result.App.WorkloadIdentity == nil ||
		result.App.WorkloadIdentity.ServiceAccount.Name != "order-api" ||
		result.App.WorkloadIdentity.ServiceAccount.Automount == nil ||
		*result.App.WorkloadIdentity.ServiceAccount.Automount {
		t.Fatalf("unexpected workload identity: %#v", result.App.WorkloadIdentity)
	}
	if len(result.App.Registry) != 1 || result.App.Registry[0].SecretName != "registry-auth" {
		t.Fatalf("unexpected registry: %#v", result.App.Registry)
	}
	if _, ok := result.App.PodRaw["volumes"]; !ok {
		t.Fatalf("pod volumes were not preserved in pod_raw: %#v", result.App.PodRaw)
	}

	main := result.App.Containers[0]
	if main.Startup == nil || len(main.Startup.Command) != 1 || len(main.Startup.Arguments) != 1 {
		t.Fatalf("startup was not imported: %#v", main.Startup)
	}
	if len(main.Envs) != 4 {
		t.Fatalf("envs = %d, want 4: %#v", len(main.Envs), main.Envs)
	}
	if main.Envs[0].Value != "" {
		t.Fatalf("empty env value changed to %q", main.Envs[0].Value)
	}
	if main.Envs[1].SecretName != "order-api" || main.Envs[1].Key != "password" {
		t.Fatalf("secret reference was not imported: %#v", main.Envs[1])
	}
	if main.Envs[2].FieldPath != "metadata.name" {
		t.Fatalf("fieldRef was not imported: %#v", main.Envs[2])
	}
	if main.Envs[3].ResourceName != "limits.cpu" || main.Envs[3].Divisor != "" {
		t.Fatalf("resourceFieldRef was not imported: %#v", main.Envs[3])
	}
	if _, ok := main.Raw["volumeMounts"]; !ok {
		t.Fatalf("volume mounts were not preserved in raw: %#v", main.Raw)
	}
	if got := main.Resources["cpu"]["from"]; got != "100m" {
		t.Fatalf("cpu.from = %#v, want 100m", got)
	}
	if got := main.Resources["cpu"]["to"]; got != "500m" {
		t.Fatalf("cpu.to = %#v, want 500m", got)
	}
	if result.Report.SecretValuesRead {
		t.Fatal("import report claims that Secret values were read")
	}
	if result.Report.RelatedObjectsRead {
		t.Fatal("deployment-only import claims that related objects were read")
	}

	content, err := MarshalApp(result.App)
	if err != nil {
		t.Fatal(err)
	}
	output := string(content)
	for _, expected := range []string{
		"name: order-api",
		"strategy: recreate",
		"selector_labels:",
		"share_process_namespace: true",
		"secret_name: order-api",
		"field_path: metadata.name",
		"from: 100m",
		"to: 500m",
		"volumeMounts:",
		"name: metrics-helper",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("imported YAML missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "sidecars:") {
		t.Fatalf("import guessed sidecars:\n%s", output)
	}
	if strings.Contains(output, "deployment.kubernetes.io/revision") {
		t.Fatalf("runtime revision annotation was imported:\n%s", output)
	}
	if strings.Contains(output, "requests:") || strings.Contains(output, "limits:") {
		t.Fatalf("import emitted compatibility resource aliases instead of canonical from/to:\n%s", output)
	}
}

func TestEnrichWithServicesBuildsCanonicalPortExposure(t *testing.T) {
	result, err := ImportDeployment([]byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tsm-gateway
  namespace: tsm-cetin-zis-sandbox
spec:
  replicas: 1
  progressDeadlineSeconds: 600
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: tsm-gateway
  template:
    metadata:
      labels:
        app: tsm-gateway
      annotations:
        kubectl.kubernetes.io/restartedAt: "2026-07-26T04:08:11Z"
        linkerd.io/inject: disabled
    spec:
      terminationGracePeriodSeconds: 30
      serviceAccount: default
      serviceAccountName: default
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      enableServiceLinks: true
      preemptionPolicy: PreemptLowerPriority
      priority: 0
      hostIPC: false
      hostNetwork: false
      hostPID: false
      setHostnameAsFQDN: false
      containers:
        - name: tsm-gateway
          image: registry.local/tsm-gateway:2.4
          stdin: false
          stdinOnce: false
          tty: false
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
          ports:
            - containerPort: 8080
              protocol: TCP
          livenessProbe:
            httpGet:
              path: /actuator/health/liveness
              port: 8080
            initialDelaySeconds: 60
            periodSeconds: 5
            timeoutSeconds: 1
            successThreshold: 1
            failureThreshold: 3
          resources:
            requests:
              cpu: 100m
              memory: 250Mi
            limits:
              cpu: "4"
              memory: 1100Mi
`), "deployment.yml")
	if err != nil {
		t.Fatal(err)
	}

	err = EnrichWithServices(&result, []byte(`
apiVersion: v1
kind: ServiceList
items:
  - apiVersion: v1
    kind: Service
    metadata:
      name: unrelated
    spec:
      selector:
        app: another-app
      ports:
        - name: http
          port: 80
          targetPort: 8080
  - apiVersion: v1
    kind: Service
    metadata:
      name: tsm-gateway
    spec:
      selector:
        app: tsm-gateway
      ports:
        - name: http
          port: 80
          protocol: TCP
          targetPort: 8080
`))
	if err != nil {
		t.Fatal(err)
	}

	if result.Report.Services != 1 || !result.Report.RelatedObjectsRead {
		t.Fatalf("unexpected service report: %#v", result.Report)
	}
	if len(result.App.Containers[0].Ports) != 1 {
		t.Fatalf("ports = %#v", result.App.Containers[0].Ports)
	}
	port := result.App.Containers[0].Ports[0]
	if port.Name != "http" || port.Port != 8080 {
		t.Fatalf("unexpected imported port: %#v", port)
	}
	if len(port.ExposeAs) != 1 ||
		port.ExposeAs[0].ServiceName != "tsm-gateway" ||
		port.ExposeAs[0].Port != 80 {
		t.Fatalf("unexpected service exposure: %#v", port.ExposeAs)
	}

	content, err := MarshalApp(result.App)
	if err != nil {
		t.Fatal(err)
	}
	output := string(content)
	for _, expected := range []string{
		"selector_labels:",
		"from: 100m",
		`to: "4"`,
		"from: 250Mi",
		"to: 1100Mi",
		"- name: http",
		"service_name: tsm-gateway",
		"port: 80",
		"linkerd.io/inject: disabled",
		"delay: 60",
		"period: 5",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("imported YAML missing %q:\n%s", expected, output)
		}
	}
	for _, unwanted := range []string{
		"requests:",
		"limits:",
		"kubectl.kubernetes.io/restartedAt",
		"termination_grace_period:",
		"progressDeadlineSeconds:",
		"revisionHistoryLimit:",
		"deployment_raw:",
		"workload_identity:",
		"timeout: 1",
		"success: 1",
		"failure: 3",
		"terminationMessagePath:",
		"terminationMessagePolicy:",
		"preemptionPolicy:",
		"priority:",
		"hostNetwork:",
		"stdin:",
		"tty:",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("imported YAML contains %q:\n%s", unwanted, output)
		}
	}
}

func TestImportDeploymentPreservesUnsupportedProbeAndCustomStrategy(t *testing.T) {
	result, err := ImportDeployment([]byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  selector:
    matchLabels:
      app: api
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 2
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: api:1
          readinessProbe:
            tcpSocket:
              port: http
`), "fixture.yml")
	if err != nil {
		t.Fatal(err)
	}
	if result.App.Strategy != "" {
		t.Fatalf("custom strategy was incorrectly simplified to %q", result.App.Strategy)
	}
	spec, ok := result.App.DeploymentRaw["spec"].(map[string]any)
	if !ok || len(mapAt(spec, "strategy")) == 0 {
		t.Fatalf("custom strategy was not preserved: %#v", result.App.DeploymentRaw)
	}
	if _, ok := result.App.Containers[0].Raw["readinessProbe"]; !ok {
		t.Fatalf("unsupported probe was not preserved: %#v", result.App.Containers[0].Raw)
	}
}

func TestImportDeploymentRejectsOtherKinds(t *testing.T) {
	_, err := ImportDeployment([]byte(`
apiVersion: v1
kind: Service
metadata:
  name: api
`), "service.yml")
	if err == nil || !strings.Contains(err.Error(), "expected kind Deployment") {
		t.Fatalf("unexpected error: %v", err)
	}
}
