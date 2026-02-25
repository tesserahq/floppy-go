package manager

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type ProcessEntry struct {
	Service string `json:"service"`
	PID     int    `json:"pid"`
	PGID    int    `json:"pgid"`
	Cwd     string `json:"cwd"`
	Cmdline string `json:"cmdline"`
}

type ProcessState struct {
	Entries map[string]ProcessEntry `json:"entries"`
}

func stateFilePath() string {
	if explicit := strings.TrimSpace(os.Getenv("FLOPPY_STATE_FILE")); explicit != "" {
		return explicit
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		home, hErr := os.UserHomeDir()
		if hErr != nil || home == "" {
			return filepath.Join(os.TempDir(), "floppy-go", "process-state.json")
		}
		cacheDir = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheDir, "floppy-go", "process-state.json")
}

func loadProcessState() ProcessState {
	path := stateFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return ProcessState{Entries: map[string]ProcessEntry{}}
	}
	var state ProcessState
	if err := json.Unmarshal(data, &state); err != nil {
		return ProcessState{Entries: map[string]ProcessEntry{}}
	}
	if state.Entries == nil {
		state.Entries = map[string]ProcessEntry{}
	}
	return state
}

func saveProcessState(state ProcessState) error {
	if state.Entries == nil {
		state.Entries = map[string]ProcessEntry{}
	}
	path := stateFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func commandContainsExpected(actual, expected string) bool {
	a := strings.TrimSpace(actual)
	e := strings.TrimSpace(expected)
	if a == "" || e == "" {
		return false
	}
	return strings.Contains(a, e)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return isSignalZeroOK(pid)
}

func isSignalZeroOK(pid int) bool {
	// SIG 0 only checks process existence/permission.
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}

func stableKeys(entries map[string]ProcessEntry) []string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func processCmdline(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
