package repository

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	root, err := sourceModelRoot(detail.Content)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	definition, err := uniqueNamedSourceItem(root["sidecar_definitions"], name)
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	if _, ok := definition["image"].(string); !ok && definition["image"] != nil {
		return DefaultsSidecarDefinition{}, fmt.Errorf("sidecar definition %q image must be a string", name)
	}
	index, _ := namedSourceItem(root["sidecar_definitions"], name)
	image, err := namedCollectionRawScalar(detail.Content, "sidecar_definitions", index, "image")
	if err != nil {
		return DefaultsSidecarDefinition{}, err
	}
	return DefaultsSidecarDefinition{
		Env: envName, FileName: defaultsFile, Name: name,
		ContentHash: detail.ContentHash, Image: image,
	}, nil
}

func namedCollectionRawScalar(content, collection string, index int, key string) (string, error) {
	lines, start, end, err := collectionItemBlockRange(content, collection, index)
	if err != nil {
		return "", err
	}
	prefix := "    " + key + ":"
	for lineIndex := start + 1; lineIndex < end; lineIndex++ {
		if !strings.HasPrefix(lines[lineIndex], prefix) {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(lines[lineIndex], prefix))
		if raw == "" {
			return "", fmt.Errorf("%s %q must be a scalar", key, collection)
		}
		return decodeRawYAMLScalar(raw), nil
	}
	return "", nil
}

func decodeRawYAMLScalar(raw string) string {
	if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		if value, err := strconv.Unquote(raw); err == nil {
			return value
		}
	}
	if strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'") && len(raw) >= 2 {
		return strings.ReplaceAll(raw[1:len(raw)-1], "''", "'")
	}
	if comment := strings.Index(raw, " #"); comment >= 0 {
		raw = raw[:comment]
	}
	return strings.TrimSpace(raw)
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
