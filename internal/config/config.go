package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"floppy-go/internal/context"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Env      map[string]any        `yaml:"env"`
	Services map[string]ServiceDef `yaml:"services"`
	Bundles  map[string][]string   `yaml:"bundles"`
}

type ServiceDef struct {
	Type          string         `yaml:"type"`
	Port          int            `yaml:"port"`
	Path          string         `yaml:"path"`
	Env           map[string]any `yaml:"env"`
	Repo          string         `yaml:"repo"`
	Command       string         `yaml:"command"`
	WorkerCommand string         `yaml:"worker_command"`
	HMRPort       int            `yaml:"hmr_port"`
	WSPort        int            `yaml:"ws_port"`
	DockerCommand string         `yaml:"docker_command"`
}

func LoadConfig(configPath string) (*Config, string, error) {
	resolved, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, "", fmt.Errorf("configuration file not found: %s", resolved)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, "", fmt.Errorf("failed to parse YAML: %w", err)
	}

	if cfg.Env == nil {
		cfg.Env = map[string]any{}
	}
	if cfg.Services == nil {
		cfg.Services = map[string]ServiceDef{}
	}
	if cfg.Bundles == nil {
		cfg.Bundles = map[string][]string{}
	}

	return &cfg, resolved, nil
}

func resolveConfigPath(configPath string) (string, error) {
	if configPath != "" {
		if _, err := os.Stat(configPath); err != nil {
			return "", fmt.Errorf("configuration file not found: %s", configPath)
		}
		return configPath, nil
	}

	// context path
	if ctxPath := context.GetServicesFilePath(); ctxPath != "" {
		if _, err := os.Stat(ctxPath); err == nil {
			return ctxPath, nil
		}
	}

	// default search
	candidates := []string{
		"services.yaml",
		filepath.Join("dev-env", "services.yaml"),
		filepath.Join("..", "dev-env", "services.yaml"),
	}
	if root := os.Getenv("SERVICES_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "dev-env", "services.yaml"))
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", errors.New("could not find services.yaml. Use -f, set-context, or place services.yaml in the default locations")
}

func (c *Config) ServiceNames() []string {
	out := make([]string, 0, len(c.Services))
	for name := range c.Services {
		out = append(out, name)
	}
	return out
}

func (c *Config) ExpandBundles(names []string) []string {
	set := map[string]struct{}{}
	for _, name := range names {
		if services, ok := c.Bundles[name]; ok {
			for _, svc := range services {
				set[svc] = struct{}{}
			}
			continue
		}
		set[name] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	return out
}

func (c *Config) ServicesRoot(configPath string) string {
	if root := os.Getenv("SERVICES_ROOT"); root != "" {
		return root
	}
	if configPath != "" {
		return filepath.Dir(configPath)
	}
	cwd, _ := os.Getwd()
	return cwd
}

func MergeEnv(base map[string]any, svc map[string]any) []string {
	merged := map[string]string{}
	for k, v := range base {
		merged[k] = fmt.Sprint(v)
	}
	for k, v := range svc {
		merged[k] = fmt.Sprint(v)
	}

	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}
