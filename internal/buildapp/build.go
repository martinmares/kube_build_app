package buildapp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const appLabel = "app.kubernetes.io/name"

type Options struct {
	Environment      string
	Root             string
	Target           string
	Profile          string
	ProfilesFile     string
	Inventory        bool
	DecryptSecured   bool
	EnvFile          string
	VarsSources      []string
	HelmEscapeAssets bool
	ReleaseManifest  string
	Down             []string
}

type Result struct {
	Deployments []string
	Services    []string
	Assets      []string
	Budgets     []string
	Autoscaling []string
	Externals   []string
	Events      []BuildEvent
}

type BuildEvent struct {
	Type      string `json:"type"`
	App       string `json:"app,omitempty"`
	Container string `json:"container,omitempty"`
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Count     int    `json:"count,omitempty"`
	Path      string `json:"path,omitempty"`
}

type ResourceSummaryData struct {
	Environment string                `json:"environment"`
	Items       []ResourceSummaryItem `json:"items"`
	Totals      ResourceSummaryTotals `json:"totals"`
}

type ResourceSummaryItem struct {
	App                   string  `json:"app"`
	Replicas              int     `json:"replicas"`
	Container             string  `json:"container"`
	CPURequest            string  `json:"cpu_request"`
	CPULimit              string  `json:"cpu_limit"`
	MemoryRequest         string  `json:"memory_request"`
	MemoryLimit           string  `json:"memory_limit"`
	JavaXms               string  `json:"java_xms"`
	JavaXmx               string  `json:"java_xmx"`
	CPURequestCores       float64 `json:"cpu_request_cores"`
	CPULimitCores         float64 `json:"cpu_limit_cores"`
	MemoryRequestMiB      float64 `json:"memory_request_mib"`
	MemoryLimitMiB        float64 `json:"memory_limit_mib"`
	TotalCPURequestCores  float64 `json:"total_cpu_request_cores"`
	TotalCPULimitCores    float64 `json:"total_cpu_limit_cores"`
	TotalMemoryRequestMiB float64 `json:"total_memory_request_mib"`
	TotalMemoryLimitMiB   float64 `json:"total_memory_limit_mib"`
}

type ResourceSummaryTotals struct {
	Apps             int     `json:"apps"`
	Containers       int     `json:"containers"`
	Replicas         int     `json:"replicas"`
	CPURequestCores  float64 `json:"cpu_request_cores"`
	CPULimitCores    float64 `json:"cpu_limit_cores"`
	MemoryRequestMiB float64 `json:"memory_request_mib"`
	MemoryLimitMiB   float64 `json:"memory_limit_mib"`
}

type envFile struct {
	Environment map[string]any `json:"environment"`
}

type variable struct {
	Name   string `yaml:"name"`
	Value  string `yaml:"value"`
	Remove bool   `yaml:"remove"`
}

type appModel struct {
	Vars                 []variable          `yaml:"vars"`
	Name                 string              `yaml:"name"`
	Kind                 string              `yaml:"kind"`
	Ignore               bool                `yaml:"ignore"`
	DisableSharedAssets  bool                `yaml:"disable_shared_assets"`
	DisableCreateService bool                `yaml:"disable_create_service"`
	Strategy             string              `yaml:"strategy"`
	SubdomainName        string              `yaml:"subdomain_name"`
	MinAvailable         any                 `yaml:"min_available"`
	MaxUnavailable       any                 `yaml:"max_unavailable"`
	Replicas             int                 `yaml:"replicas"`
	Labels               map[string]any      `yaml:"labels"`
	Annotations          map[string]any      `yaml:"annotations"`
	PodAnnotations       map[string]any      `yaml:"pod_annotations"`
	SecurityContext      map[string]any      `yaml:"security_context"`
	TerminationGrace     *int                `yaml:"termination_grace_period"`
	ServiceAccount       string              `yaml:"service_account"`
	DeploymentRaw        map[string]any      `yaml:"deployment_raw"`
	PodRaw               map[string]any      `yaml:"pod_raw"`
	Autoscaling          autoscalingSpec     `yaml:"autoscaling"`
	RolloutOn            rolloutOnSpec       `yaml:"rollout_on"`
	Tools                []toolSpec          `yaml:"tools"`
	InitContainers       []initContainerSpec `yaml:"init_containers"`
	Registry             []registrySpec      `yaml:"registry"`
	DNS                  []hostAliasSpec     `yaml:"dns"`
	Arch                 string              `yaml:"arch"`
	NodeSelector         map[string]any      `yaml:"node_selector"`
	Tolerations          []any               `yaml:"tolerations"`
	Scheduling           schedulingSpec      `yaml:"scheduling"`
	Containers           []containerSpec     `yaml:"containers"`
}

type rolloutOnSpec struct {
	Checksums map[string]rolloutChecksumSpec `yaml:"checksums"`
}

type rolloutChecksumSpec struct {
	Files []string `yaml:"files"`
}

type registrySpec struct {
	SecretName string `yaml:"secret_name"`
}

type hostAliasSpec struct {
	IP        string   `yaml:"ip"`
	Hostnames []string `yaml:"hostnames"`
}

type schedulingSpec struct {
	Arch         string           `yaml:"arch"`
	NodeSelector map[string]any   `yaml:"node_selector"`
	Tolerations  []any            `yaml:"tolerations"`
	Affinity     map[string]any   `yaml:"affinity"`
	Spread       spreadSpec       `yaml:"spread"`
	AntiAffinity antiAffinitySpec `yaml:"anti_affinity"`
}

type autoscalingSpec struct {
	Enabled     bool                  `yaml:"enabled"`
	MinReplicas int                   `yaml:"min_replicas"`
	MaxReplicas int                   `yaml:"max_replicas"`
	CPU         autoscalingMetricSpec `yaml:"cpu"`
	Memory      autoscalingMetricSpec `yaml:"memory"`
	Raw         map[string]any        `yaml:"raw"`
}

type autoscalingMetricSpec struct {
	AverageUtilization int `yaml:"average_utilization"`
}

type spreadSpec struct {
	By                string `yaml:"by"`
	Topology          string `yaml:"topology"`
	MaxSkew           int    `yaml:"max_skew"`
	WhenUnsatisfiable string `yaml:"when_unsatisfiable"`
}

type antiAffinitySpec struct {
	Self     string `yaml:"self"`
	Topology string `yaml:"topology"`
}

type containerSpec struct {
	Name                 string                    `yaml:"name"`
	Image                string                    `yaml:"image"`
	Assets               []assetSpec               `yaml:"assets"`
	Mounts               []mountSpec               `yaml:"mounts"`
	Vars                 []envVar                  `yaml:"vars"`
	MTLS                 mtlsSpec                  `yaml:"mtls"`
	Ports                []portSpec                `yaml:"ports"`
	Health               healthSpec                `yaml:"health"`
	Probe                probeSpec                 `yaml:"probe"`
	Probes               probesSpec                `yaml:"probes"`
	Lifecycle            lifecycleSpec             `yaml:"lifecycle"`
	SimpleInit           simpleInitSpec            `yaml:"simple_init"`
	Startup              startupSpec               `yaml:"startup"`
	EnableCgroupExporter bool                      `yaml:"enable_cgroup_exporter"`
	Resources            map[string]map[string]any `yaml:"resources"`
	Runtime              runtimeSpec               `yaml:"runtime"`
	SecurityContext      map[string]any            `yaml:"security_context"`
	EnvFrom              []envFromSpec             `yaml:"env_from"`
	Raw                  map[string]any            `yaml:"raw"`
}

type runtimeSpec struct {
	Java javaRuntimeSpec `yaml:"java"`
}

type javaRuntimeSpec struct {
	Xms    string                `yaml:"xms"`
	Xmx    string                `yaml:"xmx"`
	Opts   []string              `yaml:"opts"`
	Export javaRuntimeExportSpec `yaml:"export"`
}

type javaRuntimeExportSpec struct {
	EnvName string `yaml:"env_name"`
}

type initContainerSpec struct {
	Name            string                    `yaml:"name"`
	Image           string                    `yaml:"image"`
	Command         []string                  `yaml:"command"`
	Arguments       []string                  `yaml:"arguments"`
	Mounts          []mountSpec               `yaml:"mounts"`
	Vars            []envVar                  `yaml:"vars"`
	EnvFrom         []envFromSpec             `yaml:"env_from"`
	Resources       map[string]map[string]any `yaml:"resources"`
	SecurityContext map[string]any            `yaml:"security_context"`
	Raw             map[string]any            `yaml:"raw"`
}

type simpleInitSpec struct {
	Enabled bool               `yaml:"enabled"`
	Exec    simpleInitExecSpec `yaml:"exec"`
}

type lifecycleSpec struct {
	PreStop lifecycleHandlerSpec `yaml:"pre_stop"`
}

type lifecycleHandlerSpec struct {
	Command []string       `yaml:"command"`
	HTTP    httpHealthSpec `yaml:"http"`
	Raw     map[string]any `yaml:"raw"`
}

type simpleInitExecSpec struct {
	Command []string `yaml:"command"`
}

type envFromSpec struct {
	ConfigMap    string         `yaml:"config_map"`
	ConfigMapRef map[string]any `yaml:"configMapRef"`
	Secret       string         `yaml:"secret"`
	SecretRef    map[string]any `yaml:"secretRef"`
	Prefix       string         `yaml:"prefix"`
	Raw          map[string]any `yaml:"raw"`
}

type healthSpec struct {
	HTTP    httpHealthSpec `yaml:"http"`
	Command []string       `yaml:"command"`
	Delay   *int           `yaml:"delay"`
	Period  *int           `yaml:"period"`
	Timeout *int           `yaml:"timeout"`
	Success *int           `yaml:"success"`
	Failure *int           `yaml:"failure"`
}

type httpHealthSpec struct {
	Path pathValue `yaml:"path"`
	Port intValue  `yaml:"port"`
}

type intValue int

func (v *intValue) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode || strings.TrimSpace(value.Value) == "" {
		return nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value.Value))
	if err != nil {
		return err
	}
	*v = intValue(parsed)
	return nil
}

type pathValue struct {
	Scalar string
	ByType map[string]string
}

func (p *pathValue) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		p.Scalar = value.Value
	case yaml.MappingNode:
		p.ByType = map[string]string{}
		for i := 0; i+1 < len(value.Content); i += 2 {
			p.ByType[value.Content[i].Value] = value.Content[i+1].Value
		}
	}
	return nil
}

func (p pathValue) For(probeType string) string {
	if p.ByType != nil {
		if value := p.ByType[probeType]; value != "" {
			return value
		}
	}
	return p.Scalar
}

func (p pathValue) Empty() bool {
	return p.Scalar == "" && len(p.ByType) == 0
}

type probeSpec struct {
	Live  healthSpec `yaml:"live"`
	Ready healthSpec `yaml:"ready"`
	Start healthSpec `yaml:"start"`
}

type probesSpec struct {
	Preset string         `yaml:"preset"`
	Port   intValue       `yaml:"port"`
	Path   pathValue      `yaml:"path"`
	HTTP   httpHealthSpec `yaml:"http"`
	Live   healthSpec     `yaml:"live"`
	Ready  healthSpec     `yaml:"ready"`
	Start  healthSpec     `yaml:"start"`
}

type mtlsSpec struct {
	Enabled  bool   `yaml:"enabled"`
	MountDir string `yaml:"mount_dir"`
}

type assetSpec struct {
	File       string `yaml:"file"`
	To         string `yaml:"to"`
	Binary     bool   `yaml:"binary"`
	Transform  bool   `yaml:"transform"`
	HelmEscape *bool  `yaml:"helm_escape"`
	Temp       bool   `yaml:"temp"`
	PVC        bool   `yaml:"pvc"`
	Name       string `yaml:"name"`
	NFSServer  string `yaml:"nfs-server"`
	Path       string `yaml:"path"`
	HostPath   string `yaml:"host-path"`
}

type mountSpec struct {
	Type       string         `yaml:"type"`
	File       string         `yaml:"file"`
	MountPath  string         `yaml:"mount_path"`
	Binary     bool           `yaml:"binary"`
	Transform  bool           `yaml:"transform"`
	HelmEscape *bool          `yaml:"helm_escape"`
	Name       string         `yaml:"name"`
	ClaimName  string         `yaml:"claim_name"`
	Server     string         `yaml:"server"`
	Path       string         `yaml:"path"`
	HostPath   string         `yaml:"host_path"`
	Volume     map[string]any `yaml:"volume"`
	Mount      map[string]any `yaml:"mount"`
}

type resolvedAsset struct {
	VolumeName    string
	ConfigMapKey  string
	ContainerName string
	To            string
	Content       []byte
	Binary        bool
	Kind          string
	PVCName       string
	NFSServer     string
	NFSPath       string
	HostPath      string
	RawVolume     map[string]any
	RawMount      map[string]any
}

type sharedAssetsFile struct {
	Assets []assetSpec `yaml:"assets"`
}

type portSpec struct {
	Name           string       `yaml:"name"`
	Port           int          `yaml:"port"`
	Metrics        bool         `yaml:"metrics"`
	MetricsPathFor *string      `yaml:"metricsPathFor"`
	ExposeAs       []exposeSpec `yaml:"expose_as"`
}

type exposeSpec struct {
	Hostname string         `yaml:"hostname"`
	Port     int            `yaml:"port"`
	Type     string         `yaml:"type"`
	External []externalSpec `yaml:"external"`
}

type externalSpec struct {
	Name        string             `yaml:"name"`
	AsRoute     bool               `yaml:"as_route"`
	ClassName   string             `yaml:"class_name"`
	HTTP        []externalHostSpec `yaml:"http"`
	HTTPS       []externalHostSpec `yaml:"https"`
	TLS         map[string]any     `yaml:"tls"`
	Annotations map[string]any     `yaml:"annotations"`
	Labels      map[string]any     `yaml:"labels"`
}

type externalHostSpec struct {
	Hostname   string `yaml:"hostname"`
	Path       string `yaml:"path"`
	SecretName string `yaml:"secret_name"`
}

type toolSpec struct {
	Name      string `yaml:"name"`
	Image     string `yaml:"image"`
	ExposeBin string `yaml:"expose_bin"`
	As        string `yaml:"as"`
}

type envVar struct {
	Name         string `yaml:"name"`
	Value        string `yaml:"value,omitempty"`
	SecretName   string `yaml:"secret_name,omitempty"`
	Key          string `yaml:"key,omitempty"`
	ResourceName string `yaml:"resource_name,omitempty"`
	Divisor      string `yaml:"divisor,omitempty"`
	FieldPath    string `yaml:"field_path,omitempty"`
	Remove       bool   `yaml:"remove,omitempty"`
}

type containerVarDefault struct {
	Name string   `yaml:"name"`
	Vars []envVar `yaml:"vars"`
}

type startupSpec struct {
	Command   []string `yaml:"command"`
	Arguments []string `yaml:"arguments"`
}

var cgroupExporterDefaultVars = []envVar{
	{Name: "CGROUP_EXPORTER_METRICS_PREFIX", Value: "{{TSM_METRICS_PREFIX}}"},
	{Name: "CGROUP_EXPORTER_METRICS_STATIC_LABELS", Value: "cluster_name={{TSM_CLUSTER_NAME}}"},
	{Name: "CGROUP_EXPORTER_LISTEN", Value: "0.0.0.0:9393"},
	{Name: "CGROUP_EXPORTER_CPU_REQUESTS_MCPU", ResourceName: "requests.cpu", Divisor: "1m"},
	{Name: "CGROUP_EXPORTER_CPU_LIMITS_MCPU", ResourceName: "limits.cpu", Divisor: "1m"},
	{Name: "CGROUP_EXPORTER_MEMORY_REQUESTS_MIB", ResourceName: "requests.memory", Divisor: "1Mi"},
	{Name: "CGROUP_EXPORTER_MEMORY_LIMITS_MIB", ResourceName: "limits.memory", Divisor: "1Mi"},
	{Name: "CGROUP_EXPORTER_NODE_NAME", FieldPath: "spec.nodeName"},
}

func Build(opts Options) (Result, error) {
	if strings.TrimSpace(opts.Environment) == "" {
		return Result{}, errors.New("environment is required")
	}
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "environments"
	}
	if strings.TrimSpace(opts.Target) == "" {
		opts.Target = filepath.Join(opts.Root, opts.Environment, "target")
	}

	envDir := filepath.Join(opts.Root, opts.Environment)
	appsDir := filepath.Join(envDir, "apps")
	if !isDir(appsDir) {
		return Result{}, fmt.Errorf("apps directory not found: %s", appsDir)
	}

	vars, err := loadVars(envDir, opts)
	if err != nil {
		return Result{}, err
	}

	appFiles, err := listAppFiles(appsDir)
	if err != nil {
		return Result{}, err
	}

	deploymentsDir := filepath.Join(opts.Target, "deployments")
	if err := os.MkdirAll(deploymentsDir, 0o755); err != nil {
		return Result{}, err
	}
	servicesDir := filepath.Join(opts.Target, "services")
	if err := os.MkdirAll(servicesDir, 0o755); err != nil {
		return Result{}, err
	}
	externalServicesDir := filepath.Join(servicesDir, "external")
	assetsDir := filepath.Join(opts.Target, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return Result{}, err
	}
	sharedAssetsDir := filepath.Join(assetsDir, "shared")
	sharedAssets, err := loadSharedAssets(envDir, vars, opts)
	if err != nil {
		return Result{}, err
	}
	if len(sharedAssets) > 0 {
		if err := os.MkdirAll(sharedAssetsDir, 0o755); err != nil {
			return Result{}, err
		}
	}

	apps, err := loadApps(appFiles, filepath.Join(appsDir, "_defaults.yml"), vars)
	if err != nil {
		return Result{}, err
	}
	if err := applyReleaseManifest(apps, opts.ReleaseManifest); err != nil {
		return Result{}, err
	}
	if err := applyReplicaProfile(apps, envDir, opts); err != nil {
		return Result{}, err
	}
	applyScaleDown(apps, opts.Down)
	if err := validateApps(apps); err != nil {
		return Result{}, err
	}
	sharedAssetsWave := 0
	if appsHaveArgoCDWave(apps) {
		sharedAssetsWave = 999
	}

	result := Result{}
	result.Events = append(result.Events, BuildEvent{Type: "shared_assets", Count: len(sharedAssets)})
	for _, asset := range sharedAssets {
		if asset.Kind != "configmap" && asset.Kind != "" {
			continue
		}
		configMap := renderAssetConfigMap(asset, vars["NAMESPACE"], sharedAssetsWave)
		out, err := yaml.Marshal(configMap)
		if err != nil {
			return Result{}, err
		}
		outPath := filepath.Join(sharedAssetsDir, asset.VolumeName+".yml")
		if err := os.WriteFile(outPath, out, 0o644); err != nil {
			return Result{}, err
		}
		result.Assets = append(result.Assets, outPath)
		result.Events = append(result.Events, BuildEvent{Type: "asset", Kind: "shared", Name: asset.VolumeName, Path: outPath})
	}
	for _, app := range apps {
		if app.Ignore {
			continue
		}
		result.Events = append(result.Events, BuildEvent{Type: "app", App: app.Name, Count: len(app.Containers)})
		for _, container := range app.Containers {
			result.Events = append(result.Events, BuildEvent{Type: "container", App: app.Name, Container: container.Name, Count: len(container.Assets)})
		}
		appSharedAssets := sharedAssets
		if app.DisableSharedAssets {
			appSharedAssets = nil
		}
		resolvedAssets, err := resolveAssets(app, envDir, vars, opts)
		if err != nil {
			return Result{}, err
		}
		rolloutAnnotations, err := rolloutChecksumAnnotations(app, envDir)
		if err != nil {
			return Result{}, err
		}
		deployment, err := renderDeployment(app, vars["NAMESPACE"], resolvedAssets, appSharedAssets, rolloutAnnotations)
		if err != nil {
			return Result{}, err
		}
		out, err := yaml.Marshal(deployment)
		if err != nil {
			return Result{}, err
		}
		outPath := filepath.Join(deploymentsDir, app.Name+"-deployment.yml")
		if err := os.WriteFile(outPath, out, 0o644); err != nil {
			return Result{}, err
		}
		result.Deployments = append(result.Deployments, outPath)
		result.Events = append(result.Events, BuildEvent{Type: "deployment", App: app.Name, Name: app.Name, Path: outPath})

		if !app.DisableCreateService {
			services := renderServices(app, vars["NAMESPACE"], opts.Environment)
			for _, service := range services {
				out, err := yaml.Marshal(service.Object)
				if err != nil {
					return Result{}, err
				}
				outPath := filepath.Join(servicesDir, service.Name+"-service.yml")
				if err := os.WriteFile(outPath, out, 0o644); err != nil {
					return Result{}, err
				}
				result.Services = append(result.Services, outPath)
				result.Events = append(result.Events, BuildEvent{Type: "service", App: app.Name, Name: service.Name, Path: outPath})

				for _, external := range service.Externals {
					out, err := yaml.Marshal(external.Object)
					if err != nil {
						return Result{}, err
					}
					if err := os.MkdirAll(externalServicesDir, 0o755); err != nil {
						return Result{}, err
					}
					outPath := filepath.Join(externalServicesDir, external.Name+"-"+external.Kind+".yml")
					if err := os.WriteFile(outPath, out, 0o644); err != nil {
						return Result{}, err
					}
					result.Externals = append(result.Externals, outPath)
					result.Events = append(result.Events, BuildEvent{Type: "external", App: app.Name, Name: external.Name, Kind: external.Kind, Path: outPath})
				}
			}
		}

		for _, asset := range configMapResolvedAssets(resolvedAssets) {
			configMap := renderAssetConfigMap(asset, vars["NAMESPACE"], appArgoCDWave(app))
			out, err := yaml.Marshal(configMap)
			if err != nil {
				return Result{}, err
			}
			outPath := filepath.Join(assetsDir, asset.VolumeName+".yml")
			if err := os.WriteFile(outPath, out, 0o644); err != nil {
				return Result{}, err
			}
			result.Assets = append(result.Assets, outPath)
			result.Events = append(result.Events, BuildEvent{Type: "asset", App: app.Name, Container: asset.ContainerName, Name: asset.VolumeName, Path: outPath})
		}

		if budget := renderBudget(app, vars["NAMESPACE"]); budget != nil {
			out, err := yaml.Marshal(budget)
			if err != nil {
				return Result{}, err
			}
			outPath := filepath.Join(deploymentsDir, app.Name+"-budget.yml")
			if err := os.WriteFile(outPath, out, 0o644); err != nil {
				return Result{}, err
			}
			result.Budgets = append(result.Budgets, outPath)
			result.Events = append(result.Events, BuildEvent{Type: "budget", App: app.Name, Name: app.Name, Path: outPath})
		}

		if autoscaling := renderAutoscaling(app, vars["NAMESPACE"]); autoscaling != nil {
			out, err := yaml.Marshal(autoscaling)
			if err != nil {
				return Result{}, err
			}
			outPath := filepath.Join(deploymentsDir, app.Name+"-hpa.yml")
			if err := os.WriteFile(outPath, out, 0o644); err != nil {
				return Result{}, err
			}
			result.Autoscaling = append(result.Autoscaling, outPath)
			result.Events = append(result.Events, BuildEvent{Type: "autoscaling", App: app.Name, Name: app.Name, Path: outPath})
		}
	}

	return result, nil
}

func Inventory(opts Options) (map[string]any, error) {
	if strings.TrimSpace(opts.Environment) == "" {
		return nil, errors.New("environment is required")
	}
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "environments"
	}
	envDir := filepath.Join(opts.Root, opts.Environment)
	appsDir := filepath.Join(envDir, "apps")
	if !isDir(appsDir) {
		return nil, fmt.Errorf("apps directory not found: %s", appsDir)
	}
	vars, err := loadVars(envDir, opts)
	if err != nil {
		return nil, err
	}
	appFiles, err := listAppFiles(appsDir)
	if err != nil {
		return nil, err
	}
	apps, err := loadApps(appFiles, filepath.Join(appsDir, "_defaults.yml"), vars)
	if err != nil {
		return nil, err
	}
	if err := applyReleaseManifest(apps, opts.ReleaseManifest); err != nil {
		return nil, err
	}
	if err := validateApps(apps); err != nil {
		return nil, err
	}

	items := []map[string]any{}
	for _, app := range apps {
		if app.Ignore {
			continue
		}
		rolloutAnnotations, err := rolloutChecksumAnnotations(app, envDir)
		if err != nil {
			return nil, err
		}
		for _, container := range app.Containers {
			item := map[string]any{
				"env":                    opts.Environment,
				"app":                    app.Name,
				"app_kind":               app.Kind,
				"replicas":               app.Replicas,
				"rollout_checksums":      rolloutAnnotations,
				"container":              container.Name,
				"image":                  container.Image,
				"simple_init_enabled":    container.SimpleInit.Enabled,
				"enable_cgroup_exporter": container.EnableCgroupExporter,
				"mtls_enabled":           container.MTLS.Enabled,
				"resources":              container.Resources,
				"mtls_paths": map[string]any{
					"secured_json": filepath.ToSlash(filepath.Join(opts.Environment, "mtls", app.Name, container.Name+".secured.json")),
					"schema_json":  filepath.ToSlash(filepath.Join(opts.Environment, "mtls", app.Name, container.Name+".secured.schema.json")),
					"mount_target": mtlsMountDir(container),
					"files":        []string{"tls.crt", "tls.key", "ca.crt"},
				},
			}
			if javaRuntimeEnabled(container.Runtime.Java) {
				item["runtime"] = map[string]any{
					"java": map[string]any{
						"xms":             container.Runtime.Java.Xms,
						"xmx":             container.Runtime.Java.Xmx,
						"opts":            container.Runtime.Java.Opts,
						"export_env_name": javaRuntimeEnvName(container.Runtime.Java),
					},
				}
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		left := fmt.Sprint(items[i]["app"]) + "\x00" + fmt.Sprint(items[i]["container"])
		right := fmt.Sprint(items[j]["app"]) + "\x00" + fmt.Sprint(items[j]["container"])
		return left < right
	})

	return map[string]any{
		"env":              opts.Environment,
		"apps_count":       len(apps),
		"containers_count": len(items),
		"profiles":         profilesPayload(envDir, opts),
		"items":            items,
	}, nil
}

func Validate(opts Options) error {
	if strings.TrimSpace(opts.Environment) == "" {
		return errors.New("environment is required")
	}
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "environments"
	}
	envDir := filepath.Join(opts.Root, opts.Environment)
	appsDir := filepath.Join(envDir, "apps")
	if !isDir(appsDir) {
		return fmt.Errorf("apps directory not found: %s", appsDir)
	}
	vars, err := loadVars(envDir, opts)
	if err != nil {
		return err
	}
	appFiles, err := listAppFiles(appsDir)
	if err != nil {
		return err
	}
	apps, err := loadApps(appFiles, filepath.Join(appsDir, "_defaults.yml"), vars)
	if err != nil {
		return err
	}
	if err := applyReleaseManifest(apps, opts.ReleaseManifest); err != nil {
		return err
	}
	if err := applyReplicaProfile(apps, envDir, opts); err != nil {
		return err
	}
	applyScaleDown(apps, opts.Down)
	return validateApps(apps)
}

func ListApps(opts Options) ([]string, error) {
	apps, _, _, err := loadPreparedApps(opts, true)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(apps))
	for _, app := range apps {
		if !app.Ignore {
			names = append(names, app.Name)
		}
	}
	return names, nil
}

func ResourceSummary(opts Options) (ResourceSummaryData, error) {
	apps, _, _, err := loadPreparedApps(opts, true)
	if err != nil {
		return ResourceSummaryData{}, err
	}
	summary := ResourceSummaryData{Environment: opts.Environment}
	for _, app := range apps {
		if app.Ignore {
			continue
		}
		summary.Totals.Apps++
		summary.Totals.Replicas += app.Replicas
		replicas := app.Replicas
		for _, container := range app.Containers {
			cpuRequestRaw := resourceRequestValue(container.Resources, "cpu")
			cpuLimitRaw := resourceLimitValue(container.Resources, "cpu")
			memRequestRaw := resourceRequestValue(container.Resources, "memory")
			memLimitRaw := resourceLimitValue(container.Resources, "memory")
			cpuRequest := parseCPU(cpuRequestRaw)
			cpuLimit := parseCPU(cpuLimitRaw)
			memRequest := parseMemoryMiB(memRequestRaw)
			memLimit := parseMemoryMiB(memLimitRaw)
			totalCPURequest := cpuRequest * float64(replicas)
			totalCPULimit := cpuLimit * float64(replicas)
			totalMemRequest := memRequest * float64(replicas)
			totalMemLimit := memLimit * float64(replicas)
			summary.Totals.Containers++
			summary.Totals.CPURequestCores += totalCPURequest
			summary.Totals.CPULimitCores += totalCPULimit
			summary.Totals.MemoryRequestMiB += totalMemRequest
			summary.Totals.MemoryLimitMiB += totalMemLimit
			summary.Items = append(summary.Items, ResourceSummaryItem{
				App:                   app.Name,
				Replicas:              replicas,
				Container:             container.Name,
				CPURequest:            emptyDash(cpuRequestRaw),
				CPULimit:              emptyDash(cpuLimitRaw),
				MemoryRequest:         emptyDash(memRequestRaw),
				MemoryLimit:           emptyDash(memLimitRaw),
				JavaXms:               emptyDash(container.Runtime.Java.Xms),
				JavaXmx:               emptyDash(container.Runtime.Java.Xmx),
				CPURequestCores:       cpuRequest,
				CPULimitCores:         cpuLimit,
				MemoryRequestMiB:      memRequest,
				MemoryLimitMiB:        memLimit,
				TotalCPURequestCores:  totalCPURequest,
				TotalCPULimitCores:    totalCPULimit,
				TotalMemoryRequestMiB: totalMemRequest,
				TotalMemoryLimitMiB:   totalMemLimit,
			})
		}
	}
	return summary, nil
}

func FormatResourceSummaryText(summary ResourceSummaryData) string {
	rows := [][]string{{
		"App",
		"Rep",
		"Container",
		"CPU req",
		"CPU lim",
		"Mem req",
		"Mem lim",
		"JVM Xms",
		"JVM Xmx",
		"CPU req xR",
		"CPU lim xR",
		"Mem req xR",
		"Mem lim xR",
	}}
	for _, item := range summary.Items {
		rows = append(rows, []string{
			item.App,
			strconv.Itoa(item.Replicas),
			item.Container,
			item.CPURequest,
			item.CPULimit,
			item.MemoryRequest,
			item.MemoryLimit,
			item.JavaXms,
			item.JavaXmx,
			formatCPU(item.TotalCPURequestCores),
			formatCPU(item.TotalCPULimitCores),
			formatMiB(item.TotalMemoryRequestMiB),
			formatMiB(item.TotalMemoryLimitMiB),
		})
	}
	table := renderASCIITable(rows, map[int]bool{1: true, 9: true, 10: true, 11: true, 12: true})
	var out strings.Builder
	fmt.Fprintf(&out, "Resource summary for environment: %s\n\n", summary.Environment)
	out.WriteString(table)
	out.WriteString("\n\nTotals\n")
	fmt.Fprintf(&out, "  apps:       %d\n", summary.Totals.Apps)
	fmt.Fprintf(&out, "  containers: %d\n", summary.Totals.Containers)
	fmt.Fprintf(&out, "  replicas:   %d\n", summary.Totals.Replicas)
	fmt.Fprintf(&out, "  cpu req:    %s cores\n", formatCPU(summary.Totals.CPURequestCores))
	fmt.Fprintf(&out, "  cpu lim:    %s cores\n", formatCPU(summary.Totals.CPULimitCores))
	fmt.Fprintf(&out, "  mem req:    %s\n", formatMiB(summary.Totals.MemoryRequestMiB))
	fmt.Fprintf(&out, "  mem lim:    %s\n", formatMiB(summary.Totals.MemoryLimitMiB))
	return out.String()
}

func loadPreparedApps(opts Options, applyProfileAndDown bool) ([]appModel, string, map[string]string, error) {
	if strings.TrimSpace(opts.Environment) == "" {
		return nil, "", nil, errors.New("environment is required")
	}
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "environments"
	}
	envDir := filepath.Join(opts.Root, opts.Environment)
	appsDir := filepath.Join(envDir, "apps")
	if !isDir(appsDir) {
		return nil, "", nil, fmt.Errorf("apps directory not found: %s", appsDir)
	}
	vars, err := loadVars(envDir, opts)
	if err != nil {
		return nil, "", nil, err
	}
	appFiles, err := listAppFiles(appsDir)
	if err != nil {
		return nil, "", nil, err
	}
	apps, err := loadApps(appFiles, filepath.Join(appsDir, "_defaults.yml"), vars)
	if err != nil {
		return nil, "", nil, err
	}
	if err := applyReleaseManifest(apps, opts.ReleaseManifest); err != nil {
		return nil, "", nil, err
	}
	if applyProfileAndDown {
		if err := applyReplicaProfile(apps, envDir, opts); err != nil {
			return nil, "", nil, err
		}
		applyScaleDown(apps, opts.Down)
	}
	if err := validateApps(apps); err != nil {
		return nil, "", nil, err
	}
	return apps, envDir, vars, nil
}

func resourceValue(resources map[string]map[string]any, resource string, key string) string {
	if resources == nil || resources[resource] == nil {
		return ""
	}
	return fmt.Sprint(resources[resource][key])
}

func resourceRequestValue(resources map[string]map[string]any, resource string) string {
	if value := resourceValue(resources, resource, "requests"); value != "" && value != "<nil>" {
		return value
	}
	return resourceValue(resources, resource, "from")
}

func resourceLimitValue(resources map[string]map[string]any, resource string) string {
	if value := resourceValue(resources, resource, "limits"); value != "" && value != "<nil>" {
		return value
	}
	return resourceValue(resources, resource, "to")
}

func emptyDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return "-"
	}
	return value
}

func formatCPU(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func formatMiB(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64) + "Mi"
}

func renderASCIITable(rows [][]string, rightAlign map[int]bool) string {
	if len(rows) == 0 {
		return ""
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	border := func() string {
		var out strings.Builder
		out.WriteByte('+')
		for _, width := range widths {
			out.WriteString(strings.Repeat("-", width+2))
			out.WriteByte('+')
		}
		return out.String()
	}
	line := border()
	var out strings.Builder
	out.WriteString(line)
	out.WriteByte('\n')
	for rowIndex, row := range rows {
		out.WriteByte('|')
		for i, width := range widths {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			padding := width - len(cell)
			if rightAlign[i] {
				out.WriteByte(' ')
				out.WriteString(strings.Repeat(" ", padding))
				out.WriteString(cell)
				out.WriteByte(' ')
			} else {
				out.WriteByte(' ')
				out.WriteString(cell)
				out.WriteString(strings.Repeat(" ", padding))
				out.WriteByte(' ')
			}
			out.WriteByte('|')
		}
		out.WriteByte('\n')
		if rowIndex == 0 {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	out.WriteString(line)
	return out.String()
}

func parseCPU(value string) float64 {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "<nil>" {
		return 0
	}
	multiplier := 1.0
	if strings.HasSuffix(value, "m") {
		multiplier = 0.001
		value = strings.TrimSuffix(value, "m")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed * multiplier
}

func parseMemoryMiB(value string) float64 {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "<nil>" {
		return 0
	}
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "ki"):
		multiplier = 1.0 / 1024.0
		value = strings.TrimSuffix(value, "ki")
	case strings.HasSuffix(value, "mi"):
		multiplier = 1.0
		value = strings.TrimSuffix(value, "mi")
	case strings.HasSuffix(value, "gi"):
		multiplier = 1024.0
		value = strings.TrimSuffix(value, "gi")
	case strings.HasSuffix(value, "k"):
		multiplier = 1000.0 / 1024.0 / 1024.0
		value = strings.TrimSuffix(value, "k")
	case strings.HasSuffix(value, "m"):
		multiplier = 1000.0 * 1000.0 / 1024.0 / 1024.0
		value = strings.TrimSuffix(value, "m")
	case strings.HasSuffix(value, "g"):
		multiplier = 1000.0 * 1000.0 * 1000.0 / 1024.0 / 1024.0
		value = strings.TrimSuffix(value, "g")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed * multiplier
}

func loadVars(envDir string, opts Options) (map[string]string, error) {
	vars := map[string]string{}
	sources, envFile, decryptSecured, err := effectiveVarsSources(opts)
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		switch source {
		case "json":
			if decryptSecured {
				if err := loadEnvJSONFile(vars, filepath.Join(envDir, "env.secured.json"), true); err != nil {
					return nil, err
				}
			}
			if err := loadEnvJSONFile(vars, filepath.Join(envDir, "env.unsecured.json"), false); err != nil {
				return nil, err
			}
		case "env":
			for _, item := range os.Environ() {
				key, value, ok := strings.Cut(item, "=")
				if ok {
					vars[key] = value
				}
			}
		case "dot-env":
			path := envFile
			if path == "" {
				path = filepath.Join(envDir, ".env")
			}
			if !isFile(path) {
				return nil, fmt.Errorf("vars-source 'dot-env' selected but file not found: %s", path)
			}
			if err := loadDotEnvFile(vars, path); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid vars source %q", source)
		}
	}
	return vars, nil
}

func effectiveVarsSources(opts Options) ([]string, string, bool, error) {
	if strings.TrimSpace(opts.EnvFile) != "" {
		if opts.DecryptSecured {
			return nil, "", false, errors.New("-E/--env-file cannot be combined with -d/--decrypt-secured")
		}
		if len(opts.VarsSources) > 0 {
			return nil, "", false, errors.New("-E/--env-file cannot be combined with --vars-source")
		}
		return []string{"dot-env"}, opts.EnvFile, false, nil
	}

	sources := []string{}
	for _, value := range opts.VarsSources {
		for _, part := range strings.Split(value, ",") {
			source := strings.TrimSpace(part)
			if source == "" {
				continue
			}
			switch source {
			case "env", "json", "dot-env":
				sources = append(sources, source)
			default:
				return nil, "", false, fmt.Errorf("invalid --vars-source %q, expected one of: env, json, dot-env", source)
			}
		}
	}
	if len(sources) == 0 {
		sources = []string{"json", "env"}
	}
	return sources, "", opts.DecryptSecured, nil
}

func loadEnvJSONFile(vars map[string]string, path string, secured bool) error {
	if !isFile(path) {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if secured {
		content, err = decryptEnvJSONFile(path)
		if err != nil {
			return err
		}
	}
	var parsed envFile
	if err := json.Unmarshal(content, &parsed); err != nil {
		return err
	}
	if secured {
		var raw map[string]any
		if err := json.Unmarshal(content, &raw); err != nil {
			return err
		}
		if value, ok := raw["_public_key"]; ok {
			vars["_public_key"] = fmt.Sprint(value)
		}
	}
	for key, value := range parsed.Environment {
		vars[key] = fmt.Sprint(value)
	}
	return nil
}

func decryptEnvJSONFile(path string) ([]byte, error) {
	bin := encjsonBinForFile(path)
	args := []string{"decrypt"}
	if keydir := strings.TrimSpace(os.Getenv("ENCJSON_KEYDIR")); keydir != "" {
		args = append(args, "-k", keydir)
	}
	args = append(args, "-f", path)
	cmd := exec.Command(bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("decrypt %s failed: %w: %s", path, err, stderr)
			}
		}
		return nil, fmt.Errorf("decrypt %s failed: %w", path, err)
	}
	return out, nil
}

func encjsonBinForFile(path string) string {
	switch detectEncjsonAPI(path) {
	case "1.0":
		return legacyEncjsonBin()
	case "2.0":
		return rustEncjsonBin()
	default:
		if value := strings.TrimSpace(os.Getenv("ENCJSON_BIN")); value != "" {
			return value
		}
		return legacyEncjsonBin()
	}
}

func legacyEncjsonBin() string {
	for _, key := range []string{"ENCJSON_LEGACY_PATH", "ENCJSON_LEGACY_BIN", "ENCJSON_BIN"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "encjson"
}

func rustEncjsonBin() string {
	for _, key := range []string{"ENCJSON_PATH", "ENCJSON_RS_BIN"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "encjson-rs"
}

func detectEncjsonAPI(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	probe := string(content)
	if len(probe) > 8192 {
		probe = probe[:8192]
	}
	switch {
	case strings.Contains(probe, "EncJson[@api=2.0"):
		return "2.0"
	case strings.Contains(probe, "EncJson[@api=1.0"):
		return "1.0"
	default:
		return ""
	}
}

func loadDotEnvFile(vars map[string]string, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				value = value[1 : len(value)-1]
			}
		}
		vars[key] = value
	}
	return nil
}

func loadVarsLegacy(envDir string) (map[string]string, error) {
	vars := map[string]string{}
	unsecuredPath := filepath.Join(envDir, "env.unsecured.json")
	if isFile(unsecuredPath) {
		content, err := os.ReadFile(unsecuredPath)
		if err != nil {
			return nil, err
		}
		var parsed envFile
		if err := json.Unmarshal(content, &parsed); err != nil {
			return nil, err
		}
		for key, value := range parsed.Environment {
			vars[key] = fmt.Sprint(value)
		}
	}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			vars[key] = value
		}
	}
	return vars, nil
}

func loadApps(appFiles []string, defaultsPath string, vars map[string]string) ([]appModel, error) {
	apps := make([]appModel, 0, len(appFiles))
	for _, appFile := range appFiles {
		app, err := loadApp(appFile, defaultsPath, vars)
		if err != nil {
			return nil, err
		}
		if app.Kind == "" {
			app.Kind = "Deployment"
		}
		apps = append(apps, app)
	}
	return apps, nil
}

func validateApps(apps []appModel) error {
	for _, app := range apps {
		if app.Autoscaling.Enabled {
			if app.Autoscaling.MinReplicas < 1 {
				return fmt.Errorf("%s: autoscaling.min_replicas must be greater than 0", app.Name)
			}
			if app.Autoscaling.MaxReplicas < app.Autoscaling.MinReplicas {
				return fmt.Errorf("%s: autoscaling.max_replicas must be greater than or equal to min_replicas", app.Name)
			}
			if app.Autoscaling.CPU.AverageUtilization == 0 && app.Autoscaling.Memory.AverageUtilization == 0 && !autoscalingHasRawMetrics(app.Autoscaling) {
				return fmt.Errorf("%s: autoscaling requires at least one metric: cpu.average_utilization, memory.average_utilization or raw.spec.metrics", app.Name)
			}
			if !validAutoscalingUtilization(app.Autoscaling.CPU.AverageUtilization) {
				return fmt.Errorf("%s: autoscaling.cpu.average_utilization must be between 1 and 100", app.Name)
			}
			if !validAutoscalingUtilization(app.Autoscaling.Memory.AverageUtilization) {
				return fmt.Errorf("%s: autoscaling.memory.average_utilization must be between 1 and 100", app.Name)
			}
		}
		for _, container := range app.Containers {
			if javaRuntimeEnabled(container.Runtime.Java) {
				envName := javaRuntimeEnvName(container.Runtime.Java)
				if containerHasEnvVar(container.Vars, envName) {
					return fmt.Errorf("%s: container %q defines runtime.java export env %q and vars with the same name", app.Name, container.Name, envName)
				}
			}
			if !container.SimpleInit.Enabled {
				continue
			}
			if len(container.Startup.Command) > 0 || len(container.Startup.Arguments) > 0 {
				return fmt.Errorf("%s: container %q has both startup and simple_init.enabled=true (XOR violation)", app.Name, container.Name)
			}
			if len(container.SimpleInit.Exec.Command) == 0 {
				return fmt.Errorf("%s: container %q requires simple_init.exec.command as non-empty array", app.Name, container.Name)
			}
		}
	}
	return nil
}

func validAutoscalingUtilization(value int) bool {
	return value == 0 || (value >= 1 && value <= 100)
}

func autoscalingHasRawMetrics(autoscaling autoscalingSpec) bool {
	spec, ok := autoscaling.Raw["spec"].(map[string]any)
	if !ok {
		return false
	}
	metrics, ok := spec["metrics"].([]any)
	return ok && len(metrics) > 0
}

func containerHasEnvVar(items []envVar, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func applyReplicaProfile(apps []appModel, envDir string, opts Options) error {
	path := strings.TrimSpace(opts.ProfilesFile)
	if path == "" {
		path = filepath.Join(envDir, "replica-profiles.yml")
	}
	if !isFile(path) {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var parsed replicaProfilesFile
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	profileName := strings.TrimSpace(opts.Profile)
	if profileName == "" {
		profileName = strings.TrimSpace(os.Getenv("REPLICA_PROFILE"))
	}
	if profileName == "" {
		profileName = strings.TrimSpace(parsed.Defaults.Profile)
	}
	if profileName == "" {
		return nil
	}

	profile, ok := parsed.Profiles[profileName]
	if !ok {
		return nil
	}
	if profile.All != nil {
		for i := range apps {
			apps[i].Replicas = *profile.All
		}
	}

	overrides := profile.Apps
	if overrides == nil {
		overrides = map[string]int{}
		for name, replicas := range profile.LegacyApps {
			overrides[name] = replicas
		}
	}
	for appName, replicas := range overrides {
		for i := range apps {
			if apps[i].Name == appName {
				apps[i].Replicas = replicas
				break
			}
		}
	}
	return nil
}

func applyScaleDown(apps []appModel, down []string) {
	if len(down) == 0 {
		return
	}
	names := map[string]bool{}
	for _, item := range down {
		for _, part := range strings.Split(item, ",") {
			name := strings.TrimSpace(part)
			if name != "" {
				names[name] = true
			}
		}
	}
	for i := range apps {
		if names[apps[i].Name] {
			apps[i].Replicas = 0
		}
	}
}

func applyReleaseManifest(apps []appModel, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	manifest, err := loadReleaseManifest(path)
	if err != nil {
		return err
	}
	for i := range apps {
		for j := range apps[i].Containers {
			if image := manifest.imageFor(apps[i].Name, apps[i].Containers[j].Name); image != "" {
				apps[i].Containers[j].Image = image
			}
		}
	}
	return nil
}

type releaseManifest struct {
	Images []releaseImage `yaml:"images"`
}

type releaseImage struct {
	AppName       string `yaml:"app_name"`
	ContainerName string `yaml:"container_name"`
	Image         string `yaml:"image"`
	Tag           string `yaml:"tag"`
	Digest        string `yaml:"digest"`
}

func loadReleaseManifest(path string) (releaseManifest, error) {
	if !isFile(path) {
		return releaseManifest{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return releaseManifest{}, err
	}
	var manifest releaseManifest
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return releaseManifest{}, fmt.Errorf("%s: %w", path, err)
	}
	return manifest, nil
}

func (m releaseManifest) imageFor(appName string, containerName string) string {
	var match *releaseImage
	for i := range m.Images {
		item := &m.Images[i]
		if item.AppName == appName && item.ContainerName == containerName {
			match = item
			break
		}
	}
	if match == nil {
		for i := range m.Images {
			item := &m.Images[i]
			if item.AppName == appName && strings.TrimSpace(item.ContainerName) == "" {
				match = item
				break
			}
		}
	}
	if match == nil || strings.TrimSpace(match.Image) == "" {
		return ""
	}
	if digest := strings.TrimSpace(match.Digest); digest != "" {
		return match.Image + "@" + digest
	}
	if tag := strings.TrimSpace(match.Tag); tag != "" {
		return match.Image + ":" + tag
	}
	return match.Image
}

type replicaProfilesFile struct {
	Defaults replicaProfileDefaults        `yaml:"defaults"`
	Profiles map[string]replicaProfileSpec `yaml:"profiles"`
}

type replicaProfileDefaults struct {
	Profile string `yaml:"profile"`
}

type replicaProfileSpec struct {
	All        *int
	Apps       map[string]int
	LegacyApps map[string]int
}

func (p *replicaProfileSpec) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return nil
	}
	p.LegacyApps = map[string]int{}
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		node := value.Content[i+1]
		switch key {
		case "all":
			replicas, ok := yamlNodeToInt(node)
			if ok {
				p.All = &replicas
			}
		case "apps":
			apps := map[string]int{}
			if node.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(node.Content); j += 2 {
					replicas, ok := yamlNodeToInt(node.Content[j+1])
					if ok {
						apps[node.Content[j].Value] = replicas
					}
				}
			}
			p.Apps = apps
		default:
			replicas, ok := yamlNodeToInt(node)
			if ok {
				p.LegacyApps[key] = replicas
			}
		}
	}
	return nil
}

func yamlNodeToInt(node *yaml.Node) (int, bool) {
	var value int
	if err := node.Decode(&value); err == nil {
		return value, true
	}
	var text string
	if err := node.Decode(&text); err != nil {
		return 0, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func listAppFiles(appsDir string) ([]string, error) {
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		files = append(files, filepath.Join(appsDir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func loadApp(path string, defaultsPath string, vars map[string]string) (appModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return appModel{}, err
	}
	defaultsRaw, err := readOptionalFile(defaultsPath)
	if err != nil {
		return appModel{}, err
	}

	content := applyEnvPlaceholders(string(raw), vars)
	defaultsContent := applyEnvPlaceholders(defaultsRaw, vars)
	defaultVars, err := extractVariableList(defaultsContent)
	if err != nil {
		return appModel{}, fmt.Errorf("%s: %w", defaultsPath, err)
	}
	appVarList, err := extractVariableList(content)
	if err != nil {
		return appModel{}, fmt.Errorf("%s: %w", path, err)
	}
	mergedVarMap := variablesToMap(mergeVariables(defaultVars, appVarList))
	defaultsContent = applyAppVarPlaceholders(defaultsContent, mergedVarMap)
	content = applyAppVarPlaceholders(content, mergedVarMap)

	content, err = mergeDefaults(defaultsContent, content)
	if err != nil {
		return appModel{}, fmt.Errorf("%s: %w", path, err)
	}
	appVars, err := extractVars(content)
	if err != nil {
		return appModel{}, fmt.Errorf("%s: %w", path, err)
	}
	content = applyAppVarPlaceholders(content, appVars)

	var app appModel
	if err := yaml.Unmarshal([]byte(content), &app); err != nil {
		return appModel{}, fmt.Errorf("%s: %w", path, err)
	}
	if app.Name == "" {
		return appModel{}, fmt.Errorf("%s: missing app name", path)
	}
	return app, nil
}

func extractVars(content string) (map[string]string, error) {
	vars, err := extractVariableList(content)
	if err != nil {
		return nil, err
	}
	return variablesToMap(vars), nil
}

func extractVariableList(content string) ([]variable, error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	type varsOnly struct {
		Vars []variable `yaml:"vars"`
	}
	var parsed varsOnly
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, err
	}
	return parsed.Vars, nil
}

func variablesToMap(items []variable) map[string]string {
	vars := map[string]string{}
	for _, item := range items {
		if item.Name != "" && !item.Remove {
			vars[item.Name] = item.Value
		}
	}
	return vars
}

func readOptionalFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" || !isFile(path) {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func mergeDefaults(defaultsContent, appContent string) (string, error) {
	if strings.TrimSpace(defaultsContent) == "" {
		return appContent, nil
	}

	defaultsNode, err := parseYAMLMapping(defaultsContent)
	if err != nil {
		return "", fmt.Errorf("_defaults.yml: %w", err)
	}
	appNode, err := parseYAMLMapping(appContent)
	if err != nil {
		return "", err
	}

	containerVarDefaults, err := parseContainerVarDefaults(mappingValue(defaultsNode, "container_vars"))
	if err != nil {
		return "", err
	}

	merged := mergeMappingNodes(defaultsNode, appNode)
	mergedVars, err := mergeVarsFromNodes(defaultsNode, appNode)
	if err != nil {
		return "", err
	}
	setMappingValue(merged, "vars", mergedVars)
	removeMappingValue(merged, "container_vars")
	if err := applyContainerVarDefaults(merged, containerVarDefaults); err != nil {
		return "", err
	}

	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseContainerVarDefaults(node *yaml.Node) ([]containerVarDefault, error) {
	if node == nil {
		return nil, nil
	}
	var items []containerVarDefault
	if err := node.Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func applyContainerVarDefaults(root *yaml.Node, defaults []containerVarDefault) error {
	if len(defaults) == 0 {
		return nil
	}
	containersNode := mappingValue(root, "containers")
	if containersNode == nil || containersNode.Kind != yaml.SequenceNode {
		return nil
	}

	for _, containerNode := range containersNode.Content {
		if containerNode.Kind != yaml.MappingNode {
			continue
		}
		containerName := scalarMappingValue(containerNode, "name")
		effective := []envVar{}
		for _, item := range defaults {
			if item.Name == "*" {
				effective = mergeVars(effective, item.Vars)
			}
		}
		for _, item := range defaults {
			if item.Name != "*" && item.Name == containerName {
				effective = mergeVars(effective, item.Vars)
			}
		}

		local, err := parseVarsNode(mappingValue(containerNode, "vars"), true)
		if err != nil {
			return err
		}
		effective = mergeVars(effective, local)
		if len(effective) == 0 {
			removeMappingValue(containerNode, "vars")
		} else {
			setMappingValue(containerNode, "vars", varsToNode(effective))
		}
	}
	return nil
}

func parseVarsNode(node *yaml.Node, allowRemove bool) ([]envVar, error) {
	if node == nil {
		return nil, nil
	}
	var items []envVar
	if err := node.Decode(&items); err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Name == "" {
			return nil, errors.New("vars item is missing name")
		}
		if item.Remove && !allowRemove {
			return nil, errors.New("remove: true is not allowed here")
		}
	}
	return items, nil
}

func mergeVars(defaults []envVar, overrides []envVar) []envVar {
	result := make([]envVar, 0, len(defaults)+len(overrides))
	positions := map[string]int{}
	for _, item := range defaults {
		if item.Name == "" {
			continue
		}
		positions[item.Name] = len(result)
		result = append(result, item)
	}
	for _, item := range overrides {
		if item.Name == "" {
			continue
		}
		if item.Remove {
			index, ok := positions[item.Name]
			if !ok {
				continue
			}
			result = append(result[:index], result[index+1:]...)
			positions = rebuildEnvVarPositions(result)
			continue
		}
		if index, ok := positions[item.Name]; ok {
			result[index] = item
		} else {
			positions[item.Name] = len(result)
			result = append(result, item)
		}
	}
	return result
}

func rebuildEnvVarPositions(items []envVar) map[string]int {
	positions := map[string]int{}
	for index, item := range items {
		positions[item.Name] = index
	}
	return positions
}

func varsToNode(items []envVar) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode}
	for _, item := range items {
		itemNode := &yaml.Node{Kind: yaml.MappingNode}
		itemNode.Content = append(itemNode.Content, scalarNode("name"), scalarNode(item.Name))
		switch {
		case item.SecretName != "":
			itemNode.Content = append(itemNode.Content, scalarNode("secret_name"), scalarNode(item.SecretName))
			itemNode.Content = append(itemNode.Content, scalarNode("key"), scalarNode(item.Key))
		case item.ResourceName != "":
			itemNode.Content = append(itemNode.Content, scalarNode("resource_name"), scalarNode(item.ResourceName))
			itemNode.Content = append(itemNode.Content, scalarNode("divisor"), scalarNode(item.Divisor))
		case item.FieldPath != "":
			itemNode.Content = append(itemNode.Content, scalarNode("field_path"), scalarNode(item.FieldPath))
		default:
			itemNode.Content = append(itemNode.Content, scalarNode("value"), scalarNode(item.Value))
		}
		node.Content = append(node.Content, itemNode)
	}
	return node
}

func parseYAMLMapping(content string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode}, nil
	}
	node := doc.Content[0]
	if node.Kind != yaml.MappingNode {
		return nil, errors.New("YAML root must be a mapping")
	}
	return node, nil
}

func mergeMappingNodes(defaultsNode, appNode *yaml.Node) *yaml.Node {
	result := cloneNode(defaultsNode)
	for i := 0; i+1 < len(appNode.Content); i += 2 {
		key := appNode.Content[i]
		appValue := appNode.Content[i+1]
		existingIndex := mappingKeyIndex(result, key.Value)
		if existingIndex < 0 {
			result.Content = append(result.Content, cloneNode(key), cloneNode(appValue))
			continue
		}
		defaultValue := result.Content[existingIndex+1]
		if defaultValue.Kind == yaml.MappingNode && appValue.Kind == yaml.MappingNode {
			result.Content[existingIndex+1] = mergeMappingNodes(defaultValue, appValue)
		} else {
			result.Content[existingIndex+1] = cloneNode(appValue)
		}
	}
	return result
}

func mergeVarsFromNodes(defaultsNode, appNode *yaml.Node) (*yaml.Node, error) {
	defaultVars, err := parseVariableNode(mappingValue(defaultsNode, "vars"), false)
	if err != nil {
		return nil, err
	}
	appVars, err := parseVariableNode(mappingValue(appNode, "vars"), true)
	if err != nil {
		return nil, err
	}
	merged := mergeVariables(defaultVars, appVars)
	return variablesToNode(merged), nil
}

func parseVariableNode(node *yaml.Node, allowRemove bool) ([]variable, error) {
	if node == nil {
		return nil, nil
	}
	var vars []variable
	if err := node.Decode(&vars); err != nil {
		return nil, err
	}
	for _, item := range vars {
		if item.Name == "" {
			return nil, errors.New("vars item is missing name")
		}
		if item.Remove && !allowRemove {
			return nil, errors.New("remove: true is not allowed in defaults vars")
		}
	}
	return vars, nil
}

func mergeVariables(defaults []variable, overrides []variable) []variable {
	result := make([]variable, 0, len(defaults)+len(overrides))
	positions := map[string]int{}
	for _, item := range defaults {
		positions[item.Name] = len(result)
		result = append(result, item)
	}
	for _, item := range overrides {
		if item.Remove {
			index, ok := positions[item.Name]
			if !ok {
				continue
			}
			result = append(result[:index], result[index+1:]...)
			positions = rebuildVariablePositions(result)
			continue
		}
		if index, ok := positions[item.Name]; ok {
			result[index] = item
		} else {
			positions[item.Name] = len(result)
			result = append(result, item)
		}
	}
	return result
}

func rebuildVariablePositions(vars []variable) map[string]int {
	positions := map[string]int{}
	for index, item := range vars {
		positions[item.Name] = index
	}
	return positions
}

func variablesToNode(vars []variable) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode}
	for _, item := range vars {
		itemNode := &yaml.Node{Kind: yaml.MappingNode}
		itemNode.Content = append(itemNode.Content, scalarNode("name"), scalarNode(item.Name))
		itemNode.Content = append(itemNode.Content, scalarNode("value"), scalarNode(item.Value))
		node.Content = append(node.Content, itemNode)
	}
	return node
}

func mappingKeyIndex(node *yaml.Node, key string) int {
	if node == nil || node.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	index := mappingKeyIndex(node, key)
	if index < 0 {
		return nil
	}
	return node.Content[index+1]
}

func scalarMappingValue(node *yaml.Node, key string) string {
	value := mappingValue(node, key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

func setMappingValue(node *yaml.Node, key string, value *yaml.Node) {
	index := mappingKeyIndex(node, key)
	if index < 0 {
		node.Content = append(node.Content, scalarNode(key), value)
		return
	}
	node.Content[index+1] = value
}

func removeMappingValue(node *yaml.Node, key string) {
	index := mappingKeyIndex(node, key)
	if index < 0 {
		return
	}
	node.Content = append(node.Content[:index], node.Content[index+2:]...)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func cloneNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	out := *node
	if len(node.Content) > 0 {
		out.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			out.Content[i] = cloneNode(child)
		}
	}
	return &out
}

func renderDeployment(app appModel, namespace string, assets map[string][]resolvedAsset, sharedAssets []resolvedAsset, rolloutAnnotations map[string]string) (map[string]any, error) {
	labels := map[string]any{appLabel: app.Name}
	for key, value := range app.Labels {
		labels[key] = value
	}

	containers := make([]map[string]any, 0, len(app.Containers))
	for _, container := range app.Containers {
		containers = append(containers, renderContainer(container, assets[container.Name], sharedAssets, len(app.Tools) > 0))
	}
	initContainers := renderInitContainers(app, assets, len(app.Tools) > 0)

	podSpec := map[string]any{
		"containers":       containers,
		"imagePullSecrets": renderImagePullSecrets(app.Registry),
		"volumes":          renderVolumes(app, assets, sharedAssets),
	}
	if len(app.SecurityContext) > 0 {
		podSpec["securityContext"] = cloneMap(app.SecurityContext)
	}
	if app.TerminationGrace != nil {
		podSpec["terminationGracePeriodSeconds"] = *app.TerminationGrace
	}
	if strings.TrimSpace(app.ServiceAccount) != "" {
		podSpec["serviceAccountName"] = strings.TrimSpace(app.ServiceAccount)
	}
	if len(app.DNS) > 0 {
		podSpec["hostAliases"] = renderHostAliases(app.DNS)
	}
	applyScheduling(podSpec, app)
	if len(initContainers) > 0 {
		podSpec["initContainers"] = initContainers
	}
	if len(app.Tools) > 0 {
		podSpec["volumes"] = append(podSpec["volumes"].([]any), map[string]any{
			"name":     "app-tools",
			"emptyDir": map[string]any{},
		})
	}
	for key, value := range app.PodRaw {
		podSpec[key] = value
	}

	metadata := map[string]any{
		"labels":    labels,
		"name":      app.Name,
		"namespace": namespace,
	}
	if len(app.Annotations) > 0 {
		metadata["annotations"] = app.Annotations
	}
	templateMetadata := map[string]any{
		"labels": map[string]any{appLabel: app.Name},
	}
	podAnnotations := map[string]any{}
	for key, value := range app.PodAnnotations {
		podAnnotations[key] = value
	}
	for key, value := range rolloutAnnotations {
		if existing, ok := podAnnotations[key]; ok && fmt.Sprint(existing) != value {
			return nil, fmt.Errorf("pod annotation %q conflicts with computed rollout checksum", key)
		}
		podAnnotations[key] = value
	}
	if len(podAnnotations) > 0 {
		templateMetadata["annotations"] = podAnnotations
	}

	spec := map[string]any{
		"replicas": app.Replicas,
		"selector": map[string]any{
			"matchLabels": map[string]any{appLabel: app.Name},
		},
		"strategy": renderStrategy(app.Strategy),
		"template": map[string]any{
			"metadata": templateMetadata,
			"spec":     podSpec,
		},
	}
	if app.Kind == "StatefulSet" {
		delete(spec, "strategy")
		spec["updateStrategy"] = map[string]any{"type": "RollingUpdate"}
		serviceName := app.SubdomainName
		if serviceName == "" {
			serviceName = app.Name
		}
		spec["serviceName"] = serviceName
	}

	deployment := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       app.Kind,
		"metadata":   metadata,
		"spec":       spec,
	}
	mergeRawMap(deployment, app.DeploymentRaw)
	return deployment, nil
}

func applyScheduling(podSpec map[string]any, app appModel) {
	nodeSelector := map[string]any{}
	if app.Arch != "" {
		nodeSelector["kubernetes.io/arch"] = app.Arch
	}
	for key, value := range app.NodeSelector {
		nodeSelector[key] = value
	}
	if app.Scheduling.Arch != "" {
		nodeSelector["kubernetes.io/arch"] = app.Scheduling.Arch
	}
	for key, value := range app.Scheduling.NodeSelector {
		nodeSelector[key] = value
	}
	if len(nodeSelector) > 0 {
		podSpec["nodeSelector"] = nodeSelector
	}

	if len(app.Tolerations) > 0 {
		podSpec["tolerations"] = app.Tolerations
	}
	if len(app.Scheduling.Tolerations) > 0 {
		podSpec["tolerations"] = app.Scheduling.Tolerations
	}

	affinity := cloneMap(app.Scheduling.Affinity)
	if selfAntiAffinity := renderSelfAntiAffinity(app); selfAntiAffinity != nil {
		podAntiAffinity, _ := affinity["podAntiAffinity"].(map[string]any)
		if podAntiAffinity == nil {
			podAntiAffinity = map[string]any{}
			affinity["podAntiAffinity"] = podAntiAffinity
		}
		for key, value := range selfAntiAffinity {
			podAntiAffinity[key] = value
		}
	}
	if len(affinity) > 0 {
		podSpec["affinity"] = affinity
	}

	if spread := renderTopologySpread(app); spread != nil {
		podSpec["topologySpreadConstraints"] = []any{spread}
	}
}

func renderSelfAntiAffinity(app appModel) map[string]any {
	mode := strings.TrimSpace(app.Scheduling.AntiAffinity.Self)
	if mode == "" {
		return nil
	}
	topology := strings.TrimSpace(app.Scheduling.AntiAffinity.Topology)
	if topology == "" {
		topology = "kubernetes.io/hostname"
	}
	term := map[string]any{
		"topologyKey": topology,
		"labelSelector": map[string]any{
			"matchLabels": map[string]any{appLabel: app.Name},
		},
	}
	switch mode {
	case "required":
		return map[string]any{"requiredDuringSchedulingIgnoredDuringExecution": []any{term}}
	default:
		return map[string]any{"preferredDuringSchedulingIgnoredDuringExecution": []any{
			map[string]any{"weight": 100, "podAffinityTerm": term},
		}}
	}
}

func renderTopologySpread(app appModel) map[string]any {
	spread := app.Scheduling.Spread
	if spread.By == "" && spread.Topology == "" {
		return nil
	}
	topology := strings.TrimSpace(spread.Topology)
	if topology == "" {
		switch strings.TrimSpace(spread.By) {
		case "zone":
			topology = "topology.kubernetes.io/zone"
		default:
			topology = "kubernetes.io/hostname"
		}
	}
	maxSkew := spread.MaxSkew
	if maxSkew == 0 {
		maxSkew = 1
	}
	whenUnsatisfiable := strings.TrimSpace(spread.WhenUnsatisfiable)
	if whenUnsatisfiable == "" {
		whenUnsatisfiable = "ScheduleAnyway"
	}
	return map[string]any{
		"maxSkew":           maxSkew,
		"topologyKey":       topology,
		"whenUnsatisfiable": whenUnsatisfiable,
		"labelSelector": map[string]any{
			"matchLabels": map[string]any{appLabel: app.Name},
		},
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func mergeRawMap(target map[string]any, raw map[string]any) {
	for key, value := range raw {
		if nestedTarget, ok := target[key].(map[string]any); ok {
			if nestedRaw, ok := value.(map[string]any); ok {
				mergeRawMap(nestedTarget, nestedRaw)
				continue
			}
		}
		target[key] = value
	}
}

func rolloutChecksumAnnotations(app appModel, envDir string) (map[string]string, error) {
	if len(app.RolloutOn.Checksums) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for group, config := range app.RolloutOn.Checksums {
		group = strings.TrimSpace(group)
		if group == "" {
			return nil, errors.New("invalid empty rollout checksum group")
		}
		if len(config.Files) == 0 {
			return nil, fmt.Errorf("rollout_on.checksums.%s.files must not be empty", group)
		}
		files := append([]string{}, config.Files...)
		for i, file := range files {
			normalized, err := normalizeRolloutPath(file)
			if err != nil {
				return nil, err
			}
			files[i] = normalized
		}
		sort.Strings(files)
		hash := sha256.New()
		for _, relative := range files {
			fullPath := filepath.Join(envDir, relative)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, fmt.Errorf("rollout checksum file not found for %q: %s", group, relative)
			}
			hash.Write([]byte(relative))
			hash.Write([]byte{0})
			hash.Write(content)
			hash.Write([]byte{0})
		}
		out["checksum/"+group] = hex.EncodeToString(hash.Sum(nil))
	}
	return out, nil
}

func normalizeRolloutPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "..") || strings.Contains(value, "//") {
		return "", fmt.Errorf("invalid rollout checksum path: %s", value)
	}
	return filepath.ToSlash(value), nil
}

func renderStrategy(value string) map[string]any {
	switch value {
	case "recreate":
		return map[string]any{"type": "Recreate"}
	case "one-by-one":
		return map[string]any{"rollingUpdate": map[string]any{"maxSurge": 0, "maxUnavailable": 1}, "type": "RollingUpdate"}
	default:
		return map[string]any{"rollingUpdate": map[string]any{"maxSurge": "25%", "maxUnavailable": "25%"}, "type": "RollingUpdate"}
	}
}

func renderBudget(app appModel, namespace string) map[string]any {
	if app.MinAvailable == nil && app.MaxUnavailable == nil {
		return nil
	}
	labels := map[string]any{appLabel: app.Name}
	for key, value := range app.Labels {
		labels[key] = value
	}
	spec := map[string]any{"selector": map[string]any{"matchLabels": map[string]any{appLabel: app.Name}}}
	if app.MinAvailable != nil {
		spec["minAvailable"] = app.MinAvailable
	} else {
		spec["maxUnavailable"] = app.MaxUnavailable
	}
	return map[string]any{"apiVersion": "policy/v1", "kind": "PodDisruptionBudget", "metadata": map[string]any{"labels": labels, "name": app.Name, "namespace": namespace}, "spec": spec}
}

func renderAutoscaling(app appModel, namespace string) map[string]any {
	if !app.Autoscaling.Enabled {
		return nil
	}
	labels := map[string]any{appLabel: app.Name}
	for key, value := range app.Labels {
		labels[key] = value
	}
	metadata := map[string]any{
		"labels":    labels,
		"name":      app.Name,
		"namespace": namespace,
	}
	if len(app.Annotations) > 0 {
		metadata["annotations"] = cloneMap(app.Annotations)
	}
	hpa := map[string]any{
		"apiVersion": "autoscaling/v2",
		"kind":       "HorizontalPodAutoscaler",
		"metadata":   metadata,
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{
				"apiVersion": "apps/v1",
				"kind":       app.Kind,
				"name":       app.Name,
			},
			"minReplicas": app.Autoscaling.MinReplicas,
			"maxReplicas": app.Autoscaling.MaxReplicas,
			"metrics":     renderAutoscalingMetrics(app.Autoscaling),
		},
	}
	mergeRawMap(hpa, app.Autoscaling.Raw)
	return hpa
}

func renderAutoscalingMetrics(autoscaling autoscalingSpec) []any {
	metrics := []any{}
	if autoscaling.CPU.AverageUtilization > 0 {
		metrics = append(metrics, renderAutoscalingResourceMetric("cpu", autoscaling.CPU.AverageUtilization))
	}
	if autoscaling.Memory.AverageUtilization > 0 {
		metrics = append(metrics, renderAutoscalingResourceMetric("memory", autoscaling.Memory.AverageUtilization))
	}
	return metrics
}

func renderAutoscalingResourceMetric(name string, averageUtilization int) map[string]any {
	return map[string]any{
		"type": "Resource",
		"resource": map[string]any{
			"name": name,
			"target": map[string]any{
				"type":               "Utilization",
				"averageUtilization": averageUtilization,
			},
		},
	}
}

func renderImagePullSecrets(registry []registrySpec) []any {
	out := make([]any, 0, len(registry))
	for _, item := range registry {
		if item.SecretName == "" {
			continue
		}
		out = append(out, map[string]any{"name": item.SecretName})
	}
	return out
}

func renderHostAliases(dns []hostAliasSpec) []any {
	out := make([]any, 0, len(dns))
	for _, item := range dns {
		if item.IP == "" {
			continue
		}
		out = append(out, map[string]any{
			"ip":        item.IP,
			"hostnames": item.Hostnames,
		})
	}
	return out
}

func renderContainer(container containerSpec, assets []resolvedAsset, sharedAssets []resolvedAsset, mountTools bool) map[string]any {
	mounts := renderVolumeMounts(append(append([]resolvedAsset{}, assets...), sharedAssets...))
	if mountTools {
		mounts = append(mounts, map[string]any{
			"name":      "app-tools",
			"mountPath": "/app/tools",
			"readOnly":  true,
		})
	}
	out := map[string]any{
		"name":            container.Name,
		"image":           container.Image,
		"imagePullPolicy": "Always",
		"resources":       renderResources(container.Resources),
		"volumeMounts":    mounts,
	}
	if len(container.Startup.Command) > 0 {
		out["command"] = container.Startup.Command
	}
	if len(container.Startup.Arguments) > 0 {
		out["args"] = container.Startup.Arguments
	}
	if len(container.SecurityContext) > 0 {
		out["securityContext"] = cloneMap(container.SecurityContext)
	}
	vars := effectiveContainerVars(container)
	if len(vars) > 0 {
		out["env"] = renderVars(vars)
	}
	if envFrom := renderEnvFrom(container.EnvFrom); len(envFrom) > 0 {
		out["envFrom"] = envFrom
	}
	if len(container.Ports) > 0 {
		out["ports"] = renderContainerPorts(container.Ports)
	}
	if lifecycle := renderLifecycle(container.Lifecycle); lifecycle != nil {
		out["lifecycle"] = lifecycle
	}
	if probe := renderHealth(container.Health, "live"); probe != nil {
		out["livenessProbe"] = probe
	}
	if probe := renderHealth(container.Health, "ready"); probe != nil {
		out["readinessProbe"] = probe
	}
	if probe := renderHealth(container.Probe.Live, "live"); probe != nil {
		out["livenessProbe"] = probe
	}
	if probe := renderHealth(container.Probe.Ready, "live"); probe != nil {
		out["readinessProbe"] = probe
	}
	if probe := renderHealth(container.Probe.Start, "live"); probe != nil {
		out["startupProbe"] = probe
	}
	for key, probe := range renderProbes(container.Probes) {
		out[key] = probe
	}
	for key, value := range container.Raw {
		out[key] = value
	}
	return out
}

func renderInitContainers(app appModel, assets map[string][]resolvedAsset, mountTools bool) []map[string]any {
	out := renderToolInitContainers(app.Tools)
	for _, container := range app.InitContainers {
		out = append(out, renderInitContainer(container, assets[container.Name], mountTools))
	}
	return out
}

func renderInitContainer(container initContainerSpec, assets []resolvedAsset, mountTools bool) map[string]any {
	mounts := renderVolumeMounts(assets)
	if mountTools {
		mounts = append(mounts, map[string]any{
			"name":      "app-tools",
			"mountPath": "/app/tools",
			"readOnly":  true,
		})
	}
	out := map[string]any{
		"name":            container.Name,
		"image":           container.Image,
		"imagePullPolicy": "Always",
	}
	if len(container.Resources) > 0 {
		out["resources"] = renderResources(container.Resources)
	}
	if len(mounts) > 0 {
		out["volumeMounts"] = mounts
	}
	if len(container.Command) > 0 {
		out["command"] = container.Command
	}
	if len(container.Arguments) > 0 {
		out["args"] = container.Arguments
	}
	if len(container.SecurityContext) > 0 {
		out["securityContext"] = cloneMap(container.SecurityContext)
	}
	if len(container.Vars) > 0 {
		out["env"] = renderVars(container.Vars)
	}
	if envFrom := renderEnvFrom(container.EnvFrom); len(envFrom) > 0 {
		out["envFrom"] = envFrom
	}
	for key, value := range container.Raw {
		out[key] = value
	}
	return out
}

func renderEnvFrom(items []envFromSpec) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		if len(item.Raw) > 0 {
			out = append(out, cloneMap(item.Raw))
			continue
		}
		entry := map[string]any{}
		if len(item.ConfigMapRef) > 0 {
			entry["configMapRef"] = cloneMap(item.ConfigMapRef)
		} else if strings.TrimSpace(item.ConfigMap) != "" {
			entry["configMapRef"] = map[string]any{"name": strings.TrimSpace(item.ConfigMap)}
		}
		if len(item.SecretRef) > 0 {
			entry["secretRef"] = cloneMap(item.SecretRef)
		} else if strings.TrimSpace(item.Secret) != "" {
			entry["secretRef"] = map[string]any{"name": strings.TrimSpace(item.Secret)}
		}
		if strings.TrimSpace(item.Prefix) != "" {
			entry["prefix"] = strings.TrimSpace(item.Prefix)
		}
		if len(entry) > 0 {
			out = append(out, entry)
		}
	}
	return out
}

func renderLifecycle(lifecycle lifecycleSpec) map[string]any {
	out := map[string]any{}
	if preStop := renderLifecycleHandler(lifecycle.PreStop); preStop != nil {
		out["preStop"] = preStop
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func renderLifecycleHandler(handler lifecycleHandlerSpec) map[string]any {
	if len(handler.Raw) > 0 {
		return cloneMap(handler.Raw)
	}
	if len(handler.Command) > 0 {
		return map[string]any{"exec": map[string]any{"command": handler.Command}}
	}
	if hasHTTPHealth(handler.HTTP) {
		return map[string]any{"httpGet": map[string]any{
			"path": handler.HTTP.Path.For("live"),
			"port": int(handler.HTTP.Port),
		}}
	}
	return nil
}

func renderProbes(probes probesSpec) map[string]any {
	out := map[string]any{}
	if isEmptyProbes(probes) {
		return out
	}

	for _, item := range []struct {
		key      string
		kind     string
		override healthSpec
	}{
		{key: "livenessProbe", kind: "live", override: probes.Live},
		{key: "readinessProbe", kind: "ready", override: probes.Ready},
		{key: "startupProbe", kind: "start", override: probes.Start},
	} {
		if probe := renderHealth(resolveProbeHealth(probes, item.kind, item.override), item.kind); probe != nil {
			out[item.key] = probe
		}
	}
	return out
}

func resolveProbeHealth(probes probesSpec, kind string, override healthSpec) healthSpec {
	health := defaultProbeHealth(probes, kind)
	baseHTTP := commonProbeHTTP(probes, kind)
	if hasHTTPHealth(baseHTTP) {
		health.HTTP = baseHTTP
	}

	if len(override.Command) > 0 {
		health.Command = override.Command
		health.HTTP = httpHealthSpec{}
	}
	if hasHTTPHealth(override.HTTP) {
		if override.HTTP.Port != 0 {
			health.HTTP.Port = override.HTTP.Port
		}
		if !override.HTTP.Path.Empty() {
			health.HTTP.Path = override.HTTP.Path
		}
		health.Command = nil
	}
	if override.Delay != nil {
		health.Delay = override.Delay
	}
	if override.Period != nil {
		health.Period = override.Period
	}
	if override.Timeout != nil {
		health.Timeout = override.Timeout
	}
	if override.Success != nil {
		health.Success = override.Success
	}
	if override.Failure != nil {
		health.Failure = override.Failure
	}
	return health
}

func defaultProbeHealth(probes probesSpec, kind string) healthSpec {
	preset := strings.TrimSpace(probes.Preset)
	if preset == "" {
		return healthSpec{}
	}

	switch kind {
	case "live":
		return healthSpec{Period: intPtr(10), Timeout: intPtr(2), Success: intPtr(1), Failure: intPtr(5)}
	case "ready":
		return healthSpec{Period: intPtr(2), Timeout: intPtr(2), Success: intPtr(2), Failure: intPtr(2)}
	case "start":
		return healthSpec{Period: intPtr(10), Timeout: intPtr(2), Failure: intPtr(30)}
	default:
		return healthSpec{}
	}
}

func commonProbeHTTP(probes probesSpec, kind string) httpHealthSpec {
	http := probes.HTTP
	if http.Port == 0 {
		http.Port = probes.Port
	}
	if http.Path.Empty() {
		http.Path = probes.Path
	}

	if strings.TrimSpace(probes.Preset) == "spring-actuator" && http.Path.Empty() {
		switch kind {
		case "live":
			http.Path = scalarPath("/actuator/health/liveness")
		case "ready":
			http.Path = scalarPath("/actuator/health/readiness")
		case "start":
			http.Path = scalarPath("/actuator/health")
		}
	}
	return http
}

func isEmptyProbes(probes probesSpec) bool {
	return strings.TrimSpace(probes.Preset) == "" &&
		probes.Port == 0 &&
		probes.Path.Empty() &&
		!hasHTTPHealth(probes.HTTP) &&
		isEmptyHealth(probes.Live) &&
		isEmptyHealth(probes.Ready) &&
		isEmptyHealth(probes.Start)
}

func isEmptyHealth(health healthSpec) bool {
	return !hasHTTPHealth(health.HTTP) &&
		len(health.Command) == 0 &&
		health.Delay == nil &&
		health.Period == nil &&
		health.Timeout == nil &&
		health.Success == nil &&
		health.Failure == nil
}

func hasHTTPHealth(http httpHealthSpec) bool {
	return http.Port != 0 || !http.Path.Empty()
}

func scalarPath(value string) pathValue {
	return pathValue{Scalar: value}
}

func intPtr(value int) *int {
	return &value
}

func renderHealth(health healthSpec, probeType string) map[string]any {
	out := map[string]any{}
	if health.HTTP.Port != 0 || health.HTTP.Path.For(probeType) != "" {
		out["httpGet"] = map[string]any{
			"path": health.HTTP.Path.For(probeType),
			"port": int(health.HTTP.Port),
		}
	} else if len(health.Command) > 0 {
		out["exec"] = map[string]any{
			"command": health.Command,
		}
	}
	if len(out) == 0 {
		return nil
	}
	if health.Delay != nil {
		out["initialDelaySeconds"] = *health.Delay
	}
	if health.Period != nil {
		out["periodSeconds"] = *health.Period
	}
	if health.Timeout != nil {
		out["timeoutSeconds"] = *health.Timeout
	}
	if health.Success != nil {
		out["successThreshold"] = *health.Success
	}
	if health.Failure != nil {
		out["failureThreshold"] = *health.Failure
	}
	return out
}

func resolveAssets(app appModel, envDir string, vars map[string]string, opts Options) (map[string][]resolvedAsset, error) {
	out := map[string][]resolvedAsset{}
	for _, container := range app.Containers {
		assetSpecs := append([]assetSpec{}, container.Assets...)
		assetSpecs = append(assetSpecs, mtlsAssetSpecs(app.Name, container)...)
		for _, item := range assetSpecs {
			asset, err := resolveAsset(app.Name, container.Name, item, envDir, vars, opts)
			if err != nil {
				return nil, err
			}
			out[container.Name] = append(out[container.Name], asset)
		}
		for _, item := range container.Mounts {
			asset, err := resolveMount(app.Name, container.Name, item, envDir, vars, opts)
			if err != nil {
				return nil, err
			}
			out[container.Name] = append(out[container.Name], asset)
		}
	}
	for _, container := range app.InitContainers {
		for _, item := range container.Mounts {
			asset, err := resolveMount(app.Name, container.Name, item, envDir, vars, opts)
			if err != nil {
				return nil, err
			}
			out[container.Name] = append(out[container.Name], asset)
		}
	}
	return out, nil
}

func loadSharedAssets(envDir string, vars map[string]string, opts Options) ([]resolvedAsset, error) {
	path := filepath.Join(envDir, "shared.assets.yml")
	if !isFile(path) {
		return nil, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var parsed sharedAssetsFile
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make([]resolvedAsset, 0, len(parsed.Assets))
	for _, item := range parsed.Assets {
		asset, err := resolveAsset("shared", "shared", item, envDir, vars, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, asset)
	}
	return out, nil
}

func mtlsAssetSpecs(appName string, container containerSpec) []assetSpec {
	if !container.MTLS.Enabled {
		return nil
	}
	mountDir := mtlsMountDir(container)
	return []assetSpec{
		{
			File:      filepath.ToSlash(filepath.Join("mtls", appName, container.Name+".secured.json")),
			To:        mountDir + "/mtls.secured.json",
			Transform: false,
			Binary:    false,
		},
		{
			File:      filepath.ToSlash(filepath.Join("mtls", appName, container.Name+".secured.schema.json")),
			To:        mountDir + "/mtls.secured.schema.json",
			Transform: false,
			Binary:    false,
		},
	}
}

func mtlsMountDir(container containerSpec) string {
	mountDir := strings.TrimSpace(container.MTLS.MountDir)
	if mountDir == "" {
		return "/app/mtls.enc"
	}
	return mountDir
}

func profilesPayload(envDir string, opts Options) any {
	path := strings.TrimSpace(opts.ProfilesFile)
	if path == "" {
		path = filepath.Join(envDir, "replica-profiles.yml")
	}
	if !isFile(path) {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"file": path, "error": err.Error()}
	}
	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return map[string]any{"file": path, "error": err.Error()}
	}
	defaults, _ := data["defaults"].(map[string]any)
	profiles, _ := data["profiles"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	if profiles == nil {
		profiles = map[string]any{}
	}
	return map[string]any{
		"file":     path,
		"defaults": defaults,
		"profiles": profiles,
	}
}

func resolveAsset(appName string, containerName string, spec assetSpec, envDir string, vars map[string]string, opts Options) (resolvedAsset, error) {
	if spec.To == "" {
		return resolvedAsset{}, fmt.Errorf("asset %s target path is required", spec.File)
	}
	if spec.Temp {
		content := spec.To
		digest := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(spec.File+spec.To+content)))
		return resolvedAsset{VolumeName: appName + "-temp-" + digest, ContainerName: containerName, To: spec.To, Kind: "temp"}, nil
	}
	if spec.PVC {
		content := spec.To
		digest := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(spec.File+spec.To+content)))
		return resolvedAsset{VolumeName: appName + "-pvc-" + digest, ContainerName: containerName, To: spec.To, Kind: "pvc", PVCName: spec.Name}, nil
	}
	if spec.NFSServer != "" {
		content := spec.Path
		digest := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(spec.File+spec.To+content)))
		return resolvedAsset{VolumeName: appName + "-nfs-" + digest, ContainerName: containerName, To: spec.To, Kind: "nfs", NFSServer: spec.NFSServer, NFSPath: spec.Path}, nil
	}
	if spec.HostPath != "" {
		content := spec.HostPath + spec.To
		digest := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(spec.File+spec.To+content)))
		return resolvedAsset{VolumeName: appName + "-host-" + digest, ContainerName: containerName, To: spec.To, Kind: "host", HostPath: spec.HostPath}, nil
	}
	if spec.File == "" {
		return resolvedAsset{}, errors.New("asset file is required")
	}
	sourcePath := filepath.Join(envDir, spec.File)
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return resolvedAsset{}, err
	}
	renderedContent := content
	if spec.Transform && !spec.Binary {
		renderedContent = []byte(applyEnvPlainPlaceholders(string(content), vars))
	}
	if assetHelmEscape(spec, opts) && !spec.Binary {
		renderedContent = []byte(helmEscapePlaceholders(string(renderedContent)))
	}
	digestInput := spec.File + spec.To + string(renderedContent)
	digest := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(digestInput)))
	key := filepath.Base(sourcePath)
	if spec.Transform {
		key = nameWithoutLastExt(key)
	}
	return resolvedAsset{
		VolumeName:    containerName + "-asset-" + digest,
		ConfigMapKey:  key,
		ContainerName: containerName,
		To:            spec.To,
		Content:       renderedContent,
		Binary:        spec.Binary,
		Kind:          "configmap",
	}, nil
}

func resolveMount(appName string, containerName string, spec mountSpec, envDir string, vars map[string]string, opts Options) (resolvedAsset, error) {
	switch strings.TrimSpace(spec.Type) {
	case "config":
		return resolveAsset(appName, containerName, assetSpec{
			File:       spec.File,
			To:         spec.MountPath,
			Binary:     spec.Binary,
			Transform:  spec.Transform,
			HelmEscape: spec.HelmEscape,
		}, envDir, vars, opts)
	case "empty_dir":
		asset, err := resolveAsset(appName, containerName, assetSpec{Temp: true, To: spec.MountPath}, envDir, vars, opts)
		return withMountVolumeName(asset, spec.Name), err
	case "pvc":
		claimName := spec.ClaimName
		if claimName == "" {
			claimName = spec.Name
		}
		asset, err := resolveAsset(appName, containerName, assetSpec{PVC: true, Name: claimName, To: spec.MountPath}, envDir, vars, opts)
		return withMountVolumeName(asset, spec.Name), err
	case "nfs":
		asset, err := resolveAsset(appName, containerName, assetSpec{NFSServer: spec.Server, Path: spec.Path, To: spec.MountPath}, envDir, vars, opts)
		return withMountVolumeName(asset, spec.Name), err
	case "host_path":
		asset, err := resolveAsset(appName, containerName, assetSpec{HostPath: spec.Path, To: spec.MountPath}, envDir, vars, opts)
		return withMountVolumeName(asset, spec.Name), err
	case "raw":
		volumeName := rawVolumeName(spec)
		if volumeName == "" {
			return resolvedAsset{}, errors.New("raw mount requires volume.name or mount.name")
		}
		if len(spec.Volume) == 0 {
			return resolvedAsset{}, errors.New("raw mount requires volume")
		}
		if len(spec.Mount) == 0 {
			return resolvedAsset{}, errors.New("raw mount requires mount")
		}
		return resolvedAsset{VolumeName: volumeName, ContainerName: containerName, Kind: "raw", RawVolume: spec.Volume, RawMount: spec.Mount}, nil
	case "":
		return resolvedAsset{}, errors.New("mount type is required")
	default:
		return resolvedAsset{}, fmt.Errorf("unsupported mount type: %s", spec.Type)
	}
}

func withMountVolumeName(asset resolvedAsset, name string) resolvedAsset {
	if strings.TrimSpace(name) != "" {
		asset.VolumeName = strings.TrimSpace(name)
	}
	return asset
}

func rawVolumeName(spec mountSpec) string {
	if name, _ := spec.Volume["name"].(string); name != "" {
		return name
	}
	if name, _ := spec.Mount["name"].(string); name != "" {
		return name
	}
	return spec.Name
}

func renderVolumes(app appModel, assets map[string][]resolvedAsset, sharedAssets []resolvedAsset) []any {
	all := orderedResolvedAssets(app, assets)
	all = append(sharedAssets, all...)
	out := make([]any, 0, len(all))
	seenVolumes := map[string]bool{}
	for _, asset := range all {
		if seenVolumes[asset.VolumeName] {
			continue
		}
		seenVolumes[asset.VolumeName] = true
		volume := map[string]any{"name": asset.VolumeName}
		switch asset.Kind {
		case "temp":
			volume["emptyDir"] = map[string]any{}
		case "pvc":
			volume["persistentVolumeClaim"] = map[string]any{"claimName": asset.PVCName}
		case "nfs":
			volume["nfs"] = map[string]any{"server": asset.NFSServer, "path": asset.NFSPath}
		case "host":
			volume["hostPath"] = map[string]any{"path": asset.HostPath, "type": "Directory"}
		case "raw":
			volume = cloneMap(asset.RawVolume)
		default:
			volume["configMap"] = map[string]any{"defaultMode": 420, "name": asset.VolumeName}
		}
		out = append(out, volume)
	}
	return out
}

func renderVolumeMounts(assets []resolvedAsset) []any {
	out := make([]any, 0, len(assets))
	for _, asset := range assets {
		if asset.Kind == "raw" {
			out = append(out, cloneMap(asset.RawMount))
			continue
		}
		mount := map[string]any{
			"mountPath": asset.To,
			"name":      asset.VolumeName,
		}
		if asset.Kind == "configmap" || asset.Kind == "" {
			mount["readOnly"] = true
			mount["subPath"] = asset.ConfigMapKey
		}
		out = append(out, mount)
	}
	return out
}

func renderAssetConfigMap(asset resolvedAsset, namespace string, argocdWave int) map[string]any {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      asset.VolumeName,
			"namespace": namespace,
		},
	}
	if argocdWave != 0 {
		obj["metadata"].(map[string]any)["annotations"] = map[string]any{
			"argocd.argoproj.io/sync-wave": fmt.Sprint(argocdWave * -1),
		}
	}
	if asset.Binary {
		obj["binaryData"] = map[string]any{
			asset.ConfigMapKey: base64.StdEncoding.EncodeToString(asset.Content),
		}
	} else {
		obj["data"] = map[string]any{
			asset.ConfigMapKey: string(asset.Content),
		}
	}
	return obj
}

func appArgoCDWave(app appModel) int {
	for key, value := range app.Annotations {
		if key != "argocd.argoproj.io/sync-wave" {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		case string:
			parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
			return parsed
		default:
			parsed, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
			return parsed
		}
	}
	return 0
}

func appsHaveArgoCDWave(apps []appModel) bool {
	for _, app := range apps {
		if appArgoCDWave(app) != 0 {
			return true
		}
	}
	return false
}

func allResolvedAssets(assets map[string][]resolvedAsset) []resolvedAsset {
	keys := make([]string, 0, len(assets))
	for key := range assets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []resolvedAsset{}
	for _, key := range keys {
		out = append(out, assets[key]...)
	}
	return out
}

func orderedResolvedAssets(app appModel, assets map[string][]resolvedAsset) []resolvedAsset {
	out := []resolvedAsset{}
	seen := map[string]bool{}
	for _, container := range app.Containers {
		out = append(out, assets[container.Name]...)
		seen[container.Name] = true
	}
	keys := make([]string, 0, len(assets))
	for key := range assets {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, assets[key]...)
	}
	return out
}

func configMapResolvedAssets(assets map[string][]resolvedAsset) []resolvedAsset {
	all := allResolvedAssets(assets)
	out := make([]resolvedAsset, 0, len(all))
	for _, asset := range all {
		if asset.Kind == "configmap" || asset.Kind == "" {
			out = append(out, asset)
		}
	}
	return out
}

func assetHelmEscape(spec assetSpec, opts Options) bool {
	if spec.HelmEscape != nil {
		return *spec.HelmEscape
	}
	return opts.HelmEscapeAssets
}

func helmEscapePlaceholders(content string) string {
	pattern := regexp.MustCompile(`(\{\{\s*)(\w+)(\s*\}\})`)
	indexes := pattern.FindAllStringIndex(content, -1)
	if len(indexes) == 0 {
		return content
	}
	var out strings.Builder
	last := 0
	for _, index := range indexes {
		start, end := index[0], index[1]
		match := content[start:end]
		out.WriteString(content[last:start])
		if start >= 3 && end+3 <= len(content) && content[start-3:start] == "{{`" && content[end:end+3] == "`}}" {
			out.WriteString(match)
		} else {
			out.WriteString("{{`")
			out.WriteString(match)
			out.WriteString("`}}")
		}
		last = end
	}
	out.WriteString(content[last:])
	return out.String()
}

func nameWithoutLastExt(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
}

func renderContainerPorts(ports []portSpec) []map[string]any {
	out := make([]map[string]any, 0, len(ports))
	for _, port := range ports {
		out = append(out, map[string]any{
			"name":          port.Name,
			"containerPort": port.Port,
			"protocol":      "TCP",
		})
	}
	return out
}

type renderedService struct {
	Name      string
	Object    map[string]any
	Externals []renderedExternal
}

type renderedExternal struct {
	Name   string
	Kind   string
	Object map[string]any
}

func renderServices(app appModel, namespace string, environment string) []renderedService {
	servicesByHost := map[string][]map[string]any{}
	externalPortsByHost := map[string][]serviceExternalPort{}
	headlessByHost := map[string]bool{}
	metricsPathForByHost := map[string]string{}
	hasMetricsByHost := map[string]bool{}

	for _, container := range app.Containers {
		for _, port := range container.Ports {
			for _, expose := range port.ExposeAs {
				if expose.Hostname == "" {
					continue
				}
				serviceType := expose.Type
				if serviceType == "" {
					serviceType = "clusterip"
				}
				servicePort := map[string]any{
					"name":       fmt.Sprintf("%s-%d", port.Name, expose.Port),
					"port":       expose.Port,
					"targetPort": port.Port,
				}
				servicesByHost[expose.Hostname] = append(servicesByHost[expose.Hostname], servicePort)
				if len(expose.External) > 0 {
					externalPortsByHost[expose.Hostname] = append(externalPortsByHost[expose.Hostname], serviceExternalPort{
						Name:      fmt.Sprintf("%s-%d", port.Name, expose.Port),
						Port:      expose.Port,
						Externals: expose.External,
					})
				}
				if serviceType == "headless" {
					headlessByHost[expose.Hostname] = true
				}
				if port.Metrics {
					metricsPort := map[string]any{
						"name":       "metrics",
						"port":       9090,
						"targetPort": port.Port,
					}
					servicesByHost[expose.Hostname] = append(servicesByHost[expose.Hostname], metricsPort)
					if len(expose.External) > 0 {
						externalPortsByHost[expose.Hostname] = append(externalPortsByHost[expose.Hostname], serviceExternalPort{
							Name:      "metrics",
							Port:      9090,
							Externals: expose.External,
						})
					}
					hasMetricsByHost[expose.Hostname] = true
					if port.MetricsPathFor != nil {
						metricsPathForByHost[expose.Hostname] = expose.Hostname
					}
				}
			}
		}
	}

	names := make([]string, 0, len(servicesByHost))
	for name := range servicesByHost {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]renderedService, 0, len(names))
	for _, name := range names {
		metadata := map[string]any{
			"name":      name,
			"namespace": namespace,
		}
		if hasMetricsByHost[name] {
			labels := map[string]any{
				appLabel:      app.Name,
				"metrics":     "true",
				"application": app.Name,
				"environment": environment,
			}
			if metricsPathForByHost[name] != "" {
				labels["metricsPathFor"] = metricsPathForByHost[name]
			}
			metadata["labels"] = labels
		}
		spec := map[string]any{
			"selector": map[string]any{appLabel: app.Name},
			"ports":    servicesByHost[name],
		}
		if headlessByHost[name] {
			spec["clusterIP"] = "None"
		}
		out = append(out, renderedService{
			Name: name,
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   metadata,
				"spec":       spec,
			},
			Externals: renderExternals(name, namespace, externalPortsByHost[name]),
		})
	}
	return out
}

type serviceExternalPort struct {
	Name      string
	Port      int
	Externals []externalSpec
}

func renderExternals(serviceName string, namespace string, ports []serviceExternalPort) []renderedExternal {
	out := []renderedExternal{}
	for _, port := range ports {
		for _, external := range port.Externals {
			if external.Name == "" {
				continue
			}
			if external.AsRoute {
				out = append(out, renderedExternal{
					Name:   external.Name,
					Kind:   "route",
					Object: renderRoute(serviceName, namespace, port, external),
				})
			} else {
				out = append(out, renderedExternal{
					Name:   external.Name,
					Kind:   "ingress",
					Object: renderIngress(serviceName, namespace, port, external),
				})
			}
		}
	}
	return out
}

func renderIngress(serviceName string, namespace string, port serviceExternalPort, external externalSpec) map[string]any {
	metadata := map[string]any{"name": external.Name, "namespace": namespace}
	if len(external.Annotations) > 0 {
		metadata["annotations"] = external.Annotations
	}
	if len(external.Labels) > 0 {
		metadata["labels"] = external.Labels
	}
	spec := map[string]any{}
	if external.ClassName != "" {
		spec["ingressClassName"] = external.ClassName
	}
	rules := []any{}
	for _, host := range external.HTTP {
		rules = append(rules, map[string]any{
			"host": host.Hostname,
			"http": map[string]any{
				"paths": []any{
					map[string]any{
						"path": host.Path,
						"backend": map[string]any{
							"service": map[string]any{
								"name": serviceName,
								"port": map[string]any{"number": port.Port},
							},
						},
						"pathType": "ImplementationSpecific",
					},
				},
			},
		})
	}
	if len(rules) > 0 {
		spec["rules"] = rules
	}
	tls := []any{}
	for _, host := range external.HTTPS {
		entry := map[string]any{"hosts": []any{host.Hostname}}
		if host.SecretName != "" {
			entry["secretName"] = host.SecretName
		}
		tls = append(tls, entry)
	}
	if len(tls) > 0 {
		spec["tls"] = tls
	}
	return map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "Ingress",
		"metadata":   metadata,
		"spec":       spec,
	}
}

func renderRoute(serviceName string, namespace string, port serviceExternalPort, external externalSpec) map[string]any {
	host := externalHostSpec{}
	if len(external.HTTP) > 0 {
		host = external.HTTP[0]
	}
	spec := map[string]any{
		"host": host.Hostname,
		"port": map[string]any{
			"targetPort": port.Name,
		},
		"to": map[string]any{
			"kind": "Service",
			"name": serviceName,
		},
		"wildcardPolicy": "None",
	}
	if len(external.TLS) > 0 {
		tls := map[string]any{}
		if value, ok := external.TLS["termination"]; ok {
			tls["termination"] = value
		}
		if value, ok := external.TLS["insecureEdgeTerminationPolicy"]; ok {
			tls["insecureEdgeTerminationPolicy"] = value
		}
		if len(tls) > 0 {
			spec["tls"] = tls
		}
	}
	return map[string]any{
		"apiVersion": "route.openshift.io/v1",
		"kind":       "Route",
		"metadata":   map[string]any{"name": external.Name, "namespace": namespace},
		"spec":       spec,
	}
}

func renderVars(items []envVar) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		env := map[string]any{"name": item.Name}
		switch {
		case item.SecretName != "":
			env["valueFrom"] = map[string]any{
				"secretKeyRef": map[string]any{
					"key":  item.Key,
					"name": item.SecretName,
				},
			}
		case item.ResourceName != "":
			env["valueFrom"] = map[string]any{
				"resourceFieldRef": map[string]any{
					"resource": item.ResourceName,
					"divisor":  item.Divisor,
				},
			}
		case item.FieldPath != "":
			env["valueFrom"] = map[string]any{
				"fieldRef": map[string]any{
					"fieldPath": item.FieldPath,
				},
			}
		default:
			env["value"] = item.Value
		}
		out = append(out, env)
	}
	return out
}

func effectiveContainerVars(container containerSpec) []envVar {
	items := []envVar{}
	if container.EnableCgroupExporter {
		items = append(items, cgroupExporterDefaultVars...)
	}
	if item, ok := renderJavaRuntimeEnvVar(container.Runtime.Java); ok {
		items = append(items, item)
	}
	items = append(items, container.Vars...)
	if len(items) == 0 {
		return nil
	}
	positions := map[string]int{}
	out := []envVar{}
	for _, item := range items {
		if item.Name == "" {
			continue
		}
		if index, ok := positions[item.Name]; ok {
			out[index] = item
			continue
		}
		positions[item.Name] = len(out)
		out = append(out, item)
	}
	return out
}

func renderJavaRuntimeEnvVar(java javaRuntimeSpec) (envVar, bool) {
	if !javaRuntimeEnabled(java) {
		return envVar{}, false
	}
	parts := []string{}
	if strings.TrimSpace(java.Xms) != "" {
		parts = append(parts, "-Xms"+strings.TrimSpace(java.Xms))
	}
	if strings.TrimSpace(java.Xmx) != "" {
		parts = append(parts, "-Xmx"+strings.TrimSpace(java.Xmx))
	}
	for _, opt := range java.Opts {
		if strings.TrimSpace(opt) != "" {
			parts = append(parts, strings.TrimSpace(opt))
		}
	}
	return envVar{Name: javaRuntimeEnvName(java), Value: strings.Join(parts, " ")}, true
}

func javaRuntimeEnabled(java javaRuntimeSpec) bool {
	if strings.TrimSpace(java.Xms) != "" || strings.TrimSpace(java.Xmx) != "" {
		return true
	}
	for _, opt := range java.Opts {
		if strings.TrimSpace(opt) != "" {
			return true
		}
	}
	return false
}

func javaRuntimeEnvName(java javaRuntimeSpec) string {
	if strings.TrimSpace(java.Export.EnvName) != "" {
		return strings.TrimSpace(java.Export.EnvName)
	}
	return "JAVA_OPTS"
}

func renderToolInitContainers(tools []toolSpec) []map[string]any {
	out := []map[string]any{}
	for _, tool := range tools {
		if strings.TrimSpace(tool.ExposeBin) == "" {
			continue
		}
		image := strings.TrimSpace(tool.Image)
		if image == "" {
			image = strings.TrimSpace(tool.Name)
		}
		if image == "" {
			continue
		}
		relTarget := toolTargetRelative(tool)
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			name = "tool-" + filepath.Base(relTarget)
		}
		script := strings.Join([]string{
			"set -eu",
			"mkdir -p /work/tools/" + shellQuote(filepath.Dir(relTarget)),
			"cp " + shellQuote(tool.ExposeBin) + " /work/tools/" + shellQuote(relTarget),
			"chmod +x /work/tools/" + shellQuote(relTarget),
		}, "; ")
		out = append(out, map[string]any{
			"name":            name,
			"image":           image,
			"imagePullPolicy": "IfNotPresent",
			"command":         []string{"/bin/sh", "-ec", script},
			"volumeMounts": []any{
				map[string]any{
					"name":      "app-tools",
					"mountPath": "/work/tools",
				},
			},
		})
	}
	return out
}

func toolTargetRelative(tool toolSpec) string {
	target := strings.TrimSpace(tool.As)
	if target == "" {
		target = "/app/tools/" + filepath.Base(tool.ExposeBin)
	}
	if strings.HasPrefix(target, "/app/tools/") {
		rel := strings.TrimPrefix(target, "/app/tools/")
		if rel == "" {
			return filepath.Base(tool.ExposeBin)
		}
		return rel
	}
	return filepath.Base(target)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func renderResources(resources map[string]map[string]any) map[string]any {
	out := map[string]any{}
	for _, name := range []string{"cpu", "memory", "ephemeral-storage"} {
		values, ok := resources[name]
		if !ok {
			continue
		}
		if value, ok := firstResourceValue(values, "requests", "from"); ok {
			requests, _ := out["requests"].(map[string]any)
			if requests == nil {
				requests = map[string]any{}
				out["requests"] = requests
			}
			requests[name] = value
		}
		if value, ok := firstResourceValue(values, "limits", "to"); ok {
			limits, _ := out["limits"].(map[string]any)
			if limits == nil {
				limits = map[string]any{}
				out["limits"] = limits
			}
			limits[name] = value
		}
	}
	return out
}

func firstResourceValue(values map[string]any, preferred string, legacy string) (any, bool) {
	if value, ok := values[preferred]; ok {
		return value, true
	}
	value, ok := values[legacy]
	return value, ok
}

func applyEnvPlaceholders(content string, vars map[string]string) string {
	for key, value := range vars {
		pattern := regexp.MustCompile(`\{\{\s*env\s*:\s*` + regexp.QuoteMeta(key) + `\s*\}\}`)
		content = pattern.ReplaceAllStringFunc(content, func(string) string { return value })
	}
	return content
}

func applyEnvPlainPlaceholders(content string, vars map[string]string) string {
	for key, value := range vars {
		pattern := regexp.MustCompile(`\{\{\s*` + regexp.QuoteMeta(key) + `\s*\}\}`)
		content = pattern.ReplaceAllStringFunc(content, func(string) string { return value })
	}
	return content
}

func applyAppVarPlaceholders(content string, vars map[string]string) string {
	for key, value := range vars {
		pattern := regexp.MustCompile(`\{\{\s*var\s*:\s*` + regexp.QuoteMeta(key) + `\s*\}\}`)
		content = pattern.ReplaceAllStringFunc(content, func(string) string { return value })
	}
	return content
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
