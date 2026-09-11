package repository

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type SidecarEnvOverride struct {
	Env              string       `json:"env"`
	FileName         string       `json:"file_name"`
	SidecarName      string       `json:"sidecar_name"`
	EnvName          string       `json:"env_name"`
	ContentHash      string       `json:"content_hash"`
	DefaultsHash     string       `json:"defaults_hash"`
	SharedDefinition bool         `json:"shared_definition"`
	LocalPatch       bool         `json:"local_patch"`
	Local            *EnvVarModel `json:"local,omitempty"`
}

type SidecarEnvOverrideUpdate struct {
	Action string `json:"action"`
	Value  string `json:"value"`
}

type SidecarResourcesOverride struct {
	Env              string         `json:"env"`
	FileName         string         `json:"file_name"`
	SidecarName      string         `json:"sidecar_name"`
	ContentHash      string         `json:"content_hash"`
	DefaultsHash     string         `json:"defaults_hash"`
	SharedDefinition bool           `json:"shared_definition"`
	LocalPatch       bool           `json:"local_patch"`
	Local            *ResourceModel `json:"local,omitempty"`
}

type SidecarResourcesOverrideUpdate struct {
	Action    string         `json:"action"`
	Resources ResourceUpdate `json:"resources"`
}

type SidecarStartupModel struct {
	CommandPresent   bool     `json:"command_present"`
	Command          []string `json:"command"`
	ArgumentsPresent bool     `json:"arguments_present"`
	Arguments        []string `json:"arguments"`
}

type SidecarStartupOverride struct {
	Env              string               `json:"env"`
	FileName         string               `json:"file_name"`
	SidecarName      string               `json:"sidecar_name"`
	ContentHash      string               `json:"content_hash"`
	DefaultsHash     string               `json:"defaults_hash"`
	SharedDefinition bool                 `json:"shared_definition"`
	LocalPatch       bool                 `json:"local_patch"`
	Local            *SidecarStartupModel `json:"local,omitempty"`
}

type SidecarStartupOverrideUpdate struct {
	Action  string              `json:"action"`
	Startup SidecarStartupModel `json:"startup"`
}

func (r *Repository) AppSidecarStartupOverride(envName, appFile, sidecarName string) (SidecarStartupOverride, error) {
	if !referenceNamePattern.MatchString(sidecarName) {
		return SidecarStartupOverride{}, errors.New("invalid sidecar name")
	}
	references, err := r.AppReferences(envName, appFile)
	if err != nil {
		return SidecarStartupOverride{}, err
	}
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return SidecarStartupOverride{}, err
	}
	root, err := sourceModelRoot(detail.Content)
	if err != nil {
		return SidecarStartupOverride{}, err
	}
	shared := stringSet(references.Catalog.SidecarDefinitions)[sidecarName]
	selected := stringSet(references.SidecarRefNames)[sidecarName]
	localIndex, localSidecar := namedSourceItem(root["sidecars"], sidecarName)
	if shared && !selected {
		return SidecarStartupOverride{}, fmt.Errorf("shared sidecar %q is not selected in sidecar_ref_names", sidecarName)
	}
	if !shared && localIndex < 0 {
		return SidecarStartupOverride{}, fmt.Errorf("sidecar %q is neither a selected shared sidecar nor an app-local sidecar", sidecarName)
	}
	out := SidecarStartupOverride{
		Env: envName, FileName: detail.FileName, SidecarName: sidecarName,
		ContentHash: detail.ContentHash, DefaultsHash: references.DefaultsHash,
		SharedDefinition: shared, LocalPatch: localIndex >= 0,
	}
	if startup, ok := localSidecar["startup"].(map[string]any); ok {
		if err := rejectUnknownKeys(startup, "startup", "command", "arguments"); err != nil {
			return SidecarStartupOverride{}, err
		}
		_, commandPresent := startup["command"]
		_, argumentsPresent := startup["arguments"]
		out.Local = &SidecarStartupModel{
			CommandPresent: commandPresent, Command: stringSlice(startup["command"]),
			ArgumentsPresent: argumentsPresent, Arguments: stringSlice(startup["arguments"]),
		}
	}
	return out, nil
}

func (r *Repository) UpdateAppSidecarStartupOverride(envName, appFile, sidecarName string, update SidecarStartupOverrideUpdate, expectedHash, expectedDefaultsHash string) (SidecarStartupOverride, error) {
	current, err := r.AppSidecarStartupOverride(envName, appFile, sidecarName)
	if err != nil {
		return SidecarStartupOverride{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return SidecarStartupOverride{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if expectedDefaultsHash == "" || expectedDefaultsHash != current.DefaultsHash {
		return SidecarStartupOverride{}, NewConflictError("defaults file changed before save; refresh inherited values and apply the edit again")
	}
	action := strings.TrimSpace(update.Action)
	if action != "set" && action != "reset" {
		return SidecarStartupOverride{}, errors.New("action must be set or reset")
	}
	if action == "set" && !update.Startup.CommandPresent && !update.Startup.ArgumentsPresent {
		return SidecarStartupOverride{}, errors.New("select command or arguments to override; use reset to remove the local startup block")
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return SidecarStartupOverride{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return SidecarStartupOverride{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return SidecarStartupOverride{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceSidecarStartupOverride(string(contentBytes), sidecarName, action, update.Startup, current.SharedDefinition)
	if err != nil {
		return SidecarStartupOverride{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return SidecarStartupOverride{}, err
		}
	}
	return r.AppSidecarStartupOverride(envName, filepath.Base(path), sidecarName)
}

func (r *Repository) AppSidecarResourcesOverride(envName, appFile, sidecarName string) (SidecarResourcesOverride, error) {
	if !referenceNamePattern.MatchString(sidecarName) {
		return SidecarResourcesOverride{}, errors.New("invalid sidecar name")
	}
	references, err := r.AppReferences(envName, appFile)
	if err != nil {
		return SidecarResourcesOverride{}, err
	}
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return SidecarResourcesOverride{}, err
	}
	root, err := sourceModelRoot(detail.Content)
	if err != nil {
		return SidecarResourcesOverride{}, err
	}
	shared := stringSet(references.Catalog.SidecarDefinitions)[sidecarName]
	selected := stringSet(references.SidecarRefNames)[sidecarName]
	localIndex, localSidecar := namedSourceItem(root["sidecars"], sidecarName)
	if shared && !selected {
		return SidecarResourcesOverride{}, fmt.Errorf("shared sidecar %q is not selected in sidecar_ref_names", sidecarName)
	}
	if !shared && localIndex < 0 {
		return SidecarResourcesOverride{}, fmt.Errorf("sidecar %q is neither a selected shared sidecar nor an app-local sidecar", sidecarName)
	}
	out := SidecarResourcesOverride{
		Env: envName, FileName: detail.FileName, SidecarName: sidecarName,
		ContentHash: detail.ContentHash, DefaultsHash: references.DefaultsHash,
		SharedDefinition: shared, LocalPatch: localIndex >= 0,
	}
	if resources, ok := localSidecar["resources"].(map[string]any); ok {
		model := resourceModel(resources)
		out.Local = &model
	}
	return out, nil
}

func (r *Repository) UpdateAppSidecarResourcesOverride(envName, appFile, sidecarName string, update SidecarResourcesOverrideUpdate, expectedHash, expectedDefaultsHash string) (SidecarResourcesOverride, error) {
	current, err := r.AppSidecarResourcesOverride(envName, appFile, sidecarName)
	if err != nil {
		return SidecarResourcesOverride{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return SidecarResourcesOverride{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if expectedDefaultsHash == "" || expectedDefaultsHash != current.DefaultsHash {
		return SidecarResourcesOverride{}, NewConflictError("defaults file changed before save; refresh inherited values and apply the edit again")
	}
	action := strings.TrimSpace(update.Action)
	if action != "set" && action != "reset" {
		return SidecarResourcesOverride{}, errors.New("action must be set or reset")
	}
	if action == "set" {
		if err := validateResourceUpdate(update.Resources); err != nil {
			return SidecarResourcesOverride{}, err
		}
		if resourceUpdateEmpty(update.Resources) {
			return SidecarResourcesOverride{}, errors.New("at least one resource value is required; use reset to remove the local override")
		}
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return SidecarResourcesOverride{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return SidecarResourcesOverride{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return SidecarResourcesOverride{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceSidecarResourcesOverride(string(contentBytes), sidecarName, action, update.Resources, current.SharedDefinition)
	if err != nil {
		return SidecarResourcesOverride{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return SidecarResourcesOverride{}, err
		}
	}
	return r.AppSidecarResourcesOverride(envName, filepath.Base(path), sidecarName)
}

func resourceUpdateEmpty(resources ResourceUpdate) bool {
	return strings.TrimSpace(resources.CPURequest) == "" && strings.TrimSpace(resources.CPULimit) == "" &&
		strings.TrimSpace(resources.MemoryRequest) == "" && strings.TrimSpace(resources.MemoryLimit) == ""
}

func (r *Repository) AppSidecarEnvOverride(envName, appFile, sidecarName, variableName string) (SidecarEnvOverride, error) {
	if !referenceNamePattern.MatchString(sidecarName) {
		return SidecarEnvOverride{}, errors.New("invalid sidecar name")
	}
	if !variableNamePattern.MatchString(variableName) {
		return SidecarEnvOverride{}, errors.New("invalid environment variable name")
	}
	references, err := r.AppReferences(envName, appFile)
	if err != nil {
		return SidecarEnvOverride{}, err
	}
	detail, err := r.AppDetail(envName, appFile)
	if err != nil {
		return SidecarEnvOverride{}, err
	}
	root, err := sourceModelRoot(detail.Content)
	if err != nil {
		return SidecarEnvOverride{}, err
	}
	shared := stringSet(references.Catalog.SidecarDefinitions)[sidecarName]
	selected := stringSet(references.SidecarRefNames)[sidecarName]
	localIndex, localSidecar := namedSourceItem(root["sidecars"], sidecarName)
	if shared && !selected {
		return SidecarEnvOverride{}, fmt.Errorf("shared sidecar %q is not selected in sidecar_ref_names", sidecarName)
	}
	if !shared && localIndex < 0 {
		return SidecarEnvOverride{}, fmt.Errorf("sidecar %q is neither a selected shared sidecar nor an app-local sidecar", sidecarName)
	}
	out := SidecarEnvOverride{
		Env: envName, FileName: detail.FileName, SidecarName: sidecarName, EnvName: variableName,
		ContentHash: detail.ContentHash, DefaultsHash: references.DefaultsHash,
		SharedDefinition: shared, LocalPatch: localIndex >= 0,
	}
	if _, localEnv := namedSourceItem(localSidecar["envs"], variableName); localEnv != nil {
		model := envVarModel(0, localEnv)
		out.Local = &model
	}
	return out, nil
}

func (r *Repository) UpdateAppSidecarEnvOverride(envName, appFile, sidecarName, variableName string, update SidecarEnvOverrideUpdate, expectedHash, expectedDefaultsHash string) (SidecarEnvOverride, error) {
	current, err := r.AppSidecarEnvOverride(envName, appFile, sidecarName, variableName)
	if err != nil {
		return SidecarEnvOverride{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return SidecarEnvOverride{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	if expectedDefaultsHash == "" || expectedDefaultsHash != current.DefaultsHash {
		return SidecarEnvOverride{}, NewConflictError("defaults file changed before save; refresh inherited values and apply the edit again")
	}
	action := strings.TrimSpace(update.Action)
	if action != "set" && action != "reset" && action != "remove" {
		return SidecarEnvOverride{}, errors.New("action must be set, reset, or remove")
	}
	path, err := r.AppPath(envName, appFile)
	if err != nil {
		return SidecarEnvOverride{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return SidecarEnvOverride{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return SidecarEnvOverride{}, NewConflictError("app file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceSidecarEnvOverride(string(contentBytes), sidecarName, variableName, action, update.Value, current.SharedDefinition)
	if err != nil {
		return SidecarEnvOverride{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return SidecarEnvOverride{}, err
		}
	}
	return r.AppSidecarEnvOverride(envName, filepath.Base(path), sidecarName, variableName)
}

func namedSourceItem(value any, name string) (int, map[string]any) {
	for index, raw := range anySlice(value) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(stringValue(item["name"])) == name {
			return index, item
		}
	}
	return -1, nil
}

func replaceSidecarEnvOverride(content, sidecarName, envName, action, value string, shared bool) (string, error) {
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	localIndex, _ := namedSourceItem(root["sidecars"], sidecarName)
	if localIndex < 0 {
		if action == "reset" {
			return content, nil
		}
		return appendSidecarEnvOverride(content, sidecarName, envName, action, value), nil
	}
	lines, start, end, err := collectionItemBlockRange(content, "sidecars", localIndex)
	if err != nil {
		return "", err
	}
	envStart, envEnd := containerChildBlockRange(lines, start, end, "envs")
	replacement := renderSidecarEnvItem(envName, action, value)
	if envStart < 0 {
		if action == "reset" {
			return content, nil
		}
		block := append([]string{"    envs:"}, replacement...)
		lines = insertLines(lines, start+1, block)
	} else {
		itemStart, itemEnd, itemCount := sidecarEnvItemRange(lines, envStart, envEnd, envName)
		if itemStart < 0 {
			if action == "reset" {
				return content, nil
			}
			lines = insertLines(lines, envEnd, replacement)
		} else if action == "reset" && itemCount == 1 {
			lines = append(lines[:envStart], lines[envEnd:]...)
		} else {
			lines = replaceLineRange(lines, itemStart, itemEnd, replacement)
		}
	}
	updated := joinLikeSource(lines, content)
	if shared && action == "reset" {
		return cleanupEmptySharedSidecarPatch(updated, sidecarName)
	}
	return updated, nil
}

func replaceSidecarResourcesOverride(content, sidecarName, action string, resources ResourceUpdate, shared bool) (string, error) {
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	localIndex, _ := namedSourceItem(root["sidecars"], sidecarName)
	if localIndex < 0 {
		if action == "reset" {
			return content, nil
		}
		item := append([]string{"  - name: " + sidecarName}, renderResourcesBlock(resources)...)
		lines := strings.Split(content, "\n")
		start, end := topLevelBlockRange(lines, "sidecars")
		if start < 0 {
			return replaceTopLevelBlock(content, "sidecars", append([]string{"sidecars:"}, item...)), nil
		}
		return joinLikeSource(insertLines(lines, end, item), content), nil
	}
	lines, start, end, err := collectionItemBlockRange(content, "sidecars", localIndex)
	if err != nil {
		return "", err
	}
	blockStart, blockEnd := containerChildBlockRange(lines, start, end, "resources")
	if blockStart >= 0 {
		lines = append(lines[:blockStart], lines[blockEnd:]...)
	}
	if action == "set" {
		replacement := renderResourcesBlock(resources)
		insertAt := start + 1
		if blockStart >= 0 {
			insertAt = blockStart
		}
		lines = insertLines(lines, insertAt, replacement)
	}
	updated := joinLikeSource(lines, content)
	if shared && action == "reset" {
		return cleanupEmptySharedSidecarPatch(updated, sidecarName)
	}
	return updated, nil
}

func replaceSidecarStartupOverride(content, sidecarName, action string, startup SidecarStartupModel, shared bool) (string, error) {
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	localIndex, _ := namedSourceItem(root["sidecars"], sidecarName)
	replacement := renderSidecarStartupBlock(startup)
	if localIndex < 0 {
		if action == "reset" {
			return content, nil
		}
		item := append([]string{"  - name: " + sidecarName}, replacement...)
		lines := strings.Split(content, "\n")
		start, end := topLevelBlockRange(lines, "sidecars")
		if start < 0 {
			return replaceTopLevelBlock(content, "sidecars", append([]string{"sidecars:"}, item...)), nil
		}
		return joinLikeSource(insertLines(lines, end, item), content), nil
	}
	lines, start, end, err := collectionItemBlockRange(content, "sidecars", localIndex)
	if err != nil {
		return "", err
	}
	blockStart, blockEnd := containerChildBlockRange(lines, start, end, "startup")
	if blockStart >= 0 {
		lines = append(lines[:blockStart], lines[blockEnd:]...)
	}
	if action == "set" {
		insertAt := start + 1
		if blockStart >= 0 {
			insertAt = blockStart
		}
		lines = insertLines(lines, insertAt, replacement)
	}
	updated := joinLikeSource(lines, content)
	if shared && action == "reset" {
		return cleanupEmptySharedSidecarPatch(updated, sidecarName)
	}
	return updated, nil
}

func renderSidecarStartupBlock(startup SidecarStartupModel) []string {
	lines := []string{"    startup:"}
	if startup.CommandPresent {
		lines = append(lines, renderNestedStringList("      command:", startup.Command)...)
	}
	if startup.ArgumentsPresent {
		lines = append(lines, renderNestedStringList("      arguments:", startup.Arguments)...)
	}
	return lines
}

func renderNestedStringList(header string, values []string) []string {
	if len(values) == 0 {
		return []string{header + " []"}
	}
	lines := []string{header}
	for _, value := range values {
		lines = append(lines, "        - "+strconv.Quote(value))
	}
	return lines
}

func cleanupEmptySharedSidecarPatch(content, sidecarName string) (string, error) {
	root, err := sourceModelRoot(content)
	if err != nil {
		return "", err
	}
	index, item := namedSourceItem(root["sidecars"], sidecarName)
	if index < 0 || len(item) != 1 {
		return content, nil
	}
	lines, start, end, err := collectionItemBlockRange(content, "sidecars", index)
	if err != nil {
		return "", err
	}
	updated := joinLikeSource(append(lines[:start], lines[end:]...), content)
	updatedRoot, err := sourceModelRoot(updated)
	if err != nil {
		return "", err
	}
	if len(anySlice(updatedRoot["sidecars"])) == 0 {
		updated = replaceTopLevelBlock(updated, "sidecars", nil)
	}
	return updated, nil
}

func resourceModel(resources map[string]any) ResourceModel {
	return ResourceModel{
		CPURequest:    stringPtr(firstNonEmpty(nestedString(resources, "cpu", "requests"), nestedString(resources, "cpu", "from"))),
		CPULimit:      stringPtr(firstNonEmpty(nestedString(resources, "cpu", "limits"), nestedString(resources, "cpu", "to"))),
		MemoryRequest: stringPtr(firstNonEmpty(nestedString(resources, "memory", "requests"), nestedString(resources, "memory", "from"))),
		MemoryLimit:   stringPtr(firstNonEmpty(nestedString(resources, "memory", "limits"), nestedString(resources, "memory", "to"))),
		CPUFrom:       stringPtr(nestedString(resources, "cpu", "from")), CPUTo: stringPtr(nestedString(resources, "cpu", "to")),
		MemoryFrom: stringPtr(nestedString(resources, "memory", "from")), MemoryTo: stringPtr(nestedString(resources, "memory", "to")),
	}
}

func appendSidecarEnvOverride(content, sidecarName, envName, action, value string) string {
	item := []string{"  - name: " + sidecarName, "    envs:"}
	item = append(item, renderSidecarEnvItem(envName, action, value)...)
	lines := strings.Split(content, "\n")
	start, end := topLevelBlockRange(lines, "sidecars")
	if start < 0 {
		return replaceTopLevelBlock(content, "sidecars", append([]string{"sidecars:"}, item...))
	}
	lines = insertLines(lines, end, item)
	return joinLikeSource(lines, content)
}

func renderSidecarEnvItem(name, action, value string) []string {
	lines := []string{"      - name: " + name}
	if action == "remove" {
		return append(lines, "        remove: true")
	}
	return append(lines, "        value: "+strconv.Quote(value))
}

func sidecarEnvItemRange(lines []string, start, end int, name string) (int, int, int) {
	starts := []int{}
	for index := start + 1; index < end; index++ {
		if strings.HasPrefix(lines[index], "      - ") {
			starts = append(starts, index)
		}
	}
	foundStart, foundEnd := -1, -1
	for index, itemStart := range starts {
		itemEnd := end
		if index+1 < len(starts) {
			itemEnd = starts[index+1]
		}
		if varBlockName(lines[itemStart:itemEnd]) == name {
			foundStart, foundEnd = itemStart, itemEnd
		}
	}
	return foundStart, foundEnd, len(starts)
}

func insertLines(lines []string, index int, replacement []string) []string {
	result := make([]string, 0, len(lines)+len(replacement))
	result = append(result, lines[:index]...)
	result = append(result, replacement...)
	return append(result, lines[index:]...)
}

func replaceLineRange(lines []string, start, end int, replacement []string) []string {
	result := make([]string, 0, len(lines)-(end-start)+len(replacement))
	result = append(result, lines[:start]...)
	result = append(result, replacement...)
	return append(result, lines[end:]...)
}

func joinLikeSource(lines []string, source string) string {
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(source, "\n") && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}
