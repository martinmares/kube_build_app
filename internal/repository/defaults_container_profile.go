package repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"sort"
	"strings"
)

type DefaultsContainerProfile struct {
	Env         string         `json:"env"`
	FileName    string         `json:"file_name"`
	Name        string         `json:"name"`
	ContentHash string         `json:"content_hash"`
	Image       string         `json:"image"`
	Defaults    map[string]any `json:"defaults"`
}

type DefaultsContainerProfileUpdate struct {
	Image    string         `json:"image"`
	Defaults map[string]any `json:"defaults"`
}

func (r *Repository) DefaultsContainerProfile(envName, name string) (DefaultsContainerProfile, error) {
	if !referenceNamePattern.MatchString(name) {
		return DefaultsContainerProfile{}, errors.New("invalid container profile name")
	}
	defaultsFile, err := r.defaultsFile(envName)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	detail, err := r.AssetDetail(envName, defaultsFile)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	root, err := rawSourceModelRoot(detail.Content)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	profile, err := uniqueDefaultsContainerProfile(root["container_profiles"], name)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	defaults, ok := profile["defaults"].(map[string]any)
	if !ok {
		return DefaultsContainerProfile{}, fmt.Errorf("container profile %q defaults must be a mapping", name)
	}
	image, ok := defaults["image"].(string)
	if !ok && defaults["image"] != nil {
		return DefaultsContainerProfile{}, fmt.Errorf("container profile %q defaults.image must be a string", name)
	}
	return DefaultsContainerProfile{
		Env: envName, FileName: defaultsFile, Name: name,
		ContentHash: detail.ContentHash, Image: image, Defaults: defaults,
	}, nil
}

func (r *Repository) UpdateDefaultsContainerProfile(envName, name string, update DefaultsContainerProfileUpdate, expectedHash string) (DefaultsContainerProfile, error) {
	current, err := r.DefaultsContainerProfile(envName, name)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	if expectedHash == "" || expectedHash != current.ContentHash {
		return DefaultsContainerProfile{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	desired := update.Defaults
	if desired == nil {
		desired = make(map[string]any, len(current.Defaults))
		for key, value := range current.Defaults {
			desired[key] = value
		}
		desired["image"] = strings.TrimSpace(update.Image)
	}
	if profileEqual(desired, current.Defaults) {
		return current, nil
	}
	if err := validateProfileDefaults(current.Defaults, desired); err != nil {
		return DefaultsContainerProfile{}, err
	}
	path, _, err := r.AssetPath(envName, current.FileName)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	if contentHash(contentBytes) != current.ContentHash {
		return DefaultsContainerProfile{}, NewConflictError("defaults file changed before save; refresh and apply the edit again")
	}
	updated, err := replaceProfileDefaults(string(contentBytes), name, current.Defaults, desired)
	if err != nil {
		return DefaultsContainerProfile{}, err
	}
	if updated != string(contentBytes) {
		if err := atomicWriteFile(path, []byte(updated)); err != nil {
			return DefaultsContainerProfile{}, err
		}
	}
	return r.DefaultsContainerProfile(envName, name)
}

func uniqueDefaultsContainerProfile(value any, name string) (map[string]any, error) {
	var found map[string]any
	for _, raw := range anySlice(value) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(stringValue(item["name"])) != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("duplicate container profile %q", name)
		}
		found = item
	}
	if found == nil {
		return nil, fmt.Errorf("container profile %q not found", name)
	}
	return found, nil
}

// Compare JSON semantics: values arriving from the API use float64 for numbers.
func profileEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func validateProfileDefaults(before, values map[string]any) error {
	editable := map[string]bool{"image": true, "startup": true, "envs": true, "resources": true, "probes": true, "ports": true}
	for key, value := range before {
		if !editable[key] && !profileEqual(value, values[key]) {
			return fmt.Errorf("defaults.%s is not editable in this form", key)
		}
	}
	for key, value := range values {
		if profileEqual(value, before[key]) {
			continue
		}
		if !editable[key] {
			return fmt.Errorf("defaults.%s is not editable in this form", key)
		}
		if key == "image" {
			image, ok := value.(string)
			if !ok || strings.TrimSpace(image) == "" || strings.ContainsAny(image, "\r\n") {
				return errors.New("defaults.image must be a non-empty single-line string")
			}
			continue
		}
		if key == "envs" {
			list, ok := value.([]any)
			if !ok {
				return errors.New("defaults.envs must be a sequence")
			}
			items := []DefaultsSidecarEnvItem{}
			for _, raw := range list {
				item, ok := raw.(map[string]any)
				if !ok {
					return errors.New("defaults.envs entries must be mappings")
				}
				parsed, err := parseDefaultsSidecarEnvItem(item)
				if err != nil {
					return err
				}
				items = append(items, parsed)
			}
			if err := validateDefaultsSidecarEnvItems(items); err != nil {
				return fmt.Errorf("defaults.envs: %w", err)
			}
			continue
		}
		if key == "ports" {
			list, ok := value.([]any)
			if !ok {
				return errors.New("defaults.ports must be a sequence")
			}
			for i, raw := range list {
				port, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("defaults.ports[%d] must be a mapping", i)
				}
				if strings.TrimSpace(stringValue(port["name"])) == "" {
					return fmt.Errorf("defaults.ports[%d].name is required", i)
				}
				if err := validateProfilePort("defaults.ports.port", stringValue(port["port"])); err != nil {
					return err
				}
				if raw, exists := port["expose_as"]; exists {
					services, ok := raw.([]any)
					if !ok {
						return errors.New("defaults.ports.expose_as must be a sequence")
					}
					for _, raw := range services {
						service, ok := raw.(map[string]any)
						if !ok {
							return errors.New("service must be a mapping")
						}
						if strings.TrimSpace(stringValue(service["hostname"])) == "" && strings.TrimSpace(stringValue(service["service_name"])) == "" {
							return errors.New("service hostname is required")
						}
						if err := validateProfilePort("service port", stringValue(service["port"])); err != nil {
							return err
						}
					}
				}
			}
			continue
		}
		block, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("defaults.%s must be a mapping", key)
		}
		switch key {
		case "startup":
			for _, k := range []string{"command", "arguments"} {
				if _, _, err := strictStringList(block, k); err != nil {
					return fmt.Errorf("defaults.startup: %w", err)
				}
			}
		case "resources":
			if err := validateEditableResourcesMap(block); err != nil {
				return err
			}
			for resource, raw := range block {
				for field, v := range raw.(map[string]any) {
					validate := validateMemoryQuantity
					if resource == "cpu" {
						validate = validateCPUQuantity
					}
					if err := validateResourceValue("defaults.resources."+resource+"."+field, stringValue(v), validate); err != nil {
						return err
					}
				}
			}
		case "probes":
			if err := validateEditableProbesMap(block); err != nil {
				return err
			}
			for _, k := range []string{"http", "live", "ready", "start"} {
				sub := nestedMap(block, k)
				if k == "http" {
					if err := validateProbePort("defaults.probes.http.port", stringValue(sub["port"])); err != nil {
						return err
					}
					continue
				}
				http := nestedMap(sub, "http")
				cmd, _, err := strictStringList(sub, "command")
				if err != nil {
					return err
				}
				h := ProbeHealthUpdate{HTTP: ProbeHTTPUpdate{Path: stringValue(http["path"]), Port: stringValue(http["port"])}, Command: cmd, Delay: stringValue(sub["delay"]), Period: stringValue(sub["period"]), Timeout: stringValue(sub["timeout"]), Success: stringValue(sub["success"]), Failure: stringValue(sub["failure"])}
				if err := validateProbeHealthUpdate("defaults.probes."+k, h); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateProfilePort(label, value string) error {
	value = strings.TrimSpace(value)
	if value != "" && sourceTemplatePattern.FindString(value) == value {
		return nil
	}
	return validatePositivePort(label, value)
}

func profileNodeValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// Edit only changed top-level defaults blocks. Unchanged blocks, other profiles,
// and the rest of the source remain byte-for-byte intact. Unsafe YAML is refused.
func replaceProfileDefaults(content, name string, before, desired map[string]any) (string, error) {
	templates := map[string]string{}
	masked := sourceTemplatePattern.ReplaceAllStringFunc(content, func(value string) string {
		token := fmt.Sprintf("__PROFILE_TEMPLATE_%d__", len(templates))
		for strings.Contains(content, token) {
			token += "_"
		}
		templates[token] = value
		return token
	})
	var doc yaml.Node
	decoder := yaml.NewDecoder(strings.NewReader(masked))
	if err := decoder.Decode(&doc); err != nil {
		return "", err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", errors.New("profile editing requires a single YAML document")
	}
	if len(doc.Content) != 1 {
		return "", errors.New("expected one YAML document")
	}
	var unsafe func(*yaml.Node) bool
	unsafe = func(n *yaml.Node) bool {
		if n.Anchor != "" || n.Kind == yaml.AliasNode {
			return true
		}
		for _, c := range n.Content {
			if unsafe(c) {
				return true
			}
		}
		return false
	}
	if unsafe(&doc) {
		return "", errors.New("profile editing does not support YAML anchors or aliases; use source editing")
	}
	profiles := profileNodeValue(doc.Content[0], "container_profiles")
	if profiles == nil || profiles.Kind != yaml.SequenceNode || profiles.Style&yaml.FlowStyle != 0 {
		return "", errors.New("container_profiles must use block YAML for form editing")
	}
	var target *yaml.Node
	for _, p := range profiles.Content {
		if n := profileNodeValue(p, "name"); n != nil && n.Value == name {
			target = p
		}
	}
	defaults := profileNodeValue(target, "defaults")
	if defaults == nil || defaults.Kind != yaml.MappingNode || defaults.Style&yaml.FlowStyle != 0 || target.Style&yaml.FlowStyle != 0 {
		return "", errors.New("profile defaults must use block YAML for form editing")
	}
	// Node line numbers still match after template masking, including multiline scalars.
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	indent := defaults.Column - 1
	if len(defaults.Content) == 0 {
		return "", errors.New("empty flow defaults are not editable with this form")
	}
	end := len(lines)
	for i := defaults.Content[0].Line; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if len(lines[i])-len(strings.TrimLeft(lines[i], " ")) < indent {
			end = i
			break
		}
	}
	for end > defaults.Content[0].Line && (strings.TrimSpace(lines[end-1]) == "" || strings.HasPrefix(strings.TrimSpace(lines[end-1]), "#")) {
		end--
	}
	type patch struct {
		start, end int
		lines      []string
	}
	patches := []patch{}
	seen := map[string]bool{}
	for i := 0; i < len(defaults.Content); i += 2 {
		key, node := defaults.Content[i], defaults.Content[i+1]
		seen[key.Value] = true
		if profileEqual(before[key.Value], desired[key.Value]) {
			continue
		}
		stop := end
		if i+2 < len(defaults.Content) {
			stop = defaults.Content[i+2].Line - 1
		}
		// Keep comments/blank lines belonging to the following sibling outside the edit.
		for stop > key.Line && (strings.TrimSpace(lines[stop-1]) == "" || strings.HasPrefix(strings.TrimSpace(lines[stop-1]), "#")) {
			stop--
		}
		replacement := []string{}
		if value, exists := desired[key.Value]; exists {
			var fresh yaml.Node
			if err := fresh.Encode(value); err != nil {
				return "", err
			}
			fresh = *mergeProfileSourceNode(node, &fresh)
			if fresh.Kind == yaml.ScalarNode && fresh.Tag == "!!str" {
				fresh.Style = yaml.DoubleQuotedStyle
			}
			wrapper := yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.Value, LineComment: key.LineComment}, &fresh}}
			var buf bytes.Buffer
			enc := yaml.NewEncoder(&buf)
			enc.SetIndent(2)
			if err := enc.Encode(&wrapper); err != nil {
				return "", err
			}
			_ = enc.Close()
			for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
				replacement = append(replacement, strings.Repeat(" ", indent)+line)
			}
		}
		patches = append(patches, patch{key.Line - 1, stop, replacement})
	}
	additions := []string{}
	keys := []string{}
	for key := range desired {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		var data bytes.Buffer
		encoder := yaml.NewEncoder(&data)
		encoder.SetIndent(2)
		if err := encoder.Encode(map[string]any{key: desired[key]}); err != nil {
			return "", err
		}
		_ = encoder.Close()
		for _, line := range strings.Split(strings.TrimSuffix(data.String(), "\n"), "\n") {
			additions = append(additions, strings.Repeat(" ", indent)+line)
		}
	}
	if len(additions) > 0 {
		patches = append(patches, patch{end, end, additions})
	}
	sort.SliceStable(patches, func(i, j int) bool { return patches[i].start > patches[j].start })
	for _, p := range patches {
		lines = append(append(append([]string{}, lines[:p.start]...), p.lines...), lines[p.end:]...)
	}
	updated := strings.Join(lines, "\n")
	if strings.Contains(content, "\r\n") {
		updated = strings.ReplaceAll(updated, "\n", "\r\n")
	}
	// Verify the complete source model, not just the selected profile, before writing.
	oldRoot, err := rawSourceModelRoot(content)
	if err != nil {
		return "", err
	}
	oldProfile, err := uniqueDefaultsContainerProfile(oldRoot["container_profiles"], name)
	if err != nil {
		return "", err
	}
	oldProfile["defaults"] = desired
	newRoot, err := rawSourceModelRoot(updated)
	if err != nil {
		return "", err
	}
	if !profileEqual(oldRoot, newRoot) {
		return "", errors.New("profile edit could not preserve source semantics; no changes were written")
	}
	return updated, nil
}

// Retain key order, collection style and comments inside an edited block.
// Values always come from the new node, so template mask tokens cannot escape.
func mergeProfileSourceNode(old, fresh *yaml.Node) *yaml.Node {
	result := *fresh
	result.HeadComment, result.LineComment, result.FootComment = old.HeadComment, old.LineComment, old.FootComment
	if old.Kind != fresh.Kind {
		return &result
	}
	result.Style = old.Style
	if old.Kind == yaml.MappingNode {
		result.Content = nil
		seen := map[string]bool{}
		for i := 0; i < len(old.Content); i += 2 {
			key := old.Content[i]
			if value := profileNodeValue(fresh, key.Value); value != nil {
				result.Content = append(result.Content, key, mergeProfileSourceNode(old.Content[i+1], value))
				seen[key.Value] = true
			}
		}
		for i := 0; i < len(fresh.Content); i += 2 {
			if !seen[fresh.Content[i].Value] {
				result.Content = append(result.Content, fresh.Content[i], fresh.Content[i+1])
			}
		}
	} else if old.Kind == yaml.SequenceNode {
		result.Content = append([]*yaml.Node(nil), fresh.Content...)
		for i := range result.Content {
			if i < len(old.Content) {
				result.Content[i] = mergeProfileSourceNode(old.Content[i], result.Content[i])
			}
		}
	}
	return &result
}
