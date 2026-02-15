package manager

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"floppy-go/internal/config"
	"floppy-go/internal/tui"

	"github.com/creack/pty/v2"
)

type Manager struct {
	Config     *config.Config
	ConfigPath string
	Root       string

	procMu    sync.Mutex
	processes map[string]*exec.Cmd
	statuses  map[string]*ServiceStatus
}

type ServiceStatus struct {
	Name   string
	Type   string
	Port   int
	Status string
	PID    int
}

func New(cfg *config.Config, configPath string) *Manager {
	root := cfg.ServicesRoot(configPath)
	return &Manager{
		Config:     cfg,
		ConfigPath: configPath,
		Root:       root,
		processes:  map[string]*exec.Cmd{},
		statuses:   map[string]*ServiceStatus{},
	}
}

func (m *Manager) Up(services []string, detached bool, force bool, noPTY bool) error {
	if len(services) == 0 {
		services = m.Config.ServiceNames()
	}
	services = m.Config.ExpandBundles(services)
	if len(services) == 0 {
		return errors.New("no services to start")
	}

	for _, name := range services {
		svc := m.Config.Services[name]
		m.statuses[name] = &ServiceStatus{Name: name, Type: svc.Type, Port: svc.Port, Status: "starting"}
	}

	if err := m.validatePorts(services, force); err != nil {
		return err
	}

	portal := []string{}
	others := []string{}
	for _, name := range services {
		if m.Config.Services[name].Type == "portal" {
			portal = append(portal, name)
		} else {
			others = append(others, name)
		}
	}

	statusCh := make(chan tui.StatusUpdate, 64)
	logCh := make(chan tui.LogLine, 2048)

	startFn := func(name string) {
		if err := m.startService(name, detached, noPTY, logCh, statusCh); err != nil {
			statusCh <- tui.StatusUpdate{Name: name, Status: "error"}
			logCh <- tui.LogLine{Service: "ERROR", Text: fmt.Sprintf("%s: %v", name, err)}
		}
	}

	for _, name := range others {
		startFn(name)
	}

	for i, name := range portal {
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		startFn(name)
	}

	if detached {
		return nil
	}

	postgresURL := ""
	if m.Config.Stats != nil && m.Config.Stats.DB != nil && m.Config.Stats.DB.Enabled && m.Config.Stats.DB.URL != "" {
		postgresURL = m.Config.Stats.DB.URL
	}
	dockerEnabled := m.Config.Stats != nil && m.Config.Stats.Docker != nil && m.Config.Stats.Docker.Enabled
	model := tui.NewModel(logCh, statusCh, m.snapshotStatuses(), postgresURL, dockerEnabled)
	p := tui.NewProgram(model)
	if err := p.Start(); err != nil {
		return err
	}
	if model.Interrupted() {
		m.Stop(services)
	}
	return nil
}

func (m *Manager) Stop(services []string) error {
	detected := DetectRunningServices(m.Config, m.Root)

	toStop := []string{}
	if len(services) == 0 {
		for name := range detected {
			toStop = append(toStop, name)
		}
	} else {
		for _, name := range services {
			if _, ok := detected[name]; ok {
				toStop = append(toStop, name)
			}
		}
	}

	if len(toStop) == 0 {
		fmt.Println("No running services to stop")
		return nil
	}

	for _, name := range toStop {
		svc := m.Config.Services[name]
		if svc.Port > 0 {
			if err := killPort(svc.Port); err != nil {
				fmt.Printf("Failed to stop %s (port %d): %v\n", name, svc.Port, err)
			} else {
				fmt.Printf("Stopped %s\n", name)
			}
			continue
		}
		info := detected[name]
		if err := killProcess(info.PID); err != nil {
			fmt.Printf("Failed to stop %s (PID %d): %v\n", name, info.PID, err)
		} else {
			fmt.Printf("Stopped %s\n", name)
		}
	}

	return nil
}

func (m *Manager) Ps(quiet bool) {
	detected := DetectRunningServices(m.Config, m.Root)
	if len(detected) == 0 {
		fmt.Println("No services running")
		return
	}

	if quiet {
		names := make([]string, 0, len(detected))
		for name := range detected {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
		return
	}

	fmt.Printf("%-24s %-8s %-8s %-6s\n", "SERVICE", "STATUS", "PORT", "PID")
	fmt.Println(strings.Repeat("-", 52))
	keys := make([]string, 0, len(detected))
	for name := range detected {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		info := detected[name]
		fmt.Printf("%-24s %-8s %-8d %-6d\n", name, "RUN", info.Port, info.PID)
	}
}

func (m *Manager) List(grouped bool) {
	if !grouped {
		fmt.Println("Available services:")
		for name, svc := range m.Config.Services {
			port := "N/A"
			if svc.Port > 0 {
				port = fmt.Sprintf("%d", svc.Port)
			}
			fmt.Printf("  - %s (%s, port: %s)\n", name, svc.Type, port)
		}
		fmt.Println("\nAvailable bundles:")
		for name, services := range m.Config.Bundles {
			fmt.Printf("  - %s: %s\n", name, strings.Join(services, ", "))
		}
		return
	}

	byType := map[string][][3]string{}
	for name, svc := range m.Config.Services {
		port := "N/A"
		if svc.Port > 0 {
			port = fmt.Sprintf("%d", svc.Port)
		}
		path := svc.Path
		if path == "" {
			path = name
		}
		byType[svc.Type] = append(byType[svc.Type], [3]string{name, port, path})
	}

	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		rows := byType[t]
		sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
		fmt.Printf("\n%s Services (%d):\n", strings.ToUpper(t), len(rows))
		fmt.Printf("%-24s %-8s %-24s\n", "Service", "Port", "Path")
		fmt.Println(strings.Repeat("-", 60))
		for _, row := range rows {
			fmt.Printf("%-24s %-8s %-24s\n", row[0], row[1], row[2])
		}
	}

	if len(m.Config.Bundles) > 0 {
		fmt.Println("\nBundles:")
		keys := make([]string, 0, len(m.Config.Bundles))
		for name := range m.Config.Bundles {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			fmt.Printf("  - %s: %s\n", name, strings.Join(m.Config.Bundles[name], ", "))
		}
	}
}

func (m *Manager) Exec(cmdArgs []string, serviceType string, exclude []string) error {
	excludeSet := map[string]struct{}{}
	for _, name := range exclude {
		excludeSet[name] = struct{}{}
	}

	services := m.filteredServices(serviceType, excludeSet)
	if len(services) == 0 {
		fmt.Println("No services found to run command in")
		return nil
	}

	cmdStr := shellJoin(cmdArgs)
	fmt.Printf("Running '%s' in %d service(s)...\n", cmdStr, len(services))

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	for _, name := range services {
		svc := m.Config.Services[name]
		path := servicePath(m.Root, name, svc.Path)
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("⚠️  Skipping %s: directory not found (%s)\n", name, path)
			continue
		}

		header := fmt.Sprintf("═══ %s (%s) ═══", name, path)
		fmt.Printf("\n\x1b[1;38;5;15m\x1b[48;5;18m%s\x1b[0m\n", header)

		env := append(os.Environ(), config.MergeEnv(m.Config.Env, svc.Env)...)
		cmd := exec.Command(shell, "-i", "-c", cmdStr)
		cmd.Dir = path
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				fmt.Printf("  (exit code %d)\n", exitErr.ExitCode())
			} else {
				fmt.Printf("❌ Error in %s: %v\n", name, err)
			}
		}
	}

	return nil
}

func shellJoin(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote preserves arguments for POSIX shells when using `sh -c`.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if !(r == '_' || r == '-' || r == '.' || r == '/' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			// Single-quote and escape any embedded single quotes.
			return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
		}
	}
	return s
}

func (m *Manager) Pull(services []string) {
	if len(services) == 0 {
		for name, svc := range m.Config.Services {
			if svc.Repo != "" {
				services = append(services, name)
			}
		}
	}
	services = m.Config.ExpandBundles(services)
	if len(services) == 0 {
		fmt.Println("No services to pull")
		return
	}

	fmt.Printf("Pulling services: %s\n", strings.Join(services, ", "))
	for _, name := range services {
		svc := m.Config.Services[name]
		if svc.Repo == "" {
			fmt.Printf("Skipping %s: no repo configured\n", name)
			continue
		}
		path := servicePath(m.Root, name, svc.Path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("Cloning %s to %s\n", svc.Repo, path)
			cmd := exec.Command("git", "clone", svc.Repo, path)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			continue
		}
		cmd := exec.Command("git", "rev-parse", "--git-dir")
		cmd.Dir = path
		if err := cmd.Run(); err != nil {
			fmt.Printf("Skipping %s: not a git repository\n", name)
			continue
		}
		pull := exec.Command("git", "pull")
		pull.Dir = path
		pull.Stdout = os.Stdout
		pull.Stderr = os.Stderr
		_ = pull.Run()
	}
}

func (m *Manager) Reset(serviceType string, exclude []string) {
	excludeSet := map[string]struct{}{}
	for _, name := range exclude {
		excludeSet[name] = struct{}{}
	}
	services := m.filteredServices(serviceType, excludeSet)
	if len(services) == 0 {
		fmt.Println("No services found to reset")
		return
	}

	fmt.Printf("Resetting %d service(s)...\n", len(services))
	for _, name := range services {
		svc := m.Config.Services[name]
		path := servicePath(m.Root, name, svc.Path)
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("⚠️  Skipping %s: directory not found (%s)\n", name, path)
			continue
		}
		cmd := exec.Command("git", "rev-parse", "--git-dir")
		cmd.Dir = path
		if err := cmd.Run(); err != nil {
			fmt.Printf("⚠️  Skipping %s: not a git repository\n", name)
			continue
		}
		reset := exec.Command("git", "reset", "--hard", "HEAD")
		reset.Dir = path
		reset.Stdout = os.Stdout
		reset.Stderr = os.Stderr
		_ = reset.Run()
		clean := exec.Command("git", "clean", "-fd")
		clean.Dir = path
		clean.Stdout = os.Stdout
		clean.Stderr = os.Stderr
		_ = clean.Run()
		fmt.Printf("✅ Successfully reset %s\n", name)
	}
}

func (m *Manager) UpdateLib(lib string, serviceType string, exclude []string) {
	m.updateOrAddLib(lib, serviceType, exclude, true)
}

func (m *Manager) AddLib(lib string, serviceType string, exclude []string) {
	m.updateOrAddLib(lib, serviceType, exclude, false)
}

func (m *Manager) updateOrAddLib(lib string, serviceType string, exclude []string, allowUpdate bool) {
	excludeSet := map[string]struct{}{}
	for _, name := range exclude {
		excludeSet[name] = struct{}{}
	}
	services := m.filteredServices(serviceType, excludeSet)
	if len(services) == 0 {
		fmt.Printf("No services found to %s %s\n", verb(allowUpdate), lib)
		return
	}

	fmt.Printf("%s %s in %d service(s)...\n", strings.Title(verb(allowUpdate)), lib, len(services))
	for _, name := range services {
		svc := m.Config.Services[name]
		path := servicePath(m.Root, name, svc.Path)
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("⚠️  Skipping %s: directory not found (%s)\n", name, path)
			continue
		}
		if isPythonType(svc.Type) {
			m.poetryAddOrUpdate(name, path, lib, allowUpdate)
			continue
		}
		if svc.Type == "portal" {
			m.bunAdd(name, path, lib)
		}
	}
}

func (m *Manager) Setup() {
	currentPython, _ := exec.LookPath(resolveTool("python", "FLOPPY_PYTHON"))
	if currentPython == "" {
		currentPython, _ = exec.LookPath("python3")
	}

	for name, svc := range m.Config.Services {
		if !isPythonType(svc.Type) {
			continue
		}
		path := servicePath(m.Root, name, svc.Path)
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("⚠️  Directory not found for %s, skipping dependency installation.\n", name)
			continue
		}
		fmt.Printf("Installing dependencies for %s\n", name)
		env := append(os.Environ(), config.MergeEnv(m.Config.Env, svc.Env)...)
		cmd := exec.Command(resolveTool("poetry", "FLOPPY_POETRY"), "env", "use", currentPython)
		cmd.Dir = path
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()

		install := exec.Command(resolveTool("poetry", "FLOPPY_POETRY"), "install", "-v")
		install.Dir = path
		install.Env = env
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
		_ = install.Run()
	}

	dbUser := valueOr(m.Config.Env["DB_USER"], "postgres")
	dbPassword := valueOr(m.Config.Env["DB_PASSWORD"], "postgres")
	dbHost := valueOr(m.Config.Env["DB_HOST"], "localhost")
	pathEnv := os.Getenv("PATH")

	for name := range m.Config.Services {
		dbs := []string{name, fmt.Sprintf("%s_test", name)}
		for _, db := range dbs {
			query := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s'", db)
			check := exec.Command("psql", "-U", dbUser, "-h", dbHost, "-tAc", query)
			check.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", dbPassword), fmt.Sprintf("PATH=%s", pathEnv))
			out, _ := check.Output()
			if !strings.Contains(string(out), "1") {
				create := exec.Command("psql", "-U", dbUser, "-h", dbHost, "-c", fmt.Sprintf("CREATE DATABASE \"%s\";", db))
				create.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", dbPassword), fmt.Sprintf("PATH=%s", pathEnv))
				create.Stdout = os.Stdout
				create.Stderr = os.Stderr
				_ = create.Run()
			}
		}
	}

	for name, svc := range m.Config.Services {
		if !isPythonType(svc.Type) {
			continue
		}
		path := servicePath(m.Root, name, svc.Path)
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("Directory not found for %s, skipping migrations.\n", name)
			continue
		}
		migr := exec.Command(resolveTool("poetry", "FLOPPY_POETRY"), "run", "alembic", "upgrade", "heads")
		migr.Dir = path
		migr.Stdout = os.Stdout
		migr.Stderr = os.Stderr
		_ = migr.Run()
	}

	fmt.Println("Setup complete!")
}

func (m *Manager) Logs(service string, follow bool, tail int) {
	fmt.Printf("Logs for %s (follow=%v, tail=%d)\n", service, follow, tail)
	fmt.Println("Log functionality would be implemented here")
}

func (m *Manager) Doctor() {
	fmt.Println("Floppy doctor")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Config path: %s\n", m.ConfigPath)
	fmt.Printf("Services root: %s\n", m.Root)
	fmt.Printf("ASDF_DIR: %s\n", valueOr(os.Getenv("ASDF_DIR"), "(not set)"))
	fmt.Printf("FLOPPY_POETRY: %s\n", valueOr(os.Getenv("FLOPPY_POETRY"), "(not set)"))
	fmt.Printf("FLOPPY_BUN: %s\n", valueOr(os.Getenv("FLOPPY_BUN"), "(not set)"))
	fmt.Printf("FLOPPY_PYTHON: %s\n", valueOr(os.Getenv("FLOPPY_PYTHON"), "(not set)"))
	fmt.Println()
	fmt.Printf("Resolved poetry: %s\n", resolveTool("poetry", "FLOPPY_POETRY"))
	fmt.Printf("Resolved bun: %s\n", resolveTool("bun", "FLOPPY_BUN"))
	fmt.Printf("Resolved python: %s\n", resolveTool("python", "FLOPPY_PYTHON"))
}

func (m *Manager) Version(version string) {
	fmt.Printf("floppy version %s\n", version)
}

func (m *Manager) filteredServices(serviceType string, exclude map[string]struct{}) []string {
	out := []string{}
	for name, svc := range m.Config.Services {
		if _, ok := exclude[name]; ok {
			continue
		}
		if serviceType != "" && svc.Type != serviceType {
			continue
		}
		if isPythonType(svc.Type) || svc.Type == "portal" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Manager) startService(name string, detached bool, noPTY bool, logCh chan<- tui.LogLine, statusCh chan<- tui.StatusUpdate) error {
	svc, ok := m.Config.Services[name]
	if !ok {
		return fmt.Errorf("service '%s' not found", name)
	}

	cmd, err := m.buildCommand(name, svc)
	if err != nil {
		return err
	}
	m.prepareCmd(cmd, name, svc)

	if detached {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return err
		}
		m.trackProcess(name, cmd)
		statusCh <- tui.StatusUpdate{Name: name, Status: "running", PID: cmd.Process.Pid}
		go func() {
			_ = cmd.Wait()
			statusCh <- tui.StatusUpdate{Name: name, Status: "stopped"}
		}()
		return nil
	}

	if noPTY {
		return m.startWithPipes(name, cmd, logCh, statusCh)
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) {
			logCh <- tui.LogLine{Service: "WARN", Text: fmt.Sprintf("%s: PTY not permitted, falling back to pipes", name)}
			fallback, ferr := m.buildCommand(name, svc)
			if ferr != nil {
				return ferr
			}
			m.prepareCmd(fallback, name, svc)
			return m.startWithPipes(name, fallback, logCh, statusCh)
		}
		return err
	}
	m.trackProcess(name, cmd)
	statusCh <- tui.StatusUpdate{Name: name, Status: "running", PID: cmd.Process.Pid}

	go func() {
		defer func() { _ = ptmx.Close() }()
		reader := bufio.NewReader(ptmx)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				logCh <- tui.LogLine{Service: name, Text: strings.TrimRight(line, "\r\n")}
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		_ = cmd.Wait()
		statusCh <- tui.StatusUpdate{Name: name, Status: "stopped"}
	}()

	return nil
}

func (m *Manager) prepareCmd(cmd *exec.Cmd, name string, svc config.ServiceDef) {
	cmd.Dir = servicePath(m.Root, name, svc.Path)
	cmd.Env = append(os.Environ(), config.MergeEnv(m.Config.Env, svc.Env)...)
	if svc.Port > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", svc.Port))
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func (m *Manager) startWithPipes(name string, cmd *exec.Cmd, logCh chan<- tui.LogLine, statusCh chan<- tui.StatusUpdate) error {
	cmd.Stdout = nil
	cmd.Stderr = nil
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	m.trackProcess(name, cmd)
	statusCh <- tui.StatusUpdate{Name: name, Status: "running", PID: cmd.Process.Pid}

	go readLines(name, stdout, logCh)
	go readLines(name, stderr, logCh)

	go func() {
		_ = cmd.Wait()
		statusCh <- tui.StatusUpdate{Name: name, Status: "stopped"}
	}()

	return nil
}

func readLines(service string, r io.Reader, logCh chan<- tui.LogLine) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			logCh <- tui.LogLine{Service: service, Text: strings.TrimRight(line, "\r\n")}
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) buildCommand(name string, svc config.ServiceDef) (*exec.Cmd, error) {
	switch svc.Type {
	case "api", "webapp", "library", "python":
		cmd := svc.Command
		if cmd == "" {
			cmd = "dev"
		}
		return exec.Command(resolveTool("poetry", "FLOPPY_POETRY"), "run", cmd), nil
	case "worker":
		cmd := svc.WorkerCommand
		if cmd == "" {
			cmd = "worker"
		}
		return exec.Command(resolveTool("poetry", "FLOPPY_POETRY"), "run", cmd), nil
	case "portal":
		return exec.Command(resolveTool("bun", "FLOPPY_BUN"), "dev"), nil
	case "docker":
		if svc.Command == "" && svc.DockerCommand == "" {
			return nil, fmt.Errorf("docker service %s missing command", name)
		}
		cmdLine := svc.Command
		if cmdLine == "" {
			cmdLine = svc.DockerCommand
		}
		parts := strings.Fields(cmdLine)
		return exec.Command(parts[0], parts[1:]...), nil
	default:
		return nil, fmt.Errorf("unknown service type: %s", svc.Type)
	}
}

func (m *Manager) trackProcess(name string, cmd *exec.Cmd) {
	m.procMu.Lock()
	defer m.procMu.Unlock()
	m.processes[name] = cmd
}

func (m *Manager) snapshotStatuses() []tui.ServiceRow {
	rows := make([]tui.ServiceRow, 0, len(m.statuses))
	keys := make([]string, 0, len(m.statuses))
	for name := range m.statuses {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		status := m.statuses[name]
		rows = append(rows, tui.ServiceRow{Name: status.Name, Status: status.Status, Port: status.Port})
	}
	return rows
}

func (m *Manager) validatePorts(services []string, force bool) error {
	ports := map[int][]string{}
	for _, name := range services {
		svc := m.Config.Services[name]
		if svc.Port > 0 {
			ports[svc.Port] = append(ports[svc.Port], fmt.Sprintf("%s (main)", name))
		}
		if svc.Type == "portal" {
			if svc.HMRPort > 0 {
				ports[svc.HMRPort] = append(ports[svc.HMRPort], fmt.Sprintf("%s (HMR)", name))
			}
			if svc.WSPort > 0 {
				ports[svc.WSPort] = append(ports[svc.WSPort], fmt.Sprintf("%s (WebSocket)", name))
			}
			ports[24678] = append(ports[24678], fmt.Sprintf("%s (Vite default WebSocket)", name))
		}
	}

	conflicts := []PortConflict{}
	for port, users := range ports {
		procLines, err := lsofPort(port)
		if err != nil {
			fmt.Printf("Warning: could not check port %d: %v\n", port, err)
			continue
		}
		if len(procLines) > 0 {
			conflicts = append(conflicts, PortConflict{Port: port, Services: users, Processes: procLines})
		}
	}

	if len(conflicts) == 0 {
		fmt.Println("✅ All required ports are available")
		return nil
	}

	if force {
		fmt.Println("⚠️  Port conflicts detected! Killing existing processes...")
		for _, c := range conflicts {
			fmt.Printf("Port %d (needed by %s) is already in use:\n", c.Port, strings.Join(c.Services, ", "))
			for _, proc := range c.Processes {
				fmt.Printf("    %s\n", proc)
			}
			_ = killPort(c.Port)
			fmt.Printf("✅ Killed processes using port %d\n", c.Port)
		}
		return nil
	}

	fmt.Println("❌ Port conflicts detected! Cannot start services.")
	for _, c := range conflicts {
		fmt.Printf("Port %d (needed by %s) is already in use:\n", c.Port, strings.Join(c.Services, ", "))
		for _, proc := range c.Processes {
			fmt.Printf("    %s\n", proc)
		}
	}

	return fmt.Errorf("cannot start services due to port conflicts: %s", conflictsSummary(conflicts))
}

func servicePath(root, name, path string) string {
	if path == "" {
		path = name
	}
	return filepath.Join(root, path)
}

func isPythonType(t string) bool {
	switch t {
	case "api", "worker", "webapp", "library", "python":
		return true
	default:
		return false
	}
}

func valueOr(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	return fmt.Sprint(v)
}

func verb(update bool) string {
	if update {
		return "update"
	}
	return "add"
}

func (m *Manager) poetryAddOrUpdate(name, path, lib string, allowUpdate bool) {
	if _, err := os.Stat(filepath.Join(path, "pyproject.toml")); err != nil {
		fmt.Printf("⚠️  pyproject.toml not found for %s, skipping\n", name)
		return
	}
	cmd := exec.Command(resolveTool("poetry", "FLOPPY_POETRY"), "add", lib)
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil && allowUpdate {
		update := exec.Command(resolveTool("poetry", "FLOPPY_POETRY"), "update", lib)
		update.Dir = path
		update.Stdout = os.Stdout
		update.Stderr = os.Stderr
		_ = update.Run()
	}
}

func (m *Manager) bunAdd(name, path, lib string) {
	if _, err := os.Stat(filepath.Join(path, "package.json")); err != nil {
		fmt.Printf("⚠️  package.json not found for %s, skipping\n", name)
		return
	}
	cmd := exec.Command(resolveTool("bun", "FLOPPY_BUN"), "add", lib)
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func resolveTool(tool string, envKey string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}

	if asdfDir := os.Getenv("ASDF_DIR"); asdfDir != "" {
		if path := latestAsdfBin(asdfDir, tool); path != "" {
			return path
		}
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		if path := latestAsdfBin(filepath.Join(home, ".asdf"), tool); path != "" {
			return path
		}
	}

	return tool
}

func latestAsdfBin(asdfRoot string, tool string) string {
	installRoot := filepath.Join(asdfRoot, "installs", tool)
	entries, err := os.ReadDir(installRoot)
	if err != nil {
		return ""
	}

	versions := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	if len(versions) == 0 {
		return ""
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareSemver(versions[i], versions[j]) > 0
	})

	candidate := filepath.Join(installRoot, versions[0], "bin", tool)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func compareSemver(a, b string) int {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	out := [3]int{}
	part := ""
	idx := 0
	for _, r := range v {
		if r >= '0' && r <= '9' {
			part += string(r)
		} else if part != "" {
			if idx < 3 {
				out[idx] = atoi(part)
				idx++
			}
			part = ""
		}
	}
	if part != "" && idx < 3 {
		out[idx] = atoi(part)
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
