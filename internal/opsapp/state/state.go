package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store struct {
	Path string
}

type File struct {
	Environments map[string]EnvironmentState `json:"environments"`
}

type EnvironmentState struct {
	AppliedRevision string    `json:"applied_revision,omitempty"`
	AppliedCommit   string    `json:"applied_commit,omitempty"`
	AppliedDigest   string    `json:"applied_digest,omitempty"`
	SnapshotPath    string    `json:"snapshot_path,omitempty"`
	AppliedAt       time.Time `json:"applied_at,omitempty"`
	AppliedBy       string    `json:"applied_by,omitempty"`
}

func NewStore(path string) Store {
	return Store{Path: path}
}

func (s Store) SnapshotDir(environment string) (string, error) {
	if strings.TrimSpace(s.Path) == "" {
		return "", errors.New("state path is required")
	}
	return filepath.Join(filepath.Dir(s.Path), "snapshots", environment), nil
}

func (s Store) Load() (File, error) {
	if strings.TrimSpace(s.Path) == "" {
		return File{Environments: map[string]EnvironmentState{}}, nil
	}
	content, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{Environments: map[string]EnvironmentState{}}, nil
		}
		return File{}, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return File{Environments: map[string]EnvironmentState{}}, nil
	}
	var file File
	if err := json.Unmarshal(content, &file); err != nil {
		return File{}, err
	}
	if file.Environments == nil {
		file.Environments = map[string]EnvironmentState{}
	}
	return file, nil
}

func (s Store) Environment(name string) (EnvironmentState, bool, error) {
	file, err := s.Load()
	if err != nil {
		return EnvironmentState{}, false, err
	}
	state, ok := file.Environments[name]
	return state, ok, nil
}

func (s Store) SaveEnvironment(name string, env EnvironmentState) error {
	if strings.TrimSpace(s.Path) == "" {
		return errors.New("state path is required")
	}
	file, err := s.Load()
	if err != nil {
		return err
	}
	if file.Environments == nil {
		file.Environments = map[string]EnvironmentState{}
	}
	file.Environments[name] = env
	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.Path, append(content, '\n'), 0o644)
}
