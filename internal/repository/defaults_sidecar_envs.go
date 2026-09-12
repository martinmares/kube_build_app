package repository

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type DefaultsSidecarEnvItem struct {
	Name                         string `json:"name"`
	Kind                         string `json:"kind"`
	Value                        string `json:"value,omitempty"`
	SecretName                   string `json:"secret_name,omitempty"`
	Key                          string `json:"key,omitempty"`
	ResourceName                 string `json:"resource_name,omitempty"`
	Divisor                      string `json:"divisor,omitempty"`
	FieldPath                    string `json:"field_path,omitempty"`
	WorkloadIdentityTokenRefName string `json:"workload_identity_token_ref_name,omitempty"`
	SharedAssetRefName           string `json:"shared_asset_ref_name,omitempty"`
}

type DefaultsSidecarDefinitionEnvs struct {
	Env                           string                   `json:"env"`
	FileName                      string                   `json:"file_name"`
	Name                          string                   `json:"name"`
	ContentHash                   string                   `json:"content_hash"`
	EnvsPresent                   bool                     `json:"envs_present"`
	Envs                          []DefaultsSidecarEnvItem `json:"envs"`
	WorkloadIdentityTokenRefNames []string                 `json:"workload_identity_token_ref_names"`
	SharedAssetRefNames           []string                 `json:"shared_asset_ref_names"`
}

type DefaultsSidecarDefinitionEnvsUpdate struct {
	Action string                   `json:"action"`
	Envs   []DefaultsSidecarEnvItem `json:"envs"`
}

func (r *Repository) DefaultsSidecarDefinitionEnvs(envName, name string) (DefaultsSidecarDefinitionEnvs, error) {
	if !referenceNamePattern.MatchString(name) {
		return DefaultsSidecarDefinitionEnvs{}, errors.New("invalid sidecar definition name")
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	detail, err := r.AssetDetail(envName, defaultsFile)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	root, err := rawSourceModelRoot(detail.Content)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	definition, err := uniqueNamedSourceItem(root["sidecar_definitions"], name)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	out := DefaultsSidecarDefinitionEnvs{Env: envName, FileName: defaultsFile, Name: name, ContentHash: detail.ContentHash}
	out.WorkloadIdentityTokenRefNames = namedDefinitionNames(nestedMap(root, "workload_identity")["tokens"])
	out.SharedAssetRefNames, err = r.sharedAssetReferenceNames(envName)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	rawEnvs, present := definition["envs"]
	if !present {
		return out, nil
	}
	out.EnvsPresent = true
	rawItems, ok := rawEnvs.([]any)
	if !ok {
		return DefaultsSidecarDefinitionEnvs{}, fmt.Errorf("sidecar definition %q envs must be a sequence", name)
	}
	for index, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return DefaultsSidecarDefinitionEnvs{}, fmt.Errorf("sidecar definition %q envs[%d] must be a mapping", name, index)
		}
		parsed, err := parseDefaultsSidecarEnvItem(item)
		if err != nil {
			return DefaultsSidecarDefinitionEnvs{}, fmt.Errorf("sidecar definition %q envs[%d]: %w", name, index, err)
		}
		out.Envs = append(out.Envs, parsed)
	}
	if err := validateDefaultsSidecarEnvItems(out.Envs); err != nil {
		return DefaultsSidecarDefinitionEnvs{}, fmt.Errorf("sidecar definition %q envs: %w", name, err)
	}
	return out, nil
}

func (r *Repository) sharedAssetReferenceNames(envName string) ([]string, error) {
	for _, candidate := range []string{"shared.assets.yml", "shared.assets.yaml"} {
		if _, _, err := r.AssetPath(envName, candidate); err != nil {
			continue
		}
		detail, err := r.AssetDetail(envName, candidate)
		if err != nil {
			return nil, err
		}
		root, err := rawSourceModelRoot(detail.Content)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", candidate, err)
		}
		return namedDefinitionNames(root["assets"]), nil
	}
	return nil, nil
}

func parseDefaultsSidecarEnvItem(values map[string]any) (DefaultsSidecarEnvItem, error) {
	if err := rejectUnknownKeys(values, "env", "name", "value", "secret_name", "key", "resource_name", "divisor", "field_path", "workload_identity_token_ref_name", "shared_asset_ref_name", "remove"); err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item := DefaultsSidecarEnvItem{Name: strings.TrimSpace(stringValue(values["name"]))}
	value, valuePresent, err := sourceScalarString(values, "value")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item.Value = value
	item.SecretName, _, err = sourceScalarString(values, "secret_name")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item.Key, _, err = sourceScalarString(values, "key")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item.ResourceName, _, err = sourceScalarString(values, "resource_name")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item.Divisor, _, err = sourceScalarString(values, "divisor")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item.FieldPath, _, err = sourceScalarString(values, "field_path")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item.WorkloadIdentityTokenRefName, _, err = sourceScalarString(values, "workload_identity_token_ref_name")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	item.SharedAssetRefName, _, err = sourceScalarString(values, "shared_asset_ref_name")
	if err != nil {
		return DefaultsSidecarEnvItem{}, err
	}
	remove := boolValue(values["remove"])

	sources := 0
	if valuePresent {
		item.Kind = "value"
		sources++
	}
	if item.SecretName != "" || item.Key != "" {
		item.Kind = "secret"
		sources++
	}
	if item.ResourceName != "" || item.Divisor != "" {
		item.Kind = "resource"
		sources++
	}
	if item.FieldPath != "" {
		item.Kind = "field"
		sources++
	}
	if item.WorkloadIdentityTokenRefName != "" {
		item.Kind = "workload_identity_token"
		sources++
	}
	if item.SharedAssetRefName != "" {
		item.Kind = "shared_asset"
		sources++
	}
	if remove {
		item.Kind = "remove"
		sources++
	}
	if sources > 1 {
		return DefaultsSidecarEnvItem{}, errors.New("combines multiple value sources")
	}
	if sources == 0 {
		item.Kind = "value"
	}
	return item, nil
}

func sourceScalarString(values map[string]any, key string) (string, bool, error) {
	raw, present := values[key]
	if !present {
		return "", false, nil
	}
	switch raw.(type) {
	case map[string]any, []any:
		return "", true, fmt.Errorf("%s must be a scalar", key)
	}
	return stringValue(raw), true, nil
}

func (r *Repository) UpdateDefaultsSidecarDefinitionEnvs(envName, name string, update DefaultsSidecarDefinitionEnvsUpdate, expectedHash string) (DefaultsSidecarDefinitionEnvs, error) {
	current, err := r.DefaultsSidecarDefinitionEnvs(envName, name)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return DefaultsSidecarDefinitionEnvs{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	action := strings.TrimSpace(update.Action)
	if action != "set" && action != "remove" {
		return DefaultsSidecarDefinitionEnvs{}, errors.New("action must be set or remove")
	}
	if action == "set" {
		update.Envs = normalizeDefaultsSidecarEnvItems(update.Envs)
		if err := validateDefaultsSidecarEnvItems(update.Envs); err != nil {
			return DefaultsSidecarDefinitionEnvs{}, err
		}
		if err := validateDefaultsSidecarEnvReferences(update.Envs, current.WorkloadIdentityTokenRefNames, current.SharedAssetRefNames); err != nil {
			return DefaultsSidecarDefinitionEnvs{}, err
		}
		if current.EnvsPresent && reflect.DeepEqual(current.Envs, update.Envs) {
			return current, nil
		}
	} else if !current.EnvsPresent {
		return current, nil
	}
	path, _, err := r.AssetPath(envName, current.FileName)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return DefaultsSidecarDefinitionEnvs{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceDefaultsSidecarDefinitionEnvs(string(contentBytes), name, action, update.Envs)
	if err != nil {
		return DefaultsSidecarDefinitionEnvs{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return DefaultsSidecarDefinitionEnvs{}, err
		}
	}
	return r.DefaultsSidecarDefinitionEnvs(envName, name)
}

func validateDefaultsSidecarEnvReferences(items []DefaultsSidecarEnvItem, tokenNames, sharedAssetNames []string) error {
	tokens := stringSet(tokenNames)
	sharedAssets := stringSet(sharedAssetNames)
	for index, item := range items {
		if item.Kind == "workload_identity_token" && !tokens[item.WorkloadIdentityTokenRefName] {
			return fmt.Errorf("envs[%d] references unknown workload identity token %q", index, item.WorkloadIdentityTokenRefName)
		}
		if item.Kind == "shared_asset" && !sharedAssets[item.SharedAssetRefName] {
			return fmt.Errorf("envs[%d] references unknown shared asset %q", index, item.SharedAssetRefName)
		}
	}
	return nil
}

func normalizeDefaultsSidecarEnvItems(items []DefaultsSidecarEnvItem) []DefaultsSidecarEnvItem {
	out := make([]DefaultsSidecarEnvItem, len(items))
	for index, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		item.Kind = strings.TrimSpace(item.Kind)
		item.SecretName = strings.TrimSpace(item.SecretName)
		item.Key = strings.TrimSpace(item.Key)
		item.ResourceName = strings.TrimSpace(item.ResourceName)
		item.Divisor = strings.TrimSpace(item.Divisor)
		item.FieldPath = strings.TrimSpace(item.FieldPath)
		item.WorkloadIdentityTokenRefName = strings.TrimSpace(item.WorkloadIdentityTokenRefName)
		item.SharedAssetRefName = strings.TrimSpace(item.SharedAssetRefName)
		out[index] = item
	}
	return out
}

func validateDefaultsSidecarEnvItems(items []DefaultsSidecarEnvItem) error {
	seen := map[string]bool{}
	for index, item := range items {
		item = normalizeDefaultsSidecarEnvItems([]DefaultsSidecarEnvItem{item})[0]
		if item.Name == "" || !variableNamePattern.MatchString(item.Name) {
			return fmt.Errorf("envs[%d] has invalid variable name %q", index, item.Name)
		}
		if seen[item.Name] {
			return fmt.Errorf("duplicate variable name %q", item.Name)
		}
		seen[item.Name] = true
		for label, value := range map[string]string{
			"value": item.Value, "secret_name": item.SecretName, "key": item.Key,
			"resource_name": item.ResourceName, "divisor": item.Divisor, "field_path": item.FieldPath,
			"workload_identity_token_ref_name": item.WorkloadIdentityTokenRefName, "shared_asset_ref_name": item.SharedAssetRefName,
		} {
			if strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("envs[%d].%s contains unsupported newline", index, label)
			}
		}
		switch item.Kind {
		case "value":
		case "secret":
			if item.SecretName == "" || item.Key == "" {
				return fmt.Errorf("envs[%d] secret requires secret_name and key", index)
			}
		case "resource":
			if item.ResourceName == "" {
				return fmt.Errorf("envs[%d] resource requires resource_name", index)
			}
		case "field":
			if item.FieldPath == "" {
				return fmt.Errorf("envs[%d] field requires field_path", index)
			}
		case "workload_identity_token":
			if !referenceNamePattern.MatchString(item.WorkloadIdentityTokenRefName) {
				return fmt.Errorf("envs[%d] has invalid workload identity token reference", index)
			}
		case "shared_asset":
			if !referenceNamePattern.MatchString(item.SharedAssetRefName) {
				return fmt.Errorf("envs[%d] has invalid shared asset reference", index)
			}
		case "remove":
		default:
			return fmt.Errorf("envs[%d] has unsupported kind %q", index, item.Kind)
		}
		if err := rejectDefaultsSidecarEnvItemExtras(index, item); err != nil {
			return err
		}
	}
	return nil
}

func rejectDefaultsSidecarEnvItemExtras(index int, item DefaultsSidecarEnvItem) error {
	extras := map[string]string{}
	switch item.Kind {
	case "value":
		extras = map[string]string{"secret_name": item.SecretName, "key": item.Key, "resource_name": item.ResourceName, "divisor": item.Divisor, "field_path": item.FieldPath, "workload_identity_token_ref_name": item.WorkloadIdentityTokenRefName, "shared_asset_ref_name": item.SharedAssetRefName}
	case "secret":
		extras = map[string]string{"value": item.Value, "resource_name": item.ResourceName, "divisor": item.Divisor, "field_path": item.FieldPath, "workload_identity_token_ref_name": item.WorkloadIdentityTokenRefName, "shared_asset_ref_name": item.SharedAssetRefName}
	case "resource":
		extras = map[string]string{"value": item.Value, "secret_name": item.SecretName, "key": item.Key, "field_path": item.FieldPath, "workload_identity_token_ref_name": item.WorkloadIdentityTokenRefName, "shared_asset_ref_name": item.SharedAssetRefName}
	case "field":
		extras = map[string]string{"value": item.Value, "secret_name": item.SecretName, "key": item.Key, "resource_name": item.ResourceName, "divisor": item.Divisor, "workload_identity_token_ref_name": item.WorkloadIdentityTokenRefName, "shared_asset_ref_name": item.SharedAssetRefName}
	case "workload_identity_token":
		extras = map[string]string{"value": item.Value, "secret_name": item.SecretName, "key": item.Key, "resource_name": item.ResourceName, "divisor": item.Divisor, "field_path": item.FieldPath, "shared_asset_ref_name": item.SharedAssetRefName}
	case "shared_asset":
		extras = map[string]string{"value": item.Value, "secret_name": item.SecretName, "key": item.Key, "resource_name": item.ResourceName, "divisor": item.Divisor, "field_path": item.FieldPath, "workload_identity_token_ref_name": item.WorkloadIdentityTokenRefName}
	case "remove":
		extras = map[string]string{"value": item.Value, "secret_name": item.SecretName, "key": item.Key, "resource_name": item.ResourceName, "divisor": item.Divisor, "field_path": item.FieldPath, "workload_identity_token_ref_name": item.WorkloadIdentityTokenRefName, "shared_asset_ref_name": item.SharedAssetRefName}
	}
	for field, value := range extras {
		if value != "" {
			return fmt.Errorf("envs[%d] kind %s cannot define %s", index, item.Kind, field)
		}
	}
	return nil
}

func replaceDefaultsSidecarDefinitionEnvs(content, name, action string, envs []DefaultsSidecarEnvItem) (string, error) {
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
	envsStart, envsEnd := containerChildBlockRange(lines, start, end, "envs")
	if envsStart >= 0 {
		lines = append(lines[:envsStart], lines[envsEnd:]...)
	}
	if action == "set" {
		insertAt := start + 1
		if envsStart >= 0 {
			insertAt = envsStart
		}
		lines = insertLines(lines, insertAt, renderDefaultsSidecarEnvsBlock(envs))
	}
	return joinLikeSource(lines, content), nil
}

func renderDefaultsSidecarEnvsBlock(items []DefaultsSidecarEnvItem) []string {
	if len(items) == 0 {
		return []string{"    envs: []"}
	}
	lines := []string{"    envs:"}
	for _, item := range normalizeDefaultsSidecarEnvItems(items) {
		lines = append(lines, "      - name: "+item.Name)
		appendValue := func(key, value string) { lines = append(lines, "        "+key+": "+strconv.Quote(value)) }
		switch item.Kind {
		case "value":
			appendValue("value", item.Value)
		case "secret":
			appendValue("secret_name", item.SecretName)
			appendValue("key", item.Key)
		case "resource":
			appendValue("resource_name", item.ResourceName)
			if item.Divisor != "" {
				appendValue("divisor", item.Divisor)
			}
		case "field":
			appendValue("field_path", item.FieldPath)
		case "workload_identity_token":
			appendValue("workload_identity_token_ref_name", item.WorkloadIdentityTokenRefName)
		case "shared_asset":
			appendValue("shared_asset_ref_name", item.SharedAssetRefName)
		case "remove":
			lines = append(lines, "        remove: true")
		}
	}
	return lines
}
