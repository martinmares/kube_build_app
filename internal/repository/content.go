package repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type AppDetail struct {
	Env         string `json:"env"`
	FileName    string `json:"file_name"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	Summary     App    `json:"summary"`
	IsDirty     bool   `json:"is_dirty"`
}

type AssetDetail struct {
	Env          string `json:"env"`
	RelativePath string `json:"relative_path"`
	Path         string `json:"path"`
	Content      string `json:"content"`
	ContentHash  string `json:"content_hash"`
	IsDirty      bool   `json:"is_dirty"`
}

type AppRendered struct {
	Env      string `json:"env"`
	FileName string `json:"file_name"`
	Content  string `json:"content"`
}

type AppVars struct {
	Env         string    `json:"env"`
	FileName    string    `json:"file_name"`
	ContentHash string    `json:"content_hash"`
	Items       []VarItem `json:"items"`
}

type AppReplicas struct {
	Env         string `json:"env"`
	FileName    string `json:"file_name"`
	ContentHash string `json:"content_hash"`
	Replicas    *int   `json:"replicas"`
}

type AppAutoscaling struct {
	Env         string           `json:"env"`
	FileName    string           `json:"file_name"`
	ContentHash string           `json:"content_hash"`
	Autoscaling AutoscalingModel `json:"autoscaling"`
}

type AppContainerResources struct {
	Env            string        `json:"env"`
	FileName       string        `json:"file_name"`
	ContentHash    string        `json:"content_hash"`
	ContainerIndex int           `json:"container_index"`
	Resources      ResourceModel `json:"resources"`
}

type AppContainerEnvs struct {
	Env            string        `json:"env"`
	FileName       string        `json:"file_name"`
	ContentHash    string        `json:"content_hash"`
	ContainerIndex int           `json:"container_index"`
	Envs           []EnvVarModel `json:"envs"`
}

type AppContainerProbes struct {
	Env            string      `json:"env"`
	FileName       string      `json:"file_name"`
	ContentHash    string      `json:"content_hash"`
	ContainerIndex int         `json:"container_index"`
	Probes         ProbesModel `json:"probes"`
}

type AppContainerRuntime struct {
	Env            string       `json:"env"`
	FileName       string       `json:"file_name"`
	ContentHash    string       `json:"content_hash"`
	ContainerIndex int          `json:"container_index"`
	Runtime        RuntimeModel `json:"runtime"`
}

type AppContainerPorts struct {
	Env            string      `json:"env"`
	FileName       string      `json:"file_name"`
	ContentHash    string      `json:"content_hash"`
	ContainerIndex int         `json:"container_index"`
	Ports          []PortModel `json:"ports"`
}

type AutoscalingUpdate struct {
	Enabled                  bool   `json:"enabled"`
	MinReplicas              string `json:"min_replicas"`
	MaxReplicas              string `json:"max_replicas"`
	CPUAverageUtilization    string `json:"cpu_average_utilization"`
	MemoryAverageUtilization string `json:"memory_average_utilization"`
}

type ResourceUpdate struct {
	CPURequest              string `json:"cpu_request"`
	CPULimit                string `json:"cpu_limit"`
	MemoryRequest           string `json:"memory_request"`
	MemoryLimit             string `json:"memory_limit"`
	EphemeralStorageRequest string `json:"ephemeral_storage_request"`
	EphemeralStorageLimit   string `json:"ephemeral_storage_limit"`
}

type ProbeUpdate struct {
	Preset string `json:"preset"`
	Port   string `json:"port"`
	Path   string `json:"path"`
}

type JavaRuntimeUpdate struct {
	Xms           string   `json:"xms"`
	Xmx           string   `json:"xmx"`
	Opts          []string `json:"opts"`
	ExportEnvName string   `json:"export_env_name"`
}

type PortUpdate struct {
	Name           string         `json:"name"`
	Port           string         `json:"port"`
	Metrics        bool           `json:"metrics"`
	MetricsPathFor string         `json:"metrics_path_for"`
	ExposeAs       []ExposeUpdate `json:"expose_as"`
}

type ExposeUpdate struct {
	ServiceName string           `json:"service_name"`
	Port        string           `json:"port"`
	ServiceType string           `json:"service_type"`
	Externals   []ExternalUpdate `json:"externals"`
}

type ExternalUpdate struct {
	Name          string `json:"name"`
	AsRoute       bool   `json:"as_route"`
	ClassName     string `json:"class_name"`
	HTTPHostname  string `json:"http_hostname"`
	HTTPPath      string `json:"http_path"`
	HTTPSHostname string `json:"https_hostname"`
	HTTPSPath     string `json:"https_path"`
	SecretName    string `json:"secret_name"`
}

type VarItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SpecialEntries struct {
	Env         string         `json:"env"`
	SpecialFile string         `json:"special_file"`
	ContentHash string         `json:"content_hash"`
	Editable    bool           `json:"editable"`
	IsDirty     bool           `json:"is_dirty"`
	Entries     []SpecialEntry `json:"entries"`
	Warning     *string        `json:"warning"`
}

type SpecialEntry struct {
	Key       string `json:"key"`
	ValueType string `json:"value_type"`
	ValueText string `json:"value_text"`
}

type DefaultsModel struct {
	Env           string              `json:"env"`
	FileName      string              `json:"file_name"`
	Path          string              `json:"path"`
	ContentHash   string              `json:"content_hash"`
	IsDirty       bool                `json:"is_dirty"`
	Vars          []VarItem           `json:"vars"`
	ContainerEnvs []ContainerEnvGroup `json:"container_envs"`
}

type ContainerEnvGroup struct {
	ContainerRefName string        `json:"container_ref_name"`
	Envs             []EnvVarModel `json:"envs"`
}

type ContainerEnvGroupUpdate struct {
	ContainerRefName string    `json:"container_ref_name"`
	Envs             []VarItem `json:"envs"`
}

type AppModel struct {
	Env                  string           `json:"env"`
	FileName             string           `json:"file_name"`
	AppName              *string          `json:"app_name"`
	SidecarRefNames      []string         `json:"sidecar_ref_names"`
	RuntimeAssetRefNames []string         `json:"runtime_asset_ref_names"`
	Replicas             *int             `json:"replicas"`
	Kind                 *string          `json:"kind"`
	Autoscaling          AutoscalingModel `json:"autoscaling"`
	InitContainersCount  int              `json:"init_containers_count"`
	Containers           []ContainerModel `json:"containers"`
	Sidecars             []ContainerModel `json:"sidecars"`
}

type ContainerModel struct {
	Index                int           `json:"index"`
	Name                 *string       `json:"name"`
	ProfileRefNames      []string      `json:"profile_ref_names"`
	RuntimeAssetRefNames []string      `json:"runtime_asset_ref_names"`
	StartupCommand       []string      `json:"startup_command"`
	StartupArguments     []string      `json:"startup_arguments"`
	Envs                 []EnvVarModel `json:"envs"`
	Resources            ResourceModel `json:"resources"`
	Ports                []PortModel   `json:"ports"`
	Runtime              RuntimeModel  `json:"runtime"`
	Probes               ProbesModel   `json:"probes"`
	EnvFromCount         int           `json:"env_from_count"`
	MountsCount          int           `json:"mounts_count"`
}

type AppReferences struct {
	Env                  string                `json:"env"`
	FileName             string                `json:"file_name"`
	ContentHash          string                `json:"content_hash"`
	DefaultsHash         string                `json:"defaults_hash"`
	SidecarRefNames      []string              `json:"sidecar_ref_names"`
	RuntimeAssetRefNames []string              `json:"runtime_asset_ref_names"`
	Containers           []ContainerReferences `json:"containers"`
	Sidecars             []ContainerReferences `json:"sidecars"`
	Catalog              ReferenceCatalog      `json:"catalog"`
}

type ContainerReferences struct {
	Index                int      `json:"index"`
	Name                 string   `json:"name"`
	ProfileRefNames      []string `json:"profile_ref_names"`
	RuntimeAssetRefNames []string `json:"runtime_asset_ref_names"`
}

type ReferenceCatalog struct {
	ContainerProfiles       []string `json:"container_profiles"`
	SidecarDefinitions      []string `json:"sidecar_definitions"`
	RuntimeAssetDefinitions []string `json:"runtime_asset_definitions"`
}

type AppReferencesUpdate struct {
	SidecarRefNames      []string              `json:"sidecar_ref_names"`
	RuntimeAssetRefNames []string              `json:"runtime_asset_ref_names"`
	Containers           []ContainerReferences `json:"containers"`
	Sidecars             []ContainerReferences `json:"sidecars"`
	RemoveSidecarPatches []string              `json:"remove_sidecar_patches,omitempty"`
}

type EnvVarModel struct {
	Index                        int     `json:"index"`
	Name                         *string `json:"name"`
	Kind                         string  `json:"kind"`
	Value                        *string `json:"value"`
	Key                          *string `json:"key"`
	SecretName                   *string `json:"secret_name"`
	ResourceName                 *string `json:"resource_name"`
	Divisor                      *string `json:"divisor"`
	FieldPath                    *string `json:"field_path"`
	WorkloadIdentityTokenRefName *string `json:"workload_identity_token_ref_name"`
	SharedAssetRefName           *string `json:"shared_asset_ref_name"`
	Remove                       bool    `json:"remove"`
	IsValueEditable              bool    `json:"is_value_editable"`
}

type ResourceModel struct {
	CPURequest              *string `json:"cpu_request"`
	CPULimit                *string `json:"cpu_limit"`
	MemoryRequest           *string `json:"memory_request"`
	MemoryLimit             *string `json:"memory_limit"`
	EphemeralStorageRequest *string `json:"ephemeral_storage_request"`
	EphemeralStorageLimit   *string `json:"ephemeral_storage_limit"`
	CPUFrom                 *string `json:"cpu_from"`
	CPUTo                   *string `json:"cpu_to"`
	MemoryFrom              *string `json:"memory_from"`
	MemoryTo                *string `json:"memory_to"`
	EphemeralStorageFrom    *string `json:"ephemeral_storage_from"`
	EphemeralStorageTo      *string `json:"ephemeral_storage_to"`
}

type RuntimeModel struct {
	Java JavaRuntimeModel `json:"java"`
}

type JavaRuntimeModel struct {
	Enabled       bool     `json:"enabled"`
	Xms           *string  `json:"xms"`
	Xmx           *string  `json:"xmx"`
	Opts          []string `json:"opts"`
	ExportEnvName string   `json:"export_env_name"`
}

type ProbesModel struct {
	Preset      *string  `json:"preset"`
	Port        *string  `json:"port"`
	Path        *string  `json:"path"`
	Legacy      bool     `json:"legacy"`
	LegacyKinds []string `json:"legacy_kinds"`
	Enabled     bool     `json:"enabled"`
}

type AutoscalingModel struct {
	Enabled                  bool `json:"enabled"`
	MinReplicas              *int `json:"min_replicas"`
	MaxReplicas              *int `json:"max_replicas"`
	CPUAverageUtilization    *int `json:"cpu_average_utilization"`
	MemoryAverageUtilization *int `json:"memory_average_utilization"`
}

type PortModel struct {
	Index          int             `json:"index"`
	Name           *string         `json:"name"`
	Port           *string         `json:"port"`
	Metrics        bool            `json:"metrics"`
	MetricsPathFor *string         `json:"metrics_path_for"`
	ExposeAs       []ExposeAsModel `json:"expose_as"`
}

type ExposeAsModel struct {
	Index          int             `json:"index"`
	ServiceName    *string         `json:"service_name"`
	Hostname       *string         `json:"hostname"`
	Port           *string         `json:"port"`
	ServiceType    *string         `json:"service_type"`
	IngressEnabled bool            `json:"ingress_enabled"`
	ExternalCount  int             `json:"external_count"`
	Externals      []ExternalModel `json:"externals"`
}

type ExternalModel struct {
	Index         int     `json:"index"`
	Name          *string `json:"name"`
	AsRoute       bool    `json:"as_route"`
	ClassName     *string `json:"class_name"`
	HTTPHostname  *string `json:"http_hostname"`
	HTTPPath      *string `json:"http_path"`
	HTTPSHostname *string `json:"https_hostname"`
	HTTPSPath     *string `json:"https_path"`
	SecretName    *string `json:"secret_name"`
}

func (r *Repository) AppDetail(envName string, appFile string) (AppDetail, error) {
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppDetail{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AppDetail{}, err
	}
	summary, err := summarizeApp(path)
	if err != nil {
		return AppDetail{}, err
	}
	return AppDetail{Env: envName, FileName: filepath.Base(path), Path: path, Content: string(content), ContentHash: contentHash(content), Summary: summary, IsDirty: r.isDirtyPath(filepath.ToSlash(filepath.Join(envName, "apps", filepath.Base(path))))}, nil
}

func (r *Repository) AppRendered(envName string, appFile string) (AppRendered, error) {
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return AppRendered{}, err
	}
	return AppRendered{Env: envName, FileName: detail.FileName, Content: renderVarsPreview(detail.Content)}, nil
}

func (r *Repository) AppVars(envName string, appFile string) (AppVars, error) {
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return AppVars{}, err
	}
	return AppVars{Env: envName, FileName: detail.FileName, ContentHash: detail.ContentHash, Items: extractVars(detail.Content)}, nil
}

func (r *Repository) UpdateAppVars(envName string, appFile string, items []VarItem, expectedHash string) (AppVars, error) {
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppVars{}, err
	}
	if err := validateVarItems(items); err != nil {
		return AppVars{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppVars{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppVars{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated := replaceVarsBlock(string(contentBytes), items)
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppVars{}, err
	}
	return r.AppVars(envName, filepath.Base(path))
}

func (r *Repository) AppReplicas(envName string, appFile string) (AppReplicas, error) {
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return AppReplicas{}, err
	}
	return AppReplicas{Env: envName, FileName: detail.FileName, ContentHash: detail.ContentHash, Replicas: detail.Summary.Replicas}, nil
}

func (r *Repository) UpdateAppReplicas(envName string, appFile string, replicas int, expectedHash string) (AppReplicas, error) {
	if replicas < 0 {
		return AppReplicas{}, errors.New("replicas must be greater than or equal to 0")
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppReplicas{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppReplicas{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppReplicas{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated := replaceTopLevelScalar(string(contentBytes), "replicas", strconv.Itoa(replicas))
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppReplicas{}, err
	}
	return r.AppReplicas(envName, filepath.Base(path))
}

func (r *Repository) UpdateAppAutoscaling(envName string, appFile string, autoscaling AutoscalingUpdate, expectedHash string) (AppAutoscaling, error) {
	if err := validateAutoscalingUpdate(autoscaling); err != nil {
		return AppAutoscaling{}, err
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppAutoscaling{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppAutoscaling{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppAutoscaling{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if err := rejectUnsupportedAutoscalingForUpdate(string(contentBytes)); err != nil {
		return AppAutoscaling{}, err
	}
	updated := replaceAutoscalingBlock(string(contentBytes), autoscaling)
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppAutoscaling{}, err
	}
	model, err := r.AppModel(envName, filepath.Base(path))
	if err != nil {
		return AppAutoscaling{}, err
	}
	detail, err := r.AppDetail(envName, filepath.Base(path))
	if err != nil {
		return AppAutoscaling{}, err
	}
	return AppAutoscaling{Env: envName, FileName: filepath.Base(path), ContentHash: detail.ContentHash, Autoscaling: model.Autoscaling}, nil
}

func (r *Repository) UpdateAppContainerResources(envName string, appFile string, containerIndex int, resources ResourceUpdate, expectedHash string) (AppContainerResources, error) {
	if containerIndex < 0 {
		return AppContainerResources{}, errors.New("container index must be greater than or equal to 0")
	}
	if err := validateResourceUpdate(resources); err != nil {
		return AppContainerResources{}, err
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppContainerResources{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppContainerResources{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppContainerResources{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if err := rejectUnsupportedContainerBlockForUpdate(string(contentBytes), containerIndex, "resources"); err != nil {
		return AppContainerResources{}, err
	}
	updated, err := replaceContainerResourcesBlock(string(contentBytes), containerIndex, resources)
	if err != nil {
		return AppContainerResources{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppContainerResources{}, err
	}
	model, err := r.AppModel(envName, filepath.Base(path))
	if err != nil {
		return AppContainerResources{}, err
	}
	if containerIndex >= len(model.Containers) {
		return AppContainerResources{}, errors.New("container index not found after update")
	}
	detail, err := r.AppDetail(envName, filepath.Base(path))
	if err != nil {
		return AppContainerResources{}, err
	}
	return AppContainerResources{Env: envName, FileName: filepath.Base(path), ContentHash: detail.ContentHash, ContainerIndex: containerIndex, Resources: model.Containers[containerIndex].Resources}, nil
}

func (r *Repository) UpdateAppContainerEnvs(envName string, appFile string, containerIndex int, items []VarItem, expectedHash string) (AppContainerEnvs, error) {
	if containerIndex < 0 {
		return AppContainerEnvs{}, errors.New("container index must be greater than or equal to 0")
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppContainerEnvs{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppContainerEnvs{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppContainerEnvs{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceContainerEnvsBlock(string(contentBytes), containerIndex, items)
	if err != nil {
		return AppContainerEnvs{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppContainerEnvs{}, err
	}
	model, err := r.AppModel(envName, filepath.Base(path))
	if err != nil {
		return AppContainerEnvs{}, err
	}
	if containerIndex >= len(model.Containers) {
		return AppContainerEnvs{}, errors.New("container index not found after update")
	}
	detail, err := r.AppDetail(envName, filepath.Base(path))
	if err != nil {
		return AppContainerEnvs{}, err
	}
	return AppContainerEnvs{Env: envName, FileName: filepath.Base(path), ContentHash: detail.ContentHash, ContainerIndex: containerIndex, Envs: model.Containers[containerIndex].Envs}, nil
}

func (r *Repository) Defaults(envName string) (DefaultsModel, error) {
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsModel{}, err
	}
	detail, err := r.AssetDetail(envName, defaultsFile)
	if err != nil {
		return DefaultsModel{}, err
	}
	return defaultsModelFromDetail(detail)
}

func (r *Repository) UpdateDefaultsVars(envName string, items []VarItem, expectedHash string) (DefaultsModel, error) {
	if err := validateVarItems(items); err != nil {
		return DefaultsModel{}, err
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsModel{}, err
	}
	path, _, err := r.AssetPath(envName, defaultsFile)
	if err != nil {
		return DefaultsModel{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsModel{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return DefaultsModel{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	updated := replaceVarsBlock(string(contentBytes), items)
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return DefaultsModel{}, err
	}
	return r.Defaults(envName)
}

func (r *Repository) UpdateDefaultsContainerEnvs(envName string, groups []ContainerEnvGroupUpdate, expectedHash string) (DefaultsModel, error) {
	if err := validateContainerEnvGroups(groups); err != nil {
		return DefaultsModel{}, err
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsModel{}, err
	}
	path, _, err := r.AssetPath(envName, defaultsFile)
	if err != nil {
		return DefaultsModel{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsModel{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return DefaultsModel{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	if err := rejectUnsupportedDefaultsContainerEnvsForUpdate(string(contentBytes)); err != nil {
		return DefaultsModel{}, err
	}
	updated := replaceDefaultsContainerEnvsBlock(string(contentBytes), groups)
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return DefaultsModel{}, err
	}
	return r.Defaults(envName)
}

func (r *Repository) defaultsFile(envName string) (string, error) {
	for _, candidate := range []string{"_defaults.yml", "_defaults.yaml"} {
		if _, _, err := r.AssetPath(envName, candidate); err == nil {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func (r *Repository) UpdateAppContainerProbes(envName string, appFile string, containerIndex int, probes ProbeUpdate, expectedHash string) (AppContainerProbes, error) {
	if containerIndex < 0 {
		return AppContainerProbes{}, errors.New("container index must be greater than or equal to 0")
	}
	if err := validateProbeUpdate(probes); err != nil {
		return AppContainerProbes{}, err
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppContainerProbes{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppContainerProbes{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppContainerProbes{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if err := rejectUnsupportedContainerBlockForUpdate(string(contentBytes), containerIndex, "probes"); err != nil {
		return AppContainerProbes{}, err
	}
	updated, err := replaceContainerProbesBlock(string(contentBytes), containerIndex, probes)
	if err != nil {
		return AppContainerProbes{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppContainerProbes{}, err
	}
	return r.appContainerProbes(envName, filepath.Base(path), containerIndex)
}

func (r *Repository) FixAppContainerLegacyProbes(envName string, appFile string, containerIndex int, expectedHash string) (AppContainerProbes, error) {
	if containerIndex < 0 {
		return AppContainerProbes{}, errors.New("container index must be greater than or equal to 0")
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppContainerProbes{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppContainerProbes{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppContainerProbes{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated, err := fixContainerLegacyProbesBlock(string(contentBytes), containerIndex)
	if err != nil {
		return AppContainerProbes{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppContainerProbes{}, err
	}
	return r.appContainerProbes(envName, filepath.Base(path), containerIndex)
}

func (r *Repository) appContainerProbes(envName string, appFile string, containerIndex int) (AppContainerProbes, error) {
	model, err := r.AppModel(envName, appFile)
	if err != nil {
		return AppContainerProbes{}, err
	}
	if containerIndex >= len(model.Containers) {
		return AppContainerProbes{}, errors.New("container index not found after update")
	}
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return AppContainerProbes{}, err
	}
	return AppContainerProbes{Env: envName, FileName: detail.FileName, ContentHash: detail.ContentHash, ContainerIndex: containerIndex, Probes: model.Containers[containerIndex].Probes}, nil
}

func (r *Repository) UpdateAppContainerRuntime(envName string, appFile string, containerIndex int, runtime JavaRuntimeUpdate, expectedHash string) (AppContainerRuntime, error) {
	if containerIndex < 0 {
		return AppContainerRuntime{}, errors.New("container index must be greater than or equal to 0")
	}
	if err := validateJavaRuntimeUpdate(runtime); err != nil {
		return AppContainerRuntime{}, err
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppContainerRuntime{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppContainerRuntime{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppContainerRuntime{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if err := rejectUnsupportedContainerBlockForUpdate(string(contentBytes), containerIndex, "runtime"); err != nil {
		return AppContainerRuntime{}, err
	}
	updated, err := replaceContainerRuntimeBlock(string(contentBytes), containerIndex, runtime)
	if err != nil {
		return AppContainerRuntime{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppContainerRuntime{}, err
	}
	model, err := r.AppModel(envName, filepath.Base(path))
	if err != nil {
		return AppContainerRuntime{}, err
	}
	if containerIndex >= len(model.Containers) {
		return AppContainerRuntime{}, errors.New("container index not found after update")
	}
	detail, err := r.AppDetail(envName, filepath.Base(path))
	if err != nil {
		return AppContainerRuntime{}, err
	}
	return AppContainerRuntime{Env: envName, FileName: filepath.Base(path), ContentHash: detail.ContentHash, ContainerIndex: containerIndex, Runtime: model.Containers[containerIndex].Runtime}, nil
}

func (r *Repository) UpdateAppContainerPorts(envName string, appFile string, containerIndex int, ports []PortUpdate, expectedHash string) (AppContainerPorts, error) {
	if containerIndex < 0 {
		return AppContainerPorts{}, errors.New("container index must be greater than or equal to 0")
	}
	if err := validatePortUpdates(ports); err != nil {
		return AppContainerPorts{}, err
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppContainerPorts{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppContainerPorts{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return AppContainerPorts{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if err := rejectUnsupportedPortsForUpdate(string(contentBytes), containerIndex); err != nil {
		return AppContainerPorts{}, err
	}
	updated, err := replaceContainerPortsBlock(string(contentBytes), containerIndex, ports)
	if err != nil {
		return AppContainerPorts{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppContainerPorts{}, err
	}
	model, err := r.AppModel(envName, filepath.Base(path))
	if err != nil {
		return AppContainerPorts{}, err
	}
	if containerIndex >= len(model.Containers) {
		return AppContainerPorts{}, errors.New("container index not found after update")
	}
	detail, err := r.AppDetail(envName, filepath.Base(path))
	if err != nil {
		return AppContainerPorts{}, err
	}
	return AppContainerPorts{Env: envName, FileName: filepath.Base(path), ContentHash: detail.ContentHash, ContainerIndex: containerIndex, Ports: model.Containers[containerIndex].Ports}, nil
}

func (r *Repository) AppModel(envName string, appFile string) (AppModel, error) {
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return AppModel{}, err
	}
	var root any
	if err := yaml.Unmarshal([]byte(renderVarsPreview(detail.Content)), &root); err != nil {
		return AppModel{}, err
	}
	rootMap, _ := root.(map[string]any)
	model := AppModel{Env: envName, FileName: detail.FileName}
	model.AppName = stringPtr(stringValue(rootMap["name"]))
	model.SidecarRefNames = stringSlice(rootMap["sidecar_ref_names"])
	model.RuntimeAssetRefNames = stringSlice(rootMap["runtime_asset_ref_names"])
	model.Replicas = intPtr(intValue(rootMap["replicas"]))
	model.Kind = stringPtr(firstNonEmpty(stringValue(rootMap["kind"]), "Deployment"))
	model.InitContainersCount = len(anySlice(rootMap["init_containers"]))
	model.Autoscaling = autoscalingModel(nestedMap(rootMap, "autoscaling"))

	for cIdx, rawContainer := range anySlice(rootMap["containers"]) {
		containerMap, _ := rawContainer.(map[string]any)
		container := ContainerModel{
			Index:                cIdx,
			Name:                 stringPtr(stringValue(containerMap["name"])),
			ProfileRefNames:      stringSlice(containerMap["profile_ref_names"]),
			RuntimeAssetRefNames: stringSlice(containerMap["runtime_asset_ref_names"]),
			StartupCommand:       stringSlice(nestedValue(containerMap, "startup", "command")),
			StartupArguments:     stringSlice(nestedValue(containerMap, "startup", "arguments")),
			Resources: ResourceModel{
				CPURequest:              stringPtr(firstNonEmpty(nestedString(containerMap, "resources", "cpu", "requests"), nestedString(containerMap, "resources", "cpu", "from"))),
				CPULimit:                stringPtr(firstNonEmpty(nestedString(containerMap, "resources", "cpu", "limits"), nestedString(containerMap, "resources", "cpu", "to"))),
				MemoryRequest:           stringPtr(firstNonEmpty(nestedString(containerMap, "resources", "memory", "requests"), nestedString(containerMap, "resources", "memory", "from"))),
				MemoryLimit:             stringPtr(firstNonEmpty(nestedString(containerMap, "resources", "memory", "limits"), nestedString(containerMap, "resources", "memory", "to"))),
				CPUFrom:                 stringPtr(nestedString(containerMap, "resources", "cpu", "from")),
				CPUTo:                   stringPtr(nestedString(containerMap, "resources", "cpu", "to")),
				MemoryFrom:              stringPtr(nestedString(containerMap, "resources", "memory", "from")),
				MemoryTo:                stringPtr(nestedString(containerMap, "resources", "memory", "to")),
				EphemeralStorageRequest: stringPtr(firstNonEmpty(nestedString(containerMap, "resources", "ephemeral-storage", "requests"), nestedString(containerMap, "resources", "ephemeral-storage", "from"))),
				EphemeralStorageLimit:   stringPtr(firstNonEmpty(nestedString(containerMap, "resources", "ephemeral-storage", "limits"), nestedString(containerMap, "resources", "ephemeral-storage", "to"))),
				EphemeralStorageFrom:    stringPtr(nestedString(containerMap, "resources", "ephemeral-storage", "from")),
				EphemeralStorageTo:      stringPtr(nestedString(containerMap, "resources", "ephemeral-storage", "to")),
			},
			Runtime:      runtimeModel(nestedMap(containerMap, "runtime")),
			Probes:       probesModel(containerMap),
			EnvFromCount: len(anySlice(containerMap["env_from"])),
			MountsCount:  len(anySlice(containerMap["mounts"])),
		}
		for idx, rawEnv := range anySlice(containerMap["envs"]) {
			varMap, _ := rawEnv.(map[string]any)
			container.Envs = append(container.Envs, envVarModel(idx, varMap))
		}
		for pIdx, rawPort := range anySlice(containerMap["ports"]) {
			portMap, _ := rawPort.(map[string]any)
			port := PortModel{
				Index:          pIdx,
				Name:           stringPtr(stringValue(portMap["name"])),
				Port:           stringPtr(stringValue(portMap["port"])),
				Metrics:        boolValue(portMap["metrics"]),
				MetricsPathFor: stringPtr(stringValue(portMap["metricsPathFor"])),
			}
			for eIdx, rawExpose := range anySlice(portMap["expose_as"]) {
				exposeMap, _ := rawExpose.(map[string]any)
				serviceName := stringValue(exposeMap["service_name"])
				legacyHostname := stringValue(exposeMap["hostname"])
				if serviceName == "" {
					serviceName = legacyHostname
				}
				expose := ExposeAsModel{
					Index:       eIdx,
					ServiceName: stringPtr(serviceName),
					Hostname:    stringPtr(legacyHostname),
					Port:        stringPtr(stringValue(exposeMap["port"])),
					ServiceType: stringPtr(stringValue(exposeMap["type"])),
				}
				for xIdx, rawExternal := range anySlice(exposeMap["external"]) {
					externalMap, _ := rawExternal.(map[string]any)
					external := ExternalModel{Index: xIdx, Name: stringPtr(stringValue(externalMap["name"])), AsRoute: boolValue(externalMap["as_route"]), ClassName: stringPtr(stringValue(externalMap["class_name"]))}
					if httpItems := anySlice(externalMap["http"]); len(httpItems) > 0 {
						httpMap, _ := httpItems[0].(map[string]any)
						external.HTTPHostname = stringPtr(stringValue(httpMap["hostname"]))
						external.HTTPPath = stringPtr(stringValue(httpMap["path"]))
					}
					if httpsItems := anySlice(externalMap["https"]); len(httpsItems) > 0 {
						httpsMap, _ := httpsItems[0].(map[string]any)
						external.HTTPSHostname = stringPtr(stringValue(httpsMap["hostname"]))
						external.HTTPSPath = stringPtr(stringValue(httpsMap["path"]))
						external.SecretName = stringPtr(stringValue(httpsMap["secret_name"]))
					}
					expose.Externals = append(expose.Externals, external)
				}
				expose.ExternalCount = len(expose.Externals)
				expose.IngressEnabled = expose.ExternalCount > 0
				port.ExposeAs = append(port.ExposeAs, expose)
			}
			container.Ports = append(container.Ports, port)
		}
		model.Containers = append(model.Containers, container)
	}
	for cIdx, rawContainer := range anySlice(rootMap["sidecars"]) {
		containerMap, _ := rawContainer.(map[string]any)
		model.Sidecars = append(model.Sidecars, ContainerModel{
			Index:                cIdx,
			Name:                 stringPtr(stringValue(containerMap["name"])),
			ProfileRefNames:      stringSlice(containerMap["profile_ref_names"]),
			RuntimeAssetRefNames: stringSlice(containerMap["runtime_asset_ref_names"]),
		})
	}
	return model, nil
}

func (r *Repository) AppReferences(envName string, appFile string) (AppReferences, error) {
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return AppReferences{}, err
	}
	model, err := r.AppModel(envName, appFile)
	if err != nil {
		return AppReferences{}, err
	}
	defaults, err := r.Defaults(envName)
	if err != nil {
		return AppReferences{}, err
	}
	defaultsDetail, err := r.AssetDetail(envName, defaults.FileName)
	if err != nil {
		return AppReferences{}, err
	}
	defaultsRoot, err := sourceModelRoot(defaultsDetail.Content)
	if err != nil {
		return AppReferences{}, fmt.Errorf("parse defaults references: %w", err)
	}
	out := AppReferences{
		Env:                  envName,
		FileName:             detail.FileName,
		ContentHash:          detail.ContentHash,
		DefaultsHash:         defaultsDetail.ContentHash,
		SidecarRefNames:      model.SidecarRefNames,
		RuntimeAssetRefNames: model.RuntimeAssetRefNames,
		Catalog: ReferenceCatalog{
			ContainerProfiles:       namedDefinitionNames(defaultsRoot["container_profiles"]),
			SidecarDefinitions:      namedDefinitionNames(defaultsRoot["sidecar_definitions"]),
			RuntimeAssetDefinitions: namedDefinitionNames(defaultsRoot["runtime_asset_definitions"]),
		},
	}
	for _, container := range model.Containers {
		out.Containers = append(out.Containers, containerReferences(container))
	}
	for _, sidecar := range model.Sidecars {
		out.Sidecars = append(out.Sidecars, containerReferences(sidecar))
	}
	return out, nil
}

func (r *Repository) UpdateAppReferences(envName string, appFile string, update AppReferencesUpdate, expectedHash string, expectedDefaultsHash string) (AppReferences, error) {
	current, err := r.AppReferences(envName, appFile)
	if err != nil {
		return AppReferences{}, err
	}
	if expectedHash != "" && expectedHash != current.ContentHash {
		return AppReferences{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if expectedDefaultsHash != "" && expectedDefaultsHash != current.DefaultsHash {
		return AppReferences{}, NewConflictError("defaults file changed before save; refresh reference catalogs and apply the edit again")
	}
	if err := validateAppReferencesUpdate(current, update); err != nil {
		return AppReferences{}, err
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return AppReferences{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return AppReferences{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return AppReferences{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated := replaceTopLevelStringList(string(contentBytes), "runtime_asset_ref_names", update.RuntimeAssetRefNames)
	updated = replaceTopLevelStringList(updated, "sidecar_ref_names", update.SidecarRefNames)
	updated, err = replaceCollectionReferenceLists(updated, "containers", update.Containers)
	if err != nil {
		return AppReferences{}, err
	}
	updated, err = replaceCollectionReferenceLists(updated, "sidecars", update.Sidecars)
	if err != nil {
		return AppReferences{}, err
	}
	updated, err = removeNamedCollectionItems(updated, "sidecars", update.RemoveSidecarPatches)
	if err != nil {
		return AppReferences{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return AppReferences{}, err
	}
	return r.AppReferences(envName, filepath.Base(path))
}

func containerReferences(container ContainerModel) ContainerReferences {
	name := ""
	if container.Name != nil {
		name = *container.Name
	}
	return ContainerReferences{
		Index:                container.Index,
		Name:                 name,
		ProfileRefNames:      append([]string(nil), container.ProfileRefNames...),
		RuntimeAssetRefNames: append([]string(nil), container.RuntimeAssetRefNames...),
	}
}

func namedDefinitionNames(value any) []string {
	names := []string{}
	for _, raw := range anySlice(value) {
		item, _ := raw.(map[string]any)
		if name := strings.TrimSpace(stringValue(item["name"])); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func validateAppReferencesUpdate(current AppReferences, update AppReferencesUpdate) error {
	if err := validateReferenceNames("sidecar_ref_names", update.SidecarRefNames, current.Catalog.SidecarDefinitions); err != nil {
		return err
	}
	if err := validateReferenceNames("runtime_asset_ref_names", update.RuntimeAssetRefNames, current.Catalog.RuntimeAssetDefinitions); err != nil {
		return err
	}
	if err := validateReferenceScopes("containers", current.Containers, update.Containers, current.Catalog); err != nil {
		return err
	}
	if err := validateReferenceScopes("sidecars", current.Sidecars, update.Sidecars, current.Catalog); err != nil {
		return err
	}
	selectedSidecars := stringSet(update.SidecarRefNames)
	sharedSidecars := stringSet(current.Catalog.SidecarDefinitions)
	removePatches := stringSet(update.RemoveSidecarPatches)
	if len(removePatches) != len(update.RemoveSidecarPatches) {
		return errors.New("remove_sidecar_patches contains duplicate names")
	}
	currentRefs := stringSet(current.SidecarRefNames)
	localSidecars := map[string]bool{}
	for _, sidecar := range current.Sidecars {
		localSidecars[sidecar.Name] = true
	}
	for name := range removePatches {
		if !referenceNamePattern.MatchString(name) || !sharedSidecars[name] || !currentRefs[name] || selectedSidecars[name] || !localSidecars[name] {
			return fmt.Errorf("remove_sidecar_patches contains invalid removal %q", name)
		}
	}
	for name := range currentRefs {
		if !selectedSidecars[name] && localSidecars[name] && !removePatches[name] {
			return fmt.Errorf("removing sidecar reference %q also requires explicit removal of its local patch", name)
		}
	}
	for _, sidecar := range update.Sidecars {
		if sharedSidecars[sidecar.Name] && !selectedSidecars[sidecar.Name] && !removePatches[sidecar.Name] {
			return fmt.Errorf("sidecars %q matches a shared definition but is not selected in sidecar_ref_names", sidecar.Name)
		}
	}
	return nil
}

func removeNamedCollectionItems(content, collection string, names []string) (string, error) {
	if len(names) == 0 {
		return content, nil
	}
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	remove := stringSet(names)
	indices := []int{}
	for index, raw := range anySlice(root[collection]) {
		item, _ := raw.(map[string]any)
		if remove[strings.TrimSpace(stringValue(item["name"]))] {
			indices = append(indices, index)
		}
	}
	updated := content
	for index := len(indices) - 1; index >= 0; index-- {
		lines, start, end, err := collectionItemBlockRange(updated, collection, indices[index])
		if err != nil {
			return "", err
		}
		updated = joinLikeSource(append(lines[:start], lines[end:]...), updated)
	}
	updatedRoot, err := sourceModelRoot(updated)
	if err != nil {
		return "", err
	}
	if len(anySlice(updatedRoot[collection])) == 0 {
		updated = replaceTopLevelBlock(updated, collection, nil)
	}
	return updated, nil
}

func validateReferenceScopes(label string, current []ContainerReferences, updates []ContainerReferences, catalog ReferenceCatalog) error {
	if len(current) != len(updates) {
		return fmt.Errorf("%s changed before save; refresh and apply the edit again", label)
	}
	for index, update := range updates {
		if update.Index != current[index].Index || update.Name != current[index].Name {
			return NewConflictError(fmt.Sprintf("%s scope changed before save; refresh and apply the edit again", label))
		}
		if err := validateReferenceNames(fmt.Sprintf("%s[%d].profile_ref_names", label, index), update.ProfileRefNames, catalog.ContainerProfiles); err != nil {
			return err
		}
		if err := validateReferenceNames(fmt.Sprintf("%s[%d].runtime_asset_ref_names", label, index), update.RuntimeAssetRefNames, catalog.RuntimeAssetDefinitions); err != nil {
			return err
		}
	}
	return nil
}

func validateReferenceNames(label string, values []string, catalog []string) error {
	known := stringSet(catalog)
	seen := map[string]bool{}
	for _, value := range values {
		if !referenceNamePattern.MatchString(value) {
			return fmt.Errorf("%s contains invalid reference %q", label, value)
		}
		if seen[value] {
			return fmt.Errorf("%s contains duplicate reference %q", label, value)
		}
		if !known[value] {
			return fmt.Errorf("%s references unknown definition %q", label, value)
		}
		seen[value] = true
	}
	return nil
}

var referenceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func (r *Repository) AssetDetail(envName string, relativePath string) (AssetDetail, error) {
	path, rel, err := r.AssetPath(envName, relativePath)
	if err != nil {
		return AssetDetail{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AssetDetail{}, err
	}
	dirtyPath := filepath.ToSlash(filepath.Join(envName, "assets", rel))
	if isSpecialRootAsset(rel) {
		dirtyPath = filepath.ToSlash(filepath.Join(envName, rel))
	}
	if isDefaultsAsset(rel) {
		dirtyPath = filepath.ToSlash(filepath.Join(envName, "apps", rel))
	}
	return AssetDetail{Env: envName, RelativePath: rel, Path: path, Content: string(content), ContentHash: contentHash(content), IsDirty: r.isDirtyPath(dirtyPath)}, nil
}

func (r *Repository) SpecialEntries(envName string, specialFile string) (SpecialEntries, error) {
	kind, ok := specialFileKind(specialFile)
	if !ok {
		return SpecialEntries{}, errors.New("unsupported special file")
	}
	detail, err := r.AssetDetail(envName, specialFile)
	if err != nil {
		return SpecialEntries{}, err
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(detail.Content), &root); err != nil {
		return SpecialEntries{}, err
	}
	out := SpecialEntries{Env: envName, SpecialFile: specialFile, ContentHash: detail.ContentHash, Editable: !kind.secured, IsDirty: detail.IsDirty}
	if kind.secured {
		warning := "secured file is shown without decrypt; edit flow requires EncJson preflight"
		out.Warning = &warning
	}
	if kind.rootKey == "environment" {
		entries, err := orderedEnvironmentSpecialEntries(detail.Content, kind.rootKey)
		if err != nil {
			return SpecialEntries{}, err
		}
		out.Entries = entries
		return out, nil
	}
	rawSection, ok := root[kind.rootKey].(map[string]any)
	if !ok {
		return SpecialEntries{}, fmt.Errorf("missing object at .%s", kind.rootKey)
	}
	keys := make([]string, 0, len(rawSection))
	for key := range rawSection {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out.Entries = append(out.Entries, specialEntryFromValue(key, rawSection[key], kind.rootKey))
	}
	return out, nil
}

func (r *Repository) UpdateSpecialEntries(envName string, specialFile string, entries []SpecialEntry, expectedHash string) (SpecialEntries, error) {
	kind, ok := specialFileKind(specialFile)
	if !ok {
		return SpecialEntries{}, errors.New("unsupported special file")
	}
	if kind.secured {
		return SpecialEntries{}, errors.New("secured special files cannot be edited without EncJson flow")
	}
	path, _, err := r.AssetPath(envName, specialFile)
	if err != nil {
		return SpecialEntries{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return SpecialEntries{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return SpecialEntries{}, NewConflictError("special file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceSpecialEntries(string(contentBytes), kind.rootKey, entries)
	if err != nil {
		return SpecialEntries{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return SpecialEntries{}, err
	}
	return r.SpecialEntries(envName, specialFile)
}

func (r *Repository) UpdateSecuredSpecialEntriesEncrypted(envName string, specialFile string, entries []SpecialEntry, expectedHash string) (SpecialEntries, error) {
	kind, ok := specialFileKind(specialFile)
	if !ok {
		return SpecialEntries{}, errors.New("unsupported special file")
	}
	if !kind.secured {
		return SpecialEntries{}, errors.New("special file is not secured")
	}
	path, _, err := r.AssetPath(envName, specialFile)
	if err != nil {
		return SpecialEntries{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return SpecialEntries{}, err
	}
	if expectedHash != "" && expectedHash != contentHash(contentBytes) {
		return SpecialEntries{}, NewConflictError("special file changed before save; refresh and apply the edit again")
	}
	updated, err := PatchSpecialEntriesContent(string(contentBytes), kind.rootKey, entries)
	if err != nil {
		return SpecialEntries{}, err
	}
	if err := atomicWriteFile(path, []byte(updated)); err != nil {
		return SpecialEntries{}, err
	}
	return r.SpecialEntries(envName, specialFile)
}

func (r *Repository) AppPath(envName string, appFile string) (string, error) {
	envDir, err := r.envDir(envName)
	if err != nil {
		return "", err
	}
	if err := validateFileName(appFile); err != nil {
		return "", err
	}
	appsDir := filepath.Join(envDir, "apps")
	candidates := []string{filepath.Join(appsDir, appFile)}
	if filepath.Ext(appFile) == "" {
		candidates = append(candidates, filepath.Join(appsDir, appFile+".yml"), filepath.Join(appsDir, appFile+".yaml"))
	}
	for _, candidate := range candidates {
		if !strings.HasPrefix(candidate, appsDir+string(os.PathSeparator)) {
			return "", errors.New("invalid app path")
		}
		if isFile(candidate) {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func (r *Repository) AssetPath(envName string, relativePath string) (string, string, error) {
	envDir, err := r.envDir(envName)
	if err != nil {
		return "", "", err
	}
	rel, err := validateRelativePath(relativePath)
	if err != nil {
		return "", "", err
	}
	if isSpecialRootAsset(rel) {
		path := filepath.Join(envDir, filepath.FromSlash(rel))
		if !isChildPath(envDir, path) {
			return "", "", errors.New("invalid asset path")
		}
		if !isFile(path) {
			return "", "", os.ErrNotExist
		}
		return path, rel, nil
	}
	if isDefaultsAsset(rel) {
		path := filepath.Join(envDir, "apps", rel)
		appsDir := filepath.Join(envDir, "apps")
		if !isChildPath(appsDir, path) {
			return "", "", errors.New("invalid defaults path")
		}
		if !isFile(path) {
			return "", "", os.ErrNotExist
		}
		return path, rel, nil
	}
	assetsDir := filepath.Join(envDir, "assets")
	path := filepath.Join(assetsDir, filepath.FromSlash(rel))
	if !isChildPath(assetsDir, path) {
		return "", "", errors.New("invalid asset path")
	}
	if !isFile(path) {
		return "", "", os.ErrNotExist
	}
	return path, rel, nil
}

func validateFileName(value string) error {
	if strings.TrimSpace(value) == "" || value == "." || value == ".." || strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return errors.New("invalid file name")
	}
	if strings.HasPrefix(value, "_") || !hasAnyExtension(value, ".yml", ".yaml") && filepath.Ext(value) != "" {
		return errors.New("invalid app file name")
	}
	return nil
}

func validateRelativePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(value, "/")
	if value == "" || value == "." || strings.Contains(value, "//") {
		return "", errors.New("invalid relative path")
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(clean) {
		return "", errors.New("invalid relative path")
	}
	return clean, nil
}

func isSpecialRootAsset(value string) bool {
	switch value {
	case "env.secured.json", "env.unsecured.json", "assets.secured.json", "assets.unsecured.json", "shared.assets.yml", "replica-profiles.yml":
		return true
	default:
		return false
	}
}

func isDefaultsAsset(value string) bool {
	return value == "_defaults.yml" || value == "_defaults.yaml"
}

type specialFileInfo struct {
	rootKey string
	secured bool
}

func specialFileKind(name string) (specialFileInfo, bool) {
	switch name {
	case "env.unsecured.json":
		return specialFileInfo{rootKey: "environment"}, true
	case "env.secured.json":
		return specialFileInfo{rootKey: "environment", secured: true}, true
	case "assets.unsecured.json":
		return specialFileInfo{rootKey: "assets"}, true
	case "assets.secured.json":
		return specialFileInfo{rootKey: "assets", secured: true}, true
	default:
		return specialFileInfo{}, false
	}
}

func specialEntryFromValue(key string, value any, rootKey string) SpecialEntry {
	if rootKey == "assets" {
		if object, ok := value.(map[string]any); ok {
			return SpecialEntry{
				Key:       key,
				ValueType: firstNonEmpty(stringValue(object["kind"]), "unknown"),
				ValueText: stringValue(object["content"]),
			}
		}
		return SpecialEntry{Key: key, ValueType: "legacy", ValueText: stringValue(value)}
	}
	switch value.(type) {
	case string:
		return SpecialEntry{Key: key, ValueType: "string", ValueText: stringValue(value)}
	case bool:
		return SpecialEntry{Key: key, ValueType: "bool", ValueText: stringValue(value)}
	case float64, int, int64:
		return SpecialEntry{Key: key, ValueType: "number", ValueText: stringValue(value)}
	case nil:
		return SpecialEntry{Key: key, ValueType: "null", ValueText: ""}
	default:
		raw, _ := json.Marshal(value)
		return SpecialEntry{Key: key, ValueType: "json", ValueText: string(raw)}
	}
}

func specialEntryValue(entry SpecialEntry) (any, error) {
	key := strings.TrimSpace(entry.Key)
	if key == "" {
		return nil, errors.New("entry key is required")
	}
	switch strings.TrimSpace(entry.ValueType) {
	case "", "string":
		return entry.ValueText, nil
	case "number":
		if strings.TrimSpace(entry.ValueText) == "" {
			return nil, fmt.Errorf("entry %q number value is required", key)
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(entry.ValueText), 64)
		if err != nil {
			return nil, fmt.Errorf("entry %q number value is invalid", key)
		}
		return value, nil
	case "bool":
		value, err := strconv.ParseBool(strings.TrimSpace(entry.ValueText))
		if err != nil {
			return nil, fmt.Errorf("entry %q bool value is invalid", key)
		}
		return value, nil
	case "null":
		return nil, nil
	case "json":
		var value any
		if err := json.Unmarshal([]byte(entry.ValueText), &value); err != nil {
			return nil, fmt.Errorf("entry %q json value is invalid: %w", key, err)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("entry %q has unsupported value type %q", key, entry.ValueType)
	}
}

func replaceSpecialEntries(content string, rootKey string, entries []SpecialEntry) (string, error) {
	return PatchSpecialEntriesContent(content, rootKey, entries)
}

func PatchSpecialEntriesContent(content string, rootKey string, entries []SpecialEntry) (string, error) {
	if rootKey == "environment" {
		return patchEnvironmentSpecialEntries(content, rootKey, entries)
	}
	seen := map[string]bool{}
	section := map[string]any{}
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			return "", errors.New("entry key is required")
		}
		if strings.ContainsAny(key, "\r\n") {
			return "", fmt.Errorf("invalid entry key %q", key)
		}
		if seen[key] {
			return "", fmt.Errorf("duplicate entry key %q", key)
		}
		seen[key] = true
		value, err := specialEntryValue(entry)
		if err != nil {
			return "", err
		}
		section[key] = value
	}

	root := map[string]any{}
	if strings.TrimSpace(content) != "" {
		if err := json.Unmarshal([]byte(content), &root); err != nil {
			return "", err
		}
	}
	root[rootKey] = section
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

type jsonObjectMember struct {
	key         string
	memberStart int
	keyStart    int
	valueStart  int
	valueEnd    int
	memberEnd   int
	commaPos    int
	hasComma    bool
}

type encodedSpecialEntry struct {
	key       string
	valueJSON string
}

func orderedEnvironmentSpecialEntries(content string, rootKey string) ([]SpecialEntry, error) {
	rootMember, err := findJSONObjectMember(content, 0, rootKey)
	if err != nil {
		return nil, err
	}
	members, _, err := parseJSONObjectMembers(content, rootMember.valueStart)
	if err != nil {
		return nil, err
	}
	entries := make([]SpecialEntry, 0, len(members))
	for _, member := range members {
		var value any
		if err := json.Unmarshal([]byte(content[member.valueStart:member.valueEnd]), &value); err != nil {
			return nil, fmt.Errorf("invalid value for entry %q: %w", member.key, err)
		}
		entries = append(entries, specialEntryFromValue(member.key, value, rootKey))
	}
	return entries, nil
}

func patchEnvironmentSpecialEntries(content string, rootKey string, entries []SpecialEntry) (string, error) {
	encoded, err := encodeSpecialEntries(entries)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("missing JSON content")
	}
	rootMember, err := findJSONObjectMember(content, 0, rootKey)
	if err != nil {
		return "", err
	}
	members, objectEnd, err := parseJSONObjectMembers(content, rootMember.valueStart)
	if err != nil {
		return "", err
	}
	existing := map[string]jsonObjectMember{}
	for _, member := range members {
		existing[member.key] = member
	}
	seen := map[string]bool{}
	var edits []textEdit
	for _, entry := range encoded {
		seen[entry.key] = true
		member, ok := existing[entry.key]
		if !ok {
			continue
		}
		if content[member.valueStart:member.valueEnd] != entry.valueJSON {
			edits = append(edits, textEdit{start: member.valueStart, end: member.valueEnd, replacement: entry.valueJSON})
		}
	}
	for idx, member := range members {
		if !seen[member.key] {
			start, end := memberRemovalRange(members, idx)
			edits = append(edits, textEdit{start: start, end: end})
		}
	}
	edits = append(edits, environmentAdditionEdits(content, rootMember.valueStart, objectEnd, members, existing, encoded)...)
	return applyTextEdits(content, edits), nil
}

func encodeSpecialEntries(entries []SpecialEntry) ([]encodedSpecialEntry, error) {
	seen := map[string]bool{}
	encoded := make([]encodedSpecialEntry, 0, len(entries))
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			return nil, errors.New("entry key is required")
		}
		if !variableNamePattern.MatchString(key) {
			return nil, fmt.Errorf("invalid entry key %q", key)
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate entry key %q", key)
		}
		seen[key] = true
		value, err := specialEntryValue(entry)
		if err != nil {
			return nil, err
		}
		valueBytes, err := marshalJSONValue(value)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, encodedSpecialEntry{key: key, valueJSON: string(valueBytes)})
	}
	return encoded, nil
}

func marshalJSONValue(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func findJSONObjectMember(content string, objectStart int, key string) (jsonObjectMember, error) {
	members, _, err := parseJSONObjectMembers(content, objectStart)
	if err != nil {
		return jsonObjectMember{}, err
	}
	for _, member := range members {
		if member.key == key {
			if member.valueStart >= len(content) || content[member.valueStart] != '{' {
				return jsonObjectMember{}, fmt.Errorf("missing object at .%s", key)
			}
			return member, nil
		}
	}
	return jsonObjectMember{}, fmt.Errorf("missing object at .%s", key)
}

func parseJSONObjectMembers(content string, objectStart int) ([]jsonObjectMember, int, error) {
	pos := skipJSONWhitespace(content, objectStart)
	if pos >= len(content) || content[pos] != '{' {
		return nil, 0, errors.New("expected JSON object")
	}
	pos++
	var members []jsonObjectMember
	for {
		memberStart := pos
		pos = skipJSONWhitespace(content, pos)
		if pos >= len(content) {
			return nil, 0, errors.New("unterminated JSON object")
		}
		if content[pos] == '}' {
			return members, pos, nil
		}
		memberStart = min(memberStart, pos)
		if content[pos] != '"' {
			return nil, 0, errors.New("expected JSON object key")
		}
		keyStart := pos
		keyEnd, err := scanJSONStringEnd(content, keyStart)
		if err != nil {
			return nil, 0, err
		}
		var key string
		if err := json.Unmarshal([]byte(content[keyStart:keyEnd]), &key); err != nil {
			return nil, 0, err
		}
		pos = skipJSONWhitespace(content, keyEnd)
		if pos >= len(content) || content[pos] != ':' {
			return nil, 0, fmt.Errorf("expected ':' after JSON object key %q", key)
		}
		pos = skipJSONWhitespace(content, pos+1)
		valueStart := pos
		valueEnd, err := scanJSONValueEnd(content, valueStart)
		if err != nil {
			return nil, 0, err
		}
		pos = skipJSONWhitespace(content, valueEnd)
		member := jsonObjectMember{key: key, memberStart: memberStart, keyStart: keyStart, valueStart: valueStart, valueEnd: valueEnd, memberEnd: pos}
		if pos < len(content) && content[pos] == ',' {
			member.hasComma = true
			member.commaPos = pos
			member.memberEnd = pos + 1
			pos++
		}
		members = append(members, member)
	}
}

func scanJSONStringEnd(content string, start int) (int, error) {
	if start >= len(content) || content[start] != '"' {
		return 0, errors.New("expected JSON string")
	}
	for pos := start + 1; pos < len(content); pos++ {
		switch content[pos] {
		case '\\':
			pos++
		case '"':
			return pos + 1, nil
		}
	}
	return 0, errors.New("unterminated JSON string")
}

func scanJSONValueEnd(content string, start int) (int, error) {
	if start >= len(content) {
		return 0, errors.New("expected JSON value")
	}
	switch content[start] {
	case '"':
		return scanJSONStringEnd(content, start)
	case '{', '[':
		open := content[start]
		close := byte('}')
		if open == '[' {
			close = ']'
		}
		depth := 1
		for pos := start + 1; pos < len(content); pos++ {
			switch content[pos] {
			case '"':
				end, err := scanJSONStringEnd(content, pos)
				if err != nil {
					return 0, err
				}
				pos = end - 1
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return pos + 1, nil
				}
			case '{':
				if open == '[' {
					end, err := scanNestedJSON(content, pos, '{', '}')
					if err != nil {
						return 0, err
					}
					pos = end - 1
				}
			case '[':
				if open == '{' {
					end, err := scanNestedJSON(content, pos, '[', ']')
					if err != nil {
						return 0, err
					}
					pos = end - 1
				}
			}
		}
		return 0, errors.New("unterminated JSON value")
	default:
		pos := start
		for pos < len(content) && !strings.ContainsRune(",}]\r\n\t ", rune(content[pos])) {
			pos++
		}
		if pos == start {
			return 0, errors.New("expected JSON value")
		}
		return pos, nil
	}
}

func scanNestedJSON(content string, start int, open byte, close byte) (int, error) {
	depth := 1
	for pos := start + 1; pos < len(content); pos++ {
		switch content[pos] {
		case '"':
			end, err := scanJSONStringEnd(content, pos)
			if err != nil {
				return 0, err
			}
			pos = end - 1
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return pos + 1, nil
			}
		}
	}
	return 0, errors.New("unterminated JSON value")
}

func skipJSONWhitespace(content string, pos int) int {
	for pos < len(content) {
		switch content[pos] {
		case ' ', '\n', '\r', '\t':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func memberRemovalRange(members []jsonObjectMember, idx int) (int, int) {
	member := members[idx]
	if member.hasComma {
		return member.memberStart, member.commaPos + 1
	}
	if idx > 0 && members[idx-1].hasComma {
		return members[idx-1].commaPos, member.valueEnd
	}
	return member.memberStart, member.valueEnd
}

func environmentAdditionEdits(content string, objectStart int, objectEnd int, members []jsonObjectMember, existing map[string]jsonObjectMember, entries []encodedSpecialEntry) []textEdit {
	retainedCount := 0
	for _, entry := range entries {
		if _, ok := existing[entry.key]; ok {
			retainedCount++
		}
	}
	var edits []textEdit
	for idx := 0; idx < len(entries); {
		if _, ok := existing[entries[idx].key]; ok {
			idx++
			continue
		}
		startIdx := idx
		for idx < len(entries) {
			if _, ok := existing[entries[idx].key]; ok {
				break
			}
			idx++
		}
		run := entries[startIdx:idx]
		next, hasNext := nextExistingMember(entries, existing, idx)
		if hasNext {
			edits = append(edits, textEdit{start: next.keyStart, end: next.keyStart, replacement: environmentInsertionBefore(content, next, run)})
			continue
		}
		insertionPoint := environmentInsertionPoint(content, objectStart, objectEnd)
		edits = append(edits, textEdit{start: insertionPoint, end: insertionPoint, replacement: environmentInsertionAppend(content, objectStart, objectEnd, members, run, retainedCount)})
	}
	return edits
}

func nextExistingMember(entries []encodedSpecialEntry, existing map[string]jsonObjectMember, start int) (jsonObjectMember, bool) {
	for idx := start; idx < len(entries); idx++ {
		if member, ok := existing[entries[idx].key]; ok {
			return member, true
		}
	}
	return jsonObjectMember{}, false
}

func environmentInsertionBefore(content string, next jsonObjectMember, additions []encodedSpecialEntry) string {
	memberIndent, _ := environmentIndents(content, 0, next.valueEnd, []jsonObjectMember{next})
	lines := make([]string, 0, len(additions))
	for idx, entry := range additions {
		keyBytes, _ := json.Marshal(entry.key)
		indent := memberIndent
		if idx == 0 {
			indent = ""
		}
		lines = append(lines, indent+string(keyBytes)+": "+entry.valueJSON+",")
	}
	return strings.Join(lines, "\n") + "\n" + memberIndent
}

func environmentInsertionAppend(content string, objectStart int, objectEnd int, members []jsonObjectMember, additions []encodedSpecialEntry, retainedCount int) string {
	if retainedCount == 0 {
		return environmentInsertion(content, objectStart, objectEnd, nil, additions)
	}
	return environmentInsertion(content, objectStart, objectEnd, members, additions)
}

func environmentInsertion(content string, objectStart int, objectEnd int, members []jsonObjectMember, additions []encodedSpecialEntry) string {
	memberIndent, closeIndent := environmentIndents(content, objectStart, objectEnd, members)
	if memberIndent == "" && closeIndent == "" {
		inline := make([]string, 0, len(additions))
		for _, entry := range additions {
			keyBytes, _ := json.Marshal(entry.key)
			inline = append(inline, string(keyBytes)+":"+entry.valueJSON)
		}
		if len(members) == 0 {
			return strings.Join(inline, ",")
		}
		return "," + strings.Join(inline, ",")
	}
	lines := environmentEntryLines(memberIndent, additions, false)
	if len(members) == 0 {
		return "\n" + strings.Join(lines, ",\n") + "\n" + closeIndent
	}
	return ",\n" + strings.Join(lines, ",\n")
}

func environmentEntryLines(memberIndent string, additions []encodedSpecialEntry, trailingComma bool) []string {
	lines := make([]string, 0, len(additions))
	for idx, entry := range additions {
		keyBytes, _ := json.Marshal(entry.key)
		line := memberIndent + string(keyBytes) + ": " + entry.valueJSON
		if trailingComma || idx < len(additions)-1 {
			line += ","
		}
		lines = append(lines, line)
	}
	return lines
}

func environmentInsertionPoint(content string, objectStart int, objectEnd int) int {
	pos := objectEnd
	for pos > objectStart+1 {
		switch content[pos-1] {
		case ' ', '\n', '\r', '\t':
			pos--
		default:
			return pos
		}
	}
	return objectStart + 1
}

func environmentIndents(content string, objectStart int, objectEnd int, members []jsonObjectMember) (string, string) {
	closeIndent := ""
	if lineStart := strings.LastIndex(content[:objectEnd], "\n"); lineStart >= 0 {
		closeIndent = content[lineStart+1 : objectEnd]
	}
	if len(members) > 0 {
		prefix := content[members[0].memberStart:members[0].keyStart]
		if lineStart := strings.LastIndex(prefix, "\n"); lineStart >= 0 {
			return prefix[lineStart+1:], closeIndent
		}
		return prefix, closeIndent
	}
	return closeIndent + "  ", closeIndent
}

type textEdit struct {
	start       int
	end         int
	replacement string
}

func applyTextEdits(content string, edits []textEdit) string {
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start
	})
	out := content
	for _, edit := range edits {
		out = out[:edit.start] + edit.replacement + out[edit.end:]
	}
	return out
}

func defaultsModelFromDetail(detail AssetDetail) (DefaultsModel, error) {
	var root any
	if strings.TrimSpace(detail.Content) != "" {
		if err := yaml.Unmarshal([]byte(detail.Content), &root); err != nil {
			return DefaultsModel{}, err
		}
	}
	rootMap, _ := root.(map[string]any)
	model := DefaultsModel{Env: detail.Env, FileName: detail.RelativePath, Path: detail.Path, ContentHash: detail.ContentHash, IsDirty: detail.IsDirty, Vars: extractVars(detail.Content)}
	for gIdx, rawGroup := range anySlice(rootMap["container_envs"]) {
		groupMap, _ := rawGroup.(map[string]any)
		group := ContainerEnvGroup{ContainerRefName: stringValue(groupMap["container_ref_name"])}
		for eIdx, rawEnv := range anySlice(groupMap["envs"]) {
			envMap, _ := rawEnv.(map[string]any)
			group.Envs = append(group.Envs, envVarModel(eIdx, envMap))
		}
		if group.ContainerRefName == "" {
			group.ContainerRefName = fmt.Sprintf("#%d", gIdx+1)
		}
		model.ContainerEnvs = append(model.ContainerEnvs, group)
	}
	return model, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func isChildPath(root string, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func extractVars(content string) []VarItem {
	lines, start, end := splitVarsBlock(content)
	if start < 0 || end < 0 {
		return nil
	}
	return parseVarsLines(lines[start:end])
}

func replaceVarsBlock(content string, items []VarItem) string {
	lines, start, end := splitVarsBlock(content)
	var replacement []string
	if len(items) > 0 {
		replacement = renderVarsBlock(items)
	}
	if start >= 0 && end >= 0 {
		updated := make([]string, 0, len(lines)-end+start+len(replacement))
		updated = append(updated, lines[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[end:]...)
		lines = updated
	} else if len(replacement) > 0 {
		lines = append(append(replacement, ""), lines...)
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") {
		out += "\n"
	}
	return out
}

func replaceDefaultsContainerEnvsBlock(content string, groups []ContainerEnvGroupUpdate) string {
	return replaceTopLevelBlock(content, "container_envs", renderDefaultsContainerEnvsBlock(groups))
}

func renderDefaultsContainerEnvsBlock(groups []ContainerEnvGroupUpdate) []string {
	if len(groups) == 0 {
		return nil
	}
	lines := []string{"container_envs:"}
	for _, group := range groups {
		lines = append(lines, "  - container_ref_name: "+strconv.Quote(strings.TrimSpace(group.ContainerRefName)))
		if len(group.Envs) == 0 {
			lines = append(lines, "    envs: []")
			continue
		}
		lines = append(lines, "    envs:")
		for _, item := range group.Envs {
			lines = append(lines, "      - name: "+strings.TrimSpace(item.Name), "        value: "+strconv.Quote(item.Value))
		}
	}
	return lines
}

func validateContainerEnvGroups(groups []ContainerEnvGroupUpdate) error {
	seen := map[string]bool{}
	for _, group := range groups {
		name := strings.TrimSpace(group.ContainerRefName)
		if name == "" {
			return errors.New("container_ref_name is required")
		}
		if strings.ContainsAny(name, "\r\n:") {
			return fmt.Errorf("invalid container env group name %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate container env group %q", name)
		}
		seen[name] = true
		if err := validateVarItems(group.Envs); err != nil {
			return fmt.Errorf("container env group %q: %w", name, err)
		}
	}
	return nil
}

func replaceTopLevelScalar(content string, key string, value string) string {
	lines := strings.Split(content, "\n")
	prefix := key + ":"
	for idx, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[idx] = prefix + " " + value
			out := strings.Join(lines, "\n")
			if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			return out
		}
	}
	insertAt := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "vars:") {
		_, start, end := splitVarsBlock(content)
		if start == 0 && end > 0 {
			insertAt = end
		}
	}
	replacement := key + ": " + value
	updated := make([]string, 0, len(lines)+1)
	updated = append(updated, lines[:insertAt]...)
	updated = append(updated, replacement)
	updated = append(updated, lines[insertAt:]...)
	out := strings.Join(updated, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func replaceAutoscalingBlock(content string, autoscaling AutoscalingUpdate) string {
	return replaceTopLevelBlock(content, "autoscaling", renderAutoscalingBlock(autoscaling))
}

func replaceTopLevelBlock(content string, key string, replacement []string) string {
	lines := strings.Split(content, "\n")
	start, end := topLevelBlockRange(lines, key)
	insertAt := topLevelInsertIndex(lines)
	if start >= 0 {
		insertAt = start
		lines = append(lines[:start], lines[end:]...)
	}
	if len(replacement) > 0 {
		updated := make([]string, 0, len(lines)+len(replacement))
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func replaceTopLevelStringList(content string, key string, values []string) string {
	return replaceTopLevelBlock(content, key, renderStringListBlock(key, 0, values))
}

func renderStringListBlock(key string, indent int, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	prefix := strings.Repeat(" ", indent)
	lines := []string{prefix + key + ":"}
	for _, value := range values {
		lines = append(lines, prefix+"  - "+value)
	}
	return lines
}

func replaceCollectionReferenceLists(content string, collection string, updates []ContainerReferences) (string, error) {
	if len(updates) == 0 {
		return content, nil
	}
	updated := content
	for index := len(updates) - 1; index >= 0; index-- {
		var err error
		updated, err = replaceCollectionItemStringList(updated, collection, index, "runtime_asset_ref_names", updates[index].RuntimeAssetRefNames)
		if err != nil {
			return "", err
		}
		updated, err = replaceCollectionItemStringList(updated, collection, index, "profile_ref_names", updates[index].ProfileRefNames)
		if err != nil {
			return "", err
		}
	}
	return updated, nil
}

func replaceCollectionItemStringList(content string, collection string, itemIndex int, key string, values []string) (string, error) {
	lines, start, end, err := collectionItemBlockRange(content, collection, itemIndex)
	if err != nil {
		return "", err
	}
	blockStart, blockEnd := containerChildBlockRange(lines, start, end, key)
	insertAt := start + 1
	if blockStart >= 0 {
		insertAt = blockStart
		lines = append(lines[:blockStart], lines[blockEnd:]...)
	}
	replacement := renderStringListBlock(key, 4, values)
	if len(replacement) > 0 {
		result := make([]string, 0, len(lines)+len(replacement))
		result = append(result, lines[:insertAt]...)
		result = append(result, replacement...)
		result = append(result, lines[insertAt:]...)
		lines = result
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func topLevelBlockRange(lines []string, key string) (int, int) {
	prefix := key + ":"
	for idx, line := range lines {
		if strings.HasPrefix(line, prefix) {
			end := len(lines)
			for j := idx + 1; j < len(lines); j++ {
				if isTopLevelLine(lines[j]) {
					end = j
					break
				}
			}
			return idx, end
		}
	}
	return -1, -1
}

func topLevelInsertIndex(lines []string) int {
	keys := []string{"vars", "name", "kind", "replicas"}
	insertAt := 0
	for _, key := range keys {
		start, end := topLevelBlockRange(lines, key)
		if start >= 0 && end > insertAt {
			insertAt = end
		}
	}
	return insertAt
}

func replaceContainerResourcesBlock(content string, containerIndex int, resources ResourceUpdate) (string, error) {
	lines, start, end, err := containerBlockRange(content, containerIndex)
	if err != nil {
		return "", err
	}

	resStart := -1
	resEnd := -1
	for idx := start + 1; idx < end; idx++ {
		if strings.HasPrefix(lines[idx], "    resources:") {
			resStart = idx
			resEnd = end
			for j := idx + 1; j < end; j++ {
				if strings.TrimSpace(lines[j]) != "" && leadingSpaces(lines[j]) <= 4 {
					resEnd = j
					break
				}
			}
			break
		}
	}

	replacement := renderResourcesBlock(resources)
	insertAt := start + 1
	if resStart >= 0 {
		insertAt = resStart
		lines = append(lines[:resStart], lines[resEnd:]...)
	}
	if len(replacement) > 0 {
		updated := make([]string, 0, len(lines)+len(replacement))
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func replaceContainerEnvsBlock(content string, containerIndex int, items []VarItem) (string, error) {
	lines, start, end, err := containerBlockRange(content, containerIndex)
	if err != nil {
		return "", err
	}

	varsStart, varsEnd := containerChildBlockRange(lines, start, end, "envs")

	preserved := [][]string{}
	preservedNames := map[string]bool{}
	if varsStart >= 0 {
		for _, block := range splitVarItemBlocks(lines[varsStart+1 : varsEnd]) {
			name := varBlockName(block)
			if !isEditableVarBlock(block) {
				preserved = append(preserved, block)
				if name != "" {
					preservedNames[name] = true
				}
			}
		}
	}
	if err := validateContainerEnvItems(items, preservedNames); err != nil {
		return "", err
	}

	replacement := renderContainerEnvsBlock(items, preserved)
	insertAt := start + 1
	if varsStart >= 0 {
		insertAt = varsStart
		lines = append(lines[:varsStart], lines[varsEnd:]...)
	}
	if len(replacement) > 0 {
		updated := make([]string, 0, len(lines)+len(replacement))
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func replaceContainerRuntimeBlock(content string, containerIndex int, runtime JavaRuntimeUpdate) (string, error) {
	lines, start, end, err := containerBlockRange(content, containerIndex)
	if err != nil {
		return "", err
	}

	runtimeStart, runtimeEnd := containerChildBlockRange(lines, start, end, "runtime")
	replacement := renderJavaRuntimeBlock(runtime)
	insertAt := start + 1
	if runtimeStart >= 0 {
		insertAt = runtimeStart
		lines = append(lines[:runtimeStart], lines[runtimeEnd:]...)
	}
	if len(replacement) > 0 {
		updated := make([]string, 0, len(lines)+len(replacement))
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func replaceContainerPortsBlock(content string, containerIndex int, ports []PortUpdate) (string, error) {
	lines, start, end, err := containerBlockRange(content, containerIndex)
	if err != nil {
		return "", err
	}

	portsStart, portsEnd := containerChildBlockRange(lines, start, end, "ports")
	replacement := renderPortsBlock(ports)
	insertAt := start + 1
	if portsStart >= 0 {
		insertAt = portsStart
		lines = append(lines[:portsStart], lines[portsEnd:]...)
	}
	if len(replacement) > 0 {
		updated := make([]string, 0, len(lines)+len(replacement))
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func replaceContainerProbesBlock(content string, containerIndex int, probes ProbeUpdate) (string, error) {
	lines, start, end, err := containerBlockRange(content, containerIndex)
	if err != nil {
		return "", err
	}

	probesStart, probesEnd := containerChildBlockRange(lines, start, end, "probes")
	replacement := renderProbesBlock(probes)
	insertAt := start + 1
	if probesStart >= 0 {
		insertAt = probesStart
		lines = append(lines[:probesStart], lines[probesEnd:]...)
	}
	if len(replacement) > 0 {
		updated := make([]string, 0, len(lines)+len(replacement))
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func fixContainerLegacyProbesBlock(content string, containerIndex int) (string, error) {
	lines, start, end, err := containerBlockRange(content, containerIndex)
	if err != nil {
		return "", err
	}

	probesStart, _ := containerChildBlockRange(lines, start, end, "probes")
	probeStart, probeEnd := containerChildBlockRange(lines, start, end, "probe")
	healthStart, healthEnd := containerChildBlockRange(lines, start, end, "health")
	if probeStart < 0 && healthStart < 0 {
		return "", errors.New("legacy probes block not found")
	}

	replacement := []string{}
	insertAt := end
	if probesStart >= 0 {
		insertAt = probesStart
	} else if probeStart >= 0 {
		insertAt = probeStart
		replacement = legacyProbeToProbesBlock(lines[probeStart:probeEnd])
	} else {
		insertAt = healthStart
		replacement = legacyHealthToProbesBlock(lines[healthStart:healthEnd])
	}

	for _, block := range sortedRangesDesc([][2]int{{probeStart, probeEnd}, {healthStart, healthEnd}}) {
		if block[0] < 0 {
			continue
		}
		if block[0] < insertAt {
			insertAt -= block[1] - block[0]
		}
		lines = append(lines[:block[0]], lines[block[1]:]...)
	}
	if probesStart < 0 && len(replacement) > 0 {
		updated := make([]string, 0, len(lines)+len(replacement))
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
	}

	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func containerBlockRange(content string, containerIndex int) ([]string, int, int, error) {
	return collectionItemBlockRange(content, "containers", containerIndex)
}

func collectionItemBlockRange(content string, collection string, itemIndex int) ([]string, int, int, error) {
	lines := strings.Split(content, "\n")
	containersLine := -1
	for idx, line := range lines {
		if line == collection+":" {
			containersLine = idx
			break
		}
	}
	if containersLine < 0 {
		return nil, 0, 0, fmt.Errorf("%s block not found", collection)
	}
	starts := []int{}
	for idx := containersLine + 1; idx < len(lines); idx++ {
		line := lines[idx]
		if isTopLevelLine(line) {
			break
		}
		if strings.HasPrefix(line, "  - ") {
			starts = append(starts, idx)
		}
	}
	if itemIndex >= len(starts) {
		return nil, 0, 0, fmt.Errorf("%s index not found", collection)
	}
	start := starts[itemIndex]
	end := len(lines)
	if itemIndex+1 < len(starts) {
		end = starts[itemIndex+1]
	} else {
		for idx := start + 1; idx < len(lines); idx++ {
			if isTopLevelLine(lines[idx]) {
				end = idx
				break
			}
		}
	}
	return lines, start, end, nil
}

func containerChildBlockRange(lines []string, start int, end int, key string) (int, int) {
	prefix := "    " + key + ":"
	for idx := start + 1; idx < end; idx++ {
		if strings.HasPrefix(lines[idx], prefix) {
			blockEnd := end
			for j := idx + 1; j < end; j++ {
				if strings.TrimSpace(lines[j]) != "" && leadingSpaces(lines[j]) <= 4 {
					blockEnd = j
					break
				}
			}
			return idx, blockEnd
		}
	}
	return -1, -1
}

func sortedRangesDesc(ranges [][2]int) [][2]int {
	out := [][2]int{}
	for _, item := range ranges {
		if item[0] >= 0 && item[1] > item[0] {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] > out[j][0] })
	return out
}

func renderResourcesBlock(resources ResourceUpdate) []string {
	cpuRequest := strings.TrimSpace(resources.CPURequest)
	cpuLimit := strings.TrimSpace(resources.CPULimit)
	memRequest := strings.TrimSpace(resources.MemoryRequest)
	memLimit := strings.TrimSpace(resources.MemoryLimit)
	ephemeralRequest := strings.TrimSpace(resources.EphemeralStorageRequest)
	ephemeralLimit := strings.TrimSpace(resources.EphemeralStorageLimit)
	if cpuRequest == "" && cpuLimit == "" && memRequest == "" && memLimit == "" && ephemeralRequest == "" && ephemeralLimit == "" {
		return nil
	}
	lines := []string{"    resources:"}
	if cpuRequest != "" || cpuLimit != "" {
		lines = append(lines, "      cpu:")
		if cpuRequest != "" {
			lines = append(lines, "        requests: "+strconv.Quote(cpuRequest))
		}
		if cpuLimit != "" {
			lines = append(lines, "        limits: "+strconv.Quote(cpuLimit))
		}
	}
	if memRequest != "" || memLimit != "" {
		lines = append(lines, "      memory:")
		if memRequest != "" {
			lines = append(lines, "        requests: "+strconv.Quote(memRequest))
		}
		if memLimit != "" {
			lines = append(lines, "        limits: "+strconv.Quote(memLimit))
		}
	}
	if ephemeralRequest != "" || ephemeralLimit != "" {
		lines = append(lines, "      ephemeral-storage:")
		if ephemeralRequest != "" {
			lines = append(lines, "        requests: "+strconv.Quote(ephemeralRequest))
		}
		if ephemeralLimit != "" {
			lines = append(lines, "        limits: "+strconv.Quote(ephemeralLimit))
		}
	}
	return lines
}

func renderAutoscalingBlock(autoscaling AutoscalingUpdate) []string {
	lines := []string{"autoscaling:", "  enabled: " + strconv.FormatBool(autoscaling.Enabled)}
	if value := strings.TrimSpace(autoscaling.MinReplicas); value != "" {
		lines = append(lines, "  min_replicas: "+value)
	}
	if value := strings.TrimSpace(autoscaling.MaxReplicas); value != "" {
		lines = append(lines, "  max_replicas: "+value)
	}
	if value := strings.TrimSpace(autoscaling.CPUAverageUtilization); value != "" {
		lines = append(lines, "  cpu:", "    average_utilization: "+value)
	}
	if value := strings.TrimSpace(autoscaling.MemoryAverageUtilization); value != "" {
		lines = append(lines, "  memory:", "    average_utilization: "+value)
	}
	return lines
}

func renderJavaRuntimeBlock(runtime JavaRuntimeUpdate) []string {
	xms := strings.TrimSpace(runtime.Xms)
	xmx := strings.TrimSpace(runtime.Xmx)
	exportEnvName := strings.TrimSpace(runtime.ExportEnvName)
	opts := cleanStringItems(runtime.Opts)
	if xms == "" && xmx == "" && exportEnvName == "" && len(opts) == 0 {
		return nil
	}
	lines := []string{"    runtime:", "      java:"}
	if xms != "" {
		lines = append(lines, "        xms: "+strconv.Quote(xms))
	}
	if xmx != "" {
		lines = append(lines, "        xmx: "+strconv.Quote(xmx))
	}
	if len(opts) > 0 {
		lines = append(lines, "        opts:")
		for _, opt := range opts {
			lines = append(lines, "          - "+strconv.Quote(opt))
		}
	}
	if exportEnvName != "" && exportEnvName != "JAVA_OPTS" {
		lines = append(lines, "        export:", "          env_name: "+exportEnvName)
	}
	return lines
}

func renderPortsBlock(ports []PortUpdate) []string {
	if len(ports) == 0 {
		return nil
	}
	lines := []string{"    ports:"}
	for _, port := range ports {
		lines = append(lines, "      - name: "+strings.TrimSpace(port.Name), "        port: "+strings.TrimSpace(port.Port))
		if port.Metrics {
			lines = append(lines, "        metrics: true")
		}
		if value := strings.TrimSpace(port.MetricsPathFor); value != "" {
			lines = append(lines, "        metricsPathFor: "+strconv.Quote(value))
		}
		if len(port.ExposeAs) > 0 {
			lines = append(lines, "        expose_as:")
			for _, expose := range port.ExposeAs {
				lines = append(lines, "          - service_name: "+strings.TrimSpace(expose.ServiceName), "            port: "+strings.TrimSpace(expose.Port))
				if value := strings.TrimSpace(expose.ServiceType); value != "" && value != "clusterip" {
					lines = append(lines, "            type: "+value)
				}
				if len(expose.Externals) > 0 {
					lines = append(lines, "            external:")
					for _, external := range expose.Externals {
						lines = append(lines, "              - name: "+strings.TrimSpace(external.Name))
						if external.AsRoute {
							lines = append(lines, "                as_route: true")
						}
						if value := strings.TrimSpace(external.ClassName); value != "" {
							lines = append(lines, "                class_name: "+value)
						}
						if strings.TrimSpace(external.HTTPHostname) != "" {
							lines = append(lines, "                http:", "                  - hostname: "+strconv.Quote(strings.TrimSpace(external.HTTPHostname)))
							if value := strings.TrimSpace(external.HTTPPath); value != "" {
								lines = append(lines, "                    path: "+strconv.Quote(value))
							}
						}
						if strings.TrimSpace(external.HTTPSHostname) != "" {
							lines = append(lines, "                https:", "                  - hostname: "+strconv.Quote(strings.TrimSpace(external.HTTPSHostname)))
							if value := strings.TrimSpace(external.HTTPSPath); value != "" {
								lines = append(lines, "                    path: "+strconv.Quote(value))
							}
							if value := strings.TrimSpace(external.SecretName); value != "" {
								lines = append(lines, "                    secret_name: "+value)
							}
						}
					}
				}
			}
		}
	}
	return lines
}

func renderProbesBlock(probes ProbeUpdate) []string {
	preset := strings.TrimSpace(probes.Preset)
	port := strings.TrimSpace(probes.Port)
	path := strings.TrimSpace(probes.Path)
	if preset == "" && port == "" && path == "" {
		return nil
	}
	lines := []string{"    probes:"}
	if preset != "" {
		lines = append(lines, "      preset: "+strconv.Quote(preset))
	}
	if port != "" {
		lines = append(lines, "      port: "+strconv.Quote(port))
	}
	if path != "" {
		lines = append(lines, "      path: "+strconv.Quote(path))
	}
	return lines
}

func legacyProbeToProbesBlock(block []string) []string {
	if len(block) == 0 {
		return nil
	}
	out := append([]string(nil), block...)
	out[0] = strings.Replace(out[0], "    probe:", "    probes:", 1)
	return out
}

func legacyHealthToProbesBlock(block []string) []string {
	if len(block) <= 1 {
		return []string{"    probes:"}
	}
	out := []string{"    probes:", "      live:"}
	for _, line := range block[1:] {
		out = append(out, "  "+line)
	}
	out = append(out, "      ready:")
	for _, line := range block[1:] {
		out = append(out, "  "+line)
	}
	return out
}

func validateResourceUpdate(resources ResourceUpdate) error {
	for label, value := range map[string]string{"cpu_request": resources.CPURequest, "cpu_limit": resources.CPULimit} {
		if err := validateResourceValue(label, value, validateCPUQuantity); err != nil {
			return err
		}
	}
	for label, value := range map[string]string{
		"memory_request": resources.MemoryRequest, "memory_limit": resources.MemoryLimit,
		"ephemeral_storage_request": resources.EphemeralStorageRequest, "ephemeral_storage_limit": resources.EphemeralStorageLimit,
	} {
		if err := validateResourceValue(label, value, validateMemoryQuantity); err != nil {
			return err
		}
	}
	return nil
}

func validateResourceValue(label, value string, validate func(string, string) error) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s contains unsupported newline", label)
	}
	if match := sourceTemplatePattern.FindString(value); match != "" && match == value {
		return nil
	}
	return validate(label, value)
}

var cpuQuantityPattern = regexp.MustCompile(`^([0-9]+(\.[0-9]+)?|\.[0-9]+)m?$`)
var memoryQuantityPattern = regexp.MustCompile(`^([0-9]+(\.[0-9]+)?|\.[0-9]+)(Ki|Mi|Gi|Ti|Pi|Ei|K|M|G|T|P|E)?$`)
var javaMemoryQuantityPattern = regexp.MustCompile(`^[0-9]+[kKmMgGtT]?[bB]?$`)
var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateCPUQuantity(label string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !cpuQuantityPattern.MatchString(value) {
		return fmt.Errorf("%s must be a Kubernetes CPU quantity like 100m, 0.5 or 1", label)
	}
	return nil
}

func validateMemoryQuantity(label string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !memoryQuantityPattern.MatchString(value) {
		return fmt.Errorf("%s must be a Kubernetes memory quantity like 256Mi, 1Gi or 512M", label)
	}
	return nil
}

func validateJavaMemoryQuantity(label string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !javaMemoryQuantityPattern.MatchString(value) {
		return fmt.Errorf("%s must be a JVM memory quantity like 256m, 1g or 512M", label)
	}
	return nil
}

func validateProbeUpdate(probes ProbeUpdate) error {
	for label, value := range map[string]string{
		"preset": probes.Preset,
		"port":   probes.Port,
		"path":   probes.Path,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s contains unsupported newline", label)
		}
	}
	if port := strings.TrimSpace(probes.Port); port != "" {
		if err := validatePositivePort("port", port); err != nil {
			return err
		}
	}
	return nil
}

func validateAutoscalingUpdate(autoscaling AutoscalingUpdate) error {
	values := map[string]int{}
	for label, value := range map[string]string{
		"min_replicas":               autoscaling.MinReplicas,
		"max_replicas":               autoscaling.MaxReplicas,
		"cpu_average_utilization":    autoscaling.CPUAverageUtilization,
		"memory_average_utilization": autoscaling.MemoryAverageUtilization,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s contains unsupported newline", label)
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 0 {
			return fmt.Errorf("%s must be an integer greater than or equal to 0", label)
		}
		values[label] = parsed
	}
	if !autoscaling.Enabled {
		return nil
	}
	minReplicas, hasMin := values["min_replicas"]
	maxReplicas, hasMax := values["max_replicas"]
	if !hasMin || minReplicas < 1 {
		return errors.New("min_replicas must be greater than 0 when autoscaling is enabled")
	}
	if hasMax && maxReplicas < 1 {
		return errors.New("max_replicas must be greater than 0 when autoscaling is enabled")
	}
	if !hasMax || maxReplicas < minReplicas {
		return errors.New("max_replicas must be greater than or equal to min_replicas when autoscaling is enabled")
	}
	cpuAverage := values["cpu_average_utilization"]
	memoryAverage := values["memory_average_utilization"]
	if cpuAverage == 0 && memoryAverage == 0 {
		return errors.New("autoscaling requires at least one CPU or memory average utilization metric")
	}
	for label, value := range map[string]int{
		"cpu_average_utilization":    cpuAverage,
		"memory_average_utilization": memoryAverage,
	} {
		if value != 0 && (value < 1 || value > 100) {
			return fmt.Errorf("%s must be between 1 and 100", label)
		}
	}
	return nil
}

func validateJavaRuntimeUpdate(runtime JavaRuntimeUpdate) error {
	for label, value := range map[string]string{"xms": runtime.Xms, "xmx": runtime.Xmx, "export_env_name": runtime.ExportEnvName} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s contains unsupported newline", label)
		}
	}
	for label, value := range map[string]string{"xms": runtime.Xms, "xmx": runtime.Xmx} {
		if err := validateJavaMemoryQuantity(label, value); err != nil {
			return err
		}
	}
	for _, opt := range runtime.Opts {
		if strings.ContainsAny(opt, "\r\n") {
			return errors.New("runtime opts contain unsupported newline")
		}
	}
	exportEnvName := strings.TrimSpace(runtime.ExportEnvName)
	if exportEnvName != "" && !variableNamePattern.MatchString(exportEnvName) {
		return errors.New("export_env_name must be a valid environment variable name")
	}
	return nil
}

func validatePortUpdates(ports []PortUpdate) error {
	portNames := map[string]bool{}
	for _, port := range ports {
		name := strings.TrimSpace(port.Name)
		if name == "" {
			return errors.New("port name is required")
		}
		if strings.ContainsAny(name, "\r\n:") {
			return fmt.Errorf("invalid port name %q", name)
		}
		if portNames[name] {
			return fmt.Errorf("duplicate port name %q", name)
		}
		portNames[name] = true
		if err := validatePositivePort("port", port.Port); err != nil {
			return err
		}
		if strings.ContainsAny(port.MetricsPathFor, "\r\n") {
			return fmt.Errorf("metricsPathFor for port %q contains unsupported newline", name)
		}
		serviceNames := map[string]bool{}
		for _, expose := range port.ExposeAs {
			serviceName := strings.TrimSpace(expose.ServiceName)
			if serviceName == "" {
				return fmt.Errorf("service_name is required for port %q", name)
			}
			if strings.ContainsAny(serviceName, "\r\n:") {
				return fmt.Errorf("invalid service_name %q", serviceName)
			}
			if serviceNames[serviceName] {
				return fmt.Errorf("duplicate service_name %q for port %q", serviceName, name)
			}
			serviceNames[serviceName] = true
			if err := validatePositivePort("service port", expose.Port); err != nil {
				return err
			}
			if strings.ContainsAny(expose.ServiceType, "\r\n:") {
				return fmt.Errorf("invalid service type %q", expose.ServiceType)
			}
			externalNames := map[string]bool{}
			for _, external := range expose.Externals {
				externalName := strings.TrimSpace(external.Name)
				if externalName == "" {
					return fmt.Errorf("external name is required for service %q", serviceName)
				}
				if strings.ContainsAny(externalName, "\r\n:") {
					return fmt.Errorf("invalid external name %q", externalName)
				}
				if externalNames[externalName] {
					return fmt.Errorf("duplicate external name %q for service %q", externalName, serviceName)
				}
				externalNames[externalName] = true
				for label, value := range map[string]string{
					"class_name":     external.ClassName,
					"http_hostname":  external.HTTPHostname,
					"http_path":      external.HTTPPath,
					"https_hostname": external.HTTPSHostname,
					"https_path":     external.HTTPSPath,
					"secret_name":    external.SecretName,
				} {
					if strings.ContainsAny(value, "\r\n") {
						return fmt.Errorf("%s for external %q contains unsupported newline", label, externalName)
					}
				}
				if strings.TrimSpace(external.HTTPHostname) == "" && strings.TrimSpace(external.HTTPSHostname) == "" {
					return fmt.Errorf("external %q requires at least one HTTP or HTTPS hostname", externalName)
				}
			}
		}
	}
	return nil
}

func validatePositivePort(label string, value string) error {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must be an integer between 1 and 65535", label)
	}
	return nil
}

var sourceTemplatePattern = regexp.MustCompile(`\{\{[^{}\r\n]+\}\}`)

func sourceModelRoot(content string) (map[string]any, error) {
	preview := sourceTemplatePattern.ReplaceAllString(renderVarsPreview(content), "__KUBE_EDIT_TEMPLATE__")
	var root any
	if err := yaml.Unmarshal([]byte(preview), &root); err != nil {
		return nil, err
	}
	rootMap, _ := root.(map[string]any)
	return rootMap, nil
}

func rejectUnsupportedAutoscalingForUpdate(content string) error {
	root, err := sourceModelRoot(content)
	if err != nil {
		return err
	}
	autoscaling, _ := root["autoscaling"].(map[string]any)
	if err := rejectUnknownKeys(autoscaling, "autoscaling", "enabled", "min_replicas", "max_replicas", "cpu", "memory"); err != nil {
		return err
	}
	for _, metric := range []string{"cpu", "memory"} {
		values, _ := autoscaling[metric].(map[string]any)
		if err := rejectUnknownKeys(values, "autoscaling."+metric, "average_utilization"); err != nil {
			return err
		}
	}
	return nil
}

func rejectUnsupportedDefaultsContainerEnvsForUpdate(content string) error {
	root, err := sourceModelRoot(content)
	if err != nil {
		return err
	}
	for groupIndex, rawGroup := range anySlice(root["container_envs"]) {
		group, _ := rawGroup.(map[string]any)
		for envIndex, rawEnv := range anySlice(group["envs"]) {
			env, _ := rawEnv.(map[string]any)
			if _, editable := env["value"]; !editable {
				return fmt.Errorf("container_envs[%d].envs[%d] uses an advanced environment value; edit the YAML source until the advanced env editor is available", groupIndex, envIndex)
			}
		}
	}
	return nil
}

func rejectUnsupportedContainerBlockForUpdate(content string, containerIndex int, blockName string) error {
	root, err := sourceModelRoot(content)
	if err != nil {
		return err
	}
	containers := anySlice(root["containers"])
	if containerIndex >= len(containers) {
		return errors.New("container index not found")
	}
	container, _ := containers[containerIndex].(map[string]any)
	rawBlock, blockPresent := container[blockName]
	block, blockIsMap := rawBlock.(map[string]any)
	switch blockName {
	case "resources":
		if blockPresent && !blockIsMap {
			return errors.New("resources must be a mapping")
		}
		return validateEditableResourcesMap(block)
	case "probes":
		if err := rejectUnknownKeys(block, "probes", "preset", "port", "path"); err != nil {
			return fmt.Errorf("%w; edit the YAML source until the advanced probes editor is available", err)
		}
	case "runtime":
		if err := rejectUnknownKeys(block, "runtime", "java"); err != nil {
			return err
		}
		java, _ := block["java"].(map[string]any)
		if err := rejectUnknownKeys(java, "runtime.java", "xms", "xmx", "opts", "export"); err != nil {
			return err
		}
		export, _ := java["export"].(map[string]any)
		if err := rejectUnknownKeys(export, "runtime.java.export", "env_name"); err != nil {
			return err
		}
	}
	return nil
}

func validateEditableResourcesMap(resources map[string]any) error {
	if err := rejectUnknownKeys(resources, "resources", "cpu", "memory", "ephemeral-storage"); err != nil {
		return err
	}
	for _, resource := range []string{"cpu", "memory", "ephemeral-storage"} {
		rawValues, present := resources[resource]
		if !present {
			continue
		}
		values, ok := rawValues.(map[string]any)
		if !ok {
			return fmt.Errorf("resources.%s must be a mapping", resource)
		}
		if err := rejectUnknownKeys(values, "resources."+resource, "requests", "limits", "from", "to"); err != nil {
			return err
		}
		if _, canonical := values["requests"]; canonical {
			if _, alias := values["from"]; alias {
				return fmt.Errorf("resources.%s defines both requests and from; edit the YAML source to choose one spelling", resource)
			}
		}
		if _, canonical := values["limits"]; canonical {
			if _, alias := values["to"]; alias {
				return fmt.Errorf("resources.%s defines both limits and to; edit the YAML source to choose one spelling", resource)
			}
		}
	}
	return nil
}

func rejectUnknownKeys(values map[string]any, path string, allowed ...string) error {
	allowedSet := map[string]bool{}
	for _, key := range allowed {
		allowedSet[key] = true
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowedSet[key] {
			return fmt.Errorf("%s editor does not support existing field %q", path, key)
		}
	}
	return nil
}

func rejectUnsupportedPortsForUpdate(content string, containerIndex int) error {
	rootMap, err := sourceModelRoot(content)
	if err != nil {
		return err
	}
	containers := anySlice(rootMap["containers"])
	if containerIndex >= len(containers) {
		return errors.New("container index not found")
	}
	containerMap, _ := containers[containerIndex].(map[string]any)
	for _, rawPort := range anySlice(containerMap["ports"]) {
		portMap, _ := rawPort.(map[string]any)
		for key := range portMap {
			if !stringIn(key, "name", "port", "metrics", "metricsPathFor", "expose_as") {
				return fmt.Errorf("ports editor does not support existing port field %q", key)
			}
		}
		for _, rawExpose := range anySlice(portMap["expose_as"]) {
			exposeMap, _ := rawExpose.(map[string]any)
			for key := range exposeMap {
				if !stringIn(key, "service_name", "hostname", "port", "type", "external") {
					return fmt.Errorf("ports editor does not support existing expose_as field %q", key)
				}
			}
			if serviceName := stringValue(exposeMap["service_name"]); serviceName != "" {
				if legacy := stringValue(exposeMap["hostname"]); legacy != "" && legacy != serviceName {
					return errors.New("expose_as has conflicting service_name and legacy hostname")
				}
			}
			for _, rawExternal := range anySlice(exposeMap["external"]) {
				externalMap, _ := rawExternal.(map[string]any)
				for key := range externalMap {
					if !stringIn(key, "name", "as_route", "class_name", "http", "https") {
						return fmt.Errorf("ports editor does not support existing external field %q", key)
					}
				}
				if err := rejectUnsupportedExternalHosts(externalMap, "http", false); err != nil {
					return err
				}
				if err := rejectUnsupportedExternalHosts(externalMap, "https", true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func rejectUnsupportedExternalHosts(externalMap map[string]any, key string, allowSecret bool) error {
	for _, rawHost := range anySlice(externalMap[key]) {
		hostMap, _ := rawHost.(map[string]any)
		for hostKey := range hostMap {
			if hostKey == "hostname" || hostKey == "path" || (allowSecret && hostKey == "secret_name") {
				continue
			}
			return fmt.Errorf("ports editor does not support existing %s field %q", key, hostKey)
		}
	}
	return nil
}

func stringIn(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func cleanStringItems(items []string) []string {
	out := []string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func splitVarItemBlocks(lines []string) [][]string {
	blocks := [][]string{}
	start := -1
	for idx, line := range lines {
		if strings.HasPrefix(line, "      - ") {
			if start >= 0 {
				blocks = append(blocks, lines[start:idx])
			}
			start = idx
		}
	}
	if start >= 0 {
		blocks = append(blocks, lines[start:])
	}
	return blocks
}

func isEditableVarBlock(lines []string) bool {
	hasName := false
	hasValue := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "name:") {
			hasName = true
		}
		if strings.HasPrefix(trimmed, "valueFrom:") {
			return false
		}
		if strings.HasPrefix(trimmed, "value:") {
			hasValue = true
		}
	}
	return hasName && hasValue
}

func varBlockName(lines []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "name:") {
			v := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
			return unquote(v)
		}
	}
	return ""
}

func renderContainerEnvsBlock(items []VarItem, preserved [][]string) []string {
	if len(items) == 0 && len(preserved) == 0 {
		return nil
	}
	lines := []string{"    envs:"}
	for _, item := range items {
		lines = append(lines, "      - name: "+strings.TrimSpace(item.Name), "        value: "+strconv.Quote(item.Value))
	}
	for _, block := range preserved {
		lines = append(lines, block...)
	}
	return lines
}

func validateContainerEnvItems(items []VarItem, preservedNames map[string]bool) error {
	if err := validateVarItems(items); err != nil {
		return err
	}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if preservedNames[name] {
			return fmt.Errorf("variable name %q is already used by a read-only variable", name)
		}
	}
	return nil
}

func isTopLevelLine(line string) bool {
	return strings.TrimSpace(line) != "" && leadingSpaces(line) == 0
}

func renderVarsBlock(items []VarItem) []string {
	lines := []string{"vars:"}
	for _, item := range items {
		lines = append(lines, "  - name: "+strings.TrimSpace(item.Name), "    value: "+strconv.Quote(item.Value))
	}
	return lines
}

func validateVarItems(items []VarItem) error {
	seen := map[string]bool{}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return errors.New("variable name is required")
		}
		if !variableNamePattern.MatchString(name) {
			return fmt.Errorf("invalid variable name %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate variable name %q", name)
		}
		seen[name] = true
		if strings.ContainsAny(item.Value, "\r\n") {
			return fmt.Errorf("variable %q contains unsupported newline", name)
		}
	}
	return nil
}

func renderVarsPreview(content string) string {
	lines, start, end := splitVarsBlock(content)
	var vars []VarItem
	if start >= 0 && end >= 0 {
		vars = parseVarsLines(lines[start:end])
		lines = append(lines[:start], lines[end:]...)
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") {
		out += "\n"
	}
	for _, item := range vars {
		out = strings.ReplaceAll(out, "{{var:"+item.Name+"}}", item.Value)
	}
	return out
}

func splitVarsBlock(content string) ([]string, int, int) {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	start, end := -1, -1
	for i, line := range lines {
		if leadingSpaces(line) == 0 && strings.TrimSpace(line) == "vars:" {
			start = i
			end = len(lines)
			for j := i + 1; j < len(lines); j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed != "" && !strings.HasPrefix(trimmed, "#") && leadingSpaces(lines[j]) == 0 {
					end = j
					break
				}
			}
			break
		}
	}
	return lines, start, end
}

func parseVarsLines(lines []string) []VarItem {
	items := []VarItem{}
	var name *string
	var value *string
	flush := func() {
		if name != nil && value != nil {
			items = append(items, VarItem{Name: *name, Value: *value})
		}
		name = nil
		value = nil
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "name:") {
			flush()
			v := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
			unquoted := unquote(v)
			name = &unquoted
		}
		if strings.HasPrefix(trimmed, "value:") {
			v := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
			unquoted := unquote(v)
			value = &unquoted
		}
	}
	flush()
	return items
}

func envVarModel(index int, varMap map[string]any) EnvVarModel {
	model := EnvVarModel{Index: index, Name: stringPtr(stringValue(varMap["name"])), Kind: "value", IsValueEditable: true}
	if rawValue, exists := varMap["value"]; exists {
		value := stringValue(rawValue)
		model.Value = &value
		return model
	}
	if value := stringValue(varMap["workload_identity_token_ref_name"]); value != "" {
		model.Kind = "workload_identity_token"
		model.WorkloadIdentityTokenRefName = stringPtr(value)
		model.IsValueEditable = false
		return model
	}
	if value := stringValue(varMap["shared_asset_ref_name"]); value != "" {
		model.Kind = "shared_asset"
		model.SharedAssetRefName = stringPtr(value)
		model.IsValueEditable = false
		return model
	}
	if boolValue(varMap["remove"]) {
		model.Kind = "remove"
		model.Remove = true
		model.IsValueEditable = false
		return model
	}
	valueFrom, _ := varMap["valueFrom"].(map[string]any)
	if secret, ok := valueFrom["secretKeyRef"].(map[string]any); ok {
		model.Kind = "secret"
		model.SecretName = stringPtr(stringValue(secret["name"]))
		model.Key = stringPtr(stringValue(secret["key"]))
		model.IsValueEditable = false
		return model
	}
	if resource, ok := valueFrom["resourceFieldRef"].(map[string]any); ok {
		model.Kind = "resource"
		model.ResourceName = stringPtr(stringValue(resource["resource"]))
		model.Divisor = stringPtr(stringValue(resource["divisor"]))
		model.IsValueEditable = false
		return model
	}
	if field, ok := valueFrom["fieldRef"].(map[string]any); ok {
		model.Kind = "field"
		model.FieldPath = stringPtr(stringValue(field["fieldPath"]))
		model.IsValueEditable = false
		return model
	}
	return model
}

func autoscalingModel(data map[string]any) AutoscalingModel {
	return AutoscalingModel{
		Enabled:                  boolValue(data["enabled"]),
		MinReplicas:              intPtr(intValue(data["min_replicas"])),
		MaxReplicas:              intPtr(intValue(data["max_replicas"])),
		CPUAverageUtilization:    intPtr(intValue(nestedValue(data, "cpu", "average_utilization"))),
		MemoryAverageUtilization: intPtr(intValue(nestedValue(data, "memory", "average_utilization"))),
	}
}

func runtimeModel(data map[string]any) RuntimeModel {
	java := nestedMap(data, "java")
	opts := stringSlice(java["opts"])
	exportEnvName := nestedString(java, "export", "env_name")
	if exportEnvName == "" {
		exportEnvName = "JAVA_OPTS"
	}
	out := RuntimeModel{
		Java: JavaRuntimeModel{
			Xms:           stringPtr(stringValue(java["xms"])),
			Xmx:           stringPtr(stringValue(java["xmx"])),
			Opts:          opts,
			ExportEnvName: exportEnvName,
		},
	}
	out.Java.Enabled = out.Java.Xms != nil || out.Java.Xmx != nil || len(opts) > 0
	return out
}

func probesModel(containerMap map[string]any) ProbesModel {
	probes := nestedMap(containerMap, "probes")
	out := ProbesModel{
		Preset: stringPtr(stringValue(probes["preset"])),
		Port:   stringPtr(stringValue(probes["port"])),
		Path:   stringPtr(stringValue(probes["path"])),
	}
	if nestedValue(containerMap, "health") != nil {
		out.LegacyKinds = append(out.LegacyKinds, "health")
	}
	if nestedValue(containerMap, "probe") != nil {
		out.LegacyKinds = append(out.LegacyKinds, "probe")
	}
	out.Legacy = len(out.LegacyKinds) > 0
	out.Enabled = out.Preset != nil || out.Port != nil || out.Path != nil || out.Legacy ||
		nestedValue(probes, "http") != nil || nestedValue(probes, "live") != nil ||
		nestedValue(probes, "ready") != nil || nestedValue(probes, "start") != nil
	return out
}

func nestedValue(data map[string]any, keys ...string) any {
	var current any = data
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func nestedMap(data map[string]any, keys ...string) map[string]any {
	value, _ := nestedValue(data, keys...).(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nestedString(data map[string]any, keys ...string) string {
	return stringValue(nestedValue(data, keys...))
}

func anySlice(value any) []any {
	if value == nil {
		return nil
	}
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

func stringSlice(value any) []string {
	out := []string{}
	for _, item := range anySlice(value) {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		var out int
		_, _ = fmt.Sscan(fmt.Sprint(value), &out)
		return out
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return fmt.Sprint(value) == "true"
	}
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func intPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}
