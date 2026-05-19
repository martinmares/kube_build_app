package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig        `yaml:"server" json:"server"`
	Storage      StorageConfig       `yaml:"storage" json:"storage"`
	Auth         AuthConfig          `yaml:"auth" json:"auth"`
	Environments []EnvironmentConfig `yaml:"environments" json:"environments"`
}

type ServerConfig struct {
	HTTP HTTPConfig `yaml:"http" json:"http"`
}

type HTTPConfig struct {
	Listen string `yaml:"listen" json:"listen"`
}

type StorageConfig struct {
	Postgres PostgresConfig `yaml:"postgres" json:"postgres"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn" json:"dsn"`
}

type AuthConfig struct {
	Mode string `yaml:"mode" json:"mode"`
}

type EnvironmentConfig struct {
	Name           string `yaml:"name" json:"name"`
	Namespace      string `yaml:"namespace" json:"namespace"`
	Repo           string `yaml:"repo" json:"repo"`
	Branch         string `yaml:"branch" json:"branch"`
	RootPath       string `yaml:"root_path" json:"root_path"`
	EnvName        string `yaml:"env_name" json:"env_name"`
	TargetRevision string `yaml:"target_revision" json:"target_revision"`
	AutoSync       bool   `yaml:"auto_sync" json:"auto_sync"`
}

func LoadFile(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, errors.New("config path is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, err
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Server.HTTP.Listen) == "" {
		c.Server.HTTP.Listen = ":8080"
	}
	for i := range c.Environments {
		env := &c.Environments[i]
		if strings.TrimSpace(env.EnvName) == "" {
			env.EnvName = env.Name
		}
		if strings.TrimSpace(env.RootPath) == "" {
			env.RootPath = "."
		}
		if strings.TrimSpace(env.TargetRevision) == "" {
			env.TargetRevision = env.Branch
		}
	}
}

func (c Config) Validate() error {
	seen := map[string]bool{}
	for idx, env := range c.Environments {
		name := strings.TrimSpace(env.Name)
		if name == "" {
			return fmt.Errorf("environments[%d].name is required", idx)
		}
		if seen[name] {
			return fmt.Errorf("duplicate environment %q", name)
		}
		seen[name] = true
		if strings.TrimSpace(env.EnvName) == "" {
			return fmt.Errorf("environment %q env_name is required", name)
		}
		if strings.TrimSpace(env.RootPath) == "" {
			return fmt.Errorf("environment %q root_path is required", name)
		}
	}
	return nil
}

func (c Config) Environment(name string) (EnvironmentConfig, bool) {
	for _, env := range c.Environments {
		if env.Name == name {
			return env, true
		}
	}
	return EnvironmentConfig{}, false
}
