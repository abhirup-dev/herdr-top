package collector

import (
	"context"
	"testing"
	"time"
)

type discoverySource struct{ calls int }

func (s *discoverySource) List(context.Context, bool) (map[int32]rawProcess, []string) {
	s.calls++
	at := time.Unix(int64(s.calls), 0)
	cpu := float64(s.calls)
	p := func(pid, ppid int32, name string) rawProcess {
		return rawProcess{pid: pid, ppid: ppid, name: name, started: 1000, cpu: cpu, cpuAt: at, rss: ok(1), footprint: ok(1), read: ok(1), write: ok(1)}
	}
	return map[int32]rawProcess{
		10: p(10, 1, "node"),       // Herdr-mapped Pi launcher.
		11: p(11, 10, "bun"),       // Pi helper, not another row.
		20: p(20, 1, "claude.exe"), // Herdr-mapped Claude launcher.
		21: p(21, 20, "node"),      // Claude helper, not another row.
		30: p(30, 1, "node"),       // Unmapped Node is not safely identifiable.
		40: p(40, 1, "claude.exe"), // Recognizable outside-Herdr Claude.
	}, nil
}

func TestMappedLauncherRootsDriveDiscoveryWithoutHelperDuplicates(t *testing.T) {
	now := time.Unix(10, 0)
	c := &Collector{Source: &discoverySource{}, Now: func() time.Time { now = now.Add(time.Second); return now }, Sleep: func(time.Duration) {}}
	s := c.Collect(context.Background(), time.Second, map[int32]string{10: "pi", 20: "claude"})
	if len(s.Records) != 3 {
		t.Fatalf("records=%v", s.Records)
	}
	want := map[int32]string{10: "pi", 20: "claude", 40: "claude"}
	for _, r := range s.Records {
		if want[r.PID] != r.Kind {
			t.Fatalf("unexpected record pid=%d kind=%q", r.PID, r.Kind)
		}
		delete(want, r.PID)
	}
	if len(want) != 0 {
		t.Fatalf("missing roots: %v", want)
	}
}

type delayedCPUObservationSource struct{ calls int }

func (s *delayedCPUObservationSource) List(context.Context, bool) (map[int32]rawProcess, []string) {
	s.calls++
	if s.calls == 1 {
		return map[int32]rawProcess{1: {pid: 1, name: "codex", started: 1000, cpu: 1, cpuAt: time.Unix(10, 0), rss: ok(1), footprint: ok(1), read: ok(1), write: ok(1)}}, nil
	}
	return map[int32]rawProcess{1: {pid: 1, name: "codex", started: 1000, cpu: 3, cpuAt: time.Unix(12, 0), rss: ok(1), footprint: ok(1), read: ok(1), write: ok(1)}}, nil
}

func TestCPUUsesPerProcessObservationInterval(t *testing.T) {
	now := time.Unix(1, 0)
	c := &Collector{Source: &delayedCPUObservationSource{}, Now: func() time.Time { now = now.Add(10 * time.Second); return now }, Sleep: func(time.Duration) {}}
	r := c.Collect(context.Background(), time.Second, nil).Records[0]
	if !r.CPUPercent.Valid || r.CPUPercent.Value != 100 {
		t.Fatalf("CPU=%+v, want 100%% from a 2-second per-process interval", r.CPUPercent)
	}
}
