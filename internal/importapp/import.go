package importapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Result struct {
	App       AppModel
	Report    Report
	podLabels map[string]any
}

type Report struct {
	Source             string   `json:"source"`
	Kind               string   `json:"kind"`
	Name               string   `json:"name"`
	Namespace          string   `json:"namespace,omitempty"`
	Containers         int      `json:"containers"`
	InitContainers     int      `json:"init_containers"`
	Services           int      `json:"services"`
	RawPreservedFields []string `json:"raw_preserved_fields,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	RelatedObjectsRead bool     `json:"related_objects_read"`
	SecretValuesRead   bool     `json:"secret_values_read"`
}

type AppModel struct {
	Name                   string                  `yaml:"name"`
	Strategy               string                  `yaml:"strategy,omitempty"`
	Replicas               int                     `yaml:"replicas"`
	Labels                 map[string]any          `yaml:"labels,omitempty"`
	SelectorLabels         map[string]any          `yaml:"selector_labels,omitempty"`
	Annotations            map[string]any          `yaml:"annotations,omitempty"`
	PodAnnotations         map[string]any          `yaml:"pod_annotations,omitempty"`
	SecurityContext        map[string]any          `yaml:"security_context,omitempty"`
	TerminationGracePeriod *int                    `yaml:"termination_grace_period,omitempty"`
	WorkloadIdentity       *WorkloadIdentity       `yaml:"workload_identity,omitempty"`
	Pod                    *PodOptions             `yaml:"pod,omitempty"`
	Registry               []Registry              `yaml:"registry,omitempty"`
	DNS                    []HostAlias             `yaml:"dns,omitempty"`
	Scheduling             *Scheduling             `yaml:"scheduling,omitempty"`
	InitContainers         []ImportedInitContainer `yaml:"init_containers,omitempty"`
	Containers             []ImportedContainer     `yaml:"containers"`
	DeploymentRaw          map[string]any          `yaml:"deployment_raw,omitempty"`
	PodRaw                 map[string]any          `yaml:"pod_raw,omitempty"`
}

type WorkloadIdentity struct {
	ServiceAccount WorkloadServiceAccount `yaml:"service_account"`
}

type WorkloadServiceAccount struct {
	Name      string `yaml:"name,omitempty"`
	Automount *bool  `yaml:"automount,omitempty"`
}

type PodOptions struct {
	ShareProcessNamespace bool `yaml:"share_process_namespace,omitempty"`
}

type Registry struct {
	SecretName string `yaml:"secret_name"`
}

type HostAlias struct {
	IP        string   `yaml:"ip"`
	Hostnames []string `yaml:"hostnames"`
}

type Scheduling struct {
	NodeSelector map[string]any `yaml:"node_selector,omitempty"`
	Tolerations  []any          `yaml:"tolerations,omitempty"`
	Affinity     map[string]any `yaml:"affinity,omitempty"`
}

type ImportedContainer struct {
	Name            string                    `yaml:"name"`
	Image           string                    `yaml:"image"`
	ImagePullPolicy string                    `yaml:"image_pull_policy,omitempty"`
	Startup         *Startup                  `yaml:"startup,omitempty"`
	Envs            []ImportedEnv             `yaml:"envs,omitempty"`
	EnvFrom         []map[string]any          `yaml:"env_from,omitempty"`
	Ports           []ImportedPort            `yaml:"ports,omitempty"`
	Probes          map[string]any            `yaml:"probes,omitempty"`
	Resources       map[string]map[string]any `yaml:"resources,omitempty"`
	SecurityContext map[string]any            `yaml:"security_context,omitempty"`
	Raw             map[string]any            `yaml:"raw,omitempty"`
}

type ImportedInitContainer struct {
	Name            string                    `yaml:"name"`
	Image           string                    `yaml:"image"`
	ImagePullPolicy string                    `yaml:"image_pull_policy,omitempty"`
	Command         []string                  `yaml:"command,omitempty"`
	Arguments       []string                  `yaml:"arguments,omitempty"`
	Envs            []ImportedEnv             `yaml:"envs,omitempty"`
	EnvFrom         []map[string]any          `yaml:"env_from,omitempty"`
	Resources       map[string]map[string]any `yaml:"resources,omitempty"`
	SecurityContext map[string]any            `yaml:"security_context,omitempty"`
	Raw             map[string]any            `yaml:"raw,omitempty"`
}

type Startup struct {
	Command   []string `yaml:"command,omitempty"`
	Arguments []string `yaml:"arguments,omitempty"`
}

type ImportedEnv struct {
	Name         string `yaml:"name"`
	Value        string `yaml:"value,omitempty"`
	SecretName   string `yaml:"secret_name,omitempty"`
	Key          string `yaml:"key,omitempty"`
	ResourceName string `yaml:"resource_name,omitempty"`
	Divisor      string `yaml:"divisor,omitempty"`
	FieldPath    string `yaml:"field_path,omitempty"`
}

type ImportedPort struct {
	Name          string           `yaml:"name"`
	Port          int              `yaml:"port"`
	ExposeAs      []ImportedExpose `yaml:"expose_as,omitempty"`
	generatedName bool
}

type ImportedExpose struct {
	ServiceName string `yaml:"service_name"`
	Port        int    `yaml:"port"`
	Type        string `yaml:"type,omitempty"`
}

type KubectlOptions struct {
	Namespace  string
	Deployment string
	Kubeconfig string
	Context    string
	Timeout    time.Duration
}

func ImportDeployment(content []byte, source string) (Result, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return Result{}, fmt.Errorf("parse deployment YAML: %w", err)
	}
	if len(document) == 0 {
		return Result{}, errors.New("deployment YAML is empty")
	}
	kind := stringAt(document, "kind")
	if kind != "Deployment" {
		return Result{}, fmt.Errorf("expected kind Deployment, got %q", kind)
	}
	apiVersion := stringAt(document, "apiVersion")
	if apiVersion != "apps/v1" {
		return Result{}, fmt.Errorf("expected apiVersion apps/v1, got %q", apiVersion)
	}

	metadata := mapAt(document, "metadata")
	name := stringAt(metadata, "name")
	if name == "" {
		return Result{}, errors.New("deployment metadata.name is required")
	}
	namespace := stringAt(metadata, "namespace")
	spec := mapAt(document, "spec")
	template := mapAt(spec, "template")
	templateMetadata := mapAt(template, "metadata")
	podSpec := mapAt(template, "spec")
	containerItems := sliceAt(podSpec, "containers")
	if len(containerItems) == 0 {
		return Result{}, errors.New("deployment spec.template.spec.containers must not be empty")
	}

	report := Report{
		Source:             source,
		Kind:               kind,
		Name:               name,
		Namespace:          namespace,
		Containers:         len(containerItems),
		InitContainers:     len(sliceAt(podSpec, "initContainers")),
		RelatedObjectsRead: false,
		SecretValuesRead:   false,
	}
	report.addWarning("all Kubernetes containers were imported as primary containers; sidecar roles cannot be inferred safely")
	report.addWarning("Services, Ingresses, Routes, HPAs and ConfigMaps are not available in a Deployment-only import")
	if namespace != "" {
		report.addWarning("the source namespace is not stored in the app model; the target environment supplies NAMESPACE")
	}

	app := AppModel{
		Name:           name,
		Replicas:       intAtDefault(spec, "replicas", 1),
		Labels:         cleanMetadataMap(mapAt(metadata, "labels")),
		SelectorLabels: cloneMap(mapAt(mapAt(spec, "selector"), "matchLabels")),
		Annotations:    cleanDeploymentAnnotations(mapAt(metadata, "annotations")),
		PodAnnotations: cleanPodAnnotations(mapAt(templateMetadata, "annotations")),
		Containers:     make([]ImportedContainer, 0, len(containerItems)),
	}
	if len(sliceAt(mapAt(spec, "selector"), "matchExpressions")) > 0 {
		return Result{}, errors.New("deployment selectors with matchExpressions cannot be represented safely by selector_labels")
	}
	if len(app.SelectorLabels) == 0 {
		return Result{}, errors.New("deployment spec.selector.matchLabels must not be empty")
	}

	for index, item := range containerItems {
		containerMap, ok := item.(map[string]any)
		if !ok {
			return Result{}, fmt.Errorf("container %d must be a mapping", index)
		}
		container, err := importContainer(containerMap, fmt.Sprintf("containers[%d]", index), &report)
		if err != nil {
			return Result{}, err
		}
		app.Containers = append(app.Containers, container)
	}

	for index, item := range sliceAt(podSpec, "initContainers") {
		containerMap, ok := item.(map[string]any)
		if !ok {
			return Result{}, fmt.Errorf("init container %d must be a mapping", index)
		}
		container, err := importInitContainer(containerMap, fmt.Sprintf("initContainers[%d]", index), &report)
		if err != nil {
			return Result{}, err
		}
		app.InitContainers = append(app.InitContainers, container)
	}

	app.SecurityContext = nonEmptyMap(mapAt(podSpec, "securityContext"))
	if value, ok := intPointerAt(podSpec, "terminationGracePeriodSeconds"); ok && *value != 30 {
		app.TerminationGracePeriod = value
	}
	if share, ok := boolAt(podSpec, "shareProcessNamespace"); ok && share {
		app.Pod = &PodOptions{ShareProcessNamespace: true}
	}
	app.WorkloadIdentity = importServiceAccount(podSpec)
	if app.WorkloadIdentity != nil && app.WorkloadIdentity.ServiceAccount.Name != "" {
		report.addWarning("the referenced ServiceAccount is preserved with create=false because Deployment input does not prove ownership")
	}
	app.Registry = importRegistries(sliceAt(podSpec, "imagePullSecrets"))
	app.DNS = importHostAliases(sliceAt(podSpec, "hostAliases"))
	app.Scheduling = importScheduling(podSpec)

	podRaw := cloneMap(podSpec)
	removeKeys(podRaw,
		"containers", "initContainers", "securityContext", "terminationGracePeriodSeconds",
		"serviceAccount", "serviceAccountName", "automountServiceAccountToken",
		"shareProcessNamespace", "imagePullSecrets", "hostAliases", "nodeSelector",
		"tolerations", "affinity",
	)
	dropPodDefaults(podRaw)
	if len(podRaw) > 0 {
		app.PodRaw = podRaw
		for _, key := range sortedKeys(podRaw) {
			report.addRawField("pod_raw." + key)
		}
	}

	deploymentRaw := map[string]any{}
	specRaw := cloneMap(spec)
	removeKeys(specRaw, "replicas", "selector", "template")
	importDeploymentStrategy(specRaw, &app)
	dropDeploymentDefaults(specRaw)
	templateRaw := map[string]any{}
	if labels := mapAt(templateMetadata, "labels"); len(labels) > 0 {
		extraLabels := cloneMap(labels)
		for key, selectorValue := range app.SelectorLabels {
			if labelValue, ok := extraLabels[key]; ok && fmt.Sprint(labelValue) == fmt.Sprint(selectorValue) {
				delete(extraLabels, key)
			}
		}
		if len(extraLabels) > 0 {
			templateRaw["metadata"] = map[string]any{"labels": extraLabels}
		}
	}
	if len(templateRaw) > 0 {
		specRaw["template"] = templateRaw
	}
	if len(specRaw) > 0 {
		deploymentRaw["spec"] = specRaw
		for _, key := range sortedKeys(specRaw) {
			report.addRawField("deployment_raw.spec." + key)
		}
	}
	if len(deploymentRaw) > 0 {
		app.DeploymentRaw = deploymentRaw
	}

	sort.Strings(report.RawPreservedFields)
	return Result{
		App:       app,
		Report:    report,
		podLabels: cloneMap(mapAt(templateMetadata, "labels")),
	}, nil
}

func EnrichWithServices(result *Result, content []byte) error {
	if result == nil {
		return errors.New("import result is required")
	}
	items, err := parseServiceItems(content)
	if err != nil {
		return err
	}
	result.Report.RelatedObjectsRead = true
	removeWarning(&result.Report, "Services, Ingresses, Routes, HPAs and ConfigMaps are not available in a Deployment-only import")

	for _, service := range items {
		spec := mapAt(service, "spec")
		selector := mapAt(spec, "selector")
		if len(selector) == 0 || !selectorMatches(selector, result.podLabels) {
			continue
		}
		name := stringAt(mapAt(service, "metadata"), "name")
		if name == "" {
			result.Report.addWarning("a matching Service without metadata.name was skipped")
			continue
		}
		result.Report.Services++
		serviceType := ""
		kubernetesServiceType := stringAt(spec, "type")
		switch {
		case stringAt(spec, "clusterIP") == "None":
			serviceType = "headless"
		case kubernetesServiceType == "" || kubernetesServiceType == "ClusterIP":
		default:
			result.Report.addWarning(fmt.Sprintf("Service %q type %q is not represented by the metamodel and will render as ClusterIP", name, kubernetesServiceType))
		}
		for index, rawPort := range sliceAt(spec, "ports") {
			servicePort, ok := rawPort.(map[string]any)
			if !ok {
				result.Report.addWarning(fmt.Sprintf("Service %q port %d is not a mapping and was skipped", name, index))
				continue
			}
			if protocol := stringAt(servicePort, "protocol"); protocol != "" && protocol != "TCP" {
				result.Report.addWarning(fmt.Sprintf("Service %q port %d uses unsupported protocol %q and was skipped", name, index, protocol))
				continue
			}
			port, ok := intAt(servicePort, "port")
			if !ok || port <= 0 {
				result.Report.addWarning(fmt.Sprintf("Service %q port %d has an invalid service port and was skipped", name, index))
				continue
			}
			target := servicePort["targetPort"]
			if target == nil {
				target = port
			}
			containerIndex, portIndex, ok := findTargetPort(result.App.Containers, target)
			if !ok {
				targetPort, numberTarget := numericTargetPort(target)
				if numberTarget && len(result.App.Containers) == 1 {
					portName := stringAt(servicePort, "name")
					generatedName := false
					if portName == "" {
						portName = defaultPortName(targetPort)
						generatedName = true
					}
					result.App.Containers[0].Ports = append(result.App.Containers[0].Ports, ImportedPort{
						Name:          portName,
						Port:          targetPort,
						generatedName: generatedName,
					})
					containerIndex = 0
					portIndex = len(result.App.Containers[0].Ports) - 1
					ok = true
				}
			}
			if !ok {
				result.Report.addWarning(fmt.Sprintf("Service %q port %d targetPort %v could not be assigned safely to a container", name, index, target))
				continue
			}
			containerPort := &result.App.Containers[containerIndex].Ports[portIndex]
			if servicePortName := stringAt(servicePort, "name"); containerPort.generatedName && servicePortName != "" &&
				portNameAvailable(result.App.Containers[containerIndex].Ports, portIndex, servicePortName) {
				containerPort.Name = servicePortName
				containerPort.generatedName = false
			}
			if hasServiceExposure(containerPort.ExposeAs, name, port) {
				result.Report.addWarning(fmt.Sprintf("Service %q exposes container port %q more than once; only the first mapping was imported", name, containerPort.Name))
				continue
			}
			containerPort.ExposeAs = append(containerPort.ExposeAs, ImportedExpose{
				ServiceName: name,
				Port:        port,
				Type:        serviceType,
			})
		}
	}
	result.Report.addWarning("Ingresses, Routes, HPAs and ConfigMaps are not imported yet")
	sort.Strings(result.Report.Warnings)
	return nil
}

func parseServiceItems(content []byte) ([]map[string]any, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, errors.New("services YAML is empty")
	}
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("parse services YAML: %w", err)
	}
	switch stringAt(document, "kind") {
	case "List", "ServiceList":
		out := make([]map[string]any, 0, len(sliceAt(document, "items")))
		for index, raw := range sliceAt(document, "items") {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("service list item %d must be a mapping", index)
			}
			if kind := stringAt(item, "kind"); kind != "" && kind != "Service" {
				continue
			}
			out = append(out, item)
		}
		return out, nil
	case "Service":
		return []map[string]any{document}, nil
	default:
		return nil, fmt.Errorf("expected Service or ServiceList, got %q", stringAt(document, "kind"))
	}
}

func selectorMatches(selector map[string]any, labels map[string]any) bool {
	for key, expected := range selector {
		actual, ok := labels[key]
		if !ok || fmt.Sprint(actual) != fmt.Sprint(expected) {
			return false
		}
	}
	return true
}

func findTargetPort(containers []ImportedContainer, target any) (int, int, bool) {
	if port, ok := numericTargetPort(target); ok {
		foundContainer, foundPort := -1, -1
		for containerIndex := range containers {
			for portIndex := range containers[containerIndex].Ports {
				if containers[containerIndex].Ports[portIndex].Port != port {
					continue
				}
				if foundContainer >= 0 {
					return 0, 0, false
				}
				foundContainer, foundPort = containerIndex, portIndex
			}
		}
		if foundContainer >= 0 {
			return foundContainer, foundPort, true
		}
		return 0, 0, false
	}
	name := strings.TrimSpace(fmt.Sprint(target))
	if name == "" {
		return 0, 0, false
	}
	foundContainer, foundPort := -1, -1
	for containerIndex := range containers {
		for portIndex := range containers[containerIndex].Ports {
			if containers[containerIndex].Ports[portIndex].Name == name {
				if foundContainer >= 0 {
					return 0, 0, false
				}
				foundContainer, foundPort = containerIndex, portIndex
			}
		}
	}
	if foundContainer >= 0 {
		return foundContainer, foundPort, true
	}
	return 0, 0, false
}

func numericTargetPort(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed > 0
	case int64:
		return int(typed), typed > 0
	case float64:
		return int(typed), typed > 0
	default:
		parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
		return parsed, err == nil && parsed > 0
	}
}

func portNameAvailable(ports []ImportedPort, current int, name string) bool {
	for index, port := range ports {
		if index != current && port.Name == name {
			return false
		}
	}
	return true
}

func hasServiceExposure(items []ImportedExpose, serviceName string, port int) bool {
	for _, item := range items {
		if item.ServiceName == serviceName && item.Port == port {
			return true
		}
	}
	return false
}

func removeWarning(report *Report, message string) {
	out := report.Warnings[:0]
	for _, warning := range report.Warnings {
		if warning != message {
			out = append(out, warning)
		}
	}
	report.Warnings = out
}

func MarshalApp(app AppModel) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(app); err != nil {
		return nil, fmt.Errorf("marshal app model: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close app model encoder: %w", err)
	}
	return buffer.Bytes(), nil
}

func MarshalReport(report Report) ([]byte, error) {
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal import report: %w", err)
	}
	return append(content, '\n'), nil
}

func ReadDeploymentFromKubectl(ctx context.Context, options KubectlOptions) ([]byte, error) {
	namespace := strings.TrimSpace(options.Namespace)
	deployment := strings.TrimSpace(options.Deployment)
	if namespace == "" {
		return nil, errors.New("namespace is required for cluster import")
	}
	if deployment == "" {
		return nil, errors.New("deployment is required for cluster import")
	}
	return readKubectlObject(ctx, options, "deployment", deployment)
}

func ReadServicesFromKubectl(ctx context.Context, options KubectlOptions) ([]byte, error) {
	namespace := strings.TrimSpace(options.Namespace)
	if namespace == "" {
		return nil, errors.New("namespace is required for cluster import")
	}
	return readKubectlObject(ctx, options, "services", "")
}

func readKubectlObject(ctx context.Context, options KubectlOptions, resource string, name string) ([]byte, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, 0, 12)
	if value := strings.TrimSpace(options.Kubeconfig); value != "" {
		args = append(args, "--kubeconfig", value)
	}
	if value := strings.TrimSpace(options.Context); value != "" {
		args = append(args, "--context", value)
	}
	args = append(args, "-n", strings.TrimSpace(options.Namespace), "get", resource)
	if name != "" {
		args = append(args, name)
	}
	args = append(args, "-o", "yaml")
	command := exec.CommandContext(ctx, "kubectl", args...)
	content, err := command.CombinedOutput()
	if err != nil {
		action := "kubectl get " + resource
		message := strings.TrimSpace(string(content))
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s timed out after %s", action, timeout)
		}
		if message == "" {
			return nil, fmt.Errorf("%s: %w", action, err)
		}
		return nil, fmt.Errorf("%s: %w: %s", action, err, message)
	}
	return content, nil
}

func importContainer(source map[string]any, path string, report *Report) (ImportedContainer, error) {
	name := stringAt(source, "name")
	image := stringAt(source, "image")
	if name == "" || image == "" {
		return ImportedContainer{}, fmt.Errorf("%s requires name and image", path)
	}
	out := ImportedContainer{Name: name, Image: image}
	if policy := stringAt(source, "imagePullPolicy"); policy != "" && policy != "Always" {
		out.ImagePullPolicy = policy
	}
	command := stringSliceAt(source, "command")
	arguments := stringSliceAt(source, "args")
	if len(command) > 0 || len(arguments) > 0 {
		out.Startup = &Startup{Command: command, Arguments: arguments}
	}
	out.SecurityContext = nonEmptyMap(mapAt(source, "securityContext"))
	out.Resources, out.Raw = importResources(source, out.Raw, path, report)
	out.Envs, out.Raw = importEnvs(source, out.Raw, path, report)
	out.EnvFrom = importEnvFrom(sliceAt(source, "envFrom"))
	out.Ports, out.Raw = importPorts(source, out.Raw, path, report)
	out.Probes, out.Raw = importProbes(source, out.Raw, path, report)

	raw := cloneMap(source)
	removeKeys(raw,
		"name", "image", "imagePullPolicy", "command", "args", "securityContext",
		"resources", "env", "envFrom", "ports", "livenessProbe", "readinessProbe",
		"startupProbe",
	)
	dropContainerDefaults(raw)
	out.Raw = mergeMaps(out.Raw, raw)
	for _, key := range sortedKeys(raw) {
		report.addRawField(path + ".raw." + key)
	}
	return out, nil
}

func importInitContainer(source map[string]any, path string, report *Report) (ImportedInitContainer, error) {
	name := stringAt(source, "name")
	image := stringAt(source, "image")
	if name == "" || image == "" {
		return ImportedInitContainer{}, fmt.Errorf("%s requires name and image", path)
	}
	out := ImportedInitContainer{
		Name:            name,
		Image:           image,
		Command:         stringSliceAt(source, "command"),
		Arguments:       stringSliceAt(source, "args"),
		SecurityContext: nonEmptyMap(mapAt(source, "securityContext")),
		EnvFrom:         importEnvFrom(sliceAt(source, "envFrom")),
	}
	if policy := stringAt(source, "imagePullPolicy"); policy != "" && policy != "Always" {
		out.ImagePullPolicy = policy
	}
	out.Resources, out.Raw = importResources(source, out.Raw, path, report)
	out.Envs, out.Raw = importEnvs(source, out.Raw, path, report)
	raw := cloneMap(source)
	removeKeys(raw, "name", "image", "imagePullPolicy", "command", "args", "securityContext", "resources", "env", "envFrom")
	dropContainerDefaults(raw)
	out.Raw = mergeMaps(out.Raw, raw)
	for _, key := range sortedKeys(raw) {
		report.addRawField(path + ".raw." + key)
	}
	return out, nil
}

func importResources(source map[string]any, raw map[string]any, path string, report *Report) (map[string]map[string]any, map[string]any) {
	resources := mapAt(source, "resources")
	if len(resources) == 0 {
		return nil, raw
	}
	requests := mapAt(resources, "requests")
	limits := mapAt(resources, "limits")
	known := map[string]bool{"cpu": true, "memory": true, "ephemeral-storage": true}
	for key := range requests {
		if !known[key] {
			raw = putRaw(raw, "resources", cloneMap(resources))
			report.addRawField(path + ".raw.resources")
			return nil, raw
		}
	}
	for key := range limits {
		if !known[key] {
			raw = putRaw(raw, "resources", cloneMap(resources))
			report.addRawField(path + ".raw.resources")
			return nil, raw
		}
	}
	out := map[string]map[string]any{}
	for _, resource := range []string{"cpu", "memory", "ephemeral-storage"} {
		values := map[string]any{}
		if value, ok := requests[resource]; ok {
			values["from"] = value
		}
		if value, ok := limits[resource]; ok {
			values["to"] = value
		}
		if len(values) > 0 {
			out[resource] = values
		}
	}
	return out, raw
}

func importEnvs(source map[string]any, raw map[string]any, path string, report *Report) ([]ImportedEnv, map[string]any) {
	items := sliceAt(source, "env")
	if len(items) == 0 {
		return nil, raw
	}
	out := make([]ImportedEnv, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, preserveRawEnv(raw, items, path, report)
		}
		converted, ok := importEnv(entry)
		if !ok {
			return nil, preserveRawEnv(raw, items, path, report)
		}
		out = append(out, converted)
	}
	return out, raw
}

func preserveRawEnv(raw map[string]any, items []any, path string, report *Report) map[string]any {
	report.addWarning(path + " contains environment valueFrom fields outside the structured metamodel; the complete env list was preserved in raw")
	report.addRawField(path + ".raw.env")
	return putRaw(raw, "env", cloneSlice(items))
}

func importEnv(source map[string]any) (ImportedEnv, bool) {
	name := stringAt(source, "name")
	if name == "" {
		return ImportedEnv{}, false
	}
	if valueFrom := mapAt(source, "valueFrom"); len(valueFrom) > 0 {
		if secret := mapAt(valueFrom, "secretKeyRef"); len(secret) > 0 {
			if hasUnexpectedKeys(secret, "name", "key", "optional") || truthyAt(secret, "optional") {
				return ImportedEnv{}, false
			}
			return ImportedEnv{Name: name, SecretName: stringAt(secret, "name"), Key: stringAt(secret, "key")}, true
		}
		if field := mapAt(valueFrom, "fieldRef"); len(field) > 0 {
			if hasUnexpectedKeys(field, "fieldPath", "apiVersion") {
				return ImportedEnv{}, false
			}
			return ImportedEnv{Name: name, FieldPath: stringAt(field, "fieldPath")}, true
		}
		if resource := mapAt(valueFrom, "resourceFieldRef"); len(resource) > 0 {
			if hasUnexpectedKeys(resource, "resource", "divisor") {
				return ImportedEnv{}, false
			}
			imported := ImportedEnv{
				Name:         name,
				ResourceName: stringAt(resource, "resource"),
			}
			if divisor, ok := resource["divisor"]; ok && divisor != nil {
				imported.Divisor = fmt.Sprint(divisor)
			}
			return imported, true
		}
		return ImportedEnv{}, false
	}
	value := ""
	if rawValue, ok := source["value"]; ok && rawValue != nil {
		value = fmt.Sprint(rawValue)
	}
	return ImportedEnv{Name: name, Value: value}, true
}

func importEnvFrom(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if entry, ok := item.(map[string]any); ok {
			out = append(out, cloneMap(entry))
		}
	}
	return out
}

func importPorts(source map[string]any, raw map[string]any, path string, report *Report) ([]ImportedPort, map[string]any) {
	items := sliceAt(source, "ports")
	if len(items) == 0 {
		return nil, raw
	}
	out := make([]ImportedPort, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok || hasUnexpectedKeys(entry, "name", "containerPort", "protocol") {
			report.addRawField(path + ".raw.ports")
			return nil, putRaw(raw, "ports", cloneSlice(items))
		}
		if protocol := stringAt(entry, "protocol"); protocol != "" && protocol != "TCP" {
			report.addRawField(path + ".raw.ports")
			return nil, putRaw(raw, "ports", cloneSlice(items))
		}
		port, ok := intAt(entry, "containerPort")
		if !ok {
			report.addRawField(path + ".raw.ports")
			return nil, putRaw(raw, "ports", cloneSlice(items))
		}
		name := stringAt(entry, "name")
		generatedName := false
		if name == "" {
			name = defaultPortName(port)
			generatedName = true
		}
		out = append(out, ImportedPort{Name: name, Port: port, generatedName: generatedName})
	}
	return out, raw
}

func importProbes(source map[string]any, raw map[string]any, path string, report *Report) (map[string]any, map[string]any) {
	out := map[string]any{}
	for kubernetesName, modelName := range map[string]string{
		"livenessProbe":  "live",
		"readinessProbe": "ready",
		"startupProbe":   "start",
	} {
		probe := mapAt(source, kubernetesName)
		if len(probe) == 0 {
			continue
		}
		converted, ok := importProbe(probe)
		if !ok {
			raw = putRaw(raw, kubernetesName, cloneMap(probe))
			report.addRawField(path + ".raw." + kubernetesName)
			continue
		}
		out[modelName] = converted
	}
	if len(out) == 0 {
		return nil, raw
	}
	return out, raw
}

func importProbe(source map[string]any) (map[string]any, bool) {
	allowed := []string{
		"httpGet", "exec", "initialDelaySeconds", "periodSeconds", "timeoutSeconds",
		"successThreshold", "failureThreshold",
	}
	if hasUnexpectedKeys(source, allowed...) {
		return nil, false
	}
	out := map[string]any{}
	if httpGet := mapAt(source, "httpGet"); len(httpGet) > 0 {
		if hasUnexpectedKeys(httpGet, "path", "port", "scheme") {
			return nil, false
		}
		if scheme := stringAt(httpGet, "scheme"); scheme != "" && scheme != "HTTP" {
			return nil, false
		}
		port, ok := intAt(httpGet, "port")
		if !ok {
			return nil, false
		}
		out["http"] = map[string]any{"path": stringAt(httpGet, "path"), "port": port}
	} else if execHandler := mapAt(source, "exec"); len(execHandler) > 0 {
		if hasUnexpectedKeys(execHandler, "command") {
			return nil, false
		}
		out["command"] = stringSliceAt(execHandler, "command")
	} else {
		return nil, false
	}
	for kubernetesName, field := range map[string]struct {
		modelName string
		defaultV  int
	}{
		"initialDelaySeconds": {modelName: "delay", defaultV: 0},
		"periodSeconds":       {modelName: "period", defaultV: 10},
		"timeoutSeconds":      {modelName: "timeout", defaultV: 1},
		"successThreshold":    {modelName: "success", defaultV: 1},
		"failureThreshold":    {modelName: "failure", defaultV: 3},
	} {
		if value, ok := intAt(source, kubernetesName); ok && value != field.defaultV {
			out[field.modelName] = value
		}
	}
	return out, true
}

func importServiceAccount(podSpec map[string]any) *WorkloadIdentity {
	name := stringAt(podSpec, "serviceAccountName")
	if name == "" {
		name = stringAt(podSpec, "serviceAccount")
	}
	automount, hasAutomount := boolAt(podSpec, "automountServiceAccountToken")
	if name == "" || name == "default" {
		if !hasAutomount || automount {
			return nil
		}
		name = "default"
	}
	serviceAccount := WorkloadServiceAccount{Name: name}
	if hasAutomount {
		serviceAccount.Automount = &automount
	}
	return &WorkloadIdentity{ServiceAccount: serviceAccount}
}

func importRegistries(items []any) []Registry {
	out := make([]Registry, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name := stringAt(entry, "name"); name != "" {
			out = append(out, Registry{SecretName: name})
		}
	}
	return out
}

func importHostAliases(items []any) []HostAlias {
	out := make([]HostAlias, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, HostAlias{IP: stringAt(entry, "ip"), Hostnames: stringSliceAt(entry, "hostnames")})
	}
	return out
}

func importScheduling(podSpec map[string]any) *Scheduling {
	out := Scheduling{
		NodeSelector: nonEmptyMap(mapAt(podSpec, "nodeSelector")),
		Tolerations:  cloneSlice(sliceAt(podSpec, "tolerations")),
		Affinity:     nonEmptyMap(mapAt(podSpec, "affinity")),
	}
	if len(out.NodeSelector) == 0 && len(out.Tolerations) == 0 && len(out.Affinity) == 0 {
		return nil
	}
	return &out
}

func importDeploymentStrategy(specRaw map[string]any, app *AppModel) {
	strategy := mapAt(specRaw, "strategy")
	if len(strategy) == 0 {
		return
	}
	strategyType := stringAt(strategy, "type")
	rollingUpdate := mapAt(strategy, "rollingUpdate")
	switch {
	case strategyType == "Recreate" && len(rollingUpdate) == 0:
		app.Strategy = "recreate"
		delete(specRaw, "strategy")
	case strategyType == "RollingUpdate" &&
		fmt.Sprint(rollingUpdate["maxSurge"]) == "0" &&
		fmt.Sprint(rollingUpdate["maxUnavailable"]) == "1":
		app.Strategy = "one-by-one"
		delete(specRaw, "strategy")
	case strategyType == "RollingUpdate" &&
		fmt.Sprint(rollingUpdate["maxSurge"]) == "25%" &&
		fmt.Sprint(rollingUpdate["maxUnavailable"]) == "25%":
		// This is kube-build-app's default strategy.
		delete(specRaw, "strategy")
	}
}

func dropContainerDefaults(raw map[string]any) {
	if stringAt(raw, "terminationMessagePath") == "/dev/termination-log" {
		delete(raw, "terminationMessagePath")
	}
	if stringAt(raw, "terminationMessagePolicy") == "File" {
		delete(raw, "terminationMessagePolicy")
	}
	for _, key := range []string{"stdin", "stdinOnce", "tty"} {
		if value, ok := raw[key].(bool); ok && !value {
			delete(raw, key)
		}
	}
}

func dropPodDefaults(raw map[string]any) {
	if stringAt(raw, "dnsPolicy") == "ClusterFirst" {
		delete(raw, "dnsPolicy")
	}
	if stringAt(raw, "restartPolicy") == "Always" {
		delete(raw, "restartPolicy")
	}
	if stringAt(raw, "schedulerName") == "default-scheduler" {
		delete(raw, "schedulerName")
	}
	if stringAt(raw, "terminationGracePeriodSeconds") == "30" {
		delete(raw, "terminationGracePeriodSeconds")
	}
	if value, ok := raw["enableServiceLinks"].(bool); ok && value {
		delete(raw, "enableServiceLinks")
	}
	if stringAt(raw, "preemptionPolicy") == "PreemptLowerPriority" {
		delete(raw, "preemptionPolicy")
	}
	if value, ok := intAt(raw, "priority"); ok && value == 0 {
		delete(raw, "priority")
	}
	for _, key := range []string{"hostIPC", "hostNetwork", "hostPID", "setHostnameAsFQDN"} {
		if value, ok := raw[key].(bool); ok && !value {
			delete(raw, key)
		}
	}
}

func dropDeploymentDefaults(raw map[string]any) {
	if value, ok := intAt(raw, "progressDeadlineSeconds"); ok && value == 600 {
		delete(raw, "progressDeadlineSeconds")
	}
	if value, ok := intAt(raw, "revisionHistoryLimit"); ok && value == 10 {
		delete(raw, "revisionHistoryLimit")
	}
}

func defaultPortName(port int) string {
	return fmt.Sprintf("tcp-%d", port)
}

func cleanMetadataMap(source map[string]any) map[string]any {
	return nonEmptyMap(cloneMap(source))
}

func cleanDeploymentAnnotations(source map[string]any) map[string]any {
	out := cloneMap(source)
	delete(out, "deployment.kubernetes.io/revision")
	delete(out, "kubectl.kubernetes.io/last-applied-configuration")
	return nonEmptyMap(out)
}

func cleanPodAnnotations(source map[string]any) map[string]any {
	out := cloneMap(source)
	delete(out, "kubectl.kubernetes.io/restartedAt")
	return nonEmptyMap(out)
}

func (report *Report) addWarning(message string) {
	for _, current := range report.Warnings {
		if current == message {
			return
		}
	}
	report.Warnings = append(report.Warnings, message)
}

func (report *Report) addRawField(path string) {
	for _, current := range report.RawPreservedFields {
		if current == path {
			return
		}
	}
	report.RawPreservedFields = append(report.RawPreservedFields, path)
}

func mapAt(source map[string]any, key string) map[string]any {
	value, _ := source[key].(map[string]any)
	return value
}

func sliceAt(source map[string]any, key string) []any {
	value, _ := source[key].([]any)
	return value
}

func stringAt(source map[string]any, key string) string {
	value, ok := source[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func intAt(source map[string]any, key string) (int, bool) {
	value, ok := source[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		parsed, err := strconv.Atoi(fmt.Sprint(value))
		return parsed, err == nil
	}
}

func intAtDefault(source map[string]any, key string, fallback int) int {
	if value, ok := intAt(source, key); ok {
		return value
	}
	return fallback
}

func intPointerAt(source map[string]any, key string) (*int, bool) {
	value, ok := intAt(source, key)
	if !ok {
		return nil, false
	}
	return &value, true
}

func boolAt(source map[string]any, key string) (bool, bool) {
	value, ok := source[key].(bool)
	return value, ok
}

func truthyAt(source map[string]any, key string) bool {
	value, _ := source[key].(bool)
	return value
}

func stringSliceAt(source map[string]any, key string) []string {
	items := sliceAt(source, key)
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func cloneMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneSlice(source []any) []any {
	if len(source) == 0 {
		return nil
	}
	out := make([]any, len(source))
	for index, value := range source {
		out[index] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		return cloneSlice(typed)
	default:
		return typed
	}
}

func nonEmptyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	return source
}

func mergeMaps(left map[string]any, right map[string]any) map[string]any {
	if len(left) == 0 {
		return cloneMap(right)
	}
	out := cloneMap(left)
	for key, value := range right {
		out[key] = cloneValue(value)
	}
	return out
}

func putRaw(raw map[string]any, key string, value any) map[string]any {
	if raw == nil {
		raw = map[string]any{}
	}
	raw[key] = value
	return raw
}

func removeKeys(source map[string]any, keys ...string) {
	for _, key := range keys {
		delete(source, key)
	}
}

func hasUnexpectedKeys(source map[string]any, allowed ...string) bool {
	allowedSet := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = true
	}
	for key := range source {
		if !allowedSet[key] {
			return true
		}
	}
	return false
}

func sortedKeys(source map[string]any) []string {
	out := make([]string, 0, len(source))
	for key := range source {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
