package manager

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type PortConflict struct {
	Port      int
	Services  []string
	Processes []string
}

func lsofPort(port int) ([]string, error) {
	cmd := exec.Command("lsof", "-i", fmt.Sprintf("tcp:%d", port))
	out, err := cmd.Output()
	if err != nil {
		// lsof returns non-zero when no processes found
		if exitErr, ok := err.(*exec.ExitError); ok {
			if len(exitErr.Stderr) == 0 {
				return nil, nil
			}
		}
		return nil, err
	}
	lines := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func killPort(port int) error {
	cmd := exec.Command("lsof", "-t", "-i", fmt.Sprintf("tcp:%d", port))
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

func conflictsSummary(conflicts []PortConflict) string {
	ports := []string{}
	for _, c := range conflicts {
		ports = append(ports, fmt.Sprintf("%d", c.Port))
	}
	return strings.Join(ports, ", ")
}

func killProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	pgid, err := syscall.Getpgid(pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		time.Sleep(1 * time.Second)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return nil
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	time.Sleep(1 * time.Second)
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return nil
}
