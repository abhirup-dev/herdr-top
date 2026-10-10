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
	SortIO     Sort = "io"
	SortAge    Sort = "age"
)

func ParseSort(s string) (Sort, error) {
	switch s {
	case "memory", "mem":
		return SortMemory, nil
	case "cpu":
		return SortCPU, nil
	case "io", "disk":
		return SortIO, nil
	case "age":
		return SortAge, nil
	default:
		return "", fmt.Errorf("unknown sort %q (use memory, cpu, io, or age)", s)
	}
}
func Records(in []model.Record, by Sort) []model.Record {
	out := append([]model.Record(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch by {
		case SortCPU:
			return validFloatLess(a.TreeCPUPercent, b.TreeCPUPercent)
		case SortIO:
			return validLess(a.TreeDiskBytes, b.TreeDiskBytes)
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
		return fmt.Sprintf("%dd%dh", int(d/(24*time.Hour)), int(d/time.Hour)%24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d/time.Hour), int(d/time.Minute)%60)
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
func Row(r model.Record) string {
	sess, pane, state := "outside", "—", "—"
	if r.Location != nil {
		sess, pane, state = r.Location.Session, r.Location.Pane, r.Location.State
	}
	return fmt.Sprintf("%6d %-8s %6s %7s %7s %7s %7s %-10s %-12s %-7s", r.PID, r.Kind, CPU(r.TreeCPUPercent), Bytes(r.TreeFootprintBytes), Bytes(r.TreeRSSBytes), Bytes(r.TreeDiskBytes), Age(r.Age), sess, pane, state)
}
func Header() string {
	return fmt.Sprintf("%6s %-8s %6s %7s %7s %7s %7s %-10s %-12s %-7s", "PID", "KIND", "CPU%", "FOOT", "RSS", "DISKΣ", "AGE", "SESSION", "PANE", "STATE")
}
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
	if !r.TreeDiskBytes.Valid {
		d = append(d, "disk: "+r.TreeDiskBytes.Error)
	}
	d = append(d, r.Diagnostics...)
	return strings.Join(d, "; ")
}
