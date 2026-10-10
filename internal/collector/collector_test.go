package collector

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abhirup-dev/herdr-top/internal/model"
)

type fakeSource struct{ calls int }

func ok(n uint64) model.Value { return model.Value{Value: n, Valid: true} }
func (f *fakeSource) List(context.Context, bool) (map[int32]rawProcess, []string) {
	f.calls++
	cpu := map[int32]float64{1: 10, 2: 3, 3: 1}
	if f.calls == 2 {
		cpu = map[int32]float64{1: 10.2, 2: 3.1, 3: 1.1}
	}
	at := time.Unix(int64(f.calls), 0)
	return map[int32]rawProcess{1: {pid: 1, ppid: 3, name: "codex", started: 1000, cpu: cpu[1], cpuAt: at, rss: ok(10), footprint: ok(20), read: ok(2), write: ok(3)}, 2: {pid: 2, ppid: 1, name: "helper", started: 1000, cpu: cpu[2], cpuAt: at, rss: ok(4), footprint: ok(5), read: ok(7), write: ok(11)}, 3: {pid: 3, ppid: 2, name: "helper", started: 1000, cpu: cpu[3], cpuAt: at, rss: ok(6), footprint: model.Value{Error: "denied"}, read: ok(13), write: ok(17)}}, nil
}
func TestCollectAggregatesTreeDetectsCyclesAndInvalidMetrics(t *testing.T) {
	now := time.Unix(10, 0)
	c := &Collector{Source: &fakeSource{}, Now: func() time.Time { v := now; now = now.Add(time.Second); return v }, Sleep: func(time.Duration) {}}
	s := c.Collect(context.Background(), time.Second, nil)
	if len(s.Records) != 1 {
		t.Fatalf("records=%d", len(s.Records))
	}
	r := s.Records[0]
	if !r.TreeRSSBytes.Valid || r.TreeRSSBytes.Value != 20 {
		t.Fatalf("tree RSS=%+v", r.TreeRSSBytes)
	}
	if r.TreeFootprintBytes.Valid || !strings.Contains(r.TreeFootprintBytes.Error, "pid 3") {
		t.Fatalf("tree footprint=%+v", r.TreeFootprintBytes)
	}
	if !r.TreeCPUPercent.Valid || r.TreeCPUPercent.Value < 39.9 || r.TreeCPUPercent.Value > 40.1 {
		t.Fatalf("tree CPU=%+v", r.TreeCPUPercent)
	}
	if len(r.Diagnostics) == 0 || !strings.Contains(r.Diagnostics[0], "cycle") {
		t.Fatalf("diagnostics=%v", r.Diagnostics)
	}
}
func TestDescendantsHandlesSelfCycle(t *testing.T) {
	ids, cycle := descendants(7, map[int32][]int32{7: {7}})
	if !cycle || len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("ids=%v cycle=%v", ids, cycle)
	}
}

type reusedPIDSource struct{ calls int }

func (f *reusedPIDSource) List(context.Context, bool) (map[int32]rawProcess, []string) {
	f.calls++
	started := int64(1000)
	if f.calls == 2 {
		started = 2000
	}
	return map[int32]rawProcess{9: {pid: 9, name: "codex", started: started, cpu: float64(f.calls), cpuAt: time.Unix(int64(f.calls), 0), rss: ok(1), footprint: ok(1), read: ok(1), write: ok(1)}}, nil
}

func TestPIDReuseDoesNotProducePlausibleCPU(t *testing.T) {
	now := time.Unix(10, 0)
	c := &Collector{Source: &reusedPIDSource{}, Now: func() time.Time { now = now.Add(time.Second); return now }, Sleep: func(time.Duration) {}}
	r := c.Collect(context.Background(), time.Second, nil).Records[0]
	if r.CPUPercent.Valid || !strings.Contains(r.CPUPercent.Error, "matching first sample") {
		t.Fatalf("CPU after PID reuse = %+v", r.CPUPercent)
	}
}
