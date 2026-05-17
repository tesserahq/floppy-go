//go:build windows

package manager

import "os/exec"

func syscallSetup(cmd *exec.Cmd) {
	// No-op on Windows - process group handling is not applicable
}
