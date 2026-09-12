package repository

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type DefaultsSidecarDefinitionReferences struct {
	Env                     string   `json:"env"`
	FileName                string   `json:"file_name"`
	Name                    string   `json:"name"`
	ContentHash             string   `json:"content_hash"`
	ProfileRefNames         []string `json:"profile_ref_names"`
	RuntimeAssetRefNames    []string `json:"runtime_asset_ref_names"`
	ContainerProfiles       []string `json:"container_profiles"`
	RuntimeAssetDefinitions []string `json:"runtime_asset_definitions"`
}

type DefaultsSidecarDefinitionReferencesUpdate struct {
	ProfileRefNames      []string `json:"profile_ref_names"`
	RuntimeAssetRefNames []string `json:"runtime_asset_ref_names"`
}

func (r *Repository) DefaultsSidecarDefinitionReferences(envName, name string) (DefaultsSidecarDefinitionReferences, error) {
	if !referenceNamePattern.MatchString(name) {
		return DefaultsSidecarDefinitionReferences{}, errors.New("invalid sidecar definition name")
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	detail, err := r.AssetDetail(envName, defaultsFile)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	root, err := rawSourceModelRoot(detail.Content)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	definition, err := uniqueNamedSourceItem(root["sidecar_definitions"], name)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	profiles, _, err := strictStringList(definition, "profile_ref_names")
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, fmt.Errorf("sidecar definition %q: %w", name, err)
	}
	assets, _, err := strictStringList(definition, "runtime_asset_ref_names")
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, fmt.Errorf("sidecar definition %q: %w", name, err)
	}
	return DefaultsSidecarDefinitionReferences{
		Env: envName, FileName: defaultsFile, Name: name, ContentHash: detail.ContentHash,
		ProfileRefNames: profiles, RuntimeAssetRefNames: assets,
		ContainerProfiles:       namedDefinitionNames(root["container_profiles"]),
		RuntimeAssetDefinitions: namedDefinitionNames(root["runtime_asset_definitions"]),
	}, nil
}

func (r *Repository) UpdateDefaultsSidecarDefinitionReferences(envName, name string, update DefaultsSidecarDefinitionReferencesUpdate, expectedHash string) (DefaultsSidecarDefinitionReferences, error) {
	current, err := r.DefaultsSidecarDefinitionReferences(envName, name)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return DefaultsSidecarDefinitionReferences{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	update.ProfileRefNames = cleanStringItems(update.ProfileRefNames)
	update.RuntimeAssetRefNames = cleanStringItems(update.RuntimeAssetRefNames)
	if err := validateReferenceNames("profile_ref_names", update.ProfileRefNames, current.ContainerProfiles); err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	if err := validateReferenceNames("runtime_asset_ref_names", update.RuntimeAssetRefNames, current.RuntimeAssetDefinitions); err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	if reflect.DeepEqual(current.ProfileRefNames, update.ProfileRefNames) && reflect.DeepEqual(current.RuntimeAssetRefNames, update.RuntimeAssetRefNames) {
		return current, nil
	}
	path, _, err := r.AssetPath(envName, current.FileName)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return DefaultsSidecarDefinitionReferences{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceDefaultsSidecarDefinitionReferences(string(contentBytes), name, update)
	if err != nil {
		return DefaultsSidecarDefinitionReferences{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return DefaultsSidecarDefinitionReferences{}, err
		}
	}
	return r.DefaultsSidecarDefinitionReferences(envName, name)
}

func replaceDefaultsSidecarDefinitionReferences(content, name string, update DefaultsSidecarDefinitionReferencesUpdate) (string, error) {
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
	updated, err := replaceCollectionItemStringList(content, "sidecar_definitions", index, "runtime_asset_ref_names", update.RuntimeAssetRefNames)
	if err != nil {
		return "", err
	}
	return replaceCollectionItemStringList(updated, "sidecar_definitions", index, "profile_ref_names", update.ProfileRefNames)
}
