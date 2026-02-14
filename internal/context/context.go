package context

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Context struct {
	ServicesFilePath string `json:"services_file_path"`
}

func contextFilePath() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "floppy", "context.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "floppy", "context.json")
}

func ensureDir() error {
	path := filepath.Dir(contextFilePath())
	return os.MkdirAll(path, 0o755)
}

func load() Context {
	path := contextFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Context{}
	}
	var ctx Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		return Context{}
	}
	return ctx
}

func save(ctx Context) error {
	if err := ensureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(contextFilePath(), data, 0o644)
}

func GetServicesFilePath() string {
	ctx := load()
	return ctx.ServicesFilePath
}

func SetServicesFilePath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	ctx := load()
	ctx.ServicesFilePath = abs
	return save(ctx)
}

func Clear() error {
	err := os.Remove(contextFilePath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func Info() (contextPath string, servicesPath string, exists bool) {
	contextPath = contextFilePath()
	ctx := load()
	servicesPath = ctx.ServicesFilePath
	if servicesPath != "" {
		if _, err := os.Stat(servicesPath); err == nil {
			exists = true
		}
	}
	return contextPath, servicesPath, exists
}
