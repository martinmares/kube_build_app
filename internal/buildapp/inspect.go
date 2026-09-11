package buildapp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// BuildContext describes configured build inputs without exposing fetched
// environment values or HTTP header contents.
type BuildContext struct {
	VariableSources       []string `json:"variable_sources"`
	EnvFile               string   `json:"env_file,omitempty"`
	EnvURLConfigured      bool     `json:"env_url_configured"`
	EnvURLHeaderCount     int      `json:"env_url_header_count"`
	EnvURLInsecure        bool     `json:"env_url_insecure"`
	DecryptSecured        bool     `json:"decrypt_secured"`
	NamespaceOverride     string   `json:"namespace_override,omitempty"`
	ReleaseManifest       string   `json:"release_manifest,omitempty"`
	ImagePolicy           string   `json:"image_policy"`
	ImageReference        string   `json:"image_reference"`
	ImageOverrideCount    int      `json:"image_override_count"`
	ForceTagConfigured    bool     `json:"force_image_tag_configured"`
	ForcePrefixConfigured bool     `json:"force_image_prefix_configured"`
	ResourcePolicyRoot    string   `json:"resource_policy_root,omitempty"`
	ReplicaProfile        string   `json:"replica_profile,omitempty"`
	ReplicaProfilesFile   string   `json:"replica_profiles_file,omitempty"`
	SyncMetadataProfile   string   `json:"sync_metadata_profile,omitempty"`
	SyncMetadataPrefix    string   `json:"sync_metadata_prefix,omitempty"`
	SyncSet               string   `json:"sync_set,omitempty"`
	ScaleDownCount        int      `json:"scale_down_count"`
	LegacyApplyEnv        bool     `json:"legacy_apply_env"`
	HelmEscapeAssets      bool     `json:"helm_escape_assets"`
	YAMLIndent            int      `json:"yaml_indent"`
}

type Inspection struct {
	Environment    string         `json:"environment"`
	Namespace      string         `json:"namespace,omitempty"`
	Context        BuildContext   `json:"context"`
	EffectiveError string         `json:"effective_error,omitempty"`
	Apps           []InspectedApp `json:"apps"`
}

type InspectedApp struct {
	FileName       string          `json:"file_name"`
	Source         SourceDocument  `json:"source"`
	Effective      *EffectiveApp   `json:"effective,omitempty"`
	EffectiveError string          `json:"effective_error,omitempty"`
	Origins        []InspectOrigin `json:"origins"`
}

type SourceDocument struct {
	Path        string                 `json:"path"`
	ContentHash string                 `json:"content_hash"`
	RawContent  string                 `json:"raw_content"`
	Model       map[string]any         `json:"model,omitempty"`
	Fields      map[string]SourceField `json:"fields,omitempty"`
	ParseError  string                 `json:"parse_error,omitempty"`
}

type SourceField struct {
	Present  bool `json:"present"`
	Line     int  `json:"line,omitempty"`
	Column   int  `json:"column,omitempty"`
	RawValue any  `json:"raw_value,omitempty"`
}

type EffectiveApp struct {
	Name             string               `json:"name"`
	Kind             string               `json:"kind"`
	Replicas         int                  `json:"replicas"`
	Ignore           bool                 `json:"ignore"`
	Labels           map[string]any       `json:"labels,omitempty"`
	Annotations      map[string]any       `json:"annotations,omitempty"`
	PodAnnotations   map[string]any       `json:"pod_annotations,omitempty"`
	WorkloadIdentity map[string]any       `json:"workload_identity,omitempty"`
	Pod              map[string]any       `json:"pod,omitempty"`
	Autoscaling      map[string]any       `json:"autoscaling,omitempty"`
	RuntimeAssets    []map[string]any     `json:"runtime_assets,omitempty"`
	Containers       []EffectiveContainer `json:"containers"`
	Sidecars         []EffectiveContainer `json:"sidecars"`
	InitContainers   []map[string]any     `json:"init_containers,omitempty"`
}

type EffectiveContainer struct {
	Name                 string                  `json:"name"`
	Image                string                  `json:"image,omitempty"`
	Startup              map[string]any          `json:"startup,omitempty"`
	Envs                 []map[string]any        `json:"envs,omitempty"`
	Resources            map[string]any          `json:"resources,omitempty"`
	Ports                []map[string]any        `json:"ports,omitempty"`
	Probes               map[string]any          `json:"probes,omitempty"`
	Runtime              map[string]any          `json:"runtime,omitempty"`
	RuntimeAssetRefNames []string                `json:"runtime_asset_ref_names,omitempty"`
	Assets               []map[string]any        `json:"assets,omitempty"`
	Mounts               []map[string]any        `json:"mounts,omitempty"`
	EnvFrom              []map[string]any        `json:"env_from,omitempty"`
	SecurityContext      map[string]any          `json:"security_context,omitempty"`
	Raw                  map[string]any          `json:"raw,omitempty"`
	Fields               map[string]InspectField `json:"fields,omitempty"`
	EnvEntries           []InspectEnvEntry       `json:"env_entries,omitempty"`
}

type InspectEnvEntry struct {
	Name         string              `json:"name"`
	Effective    map[string]any      `json:"effective"`
	Origins      []InspectOrigin     `json:"origins,omitempty"`
	WriteTarget  *InspectWriteTarget `json:"write_target,omitempty"`
	Capabilities InspectCapabilities `json:"capabilities"`
}

type InspectOrigin struct {
	Kind           string `json:"kind"`
	Document       string `json:"document"`
	YAMLPath       string `json:"yaml_path"`
	DefinitionName string `json:"definition_name,omitempty"`
	Target         string `json:"target"`
}

type InspectField struct {
	Effective    any                 `json:"effective,omitempty"`
	Origins      []InspectOrigin     `json:"origins,omitempty"`
	WriteTarget  *InspectWriteTarget `json:"write_target,omitempty"`
	Capabilities InspectCapabilities `json:"capabilities"`
}

type InspectWriteTarget struct {
	Document      string `json:"document"`
	YAMLPath      string `json:"yaml_path"`
	Scope         string `json:"scope"`
	SourceIndex   int    `json:"source_index"`
	NameAssertion string `json:"name_assertion,omitempty"`
	ExpectedHash  string `json:"expected_hash"`
}

type InspectCapabilities struct {
	CanOverride   bool   `json:"can_override"`
	CanReset      bool   `json:"can_reset"`
	CanEditSource bool   `json:"can_edit_source"`
	Reason        string `json:"reason,omitempty"`
}

// DescribeBuildContext validates option combinations and returns a safe view.
func DescribeBuildContext(opts Options) (BuildContext, error) {
	sources, envFile, envURL, headers, insecure, decrypt, err := effectiveVarsSources(opts)
	if err != nil {
		return BuildContext{}, err
	}
	if normalizeImagePolicy(opts.ImagePolicy) != "fallback" && normalizeImagePolicy(opts.ImagePolicy) != "strict" {
		return BuildContext{}, fmt.Errorf("invalid image policy %q: expected fallback or strict", opts.ImagePolicy)
	}
	if _, err := releaseImageSelectionFromOptions(opts); err != nil {
		return BuildContext{}, err
	}
	indent, err := normalizeYAMLIndent(opts.YAMLIndent)
	if err != nil {
		return BuildContext{}, err
	}
	return BuildContext{
		VariableSources:       sources,
		EnvFile:               envFile,
		EnvURLConfigured:      envURL != "",
		EnvURLHeaderCount:     len(headers),
		EnvURLInsecure:        insecure,
		DecryptSecured:        decrypt,
		NamespaceOverride:     strings.TrimSpace(opts.Namespace),
		ReleaseManifest:       strings.TrimSpace(opts.ReleaseManifest),
		ImagePolicy:           normalizeImagePolicy(opts.ImagePolicy),
		ImageReference:        normalizeImageReference(opts.ImageReference),
		ImageOverrideCount:    len(opts.ImageOverrides),
		ForceTagConfigured:    strings.TrimSpace(opts.ForceImageTag) != "",
		ForcePrefixConfigured: strings.TrimSpace(opts.ForceImagePrefix) != "",
		ResourcePolicyRoot:    strings.TrimSpace(opts.ResourcePolicyRoot),
		ReplicaProfile:        strings.TrimSpace(opts.Profile),
		ReplicaProfilesFile:   strings.TrimSpace(opts.ProfilesFile),
		SyncMetadataProfile:   strings.TrimSpace(opts.SyncProfile),
		SyncMetadataPrefix:    strings.TrimSpace(opts.SyncPrefix),
		SyncSet:               strings.TrimSpace(opts.SyncSet),
		ScaleDownCount:        len(opts.Down),
		LegacyApplyEnv:        opts.LegacyApplyEnv,
		HelmEscapeAssets:      opts.HelmEscapeAssets,
		YAMLIndent:            indent,
	}, nil
}

// ResolveNamespace applies the same variable precedence as Build.
func ResolveNamespace(opts Options) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "environments"
	}
	vars, err := loadBuildVars(filepath.Join(opts.Root, opts.Environment), opts)
	if err != nil {
		return "", err
	}
	namespace := strings.TrimSpace(vars["NAMESPACE"])
	if namespace == "" {
		return "", fmt.Errorf("environment %q does not define NAMESPACE in the configured build variables", opts.Environment)
	}
	return namespace, nil
}

// Inspect returns source documents even when effective model evaluation fails.
func Inspect(opts Options) (Inspection, error) {
	if strings.TrimSpace(opts.Environment) == "" {
		return Inspection{}, fmt.Errorf("environment is required")
	}
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "environments"
	}
	context, err := DescribeBuildContext(opts)
	if err != nil {
		return Inspection{}, err
	}
	envDir := filepath.Join(opts.Root, opts.Environment)
	appsDir := filepath.Join(envDir, "apps")
	appFiles, err := listAppFiles(appsDir)
	if err != nil {
		return Inspection{}, err
	}
	result := Inspection{Environment: opts.Environment, Context: context}
	for _, path := range appFiles {
		source, err := inspectSourceDocument(path, opts.Root)
		if err != nil {
			return Inspection{}, err
		}
		result.Apps = append(result.Apps, InspectedApp{FileName: filepath.Base(path), Source: source})
	}

	vars, err := loadBuildVars(envDir, opts)
	if err != nil {
		result.EffectiveError = err.Error()
		return result, nil
	}
	result.Namespace = strings.TrimSpace(vars["NAMESPACE"])
	sharedAssets, err := loadSharedAssets(envDir, vars, opts)
	if err != nil {
		result.EffectiveError = err.Error()
		return result, nil
	}
	defaultsPath := filepath.Join(appsDir, "_defaults.yml")
	defaultsSource, _ := inspectSourceDocument(defaultsPath, opts.Root)
	for index, path := range appFiles {
		app, appErr := loadApp(path, defaultsPath, vars)
		if appErr == nil {
			if app.Kind == "" {
				app.Kind = "Deployment"
			}
			apps := []appModel{app}
			appErr = prepareRuntimeAssetsForApps(apps, opts.Environment)
			if appErr == nil {
				appErr = applyResourcePolicies(apps, []string{path}, opts)
			}
			if appErr == nil {
				appErr = applyImageOverrides(apps, opts)
			}
			if appErr == nil {
				appErr = applyReplicaProfile(apps, envDir, opts)
			}
			if appErr == nil {
				applyScaleDown(apps, opts.Down)
				appErr = validateApps(apps, sharedAssets)
			}
			if appErr == nil {
				result.Apps[index].Effective = effectiveInspectionApp(apps[0], result.Apps[index].Source, defaultsSource, opts)
			}
		}
		if appErr != nil {
			result.Apps[index].EffectiveError = appErr.Error()
		}
		result.Apps[index].Origins = inspectionOrigins(result.Apps[index].Source, defaultsPath, opts)
	}
	return result, nil
}

func inspectSourceDocument(path, root string) (SourceDocument, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SourceDocument{}, err
	}
	digest := sha256.Sum256(content)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	doc := SourceDocument{Path: filepath.ToSlash(rel), ContentHash: hex.EncodeToString(digest[:]), RawContent: string(content)}
	model, fields, err := parseTemplateYAMLDocument(content)
	if err != nil {
		doc.ParseError = err.Error()
	} else {
		doc.Model = model
		doc.Fields = fields
	}
	return doc, nil
}

var inspectTemplatePattern = regexp.MustCompile(`\{\{[^{}\r\n]+\}\}`)

func parseTemplateYAML(content []byte) (map[string]any, error) {
	model, _, err := parseTemplateYAMLDocument(content)
	return model, err
}

func parseTemplateYAMLDocument(content []byte) (map[string]any, map[string]SourceField, error) {
	matches := inspectTemplatePattern.FindAll(content, -1)
	replaced := append([]byte(nil), content...)
	values := map[string]string{}
	for index, match := range matches {
		token := fmt.Sprintf("__KUBE_EDIT_TEMPLATE_%06d__", index)
		replaced = []byte(strings.Replace(string(replaced), string(match), token, 1))
		values[token] = string(match)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(replaced, &node); err != nil {
		return nil, nil, err
	}
	var model map[string]any
	if err := node.Decode(&model); err != nil {
		return nil, nil, err
	}
	model = restoreTemplateValues(model, values).(map[string]any)
	fields := map[string]SourceField{}
	if len(node.Content) > 0 {
		collectSourceFields(node.Content[0], "", values, fields)
	}
	return model, fields, nil
}

func collectSourceFields(node *yaml.Node, path string, templates map[string]string, fields map[string]SourceField) {
	switch node.Kind {
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			keyNode, valueNode := node.Content[index], node.Content[index+1]
			key := restoreTemplateValues(keyNode.Value, templates).(string)
			next := key
			if path != "" {
				next = path + "." + key
			}
			fields[next] = SourceField{Present: true, Line: valueNode.Line, Column: valueNode.Column, RawValue: sourceNodeValue(valueNode, templates)}
			collectSourceFields(valueNode, next, templates, fields)
		}
	case yaml.SequenceNode:
		for index, item := range node.Content {
			next := fmt.Sprintf("%s[%d]", path, index)
			fields[next] = SourceField{Present: true, Line: item.Line, Column: item.Column, RawValue: sourceNodeValue(item, templates)}
			collectSourceFields(item, next, templates, fields)
		}
	}
}

func sourceNodeValue(node *yaml.Node, templates map[string]string) any {
	var value any
	if err := node.Decode(&value); err != nil {
		return nil
	}
	return restoreTemplateValues(value, templates)
}

func restoreTemplateValues(value any, templates map[string]string) any {
	switch typed := value.(type) {
	case string:
		for token, original := range templates {
			typed = strings.ReplaceAll(typed, token, original)
		}
		return typed
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[restoreTemplateValues(key, templates).(string)] = restoreTemplateValues(item, templates)
		}
		return result
	case []any:
		for index := range typed {
			typed[index] = restoreTemplateValues(typed[index], templates)
		}
	}
	return value
}

func effectiveInspectionApp(app appModel, source, defaults SourceDocument, opts Options) *EffectiveApp {
	result := &EffectiveApp{
		Name: app.Name, Kind: app.Kind, Replicas: app.Replicas, Ignore: app.Ignore,
		Labels: app.Labels, Annotations: app.Annotations, PodAnnotations: app.PodAnnotations,
		WorkloadIdentity: genericYAMLMap(app.WorkloadIdentity), Pod: genericYAMLMap(app.Pod),
		Autoscaling: genericYAMLMap(app.Autoscaling), RuntimeAssets: genericYAMLList(app.RuntimeAssets),
		InitContainers: genericYAMLList(app.InitContainers),
	}
	for index, container := range app.Containers {
		effective := effectiveInspectionContainer(container)
		effective.Fields = inspectContainerFields(effective, source, defaults, opts, "containers", index)
		effective.EnvEntries = inspectEnvEntries(effective, source, defaults, "containers", index)
		result.Containers = append(result.Containers, effective)
	}
	for index, sidecar := range app.Sidecars {
		effective := effectiveInspectionContainer(sidecar)
		effective.Fields = inspectContainerFields(effective, source, defaults, opts, "sidecars", index)
		effective.EnvEntries = inspectEnvEntries(effective, source, defaults, "sidecars", index)
		result.Sidecars = append(result.Sidecars, effective)
	}
	return result
}

func inspectEnvEntries(container EffectiveContainer, source, defaults SourceDocument, scope string, index int) []InspectEnvEntry {
	result := make([]InspectEnvEntry, 0, len(container.Envs))
	for _, env := range container.Envs {
		name := strings.TrimSpace(fmt.Sprint(env["name"]))
		origins, localIndex := inspectEnvOrigins(name, container.Name, source, defaults, scope, index)
		canEdit := scope == "containers"
		reason := ""
		if !canEdit {
			reason = "effective sidecar index is not a safe source selector"
		}
		path := fmt.Sprintf("%s[%d].envs[name=%s]", scope, index, name)
		var writeTarget *InspectWriteTarget
		if canEdit {
			writeTarget = &InspectWriteTarget{
				Document: source.Path, YAMLPath: path, Scope: "app_" + strings.TrimSuffix(scope, "s") + "_env",
				SourceIndex: index, NameAssertion: sourceContainerName(source, scope, index), ExpectedHash: source.ContentHash,
			}
		}
		result = append(result, InspectEnvEntry{
			Name: name, Effective: env, Origins: origins,
			WriteTarget:  writeTarget,
			Capabilities: InspectCapabilities{CanOverride: canEdit, CanReset: canEdit && localIndex >= 0, CanEditSource: canEdit, Reason: reason},
		})
	}
	return result
}

func inspectEnvOrigins(name, effectiveContainerName string, source, defaults SourceDocument, scope string, index int) ([]InspectOrigin, int) {
	target := fmt.Sprintf("%s[%d].envs[name=%s]", scope, index, name)
	origins := []InspectOrigin{}
	container := sourceContainer(source, scope, index)
	if scope == "containers" {
		for _, selector := range []string{"*", effectiveContainerName} {
			for _, raw := range inspectAnySlice(defaults.Model["container_envs"]) {
				group, _ := raw.(map[string]any)
				groupSelector := strings.TrimSpace(fmt.Sprint(group["container_ref_name"]))
				if groupSelector == selector && namedItemIndex(group["envs"], name) >= 0 {
					origins = append(origins, InspectOrigin{Kind: "container_env_defaults", Document: defaults.Path, YAMLPath: "container_envs", DefinitionName: groupSelector, Target: target})
				}
			}
		}
		for _, ref := range stringValues(container["profile_ref_names"]) {
			if definitionHasNamedItem(defaults, "container_profiles", ref, "envs", name) {
				origins = append(origins, InspectOrigin{Kind: "container_profile", Document: defaults.Path, YAMLPath: "container_profiles", DefinitionName: ref, Target: target})
			}
		}
	}
	localIndex := namedItemIndex(container["envs"], name)
	if localIndex >= 0 {
		origins = append(origins, InspectOrigin{Kind: "local", Document: source.Path, YAMLPath: fmt.Sprintf("%s[%d].envs[%d]", scope, index, localIndex), DefinitionName: name, Target: target})
	}
	return origins, localIndex
}

func effectiveInspectionContainer(container containerSpec) EffectiveContainer {
	return EffectiveContainer{
		Name: container.Name, Image: container.Image,
		Startup: genericYAMLMap(container.Startup), Envs: genericYAMLList(effectiveContainerEnvs(container)),
		Resources: genericNestedMap(container.Resources), Ports: genericYAMLList(container.Ports),
		Probes: genericYAMLMap(container.Probes), Runtime: genericYAMLMap(container.Runtime),
		RuntimeAssetRefNames: container.RuntimeAssetRefs, Assets: genericYAMLList(container.Assets),
		Mounts: genericYAMLList(container.Mounts), EnvFrom: genericYAMLList(container.EnvFrom),
		SecurityContext: container.SecurityContext, Raw: container.Raw,
	}
}

func inspectContainerFields(container EffectiveContainer, source, defaults SourceDocument, opts Options, scope string, index int) map[string]InspectField {
	values := map[string]any{
		"image": container.Image, "startup": container.Startup, "envs": container.Envs,
		"resources": container.Resources, "ports": container.Ports, "probes": container.Probes,
		"runtime": container.Runtime, "runtime_asset_ref_names": container.RuntimeAssetRefNames,
		"assets": container.Assets, "mounts": container.Mounts, "env_from": container.EnvFrom,
		"security_context": container.SecurityContext, "raw": container.Raw,
	}
	result := make(map[string]InspectField, len(values))
	for field, value := range values {
		path := fmt.Sprintf("%s[%d].%s", scope, index, field)
		_, local := source.Fields[path]
		origins := inspectFieldOrigins(source, defaults, opts, scope, index, field, local)
		reason := ""
		canEdit := scope == "containers"
		if !canEdit {
			reason = "effective sidecar index is not a safe source selector"
		}
		var writeTarget *InspectWriteTarget
		if canEdit {
			writeTarget = &InspectWriteTarget{
				Document: source.Path, YAMLPath: path, Scope: "app_" + strings.TrimSuffix(scope, "s"),
				SourceIndex: index, NameAssertion: sourceContainerName(source, scope, index), ExpectedHash: source.ContentHash,
			}
		}
		result[field] = InspectField{
			Effective:    value,
			Origins:      origins,
			WriteTarget:  writeTarget,
			Capabilities: InspectCapabilities{CanOverride: canEdit, CanReset: canEdit && local, CanEditSource: canEdit, Reason: reason},
		}
	}
	return result
}

func inspectFieldOrigins(source, defaults SourceDocument, opts Options, scope string, index int, field string, local bool) []InspectOrigin {
	target := fmt.Sprintf("%s[%d].%s", scope, index, field)
	origins := []InspectOrigin{}
	container := sourceContainer(source, scope, index)
	if scope == "containers" {
		for _, ref := range stringValues(container["profile_ref_names"]) {
			if definitionHasField(defaults, "container_profiles", ref, field) {
				origins = append(origins, InspectOrigin{Kind: "container_profile", Document: defaults.Path, YAMLPath: "container_profiles", DefinitionName: ref, Target: target})
			}
		}
		if field == "envs" && len(inspectAnySlice(defaults.Model["container_envs"])) > 0 {
			origins = append(origins, InspectOrigin{Kind: "container_env_defaults", Document: defaults.Path, YAMLPath: "container_envs", Target: target})
		}
	}
	if local {
		origins = append(origins, InspectOrigin{Kind: "local", Document: source.Path, YAMLPath: target, Target: target})
	}
	if field == "resources" && strings.TrimSpace(opts.ResourcePolicyRoot) != "" {
		origins = append(origins, InspectOrigin{Kind: "external_resource_policy", Document: filepath.ToSlash(filepath.Join(opts.ResourcePolicyRoot, opts.Environment, "apps", filepath.Base(source.Path))), YAMLPath: scope, Target: target})
	}
	if field == "image" && strings.TrimSpace(opts.ReleaseManifest) != "" {
		origins = append(origins, InspectOrigin{Kind: "release_manifest", Document: filepath.ToSlash(opts.ReleaseManifest), YAMLPath: "applications", Target: target})
	}
	return origins
}

func sourceContainer(source SourceDocument, scope string, index int) map[string]any {
	items := inspectAnySlice(source.Model[scope])
	if index < 0 || index >= len(items) {
		return nil
	}
	result, _ := items[index].(map[string]any)
	return result
}

func sourceContainerName(source SourceDocument, scope string, index int) string {
	container := sourceContainer(source, scope, index)
	if container == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(container["name"]))
}

func definitionHasField(defaults SourceDocument, catalog, name, field string) bool {
	for _, raw := range inspectAnySlice(defaults.Model[catalog]) {
		definition, _ := raw.(map[string]any)
		if strings.TrimSpace(fmt.Sprint(definition["name"])) != name {
			continue
		}
		body, _ := definition["defaults"].(map[string]any)
		if catalog == "sidecar_definitions" {
			body = definition
		}
		_, ok := body[field]
		return ok
	}
	return false
}

func definitionHasNamedItem(defaults SourceDocument, catalog, definitionName, field, itemName string) bool {
	for _, raw := range inspectAnySlice(defaults.Model[catalog]) {
		definition, _ := raw.(map[string]any)
		if strings.TrimSpace(fmt.Sprint(definition["name"])) != definitionName {
			continue
		}
		body, _ := definition["defaults"].(map[string]any)
		return namedItemIndex(body[field], itemName) >= 0
	}
	return false
}

func namedItemIndex(value any, name string) int {
	for index, raw := range inspectAnySlice(value) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(fmt.Sprint(item["name"])) == name {
			return index
		}
	}
	return -1
}

func genericYAMLMap(value any) map[string]any {
	data, _ := yaml.Marshal(value)
	var result map[string]any
	_ = yaml.Unmarshal(data, &result)
	return compactMap(result)
}

func genericNestedMap(value any) map[string]any { return genericYAMLMap(value) }

func genericYAMLList(value any) []map[string]any {
	data, _ := yaml.Marshal(value)
	var result []map[string]any
	_ = yaml.Unmarshal(data, &result)
	for index := range result {
		result[index] = compactMap(result[index])
	}
	return result
}

func compactMap(value map[string]any) map[string]any {
	result := map[string]any{}
	for key, item := range value {
		switch typed := item.(type) {
		case map[string]any:
			if nested := compactMap(typed); len(nested) > 0 {
				result[key] = nested
			}
		case []any:
			if len(typed) > 0 {
				result[key] = typed
			}
		case string:
			if typed != "" {
				result[key] = typed
			}
		case nil:
		default:
			result[key] = item
		}
	}
	return result
}

func inspectionOrigins(source SourceDocument, defaultsPath string, opts Options) []InspectOrigin {
	if source.Model == nil {
		return nil
	}
	origins := []InspectOrigin{}
	doc := source.Path
	for index, raw := range inspectAnySlice(source.Model["containers"]) {
		container, _ := raw.(map[string]any)
		target := fmt.Sprintf("containers[%d]", index)
		for _, ref := range stringValues(container["profile_ref_names"]) {
			origins = append(origins, InspectOrigin{Kind: "container_profile", Document: filepath.ToSlash(defaultsPath), YAMLPath: "container_profiles", DefinitionName: ref, Target: target})
		}
		origins = append(origins, InspectOrigin{Kind: "local", Document: doc, YAMLPath: target, Target: target})
	}
	for _, ref := range stringValues(source.Model["sidecar_ref_names"]) {
		origins = append(origins, InspectOrigin{Kind: "sidecar_definition", Document: filepath.ToSlash(defaultsPath), YAMLPath: "sidecar_definitions", DefinitionName: ref, Target: "sidecars"})
	}
	for index := range inspectAnySlice(source.Model["sidecars"]) {
		target := fmt.Sprintf("sidecars[%d]", index)
		origins = append(origins, InspectOrigin{Kind: "local", Document: doc, YAMLPath: target, Target: target})
	}
	for _, ref := range stringValues(source.Model["runtime_asset_ref_names"]) {
		origins = append(origins, InspectOrigin{Kind: "runtime_asset_definition", Document: filepath.ToSlash(defaultsPath), YAMLPath: "runtime_asset_definitions", DefinitionName: ref, Target: "containers"})
	}
	if strings.TrimSpace(opts.ResourcePolicyRoot) != "" {
		origins = append(origins, InspectOrigin{Kind: "external_resource_policy", Document: filepath.ToSlash(filepath.Join(opts.ResourcePolicyRoot, opts.Environment, "apps", filepath.Base(source.Path))), YAMLPath: "containers", Target: "containers+sidecars"})
	}
	if strings.TrimSpace(opts.ReleaseManifest) != "" {
		origins = append(origins, InspectOrigin{Kind: "release_manifest", Document: filepath.ToSlash(opts.ReleaseManifest), YAMLPath: "applications", Target: "images"})
	}
	return origins
}

func stringValues(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func inspectAnySlice(value any) []any {
	items, _ := value.([]any)
	return items
}
