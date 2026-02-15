package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceNames(t *testing.T) {
	cfg := &Config{
		Services: map[string]ServiceDef{
			"api":     {},
			"portal":  {},
			"worker":  {},
		},
	}
	names := cfg.ServiceNames()
	if len(names) != 3 {
		t.Fatalf("ServiceNames: want 3, got %d", len(names))
	}
	seen := make(map[string]bool)
	for _, n := range names {
		seen[n] = true
	}
	for _, want := range []string{"api", "portal", "worker"} {
		if !seen[want] {
			t.Errorf("ServiceNames: missing %q", want)
		}
	}
}

func TestExpandBundles(t *testing.T) {
	cfg := &Config{
		Bundles: map[string][]string{
			"all":   {"api", "worker", "portal"},
			"backend": {"api", "worker"},
		},
	}

	tests := []struct {
		name string
		in   []string
		want int
	}{
		{"single service", []string{"api"}, 1},
		{"bundle expands", []string{"all"}, 3},
		{"bundle and service", []string{"api", "backend"}, 2},
		{"duplicates deduped", []string{"all", "api", "api"}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := cfg.ExpandBundles(tt.in)
			if len(out) != tt.want {
				t.Errorf("ExpandBundles(%v): want %d names, got %d %v", tt.in, tt.want, len(out), out)
			}
		})
	}
}

func TestMergeEnv(t *testing.T) {
	base := map[string]any{"A": "1", "B": "2"}
	svc := map[string]any{"B": "overridden", "C": "3"}
	out := MergeEnv(base, svc)
	if len(out) != 3 {
		t.Fatalf("MergeEnv: want 3 entries, got %d", len(out))
	}
	env := sliceToMap(out)
	if env["A"] != "1" || env["B"] != "overridden" || env["C"] != "3" {
		t.Errorf("MergeEnv: got %v", env)
	}
}

func sliceToMap(env []string) map[string]string {
	m := make(map[string]string)
	for _, e := range env {
		for i, r := range e {
			if r == '=' {
				m[e[:i]] = e[i+1:]
				break
			}
		}
	}
	return m
}

func TestServicesRoot(t *testing.T) {
	cfg := &Config{}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "services.yaml")

	if got := cfg.ServicesRoot(cfgPath); got != dir {
		t.Errorf("ServicesRoot: want %q, got %q", dir, got)
	}

	t.Setenv("SERVICES_ROOT", "/custom/root")
	defer t.Setenv("SERVICES_ROOT", "")
	if got := cfg.ServicesRoot(""); got != "/custom/root" {
		t.Errorf("ServicesRoot with env: want /custom/root, got %q", got)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "services.yaml")
	const yaml = `
env:
  DB_HOST: localhost
services:
  api:
    type: api
    port: 8000
bundles:
  all: [api]
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, resolved, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if resolved != path {
		t.Errorf("resolved path: want %q, got %q", path, resolved)
	}
	if cfg.Env["DB_HOST"] != "localhost" {
		t.Errorf("env: got %v", cfg.Env)
	}
	if len(cfg.Services) != 1 || cfg.Services["api"].Port != 8000 {
		t.Errorf("services: got %+v", cfg.Services)
	}
	if len(cfg.Bundles) != 1 || len(cfg.Bundles["all"]) != 1 {
		t.Errorf("bundles: got %+v", cfg.Bundles)
	}
}

func TestLoadConfig_DefaultsNilMaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "services.yaml")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env == nil || cfg.Services == nil || cfg.Bundles == nil {
		t.Errorf("LoadConfig should default nil maps: Env=%v Services=%v Bundles=%v", cfg.Env, cfg.Services, cfg.Bundles)
	}
}
