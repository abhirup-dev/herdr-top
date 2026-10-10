package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/abhirup-dev/herdr-top/internal/model"
	"github.com/abhirup-dev/herdr-top/internal/present"
	tea "github.com/charmbracelet/bubbletea"
)

type RefreshFunc func(context.Context) model.Snapshot
type FocusFunc func(context.Context, model.Location) error

type refreshMsg struct{ snapshot model.Snapshot }
type focusMsg struct {
	err  error
	pane string
}

type Model struct {
	Snapshot                      model.Snapshot
	sort                          present.Sort
	previous                      present.Sort
	rows                          []model.Record
	cursor, offset, width, height int
	message                       string
	refreshing, focusing          bool
	refresh                       RefreshFunc
	focus                         FocusFunc
}

func New(s model.Snapshot, refresh RefreshFunc, focus FocusFunc) Model {
	m := Model{Snapshot: s, sort: present.SortMemory, previous: present.SortMemory, width: 100, height: 24, refresh: refresh, focus: focus}
	m.rows = present.Records(s.Records, m.sort)
	return m
}
func (m Model) Init() tea.Cmd { return nil }
func (m Model) selectedPID() int32 {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor].PID
	}
	return 0
}
func (m *Model) selectPID(pid int32) {
	m.cursor = 0
	for i := range m.rows {
		if m.rows[i].PID == pid {
			m.cursor = i
			break
		}
	}
}
func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg { return refreshMsg{m.refresh(context.Background())} }
}
func (m Model) focusCmd(loc model.Location) tea.Cmd {
	return func() tea.Msg { return focusMsg{err: m.focus(context.Background(), loc), pane: loc.Pane} }
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = x.Width, x.Height
	case refreshMsg:
		pid := m.selectedPID()
		m.Snapshot = x.snapshot
		m.rows = present.Records(x.snapshot.Records, m.sort)
		m.selectPID(pid)
		m.refreshing = false
		m.message = fmt.Sprintf("refreshed %d agents", len(m.rows))
	case focusMsg:
		m.focusing = false
		if x.err != nil {
			m.message = "focus failed: " + x.err.Error()
		} else {
			m.message = "focused pane " + x.pane
		}
	case tea.KeyMsg:
		switch x.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "r":
			if !m.refreshing && m.refresh != nil {
				m.refreshing = true
				m.message = "refreshing…"
				return m, m.refreshCmd()
			}
		case "enter":
			if len(m.rows) > 0 && !m.focusing {
				r := m.rows[m.cursor]
				if r.Location == nil {
					m.message = "agent is not mapped to a live Herdr pane"
				} else if m.focus == nil {
					m.message = "focus is unavailable"
				} else {
					m.focusing = true
					m.message = "focusing…"
					return m, m.focusCmd(*r.Location)
				}
			}
		case "esc":
			pid := m.selectedPID()
			m.sort, m.previous = m.previous, m.sort
			m.rows = present.Records(m.Snapshot.Records, m.sort)
			m.selectPID(pid)
			m.message = "sort restored: " + string(m.sort)
		default:
			var by present.Sort
			switch x.String() {
			case "m":
				by = present.SortMemory
			case "c":
				by = present.SortCPU
			case "i":
				by = present.SortIO
			case "o":
				by = present.SortAge
			}
			if by != "" {
				pid := m.selectedPID()
				m.previous, m.sort = m.sort, by
				m.rows = present.Records(m.Snapshot.Records, m.sort)
				m.selectPID(pid)
				m.message = "sorted by " + string(by)
			}
		}
	}
	listH := m.height - 4
	if listH < 1 {
		listH = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listH {
		m.offset = m.cursor - listH + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	return m, nil
}
func (m Model) View() string {
	if m.width < 20 || m.height < 5 {
		return present.Clip("herdr-top: terminal too small", m.width)
	}
	lines := []string{present.Clip(fmt.Sprintf("herdr-top — %d agents — %s sort", len(m.rows), m.sort), m.width), present.Clip("m memory · c cpu · i I/O · o age · r refresh · Enter focus · q quit", m.width), present.Clip(present.Header(), m.width)}
	h := m.height - 4
	for n := 0; n < h; n++ {
		i := m.offset + n
		if i >= len(m.rows) {
			lines = append(lines, "")
			continue
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		lines = append(lines, present.Clip(prefix+present.Row(m.rows[i]), m.width))
	}
	status := m.message
	if status == "" && len(m.rows) > 0 {
		status = present.Diagnostics(m.rows[m.cursor])
	}
	if status == "" && len(m.Snapshot.Diagnostics) > 0 {
		status = strings.Join(m.Snapshot.Diagnostics, "; ")
	}
	if status == "" {
		status = fmt.Sprintf("%d/%d", min(m.cursor+1, len(m.rows)), len(m.rows))
	}
	lines = append(lines, present.Clip(status, m.width))
	return strings.Join(lines, "\n")
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
