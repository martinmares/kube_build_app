package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type AppDetail struct {
	Env      string `json:"env"`
	FileName string `json:"file_name"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Summary  App    `json:"summary"`
	IsDirty  bool   `json:"is_dirty"`
}

type AssetDetail struct {
	Env          string `json:"env"`
	RelativePath string `json:"relative_path"`
	Path         string `json:"path"`
	Content      string `json:"content"`
	IsDirty      bool   `json:"is_dirty"`
}

type AppRendered struct {
	Env      string `json:"env"`
	FileName string `json:"file_name"`
	Content  string `json:"content"`
}

type AppVars struct {
	Env      string    `json:"env"`
	FileName string    `json:"file_name"`
	Items    []VarItem `json:"items"`
}

type VarItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SpecialEntries struct {
	Env         string         `json:"env"`
	SpecialFile string         `json:"special_file"`
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

type AppModel struct {
	Env        string           `json:"env"`
	FileName   string           `json:"file_name"`
	AppName    *string          `json:"app_name"`
	Replicas   *int             `json:"replicas"`
	Containers []ContainerModel `json:"containers"`
}

type ContainerModel struct {
	Index            int           `json:"index"`
	Name             *string       `json:"name"`
	StartupCommand   []string      `json:"startup_command"`
	StartupArguments []string      `json:"startup_arguments"`
	EnvVars          []EnvVarModel `json:"env_vars"`
	Resources        ResourceModel `json:"resources"`
	Ports            []PortModel   `json:"ports"`
}

type EnvVarModel struct {
	Index           int     `json:"index"`
	Name            *string `json:"name"`
	Kind            string  `json:"kind"`
	Value           *string `json:"value"`
	Key             *string `json:"key"`
	SecretName      *string `json:"secret_name"`
	ResourceName    *string `json:"resource_name"`
	Divisor         *string `json:"divisor"`
	FieldPath       *string `json:"field_path"`
	IsValueEditable bool    `json:"is_value_editable"`
}

type ResourceModel struct {
	CPUFrom    *string `json:"cpu_from"`
	CPUTo      *string `json:"cpu_to"`
	MemoryFrom *string `json:"memory_from"`
	MemoryTo   *string `json:"memory_to"`
}

type PortModel struct {
	Index    int             `json:"index"`
	Name     *string         `json:"name"`
	Port     *string         `json:"port"`
	ExposeAs []ExposeAsModel `json:"expose_as"`
}

type ExposeAsModel struct {
	Index          int             `json:"index"`
	Hostname       *string         `json:"hostname"`
	Port           *string         `json:"port"`
	ServiceType    *string         `json:"service_type"`
	IngressEnabled bool            `json:"ingress_enabled"`
	ExternalCount  int             `json:"external_count"`
	Externals      []ExternalModel `json:"externals"`
}

type ExternalModel struct {
	Index        int     `json:"index"`
	Name         *string `json:"name"`
	AsRoute      bool    `json:"as_route"`
	HTTPHostname *string `json:"http_hostname"`
	HTTPPath     *string `json:"http_path"`
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
	return AppDetail{Env: envName, FileName: filepath.Base(path), Path: path, Content: string(content), Summary: summary}, nil
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
	return AppVars{Env: envName, FileName: detail.FileName, Items: extractVars(detail.Content)}, nil
}

func (r *Repository) AppModel(envName string, appFile string) (AppModel, error) {
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return AppModel{}, err
	}
	var root any
	if err := yaml.Unmarshal([]byte(detail.Content), &root); err != nil {
		return AppModel{}, err
	}
	rootMap, _ := root.(map[string]any)
	model := AppModel{Env: envName, FileName: detail.FileName}
	model.AppName = stringPtr(stringValue(rootMap["name"]))
	model.Replicas = intPtr(intValue(rootMap["replicas"]))

	for cIdx, rawContainer := range anySlice(rootMap["containers"]) {
		containerMap, _ := rawContainer.(map[string]any)
		container := ContainerModel{
			Index:            cIdx,
			Name:             stringPtr(stringValue(containerMap["name"])),
			StartupCommand:   stringSlice(nestedValue(containerMap, "startup", "command")),
			StartupArguments: stringSlice(nestedValue(containerMap, "startup", "arguments")),
			Resources: ResourceModel{
				CPUFrom:    stringPtr(nestedString(containerMap, "resources", "cpu", "from")),
				CPUTo:      stringPtr(nestedString(containerMap, "resources", "cpu", "to")),
				MemoryFrom: stringPtr(nestedString(containerMap, "resources", "memory", "from")),
				MemoryTo:   stringPtr(nestedString(containerMap, "resources", "memory", "to")),
			},
		}
		for idx, rawEnv := range anySlice(containerMap["env_vars"]) {
			envMap, _ := rawEnv.(map[string]any)
			container.EnvVars = append(container.EnvVars, envVarModel(idx, envMap))
		}
		for pIdx, rawPort := range anySlice(containerMap["ports"]) {
			portMap, _ := rawPort.(map[string]any)
			port := PortModel{Index: pIdx, Name: stringPtr(stringValue(portMap["name"])), Port: stringPtr(stringValue(portMap["port"]))}
			for eIdx, rawExpose := range anySlice(portMap["expose_as"]) {
				exposeMap, _ := rawExpose.(map[string]any)
				expose := ExposeAsModel{Index: eIdx, Hostname: stringPtr(stringValue(exposeMap["hostname"])), Port: stringPtr(stringValue(exposeMap["port"])), ServiceType: stringPtr(stringValue(exposeMap["type"]))}
				for xIdx, rawExternal := range anySlice(exposeMap["external"]) {
					externalMap, _ := rawExternal.(map[string]any)
					external := ExternalModel{Index: xIdx, Name: stringPtr(stringValue(externalMap["name"])), AsRoute: boolValue(externalMap["as_route"])}
					if httpItems := anySlice(externalMap["http"]); len(httpItems) > 0 {
						httpMap, _ := httpItems[0].(map[string]any)
						external.HTTPHostname = stringPtr(stringValue(httpMap["hostname"]))
						external.HTTPPath = stringPtr(stringValue(httpMap["path"]))
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
	return model, nil
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
	return AssetDetail{Env: envName, RelativePath: rel, Path: path, Content: string(content)}, nil
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
	rawSection, ok := root[kind.rootKey].(map[string]any)
	if !ok {
		return SpecialEntries{}, fmt.Errorf("missing object at .%s", kind.rootKey)
	}
	out := SpecialEntries{Env: envName, SpecialFile: specialFile, Editable: !kind.secured}
	if kind.secured {
		warning := "secured file is shown without decrypt; edit flow requires EncJson preflight"
		out.Warning = &warning
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

func renderVarsPreview(content string) string {
	lines, start, end := splitVarsBlock(content)
	vars := parseVarsLines(lines)
	if start >= 0 && end >= 0 {
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

func envVarModel(index int, envMap map[string]any) EnvVarModel {
	model := EnvVarModel{Index: index, Name: stringPtr(stringValue(envMap["name"])), Kind: "value", IsValueEditable: true}
	if value := stringValue(envMap["value"]); value != "" {
		model.Value = &value
		return model
	}
	valueFrom, _ := envMap["valueFrom"].(map[string]any)
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
