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
		updatedRoot, err := sourceModelRoot(updated)
		if err != nil {
			return "", err
		}
		updatedIndex, item := namedSourceItem(updatedRoot["sidecars"], sidecarName)
		if updatedIndex >= 0 && len(item) == 1 {
			updatedLines, itemStart, itemEnd, err := collectionItemBlockRange(updated, "sidecars", updatedIndex)
			if err != nil {
				return "", err
			}
			updatedLines = append(updatedLines[:itemStart], updatedLines[itemEnd:]...)
			updated = joinLikeSource(updatedLines, updated)
			rootAfterRemoval, err := sourceModelRoot(updated)
			if err != nil {
				return "", err
			}
			if len(anySlice(rootAfterRemoval["sidecars"])) == 0 {
				updated = replaceTopLevelBlock(updated, "sidecars", nil)
			}
		}
	}
	return updated, nil
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
