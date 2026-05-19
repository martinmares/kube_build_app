package render

import (
	"fmt"
	"os"
	"path/filepath"

	"kube-env/internal/buildapp"
	"kube-env/internal/opsapp/config"
	"kube-env/internal/opsapp/digest"
)

type DigestResult struct {
	Environment    string       `json:"environment"`
	EnvName        string       `json:"env_name"`
	Namespace      string       `json:"namespace,omitempty"`
	TargetRevision string       `json:"target_revision,omitempty"`
	Digest         string       `json:"digest"`
	Files          int          `json:"files"`
	Bytes          int64        `json:"bytes"`
	Paths          []string     `json:"paths,omitempty"`
	Build          buildSummary `json:"build"`
}

type buildSummary struct {
	Deployments int `json:"deployments"`
	Services    int `json:"services"`
	Assets      int `json:"assets"`
	Budgets     int `json:"budgets"`
	Autoscaling int `json:"autoscaling"`
	Externals   int `json:"externals"`
}

func RenderDigest(env config.EnvironmentConfig) (DigestResult, error) {
	target, err := os.MkdirTemp("", "kube-ops-render-*")
	if err != nil {
		return DigestResult{}, err
	}
	defer os.RemoveAll(target)

	build, err := buildapp.Build(buildapp.Options{Environment: env.EnvName, Root: env.RootPath, Target: target})
	if err != nil {
		return DigestResult{}, fmt.Errorf("render %q: %w", env.Name, err)
	}
	d, err := digest.Directory(target)
	if err != nil {
		return DigestResult{}, err
	}
	return DigestResult{
		Environment:    env.Name,
		EnvName:        env.EnvName,
		Namespace:      env.Namespace,
		TargetRevision: env.TargetRevision,
		Digest:         d.Digest,
		Files:          d.Files,
		Bytes:          d.Bytes,
		Paths:          d.Paths,
		Build: buildSummary{
			Deployments: len(build.Deployments),
			Services:    len(build.Services),
			Assets:      len(build.Assets),
			Budgets:     len(build.Budgets),
			Autoscaling: len(build.Autoscaling),
			Externals:   len(build.Externals),
		},
	}, nil
}

func RenderDigestByName(cfg config.Config, name string) (DigestResult, error) {
	env, ok := cfg.Environment(name)
	if !ok {
		return DigestResult{}, fmt.Errorf("environment %q not found", name)
	}
	if !filepath.IsAbs(env.RootPath) {
		abs, err := filepath.Abs(env.RootPath)
		if err != nil {
			return DigestResult{}, err
		}
		env.RootPath = abs
	}
	return RenderDigest(env)
}
