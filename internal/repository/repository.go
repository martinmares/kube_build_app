package repository

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Environment struct {
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	HasAppsDir            bool   `json:"has_apps_dir"`
	HasAssetsDir          bool   `json:"has_assets_dir"`
	HasEnvUnsecuredJSON   bool   `json:"has_env_unsecured_json"`
	HasEnvSecuredJSON     bool   `json:"has_env_secured_json"`
	HasSharedAssetsYML    bool   `json:"has_shared_assets_yml"`
	HasReplicaProfilesYML bool   `json:"has_replica_profiles_yml"`
	AppFilesCount         int    `json:"app_files_count"`
	AssetFilesCount       int    `json:"asset_files_count"`
}

type App struct {
	FileName        string `json:"file_name"`
	Path            string `json:"path"`
	AppName         string `json:"app_name,omitempty"`
	Replicas        *int   `json:"replicas,omitempty"`
	ContainersCount int    `json:"containers_count"`
}

type Asset struct {
	FileName     string `json:"file_name"`
	RelativePath string `json:"relative_path"`
	Path         string `json:"path"`
	SizeBytes    int64  `json:"size_bytes"`
	Driver       string `json:"driver"`
	DriverSource string `json:"driver_source"`
}

type Repository struct {
	root string
}

func New(root string) (*Repository, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("repository root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("repository root is not a directory")
	}
	return &Repository{root: abs}, nil
}

func (r *Repository) Root() string {
	return r.root
}

func (r *Repository) Environments() ([]Environment, error) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return nil, err
	}

	environments := make([]Environment, 0)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		envPath := filepath.Join(r.root, entry.Name())
		environments = append(environments, summarizeEnvironment(entry.Name(), envPath))
	}
	sort.Slice(environments, func(i, j int) bool {
		return environments[i].Name < environments[j].Name
	})
	return environments, nil
}

func (r *Repository) Apps(envName string) ([]App, error) {
	envDir, err := r.envDir(envName)
	if err != nil {
		return nil, err
	}
	appsDir := filepath.Join(envDir, "apps")
	entries, err := os.ReadDir(appsDir)
	if errors.Is(err, os.ErrNotExist) {
		return []App{}, nil
	}
	if err != nil {
		return nil, err
	}

	apps := make([]App, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if strings.HasPrefix(fileName, "_") || !hasAnyExtension(fileName, ".yml", ".yaml") {
			continue
		}
		path := filepath.Join(appsDir, fileName)
		app, err := summarizeApp(path)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].FileName < apps[j].FileName
	})
	return apps, nil
}

func (r *Repository) Assets(envName string) ([]Asset, error) {
	envDir, err := r.envDir(envName)
	if err != nil {
		return nil, err
	}

	assets := make([]Asset, 0)
	assetsDir := filepath.Join(envDir, "assets")
	if isDir(assetsDir) {
		err = filepath.WalkDir(assetsDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(assetsDir, path)
			if err != nil {
				return err
			}
			asset, err := summarizeAsset(path, filepath.ToSlash(relative), "configmap", "default")
			if err != nil {
				return err
			}
			assets = append(assets, asset)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	for _, fileName := range []string{"assets.secured.json", "assets.unsecured.json"} {
		path := filepath.Join(envDir, fileName)
		if !isFile(path) {
			continue
		}
		asset, err := summarizeAsset(path, fileName, "special", "special-root")
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}

	sort.Slice(assets, func(i, j int) bool {
		return assets[i].RelativePath < assets[j].RelativePath
	})
	return assets, nil
}

func (r *Repository) envDir(envName string) (string, error) {
	if strings.TrimSpace(envName) == "" {
		return "", errors.New("environment name is required")
	}
	if strings.Contains(envName, "/") || strings.Contains(envName, "\\") || envName == "." || envName == ".." {
		return "", errors.New("invalid environment name")
	}
	envDir := filepath.Join(r.root, envName)
	info, err := os.Stat(envDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("environment is not a directory")
	}
	return envDir, nil
}

func summarizeEnvironment(name, path string) Environment {
	appsDir := filepath.Join(path, "apps")
	assetsDir := filepath.Join(path, "assets")
	return Environment{
		Name:                  name,
		Path:                  path,
		HasAppsDir:            isDir(appsDir),
		HasAssetsDir:          isDir(assetsDir),
		HasEnvUnsecuredJSON:   isFile(filepath.Join(path, "env.unsecured.json")),
		HasEnvSecuredJSON:     isFile(filepath.Join(path, "env.secured.json")),
		HasSharedAssetsYML:    isFile(filepath.Join(path, "shared.assets.yml")),
		HasReplicaProfilesYML: isFile(filepath.Join(path, "replica-profiles.yml")),
		AppFilesCount:         countFilesWithExtensions(appsDir, ".yml", ".yaml"),
		AssetFilesCount:       countFilesRecursive(assetsDir),
	}
}

func summarizeApp(path string) (App, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return App{}, err
	}
	text := string(content)
	if app, ok := summarizeAppYAML(path, text); ok {
		return app, nil
	}
	return App{
		FileName:        filepath.Base(path),
		Path:            path,
		AppName:         firstTopLevelScalar(text, "name"),
		Replicas:        firstTopLevelInt(text, "replicas"),
		ContainersCount: countTopLevelSequenceItems(text, "containers"),
	}, nil
}

func summarizeAppYAML(path string, content string) (App, bool) {
	var root any
	if err := yaml.Unmarshal([]byte(renderVarsPreview(content)), &root); err != nil {
		return App{}, false
	}
	rootMap, ok := root.(map[string]any)
	if !ok {
		return App{}, false
	}
	return App{
		FileName:        filepath.Base(path),
		Path:            path,
		AppName:         stringValue(rootMap["name"]),
		Replicas:        intPtr(intValue(rootMap["replicas"])),
		ContainersCount: len(anySlice(rootMap["containers"])),
	}, true
}

func summarizeAsset(path, relativePath, driver, driverSource string) (Asset, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Asset{}, err
	}
	return Asset{
		FileName:     filepath.Base(path),
		RelativePath: relativePath,
		Path:         path,
		SizeBytes:    info.Size(),
		Driver:       driver,
		DriverSource: driverSource,
	}, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func countFilesWithExtensions(dir string, extensions ...string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if hasAnyExtension(entry.Name(), extensions...) {
			count++
		}
	}
	return count
}

func countFilesRecursive(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	return count
}

func hasAnyExtension(fileName string, extensions ...string) bool {
	ext := filepath.Ext(fileName)
	for _, expected := range extensions {
		if ext == expected {
			return true
		}
	}
	return false
}

func firstTopLevelScalar(content, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if leadingSpaces(line) != 0 || !strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix))
		return unquote(value)
	}
	return ""
}

func firstTopLevelInt(content, key string) *int {
	value := firstTopLevelScalar(content, key)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func countTopLevelSequenceItems(content, key string) int {
	lines := strings.Split(content, "\n")
	inBlock := false
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if !inBlock {
			if indent == 0 && trimmed == key+":" {
				inBlock = true
			}
			continue
		}
		if indent == 0 {
			break
		}
		if strings.HasPrefix(strings.TrimLeft(line, " "), "- ") {
			count++
		}
	}
	return count
}

func leadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
