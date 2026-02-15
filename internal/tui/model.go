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
	logs        []LogLine
	statuses    map[string]ServiceRow
	filters     map[string]bool
	colors      map[string]lipgloss.Color
	width       int
	height      int
	interrupted bool
	follow      bool
	focusStatus bool
	selected    int
	filterMode  bool
	filterText  string
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
		logs:     []LogLine{},
		statuses: statuses,
		filters:  map[string]bool{},
		colors:   map[string]lipgloss.Color{},
		follow:   true,
	}
}

func NewProgram(model *Model) *tea.Program {
	return tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
		if m.filterMode {
			switch msg.String() {
			case "esc":
				m.filterMode = false
				m.filterText = ""
				return m, nil
			case "enter":
				m.filterMode = false
				return m, nil
			case "backspace", "ctrl+h":
				if len(m.filterText) > 0 {
					m.filterText = m.filterText[:len(m.filterText)-1]
				}
				return m, nil
			default:
				if msg.Type == tea.KeyRunes {
					m.filterText += msg.String()
				}
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.interrupted = true
			return m, tea.Quit
		case "tab":
			m.focusStatus = !m.focusStatus
			return m, nil
		case "/":
			m.focusStatus = true
			m.filterMode = true
			return m, nil
		case " ":
			if m.focusStatus {
				m.toggleSelectedFilter()
				return m, nil
			}
		case "a":
			if m.focusStatus {
				m.setAllFilters(true)
				return m, nil
			}
		case "n":
			if m.focusStatus {
				m.setAllFilters(false)
				return m, nil
			}
		case "f":
			if !m.focusStatus {
				m.follow = !m.follow
				if m.follow {
					m.viewport.GotoBottom()
				}
				return m, nil
			}
		case "j", "down":
			if m.focusStatus {
				m.moveSelection(1)
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.follow = m.viewport.AtBottom()
			return m, cmd
		case "k", "up":
			if m.focusStatus {
				m.moveSelection(-1)
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.follow = m.viewport.AtBottom()
			return m, cmd
		case "g":
			if m.focusStatus {
				m.selected = 0
				return m, nil
			}
			m.viewport.GotoTop()
			m.follow = false
			return m, nil
		case "G", "end":
			if m.focusStatus {
				m.selected = m.maxSelection()
				return m, nil
			}
			m.viewport.GotoBottom()
			m.follow = true
			return m, nil
		case "pgup", "pgdown", "home":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.follow = m.viewport.AtBottom()
			return m, cmd
		}
	case tea.MouseMsg:
		if !m.focusStatus && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.follow = m.viewport.AtBottom()
			return m, cmd
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
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
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
			if _, ok := m.filters[update.Name]; !ok {
				m.filters[update.Name] = true
			}
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
	m.logs = append(m.logs, LogLine{Service: service, Text: line.Text})
	if len(m.logs) > 2000 {
		m.logs = m.logs[len(m.logs)-2000:]
	}
	if _, ok := m.filters[service]; !ok {
		m.filters[service] = true
	}
}

func (m *Model) renderViewport() {
	lines := make([]string, 0, len(m.logs))
	showAll := len(m.filters) == 0
	for _, line := range m.logs {
		if !showAll {
			if ok := m.filters[line.Service]; !ok {
				continue
			}
		}
		color := m.colorFor(line.Service)
		prefix := lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("[%s]", line.Service))
		lines = append(lines, fmt.Sprintf("%s %s", prefix, line.Text))
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	if m.follow {
		m.viewport.GotoBottom()
	}
}

func (m *Model) resize() {
	rightWidth := 52
	leftWidth := m.width - rightWidth
	if leftWidth < 20 {
		leftWidth = 20
	}
	m.viewport.Width = leftWidth - 2
	m.viewport.Height = m.height - 5
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
	rows := m.sortedRows()
	if m.filterText != "" {
		filtered := rows[:0]
		needle := strings.ToLower(m.filterText)
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Name), needle) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	title := "Service                Status   Port"
	if m.focusStatus {
		title = lipgloss.NewStyle().Bold(true).Render(title)
	}
	statusLines := []string{title}
	for i, row := range rows {
		checked := "[ ]"
		if m.filters[row.Name] {
			checked = "[x]"
		}
		name := row.Name
		if m.focusStatus && i == m.selected {
			name = lipgloss.NewStyle().Bold(true).Render(name)
		}
		statusLines = append(statusLines, fmt.Sprintf("%s %-19s %-7s %5s", checked, name, statusDot(row.Status), portStr(row.Port)))
	}
	content := strings.Join(statusLines, "\n")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	return box.Width(52).Render(content)
}

func (m *Model) moveSelection(delta int) {
	max := m.maxSelection()
	if max < 0 {
		m.selected = 0
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected > max {
		m.selected = max
	}
}

func (m *Model) maxSelection() int {
	if len(m.statuses) == 0 {
		return -1
	}
	rows := m.sortedRows()
	if m.filterText != "" {
		needle := strings.ToLower(m.filterText)
		filtered := rows[:0]
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Name), needle) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if len(rows) == 0 {
		return -1
	}
	return len(rows) - 1
}

func (m *Model) toggleSelectedFilter() {
	rows := m.sortedRows()
	if m.filterText != "" {
		needle := strings.ToLower(m.filterText)
		filtered := rows[:0]
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Name), needle) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if len(rows) == 0 {
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(rows) {
		m.selected = len(rows) - 1
	}
	name := rows[m.selected].Name
	m.filters[name] = !m.filters[name]
}

func (m *Model) setAllFilters(val bool) {
	for name := range m.statuses {
		m.filters[name] = val
	}
}

func (m *Model) sortedRows() []ServiceRow {
	rows := make([]ServiceRow, 0, len(m.statuses))
	for _, row := range m.statuses {
		rows = append(rows, row)
	}
	for i := 0; i < len(rows)-1; i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].Name < rows[i].Name {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	return rows
}

func (m *Model) renderFooter() string {
	keys := "keys: q quit • tab focus • / filter • space toggle • a all • n none • j/k scroll • g/G top/bottom • f follow"
	if m.focusStatus {
		keys = "keys: q quit • tab focus • / filter • space toggle • a all • n none • j/k select • g/G top/bottom • esc clear filter"
	}
	if m.filterText != "" {
		keys += " • filter: " + m.filterText
		if m.filterMode {
			keys += " (typing...)"
		}
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	return style.Width(m.width).Render(keys)
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
