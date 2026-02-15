package manager

import (
	"path/filepath"
	"testing"

	"floppy-go/internal/config"
)

func Test_shellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"simple", "simple"},
		{"with space", "'with space'"},
		{"with'squote", "'with'\"'\"'squote'"},
		{"path/to/thing", "path/to/thing"},
		{"UPPER_123", "UPPER_123"},
		{"$VAR", "'$VAR'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func Test_shellJoin(t *testing.T) {
	if got := shellJoin([]string{"a", "b c", "d"}); got != "a 'b c' d" {
		t.Errorf("shellJoin = %q", got)
	}
	if got := shellJoin(nil); got != "" {
		t.Errorf("shellJoin(nil) = %q", got)
	}
}

func Test_servicePath(t *testing.T) {
	root := "/app"
	if got := servicePath(root, "api", ""); got != filepath.Join(root, "api") {
		t.Errorf("servicePath(empty path) = %q", got)
	}
	if got := servicePath(root, "api", "custom/path"); got != filepath.Join(root, "custom/path") {
		t.Errorf("servicePath(custom) = %q", got)
	}
}

func Test_isPythonType(t *testing.T) {
	yes := []string{"api", "worker", "webapp", "library", "python"}
	for _, typ := range yes {
		if !isPythonType(typ) {
			t.Errorf("isPythonType(%q) = false, want true", typ)
		}
	}
	no := []string{"portal", "docker", ""}
	for _, typ := range no {
		if isPythonType(typ) {
			t.Errorf("isPythonType(%q) = true, want false", typ)
		}
	}
}

func Test_compareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"3.11.2", "3.11.1", 1},
	}
	for _, tt := range tests {
		got := compareSemver(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func Test_parseSemver(t *testing.T) {
	p := parseSemver("1.2.3")
	if p[0] != 1 || p[1] != 2 || p[2] != 3 {
		t.Errorf("parseSemver(1.2.3) = %v", p)
	}
	p = parseSemver("10.0")
	if p[0] != 10 || p[1] != 0 || p[2] != 0 {
		t.Errorf("parseSemver(10.0) = %v", p)
	}
}

func Test_valueOr(t *testing.T) {
	if got := valueOr(nil, "fallback"); got != "fallback" {
		t.Errorf("valueOr(nil) = %q", got)
	}
	if got := valueOr("x", "fallback"); got != "x" {
		t.Errorf("valueOr(x) = %q", got)
	}
	if got := valueOr(42, "fallback"); got != "42" {
		t.Errorf("valueOr(42) = %q", got)
	}
}

func Test_verb(t *testing.T) {
	if got := verb(true); got != "update" {
		t.Errorf("verb(true) = %q", got)
	}
	if got := verb(false); got != "add" {
		t.Errorf("verb(false) = %q", got)
	}
}

func Test_conflictsSummary(t *testing.T) {
	conflicts := []PortConflict{
		{Port: 8000, Services: []string{"api"}},
		{Port: 3000, Services: []string{"portal"}},
	}
	got := conflictsSummary(conflicts)
	if got != "8000, 3000" && got != "3000, 8000" {
		t.Errorf("conflictsSummary: got %q", got)
	}
}

func TestNew(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.ServiceDef{"api": {Type: "api", Port: 8000}},
	}
	m := New(cfg, "/path/to/services.yaml")
	if m.Config != cfg || m.ConfigPath != "/path/to/services.yaml" {
		t.Errorf("New: Config or ConfigPath wrong")
	}
	if m.Root != "/path/to" {
		t.Errorf("New: Root = %q, want /path/to", m.Root)
	}
}
