//go:build unix

package manager

import (
	"os/exec"
	"syscall"
)

func syscallSetup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
