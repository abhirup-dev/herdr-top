package ui

import (
	"hash/fnv"

	"github.com/abhirup-dev/herdr-top/internal/model"
	"github.com/abhirup-dev/herdr-top/internal/present"
	"github.com/charmbracelet/lipgloss"
)

// Adaptive colors keep the table readable in both light and dark themes.
var (
	cRed     = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#FF6B6B"}
	cYellow  = lipgloss.AdaptiveColor{Light: "#B26A00", Dark: "#FFD166"}
	cGreen   = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#7BD88F"}
	cBlue    = lipgloss.AdaptiveColor{Light: "#1565C0", Dark: "#6CB6FF"}
	cCyan    = lipgloss.AdaptiveColor{Light: "#00838F", Dark: "#5EE1E6"}
	cMagenta = lipgloss.AdaptiveColor{Light: "#8E24AA", Dark: "#D291FF"}
	cOrange  = lipgloss.AdaptiveColor{Light: "#D84315", Dark: "#FF9E64"}
	cDim     = lipgloss.AdaptiveColor{Light: "#757575", Dark: "#8B8B8B"}
	cAccent  = lipgloss.AdaptiveColor{Light: "#5E35B1", Dark: "#B39DFF"}
	cSelBG   = lipgloss.AdaptiveColor{Light: "#D1C4E9", Dark: "#3B3361"}
	cHeadBG  = lipgloss.AdaptiveColor{Light: "#ECEFF1", Dark: "#262B36"}
)

var st = struct {
	title, accent, dim, key, header, sortCol, selected, live, paused, ok, err, warn lipgloss.Style
}{
	title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent),
	accent:   lipgloss.NewStyle().Bold(true).Foreground(cAccent),
	dim:      lipgloss.NewStyle().Foreground(cDim),
	key:      lipgloss.NewStyle().Bold(true).Foreground(cCyan),
	header:   lipgloss.NewStyle().Bold(true).Background(cHeadBG),
	sortCol:  lipgloss.NewStyle().Bold(true).Background(cHeadBG).Foreground(cAccent),
	selected: lipgloss.NewStyle().Bold(true).Background(cSelBG),
	live:     lipgloss.NewStyle().Bold(true).Foreground(cGreen),
	paused:   lipgloss.NewStyle().Bold(true).Foreground(cYellow),
	ok:       lipgloss.NewStyle().Foreground(cGreen),
	err:      lipgloss.NewStyle().Bold(true).Foreground(cRed),
	warn:     lipgloss.NewStyle().Foreground(cYellow),
}

var (
	kindColors = map[string]lipgloss.TerminalColor{
		"claude": cOrange, "codex": cGreen, "pi": cMagenta, "gemini": cBlue,
		"opencode": cCyan, "cursor": cYellow, "amp": cRed,
	}
	stateColors = map[string]lipgloss.TerminalColor{
		"working": cYellow, "blocked": cRed, "done": cCyan, "idle": cGreen,
	}
	sessionPalette = []lipgloss.TerminalColor{cBlue, cMagenta, cCyan, cOrange, cGreen, cYellow}
)

const mib = 1 << 20

func fg(c lipgloss.TerminalColor) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// cellStyle colors one table cell; unavailable values are always dim.
func cellStyle(col int, text string, r model.Record) lipgloss.Style {
	if text == "—" {
		return st.dim
	}
	switch col {
	case present.ColPID, present.ColAge:
		return st.dim
	case present.ColName:
		// A Herdr-derived title is dim; a name the user gave the pane stands out.
		if r.Location != nil && r.Location.PaneLabel != "" {
			return fg(cAccent).Bold(true)
		}
		return st.dim
	case present.ColAgent:
		return fg(cCyan)
	case present.ColKind:
		if c, ok := kindColors[r.Kind]; ok {
			return fg(c).Bold(true)
		}
		return st.accent
	case present.ColCPU:
		return fg(level(r.TreeCPUPercent.Value, 25, 75)).Bold(r.TreeCPUPercent.Value >= 75)
	case present.ColFoot:
		return fg(level(float64(r.TreeFootprintBytes.Value), 256*mib, 1024*mib))
	case present.ColRSS:
		return fg(level(float64(r.TreeRSSBytes.Value), 256*mib, 1024*mib))
	case present.ColSession:
		if r.Location == nil {
			return st.dim
		}
		h := fnv.New32a()
		h.Write([]byte(text))
		return fg(sessionPalette[h.Sum32()%uint32(len(sessionPalette))])
	case present.ColState:
		if c, ok := stateColors[text]; ok {
			return fg(c)
		}
	}
	return lipgloss.NewStyle()
}

func level(v, warn, hot float64) lipgloss.TerminalColor {
	switch {
	case v >= hot:
		return cRed
	case v >= warn:
		return cYellow
	default:
		return cGreen
	}
}
