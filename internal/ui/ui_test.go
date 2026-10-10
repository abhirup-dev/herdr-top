package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/abhirup-dev/herdr-top/internal/model"
	tea "github.com/charmbracelet/bubbletea"
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
	m := New(model.Snapshot{Records: []model.Record{*b, *a}}, func(context.Context) model.Snapshot { return model.Snapshot{Records: []model.Record{*a, *b}} }, nil)
	m.cursor = 1
	pid := m.selectedPID()
	next, cmd := m.Update(key("r"))
	if cmd == nil {
		t.Fatal("missing refresh command")
	}
	next, _ = next.Update(cmd())
	got := next.(Model)
	if got.selectedPID() != pid {
		t.Fatalf("selected pid=%d want %d", got.selectedPID(), pid)
	}
	sorted, _ := got.Update(key("c"))
	if sorted.(Model).selectedPID() != pid {
		t.Fatal("sort did not preserve selection")
	}
}
func TestFocusFailureIsVisibleAndAwaited(t *testing.T) {
	r := rec(1, 1)
	m := New(model.Snapshot{Records: []model.Record{*r}}, nil, func(context.Context, model.Location) error { return errors.New("synthetic rejection") })
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
	m := New(model.Snapshot{Records: []model.Record{*r}}, nil, nil)
	m.width = 24
	m.height = 7
	for _, line := range strings.Split(m.View(), "\n") {
		if len([]rune(line)) > 24 {
			t.Fatalf("line too wide (%d): %q", len([]rune(line)), line)
		}
	}
}
