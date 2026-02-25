package manager

import (
	"path/filepath"
	"testing"
)

func Test_stateSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "process-state.json")
	t.Setenv("FLOPPY_STATE_FILE", statePath)

	initial := ProcessState{
		Entries: map[string]ProcessEntry{
			"api": {
				Service: "api",
				PID:     1234,
				PGID:    1234,
				Cwd:     "/tmp/api",
				Cmdline: "poetry run dev",
			},
		},
	}
	if err := saveProcessState(initial); err != nil {
		t.Fatalf("saveProcessState error: %v", err)
	}

	loaded := loadProcessState()
	entry, ok := loaded.Entries["api"]
	if !ok {
		t.Fatalf("expected api entry in loaded state")
	}
	if entry.PID != 1234 || entry.PGID != 1234 {
		t.Fatalf("unexpected loaded entry: %+v", entry)
	}
}

func Test_commandContainsExpected(t *testing.T) {
	if !commandContainsExpected("/usr/local/bin/poetry run dev", "poetry run dev") {
		t.Fatalf("expected command match")
	}
	if commandContainsExpected("bun dev", "poetry run dev") {
		t.Fatalf("did not expect command match")
	}
}

