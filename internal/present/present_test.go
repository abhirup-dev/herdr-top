package present

import (
	"testing"
	"time"

	"github.com/abhirup-dev/herdr-top/internal/model"
)

func TestAllSortModesAndUnavailableLast(t *testing.T) {
	v := func(n uint64) model.Value { return model.Value{Value: n, Valid: true} }
	f := func(n float64) model.FloatValue { return model.FloatValue{Value: n, Valid: true} }
	rows := []model.Record{{PID: 1, TreeFootprintBytes: v(30), TreeCPUPercent: f(1), TreeDiskBytes: v(2), Age: time.Hour}, {PID: 2, TreeFootprintBytes: v(20), TreeCPUPercent: f(3), TreeDiskBytes: v(1), Age: 3 * time.Hour}, {PID: 3}}
	cases := map[Sort]int32{SortMemory: 1, SortCPU: 2, SortAge: 2}
	for by, want := range cases {
		got := Records(rows, by)
		if got[0].PID != want || got[len(got)-1].PID != 3 {
			t.Fatalf("sort %s order=%v", by, []int32{got[0].PID, got[1].PID, got[2].PID})
		}
	}
}

func TestNameFallsBackToTitleAndCellsClip(t *testing.T) {
	r := model.Record{PID: 1, Location: &model.Location{Session: "s", PaneTitle: "a very long Claude session title here", AgentName: "bot"}}
	c := Cells(r)
	if c[ColName] != "a very long Claude session title here" || c[ColAgent] != "bot" {
		t.Fatalf("cells=%q", c)
	}
	r.Location.PaneLabel = "mine"
	if Cells(r)[ColName] != "mine" {
		t.Fatal("pane label did not take precedence over title")
	}
	if got := Pad(c[ColName], ColName); len([]rune(got)) != 26 || []rune(got)[25] != '…' {
		t.Fatalf("pad=%q", got)
	}
}
