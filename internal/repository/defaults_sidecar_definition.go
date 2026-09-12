package repository

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type DefaultsSidecarDefinition struct {
	Env         string `json:"env"`
	FileName    string `json:"file_name"`
	Name        string `json:"name"`
	ContentHash string `json:"content_hash"`
	Image       string `json:"image"`
}

type DefaultsSidecarDefinitionUpdate struct {
	Image string `json:"image"`
}

type DefaultsSidecarDefinitionStartup struct {
	Env         string               `json:"env"`
	FileName    string               `json:"file_name"`
	Name        string               `json:"name"`
	ContentHash string               `json:"content_hash"`
	Startup     *SidecarStartupModel `json:"startup,omitempty"`
}

type DefaultsSidecarDefinitionStartupUpdate struct {
	Action  string              `json:"action"`
	Startup SidecarStartupModel `json:"startup"`
}

type DefaultsSidecarDefinitionResources struct {
	Env         string         `json:"env"`
	FileName    string         `json:"file_name"`
	Name        string         `json:"name"`
	ContentHash string         `json:"content_hash"`
	Resources   *ResourceModel `json:"resources,omitempty"`
}

type DefaultsSidecarDefinitionResourcesUpdate struct {
	Action    string         `json:"action"`
	Resources ResourceUpdate `json:"resources"`
}

func (r *Repository) DefaultsSidecarDefinition(envName, name string) (DefaultsSidecarDefinition, error) {
	if !referenceNamePattern.MatchString(name) {
		return DefaultsSidecarDefinition{}, errors.New("invalid sidecar definition name")
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	detail, err := r.AssetDetail(envName, defaultsFile)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	root, err := rawSourceModelRoot(detail.Content)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	definition, err := uniqueNamedSourceItem(root["sidecar_definitions"], name)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	image, ok := definition["image"].(string)
	if !ok && definition["image"] != nil {
		return DefaultsSidecarDefinition{}, fmt.Errorf("sidecar definition %q image must be a string", name)
	}
	return DefaultsSidecarDefinition{
		Env: envName, FileName: defaultsFile, Name: name,
		ContentHash: detail.ContentHash, Image: image,
	}, nil
}

func rawSourceModelRoot(content string) (map[string]any, error) {
	templates := map[string]string{}
	index := 0
	preview := sourceTemplatePattern.ReplaceAllStringFunc(content, func(value string) string {
		placeholder := fmt.Sprintf("__KUBE_EDIT_RAW_TEMPLATE_%d__", index)
		index++
		templates[placeholder] = value
		return placeholder
	})
	var root any
	if err := yaml.Unmarshal([]byte(preview), &root); err != nil {
		return nil, err
	}
	restored, _ := restoreRawSourceTemplates(root, templates).(map[string]any)
	return restored, nil
}

func restoreRawSourceTemplates(value any, templates map[string]string) any {
	switch typed := value.(type) {
	case string:
		for placeholder, template := range templates {
			typed = strings.ReplaceAll(typed, placeholder, template)
		}
		return typed
	case []any:
		for index, item := range typed {
			typed[index] = restoreRawSourceTemplates(item, templates)
		}
		return typed
	case map[string]any:
		for key, item := range typed {
			typed[key] = restoreRawSourceTemplates(item, templates)
		}
		return typed
	default:
		return value
	}
}

func (r *Repository) DefaultsSidecarDefinitionStartup(envName, name string) (DefaultsSidecarDefinitionStartup, error) {
	if !referenceNamePattern.MatchString(name) {
		return DefaultsSidecarDefinitionStartup{}, errors.New("invalid sidecar definition name")
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	detail, err := r.AssetDetail(envName, defaultsFile)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	root, err := rawSourceModelRoot(detail.Content)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	definition, err := uniqueNamedSourceItem(root["sidecar_definitions"], name)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	out := DefaultsSidecarDefinitionStartup{Env: envName, FileName: defaultsFile, Name: name, ContentHash: detail.ContentHash}
	rawStartup, present := definition["startup"]
	if !present {
		return out, nil
	}
	startup, ok := rawStartup.(map[string]any)
	if !ok {
		return DefaultsSidecarDefinitionStartup{}, fmt.Errorf("sidecar definition %q startup must be a mapping", name)
	}
	if err := rejectUnknownKeys(startup, "startup", "command", "arguments"); err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	command, commandPresent, err := strictStringList(startup, "command")
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, fmt.Errorf("sidecar definition %q startup: %w", name, err)
	}
	arguments, argumentsPresent, err := strictStringList(startup, "arguments")
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, fmt.Errorf("sidecar definition %q startup: %w", name, err)
	}
	out.Startup = &SidecarStartupModel{CommandPresent: commandPresent, Command: command, ArgumentsPresent: argumentsPresent, Arguments: arguments}
	return out, nil
}

func strictStringList(values map[string]any, key string) ([]string, bool, error) {
	raw, present := values[key]
	if !present {
		return nil, false, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, true, fmt.Errorf("%s must be a sequence", key)
	}
	out := make([]string, 0, len(items))
	for index, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, true, fmt.Errorf("%s[%d] must be a string", key, index)
		}
		out = append(out, value)
	}
	return out, true, nil
}

func (r *Repository) DefaultsSidecarDefinitionResources(envName, name string) (DefaultsSidecarDefinitionResources, error) {
	if !referenceNamePattern.MatchString(name) {
		return DefaultsSidecarDefinitionResources{}, errors.New("invalid sidecar definition name")
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	detail, err := r.AssetDetail(envName, defaultsFile)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	root, err := rawSourceModelRoot(detail.Content)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	definition, err := uniqueNamedSourceItem(root["sidecar_definitions"], name)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	out := DefaultsSidecarDefinitionResources{Env: envName, FileName: defaultsFile, Name: name, ContentHash: detail.ContentHash}
	rawResources, present := definition["resources"]
	if !present {
		return out, nil
	}
	resources, ok := rawResources.(map[string]any)
	if !ok {
		return DefaultsSidecarDefinitionResources{}, fmt.Errorf("sidecar definition %q resources must be a mapping", name)
	}
	if err := validateEditableResourcesMap(resources); err != nil {
		return DefaultsSidecarDefinitionResources{}, fmt.Errorf("sidecar definition %q: %w", name, err)
	}
	model := resourceModel(resources)
	out.Resources = &model
	return out, nil
}

func (r *Repository) UpdateDefaultsSidecarDefinitionResources(envName, name string, update DefaultsSidecarDefinitionResourcesUpdate, expectedHash string) (DefaultsSidecarDefinitionResources, error) {
	current, err := r.DefaultsSidecarDefinitionResources(envName, name)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return DefaultsSidecarDefinitionResources{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	action := strings.TrimSpace(update.Action)
	if action != "set" && action != "remove" {
		return DefaultsSidecarDefinitionResources{}, errors.New("action must be set or remove")
	}
	if action == "set" {
		if err := validateDefinitionResourceUpdate(update.Resources); err != nil {
			return DefaultsSidecarDefinitionResources{}, err
		}
		if resourceUpdateEmpty(update.Resources) {
			return DefaultsSidecarDefinitionResources{}, errors.New("at least one resource value is required; use remove to delete the resources block")
		}
		if current.Resources != nil && reflect.DeepEqual(resourceUpdateFromModel(*current.Resources), cleanResourceUpdate(update.Resources)) {
			return current, nil
		}
	} else if current.Resources == nil {
		return current, nil
	}
	path, _, err := r.AssetPath(envName, current.FileName)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return DefaultsSidecarDefinitionResources{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceDefaultsSidecarDefinitionResources(string(contentBytes), name, action, update.Resources, current.Resources)
	if err != nil {
		return DefaultsSidecarDefinitionResources{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return DefaultsSidecarDefinitionResources{}, err
		}
	}
	return r.DefaultsSidecarDefinitionResources(envName, name)
}

func cleanResourceUpdate(resources ResourceUpdate) ResourceUpdate {
	return ResourceUpdate{
		CPURequest: strings.TrimSpace(resources.CPURequest), CPULimit: strings.TrimSpace(resources.CPULimit),
		MemoryRequest: strings.TrimSpace(resources.MemoryRequest), MemoryLimit: strings.TrimSpace(resources.MemoryLimit),
		EphemeralStorageRequest: strings.TrimSpace(resources.EphemeralStorageRequest), EphemeralStorageLimit: strings.TrimSpace(resources.EphemeralStorageLimit),
	}
}

func resourceUpdateFromModel(resources ResourceModel) ResourceUpdate {
	value := func(item *string) string {
		if item == nil {
			return ""
		}
		return *item
	}
	return cleanResourceUpdate(ResourceUpdate{
		CPURequest: value(resources.CPURequest), CPULimit: value(resources.CPULimit),
		MemoryRequest: value(resources.MemoryRequest), MemoryLimit: value(resources.MemoryLimit),
		EphemeralStorageRequest: value(resources.EphemeralStorageRequest), EphemeralStorageLimit: value(resources.EphemeralStorageLimit),
	})
}

func validateDefinitionResourceUpdate(resources ResourceUpdate) error {
	resources = cleanResourceUpdate(resources)
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

func (r *Repository) UpdateDefaultsSidecarDefinitionStartup(envName, name string, update DefaultsSidecarDefinitionStartupUpdate, expectedHash string) (DefaultsSidecarDefinitionStartup, error) {
	current, err := r.DefaultsSidecarDefinitionStartup(envName, name)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return DefaultsSidecarDefinitionStartup{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	action := strings.TrimSpace(update.Action)
	if action != "set" && action != "remove" {
		return DefaultsSidecarDefinitionStartup{}, errors.New("action must be set or remove")
	}
	if action == "set" && !update.Startup.CommandPresent && !update.Startup.ArgumentsPresent {
		return DefaultsSidecarDefinitionStartup{}, errors.New("select command or arguments to define; use remove to delete the startup block")
	}
	if action == "set" && current.Startup != nil && reflect.DeepEqual(*current.Startup, update.Startup) {
		return current, nil
	}
	if action == "remove" && current.Startup == nil {
		return current, nil
	}
	path, _, err := r.AssetPath(envName, current.FileName)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return DefaultsSidecarDefinitionStartup{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceDefaultsSidecarDefinitionStartup(string(contentBytes), name, action, update.Startup)
	if err != nil {
		return DefaultsSidecarDefinitionStartup{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return DefaultsSidecarDefinitionStartup{}, err
		}
	}
	return r.DefaultsSidecarDefinitionStartup(envName, name)
}

func (r *Repository) UpdateDefaultsSidecarDefinition(envName, name string, update DefaultsSidecarDefinitionUpdate, expectedHash string) (DefaultsSidecarDefinition, error) {
	current, err := r.DefaultsSidecarDefinition(envName, name)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return DefaultsSidecarDefinition{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	image := strings.TrimSpace(update.Image)
	if image == "" {
		return DefaultsSidecarDefinition{}, errors.New("sidecar definition image is required")
	}
	if image == current.Image {
		return current, nil
	}
	path, _, err := r.AssetPath(envName, current.FileName)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return DefaultsSidecarDefinition{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceDefaultsSidecarDefinitionImage(string(contentBytes), name, image)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return DefaultsSidecarDefinition{}, err
		}
	}
	return r.DefaultsSidecarDefinition(envName, name)
}

func uniqueNamedSourceItem(value any, name string) (map[string]any, error) {
	var found map[string]any
	for _, raw := range anySlice(value) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(stringValue(item["name"])) != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("duplicate sidecar definition %q", name)
		}
		found = item
	}
	if found == nil {
		return nil, fmt.Errorf("sidecar definition %q not found", name)
	}
	return found, nil
}

func replaceDefaultsSidecarDefinitionImage(content, name, image string) (string, error) {
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	index := -1
	for itemIndex, raw := range anySlice(root["sidecar_definitions"]) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(stringValue(item["name"])) == name {
			if index >= 0 {
				return "", fmt.Errorf("duplicate sidecar definition %q", name)
			}
			index = itemIndex
		}
	}
	if index < 0 {
		return "", fmt.Errorf("sidecar definition %q not found", name)
	}
	lines, start, end, err := collectionItemBlockRange(content, "sidecar_definitions", index)
	if err != nil {
		return "", err
	}
	imageStart, imageEnd := containerChildBlockRange(lines, start, end, "image")
	replacement := "    image: " + strconv.Quote(image)
	if imageStart < 0 {
		lines = insertLines(lines, start+1, []string{replacement})
	} else {
		lines = replaceLineRange(lines, imageStart, imageEnd, []string{replacement})
	}
	return joinLikeSource(lines, content), nil
}

func replaceDefaultsSidecarDefinitionStartup(content, name, action string, startup SidecarStartupModel) (string, error) {
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	index := -1
	for itemIndex, raw := range anySlice(root["sidecar_definitions"]) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(stringValue(item["name"])) == name {
			if index >= 0 {
				return "", fmt.Errorf("duplicate sidecar definition %q", name)
			}
			index = itemIndex
		}
	}
	if index < 0 {
		return "", fmt.Errorf("sidecar definition %q not found", name)
	}
	lines, start, end, err := collectionItemBlockRange(content, "sidecar_definitions", index)
	if err != nil {
		return "", err
	}
	startupStart, startupEnd := containerChildBlockRange(lines, start, end, "startup")
	if startupStart >= 0 {
		lines = append(lines[:startupStart], lines[startupEnd:]...)
	}
	if action == "set" {
		insertAt := start + 1
		if startupStart >= 0 {
			insertAt = startupStart
		}
		lines = insertLines(lines, insertAt, renderSidecarStartupBlock(startup))
	}
	return joinLikeSource(lines, content), nil
}

func replaceDefaultsSidecarDefinitionResources(content, name, action string, resources ResourceUpdate, current *ResourceModel) (string, error) {
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	index := -1
	for itemIndex, raw := range anySlice(root["sidecar_definitions"]) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(stringValue(item["name"])) == name {
			if index >= 0 {
				return "", fmt.Errorf("duplicate sidecar definition %q", name)
			}
			index = itemIndex
		}
	}
	if index < 0 {
		return "", fmt.Errorf("sidecar definition %q not found", name)
	}
	lines, start, end, err := collectionItemBlockRange(content, "sidecar_definitions", index)
	if err != nil {
		return "", err
	}
	resourcesStart, resourcesEnd := containerChildBlockRange(lines, start, end, "resources")
	if resourcesStart >= 0 {
		lines = append(lines[:resourcesStart], lines[resourcesEnd:]...)
	}
	if action == "set" {
		insertAt := start + 1
		if resourcesStart >= 0 {
			insertAt = resourcesStart
		}
		lines = insertLines(lines, insertAt, renderDefaultsSidecarResourcesBlock(resources, current))
	}
	return joinLikeSource(lines, content), nil
}

func renderDefaultsSidecarResourcesBlock(resources ResourceUpdate, current *ResourceModel) []string {
	resources = cleanResourceUpdate(resources)
	lines := []string{"    resources:"}
	appendResource := func(name, request, limit string, aliases bool) {
		if request == "" && limit == "" {
			return
		}
		lines = append(lines, "      "+name+":")
		requestKey, limitKey := "requests", "limits"
		if aliases {
			requestKey, limitKey = "from", "to"
		}
		if request != "" {
			lines = append(lines, "        "+requestKey+": "+strconv.Quote(request))
		}
		if limit != "" {
			lines = append(lines, "        "+limitKey+": "+strconv.Quote(limit))
		}
	}
	cpuAliases := current != nil && (current.CPUFrom != nil || current.CPUTo != nil)
	memoryAliases := current != nil && (current.MemoryFrom != nil || current.MemoryTo != nil)
	ephemeralAliases := current != nil && (current.EphemeralStorageFrom != nil || current.EphemeralStorageTo != nil)
	appendResource("cpu", resources.CPURequest, resources.CPULimit, cpuAliases)
	appendResource("memory", resources.MemoryRequest, resources.MemoryLimit, memoryAliases)
	appendResource("ephemeral-storage", resources.EphemeralStorageRequest, resources.EphemeralStorageLimit, ephemeralAliases)
	return lines
}
