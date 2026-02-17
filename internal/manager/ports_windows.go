//go:build windows

package manager

// Windows stubs for Unix-specific port handling functions

type PortConflict struct {
	Port      int
	Services  []string
	Processes []string
}

func lsofPort(port int) ([]string, error) {
	return nil, nil
}

func killPort(port int) error {
	return nil
}

func killProcess(pid int) error {
	return nil
}

func conflictsSummary(conflicts []PortConflict) string {
	return ""
}
