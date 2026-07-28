package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const scaffoldConfigVersion = 1

var supportedScaffoldRuntimes = map[string]bool{
	"java-jib": true,
	"java-jar": true,
	"cpp":      true,
	"binary":   true,
	"custom":   true,
}

type scaffoldAppOptions struct {
	Profile           string
	Runtime           string
	Image             string
	Container         string
	Replicas          int
	CPUFrom           string
	CPUTo             string
	MemoryFrom        string
	MemoryTo          string
	WrapperSource     string
	WrapperFile       string
	WrapperPath       string
	WrapperCommand    []string
	WrapperArguments  []string
	Assets            []string
	Sidecars          []string
	SidecarCPUFrom    string
	SidecarCPUTo      string
	SidecarMemoryFrom string
	SidecarMemoryTo   string
	Force             bool
	DryRun            bool
}

type scaffoldConfig struct {
	Version  int                        `yaml:"version"`
	Defaults scaffoldProfile            `yaml:"defaults"`
	Profiles map[string]scaffoldProfile `yaml:"profiles"`
}

type scaffoldProfile struct {
	Runtime           string                 `yaml:"runtime"`
	Image             string                 `yaml:"image"`
	Container         string                 `yaml:"container"`
	Replicas          *int                   `yaml:"replicas"`
	Resources         scaffoldResources      `yaml:"resources"`
	Wrapper           scaffoldWrapper        `yaml:"wrapper"`
	Assets            []scaffoldProfileAsset `yaml:"assets"`
	Sidecars          []map[string]any       `yaml:"sidecars"`
	App               map[string]any         `yaml:"app"`
	ContainerDefaults map[string]any         `yaml:"container_defaults"`
}

type scaffoldResources struct {
	CPU    scaffoldRange `yaml:"cpu"`
	Memory scaffoldRange `yaml:"memory"`
}

type scaffoldRange struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type scaffoldWrapper struct {
	Source    string   `yaml:"source"`
	File      string   `yaml:"file"`
	Path      string   `yaml:"path"`
	Command   []string `yaml:"command"`
	Arguments []string `yaml:"arguments"`
}

type scaffoldProfileAsset struct {
	File       string `yaml:"file"`
	To         string `yaml:"to"`
	Binary     bool   `yaml:"binary,omitempty"`
	Transform  bool   `yaml:"transform,omitempty"`
	HelmEscape *bool  `yaml:"helm_escape,omitempty"`
}

type generatedScaffoldApp struct {
	Name       string                       `yaml:"name"`
	Replicas   int                          `yaml:"replicas"`
	Containers []generatedScaffoldContainer `yaml:"containers"`
	Sidecars   []map[string]any             `yaml:"sidecars,omitempty"`
	Extra      map[string]any               `yaml:",inline"`
}

type generatedScaffoldContainer struct {
	Name      string                 `yaml:"name"`
	Image     string                 `yaml:"image"`
	Startup   *generatedStartup      `yaml:"startup,omitempty"`
	Assets    []scaffoldProfileAsset `yaml:"assets,omitempty"`
	Resources scaffoldResources      `yaml:"resources"`
	Extra     map[string]any         `yaml:",inline"`
}

type generatedStartup struct {
	Command   []string `yaml:"command,omitempty"`
	Arguments []string `yaml:"arguments,omitempty"`
}

type sharedScaffoldAssets struct {
	Assets []struct {
		To string `yaml:"to"`
	} `yaml:"assets"`
}

func newScaffoldCommand(global *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Generate environment and app model files",
	}

	envCmd := &cobra.Command{
		Use:   "env",
		Short: "Generate an environment skeleton",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkeletonEnv(cmd, global)
		},
	}
	envCmd.Flags().StringVar(&global.skeletonEnv, "env", "", "environment name to create")
	envCmd.Flags().StringVar(&global.skeletonNS, "namespace", "", "Kubernetes namespace value")
	envCmd.Flags().StringVar(&global.skeletonRegistry, "registry-url", "registry.example.com/project", "default REGISTRY_URL value")
	envCmd.Flags().StringVar(&global.skeletonRelease, "release-id", "latest", "default RELEASE_ID value")
	envCmd.Flags().BoolVar(&global.force, "force", false, "overwrite existing generated files")

	appOpts := defaultScaffoldAppOptions()
	appCmd := &cobra.Command{
		Use:   "app NAME",
		Short: "Generate an app model from repository scaffold rules",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScaffoldApp(cmd, global, appOpts, args[0], "scaffold app")
		},
	}
	bindScaffoldAppFlags(appCmd, appOpts)
	cmd.AddCommand(envCmd, appCmd)
	return cmd
}

func defaultScaffoldAppOptions() *scaffoldAppOptions {
	return &scaffoldAppOptions{
		Replicas:          1,
		CPUFrom:           "100m",
		CPUTo:             "500m",
		MemoryFrom:        "128Mi",
		MemoryTo:          "512Mi",
		SidecarCPUFrom:    "10m",
		SidecarCPUTo:      "100m",
		SidecarMemoryFrom: "32Mi",
		SidecarMemoryTo:   "128Mi",
	}
}

func bindScaffoldAppFlags(cmd *cobra.Command, opts *scaffoldAppOptions) {
	cmd.Flags().StringVar(&opts.Profile, "scaffold-profile", "", "profile from root and environment _scaffold.yml files")
	cmd.Flags().StringVar(&opts.Runtime, "runtime", "", "scaffold runtime: java-jib, java-jar, cpp, binary or custom")
	cmd.Flags().StringVar(&opts.Image, "image", "", "container image; defaults to {{REGISTRY_URL}}/<app>:{{RELEASE_ID}}")
	cmd.Flags().StringVar(&opts.Container, "container", "", "container name; defaults to app name")
	cmd.Flags().IntVar(&opts.Replicas, "replicas", 1, "initial replica count")
	cmd.Flags().StringVar(&opts.CPUFrom, "cpu-from", "100m", "container CPU request")
	cmd.Flags().StringVar(&opts.CPUTo, "cpu-to", "500m", "container CPU limit")
	cmd.Flags().StringVar(&opts.MemoryFrom, "memory-from", "128Mi", "container memory request")
	cmd.Flags().StringVar(&opts.MemoryTo, "memory-to", "512Mi", "container memory limit")
	cmd.Flags().StringVar(&opts.WrapperSource, "wrapper-source", "", "wrapper source: shared, asset or image")
	cmd.Flags().StringVar(&opts.WrapperFile, "wrapper-file", "", "wrapper file relative to <environment>/assets")
	cmd.Flags().StringVar(&opts.WrapperPath, "wrapper-path", "", "absolute wrapper path in the container")
	cmd.Flags().StringArrayVar(&opts.WrapperCommand, "wrapper-command", nil, "wrapper command item; repeatable")
	cmd.Flags().StringArrayVar(&opts.WrapperArguments, "wrapper-arg", nil, "wrapper argument; repeatable")
	cmd.Flags().StringArrayVar(&opts.Assets, "asset", nil, "asset SOURCE=TARGET; repeatable, SOURCE is relative to <environment>/assets")
	cmd.Flags().StringArrayVar(&opts.Sidecars, "sidecar", nil, "sidecar NAME=IMAGE; repeatable")
	cmd.Flags().StringVar(&opts.SidecarCPUFrom, "sidecar-cpu-from", "10m", "CPU request for CLI sidecars")
	cmd.Flags().StringVar(&opts.SidecarCPUTo, "sidecar-cpu-to", "100m", "CPU limit for CLI sidecars")
	cmd.Flags().StringVar(&opts.SidecarMemoryFrom, "sidecar-memory-from", "32Mi", "memory request for CLI sidecars")
	cmd.Flags().StringVar(&opts.SidecarMemoryTo, "sidecar-memory-to", "128Mi", "memory limit for CLI sidecars")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing app file")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print generated YAML without writing a file")
}

func runScaffoldApp(cmd *cobra.Command, global *cliOptions, cli *scaffoldAppOptions, rawAppName, operation string) error {
	envName := strings.TrimSpace(global.envName)
	if envName == "" {
		return fmt.Errorf("%s failed: --environment/-e is required", operation)
	}
	appName, err := validateScaffoldName(rawAppName, "app")
	if err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	envDir := filepath.Join(global.root, envName)
	appsDir := filepath.Join(envDir, "apps")
	config, err := loadScaffoldConfigHierarchy(
		filepath.Join(global.root, "_scaffold.yml"),
		filepath.Join(appsDir, "_scaffold.yml"),
	)
	if err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	profile, err := selectScaffoldProfile(config, cli.Profile)
	if err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	if err := applyScaffoldCLIOverrides(cmd, &profile, cli); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}

	containerName := strings.TrimSpace(profile.Container)
	if containerName == "" {
		containerName = appName
	}
	if _, err := validateScaffoldName(containerName, "container"); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	replacements := map[string]string{
		"${APP_NAME}":       appName,
		"${CONTAINER_NAME}": containerName,
	}
	expandScaffoldProfile(&profile, replacements)
	if err := validateScaffoldRuntime(profile.Runtime); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}

	image := strings.TrimSpace(profile.Image)
	if image == "" {
		image = "{{REGISTRY_URL}}/" + appName + ":{{RELEASE_ID}}"
	}
	replicas := 1
	if profile.Replicas != nil {
		replicas = *profile.Replicas
	}
	if replicas < 0 {
		return fmt.Errorf("%s failed: replicas must be zero or greater", operation)
	}
	if err := validateScaffoldResources(profile.Resources); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}

	assets, err := prepareScaffoldAssets(envDir, profile.Assets)
	if err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	startup, wrapperAsset, err := prepareScaffoldWrapper(envDir, profile.Runtime, profile.Wrapper)
	if err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	if wrapperAsset != nil {
		assets = appendUniqueScaffoldAsset(assets, *wrapperAsset)
	}
	if err := validateScaffoldSidecars(profile.Sidecars); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	if err := validateScaffoldFragmentKeys(profile.App, "app", "name", "replicas", "containers", "sidecars"); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	if err := validateScaffoldFragmentKeys(profile.ContainerDefaults, "container_defaults", "name", "image", "startup", "assets", "resources"); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}

	model := generatedScaffoldApp{
		Name:     appName,
		Replicas: replicas,
		Containers: []generatedScaffoldContainer{{
			Name:      containerName,
			Image:     image,
			Startup:   startup,
			Assets:    assets,
			Resources: profile.Resources,
			Extra:     profile.ContainerDefaults,
		}},
		Sidecars: profile.Sidecars,
		Extra:    profile.App,
	}
	var rendered bytes.Buffer
	encoder := yaml.NewEncoder(&rendered)
	encoder.SetIndent(2)
	if err := encoder.Encode(model); err != nil {
		return fmt.Errorf("%s failed: render YAML: %w", operation, err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("%s failed: render YAML: %w", operation, err)
	}
	content := rendered.Bytes()
	if cli.DryRun {
		_, err := cmd.OutOrStdout().Write(content)
		return err
	}
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	path := filepath.Join(appsDir, appName+".yml")
	if err := writeFileNoClobber(path, content, cli.Force); err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created app model: %s\n", path)
	return nil
}

func defaultGlobalScaffoldConfigContent() []byte {
	return []byte(`# Scaffold profiles are used only by "kube-build-app scaffold app".
# They are not merged into the build metamodel.
version: 1
defaults:
  replicas: 1
  resources:
    cpu:
      from: 100m
      to: 500m
    memory:
      from: 128Mi
      to: 512Mi
profiles: {}
`)
}

func defaultEnvironmentScaffoldConfigContent() []byte {
	return []byte(`# Optional overrides for ../../_scaffold.yml.
# This file is used only by "kube-build-app scaffold app".
version: 1
defaults: {}
profiles: {}
`)
}

func loadScaffoldConfigHierarchy(globalPath, environmentPath string) (scaffoldConfig, error) {
	global, err := loadScaffoldConfig(globalPath)
	if err != nil {
		return scaffoldConfig{}, err
	}
	environment, err := loadScaffoldConfig(environmentPath)
	if err != nil {
		return scaffoldConfig{}, err
	}
	return mergeScaffoldConfigs(global, environment), nil
}

func loadScaffoldConfig(path string) (scaffoldConfig, error) {
	config := scaffoldConfig{Version: scaffoldConfigVersion, Profiles: map[string]scaffoldProfile{}}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return scaffoldConfig{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return scaffoldConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	if config.Version == 0 {
		config.Version = scaffoldConfigVersion
	}
	if config.Version != scaffoldConfigVersion {
		return scaffoldConfig{}, fmt.Errorf("%s: unsupported scaffold version %d", path, config.Version)
	}
	if config.Profiles == nil {
		config.Profiles = map[string]scaffoldProfile{}
	}
	return config, nil
}

func mergeScaffoldConfigs(base, override scaffoldConfig) scaffoldConfig {
	result := scaffoldConfig{
		Version:  scaffoldConfigVersion,
		Defaults: mergeScaffoldProfiles(base.Defaults, override.Defaults),
		Profiles: make(map[string]scaffoldProfile, len(base.Profiles)+len(override.Profiles)),
	}
	for name, profile := range base.Profiles {
		result.Profiles[name] = profile
	}
	for name, profile := range override.Profiles {
		if inherited, ok := result.Profiles[name]; ok {
			result.Profiles[name] = mergeScaffoldProfiles(inherited, profile)
		} else {
			result.Profiles[name] = profile
		}
	}
	return result
}

func selectScaffoldProfile(config scaffoldConfig, name string) (scaffoldProfile, error) {
	result := config.Defaults
	name = strings.TrimSpace(name)
	if name == "" {
		return result, nil
	}
	profile, ok := config.Profiles[name]
	if !ok {
		return scaffoldProfile{}, fmt.Errorf("scaffold profile %q does not exist", name)
	}
	return mergeScaffoldProfiles(result, profile), nil
}

func mergeScaffoldProfiles(base, override scaffoldProfile) scaffoldProfile {
	result := base
	if override.Runtime != "" {
		result.Runtime = override.Runtime
	}
	if override.Image != "" {
		result.Image = override.Image
	}
	if override.Container != "" {
		result.Container = override.Container
	}
	if override.Replicas != nil {
		result.Replicas = override.Replicas
	}
	result.Resources = mergeScaffoldResources(result.Resources, override.Resources)
	result.Wrapper = mergeScaffoldWrapper(result.Wrapper, override.Wrapper)
	result.Assets = mergeScaffoldAssets(base.Assets, override.Assets)
	result.Sidecars = mergeScaffoldSidecars(base.Sidecars, override.Sidecars)
	result.App = mergeScaffoldMap(base.App, override.App)
	result.ContainerDefaults = mergeScaffoldMap(base.ContainerDefaults, override.ContainerDefaults)
	return result
}

func mergeScaffoldAssets(base, override []scaffoldProfileAsset) []scaffoldProfileAsset {
	result := append([]scaffoldProfileAsset{}, base...)
	index := make(map[string]int, len(result))
	for i, asset := range result {
		index[scaffoldAssetMergeKey(asset)] = i
	}
	for _, asset := range override {
		key := scaffoldAssetMergeKey(asset)
		if i, ok := index[key]; ok {
			result[i] = asset
			continue
		}
		index[key] = len(result)
		result = append(result, asset)
	}
	return result
}

func scaffoldAssetMergeKey(asset scaffoldProfileAsset) string {
	if target := strings.TrimSpace(asset.To); target != "" {
		return "to:" + target
	}
	return "file:" + strings.TrimSpace(asset.File)
}

func mergeScaffoldSidecars(base, override []map[string]any) []map[string]any {
	result := append([]map[string]any{}, base...)
	index := make(map[string]int, len(result))
	for i, sidecar := range result {
		if name, ok := sidecar["name"].(string); ok && strings.TrimSpace(name) != "" {
			index[strings.TrimSpace(name)] = i
		}
	}
	for _, sidecar := range override {
		name, _ := sidecar["name"].(string)
		name = strings.TrimSpace(name)
		if i, ok := index[name]; name != "" && ok {
			result[i] = mergeScaffoldMap(result[i], sidecar)
			continue
		}
		if name != "" {
			index[name] = len(result)
		}
		result = append(result, sidecar)
	}
	return result
}

func mergeScaffoldMap(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range override {
		if baseMap, ok := result[key].(map[string]any); ok {
			if overrideMap, ok := value.(map[string]any); ok {
				result[key] = mergeScaffoldMap(baseMap, overrideMap)
				continue
			}
		}
		result[key] = value
	}
	return result
}

func mergeScaffoldResources(base, override scaffoldResources) scaffoldResources {
	if override.CPU.From != "" {
		base.CPU.From = override.CPU.From
	}
	if override.CPU.To != "" {
		base.CPU.To = override.CPU.To
	}
	if override.Memory.From != "" {
		base.Memory.From = override.Memory.From
	}
	if override.Memory.To != "" {
		base.Memory.To = override.Memory.To
	}
	return base
}

func mergeScaffoldWrapper(base, override scaffoldWrapper) scaffoldWrapper {
	if override.Source != "" {
		if override.Source != base.Source && override.File == "" {
			base.File = ""
		}
		base.Source = override.Source
	}
	if override.File != "" {
		base.File = override.File
	}
	if override.Path != "" {
		base.Path = override.Path
	}
	if override.Command != nil {
		base.Command = append([]string{}, override.Command...)
	}
	if override.Arguments != nil {
		base.Arguments = append([]string{}, override.Arguments...)
	}
	return base
}

func applyScaffoldCLIOverrides(cmd *cobra.Command, profile *scaffoldProfile, cli *scaffoldAppOptions) error {
	changed := cmd.Flags().Changed
	if changed("runtime") {
		profile.Runtime = cli.Runtime
	}
	if changed("image") {
		profile.Image = cli.Image
	}
	if changed("container") {
		profile.Container = cli.Container
	}
	if changed("replicas") {
		profile.Replicas = intPointer(cli.Replicas)
	}
	if profile.Replicas == nil {
		profile.Replicas = intPointer(cli.Replicas)
	}
	if changed("cpu-from") || profile.Resources.CPU.From == "" {
		profile.Resources.CPU.From = cli.CPUFrom
	}
	if changed("cpu-to") || profile.Resources.CPU.To == "" {
		profile.Resources.CPU.To = cli.CPUTo
	}
	if changed("memory-from") || profile.Resources.Memory.From == "" {
		profile.Resources.Memory.From = cli.MemoryFrom
	}
	if changed("memory-to") || profile.Resources.Memory.To == "" {
		profile.Resources.Memory.To = cli.MemoryTo
	}
	if changed("wrapper-source") {
		profile.Wrapper.Source = cli.WrapperSource
		if cli.WrapperSource != "asset" && !changed("wrapper-file") {
			profile.Wrapper.File = ""
		}
	}
	if changed("wrapper-file") {
		profile.Wrapper.File = cli.WrapperFile
	}
	if changed("wrapper-path") {
		profile.Wrapper.Path = cli.WrapperPath
	}
	if changed("wrapper-command") {
		profile.Wrapper.Command = append([]string{}, cli.WrapperCommand...)
	}
	if changed("wrapper-arg") {
		profile.Wrapper.Arguments = append([]string{}, cli.WrapperArguments...)
	}
	for _, raw := range cli.Assets {
		source, target, ok := strings.Cut(raw, "=")
		if !ok {
			return fmt.Errorf("invalid --asset %q; expected SOURCE=TARGET", raw)
		}
		profile.Assets = append(profile.Assets, scaffoldProfileAsset{File: source, To: target})
	}
	for _, raw := range cli.Sidecars {
		name, image, ok := strings.Cut(raw, "=")
		if !ok {
			return fmt.Errorf("invalid --sidecar %q; expected NAME=IMAGE", raw)
		}
		sidecar := map[string]any{"name": strings.TrimSpace(name)}
		sidecar["image"] = strings.TrimSpace(image)
		sidecar["resources"] = map[string]any{
			"cpu":    map[string]any{"from": cli.SidecarCPUFrom, "to": cli.SidecarCPUTo},
			"memory": map[string]any{"from": cli.SidecarMemoryFrom, "to": cli.SidecarMemoryTo},
		}
		profile.Sidecars = append(profile.Sidecars, sidecar)
	}
	return nil
}

func prepareScaffoldAssets(envDir string, specs []scaffoldProfileAsset) ([]scaffoldProfileAsset, error) {
	result := make([]scaffoldProfileAsset, 0, len(specs))
	seenTargets := map[string]bool{}
	for _, spec := range specs {
		relative, err := validateScaffoldAssetSource(spec.File)
		if err != nil {
			return nil, err
		}
		spec.File = filepath.ToSlash(filepath.Join("assets", relative))
		spec.To = strings.TrimSpace(spec.To)
		if spec.To == "" || !strings.HasPrefix(spec.To, "/") {
			return nil, fmt.Errorf("asset %q requires an absolute target path", relative)
		}
		if seenTargets[spec.To] {
			return nil, fmt.Errorf("duplicate asset target %q", spec.To)
		}
		if err := validateScaffoldAssetFile(envDir, relative); err != nil {
			return nil, err
		}
		seenTargets[spec.To] = true
		result = append(result, spec)
	}
	return result, nil
}

func prepareScaffoldWrapper(envDir, runtime string, wrapper scaffoldWrapper) (*generatedStartup, *scaffoldProfileAsset, error) {
	source := strings.TrimSpace(wrapper.Source)
	path := strings.TrimSpace(wrapper.Path)
	file := strings.TrimSpace(wrapper.File)
	if source == "" && path == "" && file == "" && len(wrapper.Command) == 0 && len(wrapper.Arguments) == 0 {
		return nil, nil, nil
	}
	if source != "shared" && source != "asset" && source != "image" {
		return nil, nil, fmt.Errorf("wrapper source must be shared, asset or image")
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, nil, fmt.Errorf("wrapper requires an absolute path")
	}
	var asset *scaffoldProfileAsset
	switch source {
	case "shared":
		if file != "" {
			return nil, nil, fmt.Errorf("shared wrapper must not define file")
		}
		ok, err := sharedScaffoldTargetExists(filepath.Join(envDir, "shared.assets.yml"), path)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, fmt.Errorf("shared wrapper target %q is not provided by shared.assets.yml", path)
		}
	case "asset":
		relative, err := validateScaffoldAssetSource(file)
		if err != nil {
			return nil, nil, fmt.Errorf("wrapper: %w", err)
		}
		if err := validateScaffoldAssetFile(envDir, relative); err != nil {
			return nil, nil, fmt.Errorf("wrapper: %w", err)
		}
		asset = &scaffoldProfileAsset{File: filepath.ToSlash(filepath.Join("assets", relative)), To: path}
	case "image":
		if file != "" {
			return nil, nil, fmt.Errorf("image wrapper must not define file")
		}
	}

	command := append([]string{}, wrapper.Command...)
	arguments := append([]string{}, wrapper.Arguments...)
	if len(command) == 0 {
		if runtime == "binary" || source == "image" {
			command = []string{path}
		} else {
			command = []string{"/bin/sh"}
			arguments = append([]string{path}, arguments...)
		}
	} else {
		arguments = append([]string{path}, arguments...)
	}
	return &generatedStartup{Command: command, Arguments: arguments}, asset, nil
}

func sharedScaffoldTargetExists(path, target string) (bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var parsed sharedScaffoldAssets
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	for _, asset := range parsed.Assets {
		if strings.TrimSpace(asset.To) == target {
			return true, nil
		}
	}
	return false, nil
}

func validateScaffoldSidecars(sidecars []map[string]any) error {
	seen := map[string]bool{}
	for i, sidecar := range sidecars {
		name, _ := sidecar["name"].(string)
		image, _ := sidecar["image"].(string)
		name = strings.TrimSpace(name)
		image = strings.TrimSpace(image)
		if name == "" || image == "" {
			return fmt.Errorf("sidecars[%d] requires name and image", i)
		}
		if _, err := validateScaffoldName(name, "sidecar"); err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("duplicate sidecar name %q", name)
		}
		seen[name] = true
	}
	return nil
}

func validateScaffoldRuntime(runtime string) error {
	runtime = strings.TrimSpace(runtime)
	if runtime == "" {
		return nil
	}
	if !supportedScaffoldRuntimes[runtime] {
		return fmt.Errorf("unsupported runtime %q; expected java-jib, java-jar, cpp, binary or custom", runtime)
	}
	return nil
}

func validateScaffoldResources(resources scaffoldResources) error {
	for name, value := range map[string]string{
		"resources.cpu.from":    resources.CPU.From,
		"resources.cpu.to":      resources.CPU.To,
		"resources.memory.from": resources.Memory.From,
		"resources.memory.to":   resources.Memory.To,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

func validateScaffoldName(raw, kind string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("%s name is required", kind)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			continue
		}
		return "", fmt.Errorf("%s name %q contains unsupported character %q", kind, name, r)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid %s name %q", kind, name)
	}
	return name, nil
}

func validateScaffoldAssetSource(raw string) (string, error) {
	source := filepath.ToSlash(strings.TrimSpace(raw))
	if source == "" {
		return "", errors.New("asset source is required")
	}
	if filepath.IsAbs(source) || strings.HasPrefix(source, "/") {
		return "", fmt.Errorf("asset source %q must be relative to <environment>/assets", raw)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(source)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("asset source %q escapes <environment>/assets", raw)
	}
	return clean, nil
}

func expandScaffoldProfile(profile *scaffoldProfile, replacements map[string]string) {
	profile.Runtime = replaceScaffoldTokens(profile.Runtime, replacements)
	profile.Image = replaceScaffoldTokens(profile.Image, replacements)
	profile.Container = replaceScaffoldTokens(profile.Container, replacements)
	profile.Resources.CPU.From = replaceScaffoldTokens(profile.Resources.CPU.From, replacements)
	profile.Resources.CPU.To = replaceScaffoldTokens(profile.Resources.CPU.To, replacements)
	profile.Resources.Memory.From = replaceScaffoldTokens(profile.Resources.Memory.From, replacements)
	profile.Resources.Memory.To = replaceScaffoldTokens(profile.Resources.Memory.To, replacements)
	profile.Wrapper.Source = replaceScaffoldTokens(profile.Wrapper.Source, replacements)
	profile.Wrapper.File = replaceScaffoldTokens(profile.Wrapper.File, replacements)
	profile.Wrapper.Path = replaceScaffoldTokens(profile.Wrapper.Path, replacements)
	for i := range profile.Wrapper.Command {
		profile.Wrapper.Command[i] = replaceScaffoldTokens(profile.Wrapper.Command[i], replacements)
	}
	for i := range profile.Wrapper.Arguments {
		profile.Wrapper.Arguments[i] = replaceScaffoldTokens(profile.Wrapper.Arguments[i], replacements)
	}
	for i := range profile.Assets {
		profile.Assets[i].File = replaceScaffoldTokens(profile.Assets[i].File, replacements)
		profile.Assets[i].To = replaceScaffoldTokens(profile.Assets[i].To, replacements)
	}
	for i := range profile.Sidecars {
		profile.Sidecars[i] = replaceScaffoldMap(profile.Sidecars[i], replacements)
	}
	profile.App = replaceScaffoldMap(profile.App, replacements)
	profile.ContainerDefaults = replaceScaffoldMap(profile.ContainerDefaults, replacements)
}

func replaceScaffoldMap(input map[string]any, replacements map[string]string) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = replaceScaffoldValue(value, replacements)
	}
	return output
}

func replaceScaffoldValue(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case string:
		return replaceScaffoldTokens(typed, replacements)
	case map[string]any:
		return replaceScaffoldMap(typed, replacements)
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = replaceScaffoldValue(item, replacements)
		}
		return result
	default:
		return value
	}
}

func replaceScaffoldTokens(value string, replacements map[string]string) string {
	for token, replacement := range replacements {
		value = strings.ReplaceAll(value, token, replacement)
	}
	return value
}

func appendUniqueScaffoldAsset(assets []scaffoldProfileAsset, candidate scaffoldProfileAsset) []scaffoldProfileAsset {
	for _, asset := range assets {
		if asset.To == candidate.To {
			return assets
		}
	}
	return append(assets, candidate)
}

func validateScaffoldFragmentKeys(fragment map[string]any, section string, reserved ...string) error {
	for _, key := range reserved {
		if _, exists := fragment[key]; exists {
			return fmt.Errorf("%s must not define reserved field %q", section, key)
		}
	}
	return nil
}

func validateScaffoldAssetFile(envDir, relative string) error {
	root := filepath.Join(envDir, "assets")
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Stat(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("asset source %q does not exist in %s", relative, root)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("asset source %q is not a regular file", relative)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return err
	}
	within, err := filepath.Rel(realRoot, realCandidate)
	if err != nil {
		return err
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return fmt.Errorf("asset source %q resolves outside <environment>/assets", relative)
	}
	return nil
}

func intPointer(value int) *int {
	return &value
}
