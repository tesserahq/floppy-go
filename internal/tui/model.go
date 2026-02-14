package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type LogLine struct {
	Service string
	Text    string
}

type StatusUpdate struct {
	Name   string
	Status string
	PID    int
}

type ServiceRow struct {
	Name   string
	Status string
	Port   int
}

type Model struct {
	viewport    viewport.Model
	logCh       <-chan LogLine
	statusCh    <-chan StatusUpdate
	logs        []string
	statuses    map[string]ServiceRow
	colors      map[string]lipgloss.Color
	width       int
	height      int
	interrupted bool
	mu          sync.Mutex
	initialized bool
}

type tickMsg time.Time

func NewModel(logCh <-chan LogLine, statusCh <-chan StatusUpdate, initial []ServiceRow) *Model {
	statuses := map[string]ServiceRow{}
	for _, row := range initial {
		statuses[row.Name] = row
	}

	return &Model{
		viewport: viewport.New(10, 10),
		logCh:    logCh,
		statusCh: statusCh,
		logs:     []string{},
		statuses: statuses,
		colors:   map[string]lipgloss.Color{},
	}
}

func NewProgram(model *Model) *tea.Program {
	return tea.NewProgram(model, tea.WithAltScreen())
}

func (m *Model) Init() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.interrupted = true
			return m, tea.Quit
		}
	case tickMsg:
		m.drainLogs()
		m.drainStatuses()
		m.renderViewport()
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
	}
	return m, nil
}

func (m *Model) View() string {
	left := m.renderLogsPanel()
	right := m.renderStatusPanel()
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m *Model) Interrupted() bool {
	return m.interrupted
}

func (m *Model) drainLogs() {
	for {
		select {
		case line := <-m.logCh:
			m.appendLog(line)
		default:
			return
		}
	}
}

func (m *Model) drainStatuses() {
	for {
		select {
		case update := <-m.statusCh:
			row := m.statuses[update.Name]
			row.Name = update.Name
			if update.Status != "" {
				row.Status = update.Status
			}
			m.statuses[update.Name] = row
		default:
			return
		}
	}
}

func (m *Model) appendLog(line LogLine) {
	service := line.Service
	if service == "" {
		service = "INFO"
	}
	color := m.colorFor(service)
	prefix := lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("[%s]", service))
	m.logs = append(m.logs, fmt.Sprintf("%s %s", prefix, line.Text))
	if len(m.logs) > 2000 {
		m.logs = m.logs[len(m.logs)-2000:]
	}
}

func (m *Model) renderViewport() {
	m.viewport.SetContent(strings.Join(m.logs, "\n"))
	m.viewport.GotoBottom()
}

func (m *Model) resize() {
	rightWidth := 52
	leftWidth := m.width - rightWidth
	if leftWidth < 20 {
		leftWidth = 20
	}
	m.viewport.Width = leftWidth - 2
	m.viewport.Height = m.height - 4
	if m.viewport.Height < 5 {
		m.viewport.Height = 5
	}
}

func (m *Model) renderLogsPanel() string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	content := m.viewport.View()
	if !m.initialized && m.width > 0 {
		m.initialized = true
		m.renderViewport()
	}
	return box.Width(m.viewport.Width + 2).Height(m.viewport.Height + 2).Render(content)
}

func (m *Model) renderStatusPanel() string {
	rows := make([]ServiceRow, 0, len(m.statuses))
	for _, row := range m.statuses {
		rows = append(rows, row)
	}
	// stable order
	for i := 0; i < len(rows)-1; i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].Name < rows[i].Name {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}

	statusLines := []string{"Service                Status   Port"}
	for _, row := range rows {
		statusLines = append(statusLines, fmt.Sprintf("%-22s %-7s %5s", row.Name, statusDot(row.Status), portStr(row.Port)))
	}
	content := strings.Join(statusLines, "\n")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	return box.Width(52).Render(content)
}

func (m *Model) colorFor(service string) lipgloss.Color {
	if color, ok := m.colors[service]; ok {
		return color
	}
	palette := []lipgloss.Color{"2", "3", "4", "5", "6", "9", "10", "11", "12", "13"}
	color := palette[len(m.colors)%len(palette)]
	m.colors[service] = color
	return color
}

func statusDot(status string) string {
	switch status {
	case "running":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● RUN")
	case "starting":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("○ ...")
	case "error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ ERR")
	case "stopped":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ ---")
	default:
		return ""
	}
}

func portStr(port int) string {
	if port == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", port)
}
