package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abhirup-dev/herdr-top/internal/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
func rec(pid int32, mem uint64) *model.Record {
	return &model.Record{PID: pid, Kind: "codex", TreeFootprintBytes: model.Value{Value: mem, Valid: true}, TreeRSSBytes: model.Value{Valid: true}, TreeDiskBytes: model.Value{Valid: true}, Location: &model.Location{Session: "default", Pane: "p1"}}
}
func TestSortAndRefreshPreserveSelectedPID(t *testing.T) {
	a, b := rec(1, 20), rec(2, 10)
	m := New(model.Snapshot{Records: []model.Record{*b, *a}}, func(context.Context) model.Snapshot { return model.Snapshot{Records: []model.Record{*a, *b}} }, nil, 0)
	m.cursor = 1
	pid := m.selected()
	next, cmd := m.Update(key("r"))
	if cmd == nil {
		t.Fatal("missing refresh command")
	}
	next, _ = next.Update(cmd())
	got := next.(Model)
	if got.selected() != pid {
		t.Fatalf("selected=%v want %v", got.selected(), pid)
	}
	sorted, _ := got.Update(key("c"))
	if sorted.(Model).selected() != pid {
		t.Fatal("sort did not preserve selection")
	}
}
func TestFocusFailureIsVisibleAndAwaited(t *testing.T) {
	r := rec(1, 1)
	m := New(model.Snapshot{Records: []model.Record{*r}}, nil, func(context.Context, model.Location) error { return errors.New("synthetic rejection") }, 0)
	next, cmd := m.Update(key("enter"))
	if cmd == nil || !next.(Model).focusing {
		t.Fatal("focus command was not started")
	}
	next, _ = next.Update(cmd())
	got := next.(Model)
	if !strings.Contains(got.View(), "synthetic rejection") || got.focusing {
		t.Fatalf("view=%q focusing=%v", got.View(), got.focusing)
	}
}
func TestNarrowViewClipsEveryLine(t *testing.T) {
	r := rec(1, 1)
	m := New(model.Snapshot{Records: []model.Record{*r}}, nil, nil, 0)
	m.width = 24
	m.height = 7
	for _, line := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(line) > 24 {
			t.Fatalf("line too wide (%d): %q", ansi.StringWidth(line), line)
		}
	}
}

func TestWatchRefreshesWithoutOverlapAndPauses(t *testing.T) {
	a, b := rec(1, 20), rec(2, 10)
	calls := 0
	refresh := func(context.Context) model.Snapshot { calls++; return model.Snapshot{Records: []model.Record{*a, *b}} }
	m := New(model.Snapshot{Records: []model.Record{*b}}, refresh, nil, 2*time.Second)
	if m.Init() == nil {
		t.Fatal("watch did not schedule a first tick")
	}
	next, cmd := m.Update(tickMsg{gen: m.gen})
	if cmd == nil || !next.(Model).refreshing {
		t.Fatal("tick did not start a refresh")
	}
	// A duplicate tick while a refresh is in flight must not start another.
	if _, dup := next.Update(tickMsg{gen: next.(Model).gen}); dup != nil {
		t.Fatal("overlapping refresh started")
	}
	next, again := next.Update(cmd())
	got := next.(Model)
	if len(got.rows) != 2 || got.selected().pid != 2 || got.refreshing || again == nil || calls != 1 {
		t.Fatalf("rows=%d selected=%d refreshing=%v rescheduled=%v calls=%d", len(got.rows), got.selected().pid, got.refreshing, again != nil, calls)
	}
	if got.message != "" {
		t.Fatalf("auto refresh overwrote status: %q", got.message)
	}
	// Pausing invalidates the pending tick; resuming schedules a fresh one.
	paused, _ := got.Update(key("p"))
	if _, c := paused.Update(tickMsg{gen: got.gen}); c != nil || !strings.Contains(paused.View(), "paused") {
		t.Fatal("stale tick ran while paused")
	}
	if _, c := paused.Update(key("p")); c == nil {
		t.Fatal("resume did not schedule a tick")
	}
}
func TestRefreshKeepsSelectionOffReusedPID(t *testing.T) {
	old := rec(7, 10)
	old.StartedAt = time.Unix(100, 0)
	other := rec(8, 5)
	reused := rec(7, 50)
	reused.StartedAt = time.Unix(200, 0)
	m := New(model.Snapshot{Records: []model.Record{*old, *other}}, nil, nil, time.Second)
	next, _ := m.Update(refreshMsg{snapshot: model.Snapshot{Records: []model.Record{*reused, *other}}})
	got := next.(Model)
	if got.selected().started.Equal(old.StartedAt) || got.rows[got.cursor].StartedAt.Equal(old.StartedAt) {
		t.Fatal("selection followed a reused PID")
	}
}

func TestQuitOnFocusExitsOnlyAfterSuccess(t *testing.T) {
	r := rec(1, 1)
	m := New(model.Snapshot{Records: []model.Record{*r}}, nil, func(context.Context, model.Location) error { return nil }, 0)
	m.QuitOnFocus = true
	next, cmd := m.Update(key("enter"))
	if _, cmd = next.Update(cmd()); cmd == nil {
		t.Fatal("successful focus did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("successful focus did not quit")
	}
}
