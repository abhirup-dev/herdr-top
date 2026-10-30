package present

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abhirup-dev/herdr-top/internal/model"
)

type Sort string

const (
	SortMemory Sort = "memory"
	SortCPU    Sort = "cpu"
	SortAge    Sort = "age"
)

func ParseSort(s string) (Sort, error) {
	switch s {
	case "memory", "mem":
		return SortMemory, nil
	case "cpu":
		return SortCPU, nil
	case "age":
		return SortAge, nil
	default:
		return "", fmt.Errorf("unknown sort %q (use memory, cpu, or age)", s)
	}
}
func Records(in []model.Record, by Sort) []model.Record {
	out := append([]model.Record(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch by {
		case SortCPU:
			return validFloatLess(a.TreeCPUPercent, b.TreeCPUPercent)
		case SortAge:
			return a.Age > b.Age
		default:
			return validLess(a.TreeFootprintBytes, b.TreeFootprintBytes)
		}
	})
	return out
}
func validLess(a, b model.Value) bool {
	if a.Valid != b.Valid {
		return a.Valid
	}
	if !a.Valid {
		return false
	}
	return a.Value > b.Value
}
func validFloatLess(a, b model.FloatValue) bool {
	if a.Valid != b.Valid {
		return a.Valid
	}
	if !a.Valid {
		return false
	}
	return a.Value > b.Value
}
func Bytes(v model.Value) string {
	if !v.Valid {
		return "—"
	}
	const (
		k = 1024
	)
	switch {
	case v.Value >= k*k*k:
		return fmt.Sprintf("%.1fG", float64(v.Value)/(k*k*k))
	case v.Value >= k*k:
		return fmt.Sprintf("%.0fM", float64(v.Value)/(k*k))
	case v.Value >= k:
		return fmt.Sprintf("%.0fK", float64(v.Value)/k)
	default:
		return fmt.Sprintf("%dB", v.Value)
	}
}
func CPU(v model.FloatValue) string {
	if !v.Valid {
		return "—"
	}
	return fmt.Sprintf("%.1f", v.Value)
}
func Age(d time.Duration) string {
	if d < 0 {
		return "—"
	}
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd %dh", int(d/(24*time.Hour)), int(d/time.Hour)%24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh %02dm", int(d/time.Hour), int(d/time.Minute)%60)
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}
func Clip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// Column widths; negative means left-aligned. Shared by the plain and styled tables.
var Widths = []int{6, -8, 6, 7, 7, 7, -10, -8, -26, -24}

var headers = []string{"PID", "KIND", "CPU%", "FOOT", "RSS", "AGE", "SESSION", "STATE", "NAME", "AGENT"}

// Column indexes into Cells, Headers, and Widths.
const (
	ColPID = iota
	ColKind
	ColCPU
	ColFoot
	ColRSS
	ColAge
	ColSession
	ColState
	ColName
	ColAgent
)

// Cells returns the unpadded display values of a record, one per column.
func Cells(r model.Record) []string {
	sess, state, name, agent := "outside", "—", "", ""
	if l := r.Location; l != nil {
		sess, state, name, agent = l.Session, l.State, l.PaneLabel, l.AgentName
		if name == "" {
			name = l.PaneTitle
		}
	}
	return []string{fmt.Sprint(r.PID), r.Kind, CPU(r.TreeCPUPercent), Bytes(r.TreeFootprintBytes), Bytes(r.TreeRSSBytes), Age(r.Age), sess, state, name, agent}
}
func Headers() []string { return append([]string(nil), headers...) }

// Pad aligns s to column i's width, clipping text that would overflow it.
func Pad(s string, i int) string {
	return fmt.Sprintf("%*s", Widths[i], Clip(s, max(Widths[i], -Widths[i])))
}

// SortColumn is the column a sort orders by.
func SortColumn(by Sort) int {
	switch by {
	case SortCPU:
		return ColCPU
	case SortAge:
		return ColAge
	default:
		return ColFoot
	}
}
func join(cells []string) string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = Pad(c, i)
	}
	return strings.Join(out, " ")
}
func Row(r model.Record) string { return join(Cells(r)) }
func Header() string            { return join(headers) }
func Diagnostics(r model.Record) string {
	var d []string
	if !r.TreeCPUPercent.Valid {
		d = append(d, "CPU: "+r.TreeCPUPercent.Error)
	}
	if !r.TreeFootprintBytes.Valid {
		d = append(d, "footprint: "+r.TreeFootprintBytes.Error)
	}
	if !r.TreeRSSBytes.Valid {
		d = append(d, "RSS: "+r.TreeRSSBytes.Error)
	}
	d = append(d, r.Diagnostics...)
	return strings.Join(d, "; ")
}
