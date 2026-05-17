package manager

import (
	"os/exec"
	"testing"
)

func Test_lsofPort_unusedHighPort(t *testing.T) {
	const port = 59999
	lines, err := lsofPort(port)
	if err != nil {
		t.Fatalf("lsofPort(%d) on free port: %v", port, err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no listeners, got %v", lines)
	}
}

func Test_lsofExitMeansPortFree(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if !lsofExitMeansPortFree(err) {
		t.Fatalf("expected exit 1 to mean port free, got %v", err)
	}

	cmd = exec.Command("sh", "-c", "exit 2")
	err = cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if lsofExitMeansPortFree(err) {
		t.Fatalf("expected exit 2 to be a real error, got %v", err)
	}

	if lsofExitMeansPortFree(exec.ErrNotFound) {
		t.Fatal("expected ErrNotFound to be a real error")
	}
}
