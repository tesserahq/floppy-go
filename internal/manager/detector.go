package manager

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"floppy-go/internal/config"
)

type RunningService struct {
	Name string
	Port int
	PID  int
	Type string
}

func DetectRunningServices(cfg *config.Config, root string) map[string]RunningService {
	out := map[string]RunningService{}
	for name, svc := range cfg.Services {
		if svc.Port <= 0 {
			continue
		}
		pid := pidForPort(svc.Port)
		if pid > 0 {
			out[name] = RunningService{Name: name, Port: svc.Port, PID: pid, Type: svc.Type}
		}
	}
	return out
}

func pidForPort(port int) int {
	cmd := exec.Command("lsof", "-t", "-i", fmt.Sprintf("tcp:%d", port))
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err == nil {
			return pid
		}
	}
	return 0
}
