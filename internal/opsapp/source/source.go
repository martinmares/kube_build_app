package source

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"kube-env/internal/opsapp/config"
	opsgit "kube-env/internal/opsapp/git"
)

// Options controls how an environment source tree is prepared for render/status commands.
type Options struct {
	FromGit bool
	WorkDir string
}

// Prepared contains an environment config with RootPath pointing at the concrete source tree.
type Prepared struct {
	Environment config.EnvironmentConfig `json:"environment"`
	Git         *opsgit.Resolution       `json:"git,omitempty"`
}

// Prepare resolves the concrete source root. Local mode absolutizes RootPath; Git mode checks out
// target_revision and interprets RootPath as a path inside the checked out repository.
func Prepare(env config.EnvironmentConfig, opts Options) (Prepared, error) {
	if !opts.FromGit {
		if !filepath.IsAbs(env.RootPath) {
			abs, err := filepath.Abs(env.RootPath)
			if err != nil {
				return Prepared{}, err
			}
			env.RootPath = abs
		}
		return Prepared{Environment: env}, nil
	}
	repo := strings.TrimSpace(env.Repo)
	if repo == "" {
		return Prepared{}, errors.New("environment repo is required in --from-git mode")
	}
	resolution, err := opsgit.Checkout(opsgit.CheckoutOptions{
		Repo:     repo,
		Revision: env.TargetRevision,
		WorkDir:  opts.WorkDir,
		Name:     env.Name,
	})
	if err != nil {
		return Prepared{}, err
	}
	rootPath := strings.TrimSpace(env.RootPath)
	if rootPath == "" {
		rootPath = "."
	}
	if filepath.IsAbs(rootPath) {
		return Prepared{}, fmt.Errorf("environment %q root_path must be relative in --from-git mode", env.Name)
	}
	env.RootPath = filepath.Join(resolution.WorktreePath, rootPath)
	env.ResolvedCommit = resolution.ResolvedCommit
	return Prepared{Environment: env, Git: &resolution}, nil
}
