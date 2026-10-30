package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abhirup-dev/herdr-top/internal/model"
	"github.com/abhirup-dev/herdr-top/internal/present"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type RefreshFunc func(context.Context) model.Snapshot
type FocusFunc func(context.Context, model.Location) error

type refreshMsg struct {
	snapshot model.Snapshot
	manual   bool
}
type focusMsg struct {
	err  error
	pane string
}

// tickMsg carries the watch generation so a tick scheduled before a pause or a
// manual refresh cannot start a second, overlapping refresh.
type tickMsg struct{ gen int }

// identity distinguishes a reused PID from the process the user selected.
type identity struct {
	pid     int32
	started time.Time
}

type Model struct {
	Snapshot                      model.Snapshot
	sort                          present.Sort
	previous                      present.Sort
	rows                          []model.Record
	cursor, offset, width, height int
	message                       string
	failed                        bool
	refreshing, focusing          bool
	refresh                       RefreshFunc
	focus                         FocusFunc
	interval                      time.Duration
	paused                        bool
	gen                           int
	// QuitOnFocus exits after a successful focus so a Herdr popup closes onto the target pane.
	QuitOnFocus bool
}

// New builds the interactive model. A positive interval enables watch mode:
// the next refresh starts that long after the previous one completes.
func New(s model.Snapshot, refresh RefreshFunc, focus FocusFunc, interval time.Duration) Model {
	m := Model{Snapshot: s, sort: present.SortMemory, previous: present.SortMemory, width: 100, height: 24, refresh: refresh, focus: focus, interval: interval}
	m.rows = present.Records(s.Records, m.sort)
	return m
}
func (m Model) watching() bool { return m.interval > 0 && m.refresh != nil && !m.paused }
func (m Model) Init() tea.Cmd  { return m.schedule() }
func (m Model) schedule() tea.Cmd {
	if !m.watching() {
		return nil
	}
	gen := m.gen
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{gen} })
}
func (m Model) selected() identity {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return identity{m.rows[m.cursor].PID, m.rows[m.cursor].StartedAt}
	}
	return identity{}
}
func (m *Model) reselect(id identity) {
	if m.cursor >= len(m.rows) {
		m.cursor = max(len(m.rows)-1, 0)
	}
	for i := range m.rows {
		if m.rows[i].PID == id.pid && m.rows[i].StartedAt.Equal(id.started) {
			m.cursor = i
			return
		}
	}
}
func (m *Model) resort(by present.Sort) {
	id := m.selected()
	m.sort = by
	m.rows = present.Records(m.Snapshot.Records, m.sort)
	m.reselect(id)
}
func (m *Model) say(msg string, failed bool) { m.message, m.failed = msg, failed }
func (m Model) refreshCmd(manual bool) tea.Cmd {
	return func() tea.Msg { return refreshMsg{m.refresh(context.Background()), manual} }
}
func (m Model) focusCmd(loc model.Location) tea.Cmd {
	return func() tea.Msg { return focusMsg{err: m.focus(context.Background(), loc), pane: loc.Pane} }
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = x.Width, x.Height
	case tickMsg:
		if x.gen == m.gen && m.watching() && !m.refreshing {
			m.refreshing = true
			cmd = m.refreshCmd(false)
		}
	case refreshMsg:
		m.Snapshot = x.snapshot
		m.resort(m.sort)
		m.refreshing = false
		if x.manual {
			m.say(fmt.Sprintf("refreshed %d agents", len(m.rows)), false)
		}
		m.gen++
		cmd = m.schedule()
	case focusMsg:
		m.focusing = false
		if x.err != nil {
			m.say("focus failed: "+x.err.Error(), true)
		} else if m.QuitOnFocus {
			return m, tea.Quit
		} else {
			m.say("focused pane "+x.pane, false)
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
				m.say("refreshing…", false)
				cmd = m.refreshCmd(true)
			}
		case "p":
			if m.interval > 0 && m.refresh != nil {
				m.paused = !m.paused
				m.gen++
				if m.paused {
					m.say("watch paused", false)
				} else {
					m.say("watch resumed", false)
					if !m.refreshing {
						cmd = m.schedule()
					}
				}
			}
		case "enter":
			if len(m.rows) > 0 && !m.focusing {
				r := m.rows[m.cursor]
				if r.Location == nil {
					m.say("agent is not mapped to a live Herdr pane", true)
				} else if m.focus == nil {
					m.say("focus is unavailable", true)
				} else {
					m.focusing = true
					m.say("focusing…", false)
					cmd = m.focusCmd(*r.Location)
				}
			}
		case "esc":
			prev := m.sort
			m.resort(m.previous)
			m.previous = prev
			m.say("sort restored: "+string(m.sort), false)
		default:
			var by present.Sort
			switch x.String() {
			case "m":
				by = present.SortMemory
			case "c":
				by = present.SortCPU
			case "o":
				by = present.SortAge
			}
			if by != "" {
				m.previous = m.sort
				m.resort(by)
				m.say("sorted by "+string(by), false)
			}
		}
	}
	listH := max(m.height-4, 1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listH {
		m.offset = m.cursor - listH + 1
	}
	m.offset = max(m.offset, 0)
	return m, cmd
}

func (m Model) watchLabel() string {
	switch {
	case m.interval <= 0 || m.refresh == nil:
		return ""
	case m.paused:
		return st.paused.Render("⏸ paused")
	case m.refreshing:
		return st.live.Render("● live " + m.interval.String() + " ⟳")
	default:
		return st.live.Render("● live " + m.interval.String())
	}
}
func (m Model) View() string {
	if m.width < 20 || m.height < 5 {
		return present.Clip("herdr-top: terminal too small", m.width)
	}
	clip := func(s string) string { return ansi.Truncate(s, m.width, "…") }
	title := st.title.Render(" herdr-top ") + st.dim.Render(fmt.Sprintf(" %d agents · sort ", len(m.rows))) + st.accent.Render(string(m.sort))
	if w := m.watchLabel(); w != "" {
		title += st.dim.Render(" · ") + w
	}
	keys := []string{"m", "mem", "c", "cpu", "o", "age", "r", "refresh"}
	if m.interval > 0 && m.refresh != nil {
		keys = append(keys, "p", "pause")
	}
	keys = append(keys, "⏎", "focus", "q", "quit")
	var help []string
	for i := 0; i < len(keys); i += 2 {
		help = append(help, st.key.Render(keys[i])+" "+st.dim.Render(keys[i+1]))
	}
	lines := []string{clip(title), clip(strings.Join(help, "  ")), clip(m.header())}
	for n := 0; n < m.height-4; n++ {
		i := m.offset + n
		switch {
		case i >= len(m.rows):
			lines = append(lines, "")
		case i == m.cursor:
			// Plain text under one style so the selection bar stays unbroken.
			lines = append(lines, st.selected.Render(ansi.Truncate(fmt.Sprintf("%-*s", m.width, "▌"+present.Row(m.rows[i])), m.width, "…")))
		default:
			lines = append(lines, clip(" "+m.row(m.rows[i])))
		}
	}
	status, style := m.message, st.ok
	if m.failed {
		style = st.err
	}
	if status == "" && len(m.rows) > 0 {
		status, style = present.Diagnostics(m.rows[m.cursor]), st.warn
	}
	if status == "" && len(m.Snapshot.Diagnostics) > 0 {
		status, style = strings.Join(m.Snapshot.Diagnostics, "; "), st.warn
	}
	if status == "" {
		status, style = fmt.Sprintf("%d/%d", min(m.cursor+1, len(m.rows)), len(m.rows)), st.dim
	}
	lines = append(lines, clip(style.Render(status)))
	return strings.Join(lines, "\n")
}
func (m Model) header() string {
	hs := present.Headers()
	active := present.SortColumn(m.sort)
	out := make([]string, len(hs))
	for i, h := range hs {
		if i == active {
			out[i] = st.sortCol.Render(present.Pad(h+"▼", i))
		} else {
			out[i] = st.header.Render(present.Pad(h, i))
		}
	}
	return " " + strings.Join(out, st.header.Render(" "))
}
func (m Model) row(r model.Record) string {
	cells := present.Cells(r)
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = cellStyle(i, c, r).Render(present.Pad(c, i))
	}
	return strings.Join(out, " ")
}
