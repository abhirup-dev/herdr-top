package collector

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/abhirup-dev/herdr-top/internal/footprint"
	"github.com/abhirup-dev/herdr-top/internal/model"
	"github.com/shirou/gopsutil/v4/process"
)

var agentKinds = map[string]bool{
	"aider": true, "amp": true, "claude": true, "cline": true, "codex": true,
	"copilot": true, "cursor": true, "droid": true, "gemini": true, "grok": true,
	"kilo": true, "kimi": true, "omp": true, "opencode": true, "pi": true, "qwen": true,
}

type rawProcess struct {
	pid, ppid                   int32
	name                        string
	started                     int64
	cpu                         float64
	cpuAt                       time.Time
	rss, footprint, read, write model.Value
	cpuErr                      string
}

type processSource interface {
	List(context.Context, bool) (map[int32]rawProcess, []string)
}

type gopsutilSource struct{ now func() time.Time }

func metric(v uint64, err error) model.Value {
	if err != nil {
		return model.Value{Error: err.Error()}
	}
	return model.Value{Value: v, Valid: true}
}

func (g gopsutilSource) List(ctx context.Context, detailed bool) (map[int32]rawProcess, []string) {
	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, []string{"process enumeration failed: " + err.Error()}
	}
	out := make(map[int32]rawProcess, len(ps))
	var diagnostics []string
	for _, p := range ps {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("pid %d skipped: name: %v", p.Pid, err))
			continue
		}
		ppid, err := p.PpidWithContext(ctx)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("pid %d skipped: parent: %v", p.Pid, err))
			continue
		}
		started, err := p.CreateTimeWithContext(ctx)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("pid %d skipped: start time: %v", p.Pid, err))
			continue
		}
		times, err := p.TimesWithContext(ctx)
		r := rawProcess{pid: p.Pid, ppid: ppid, name: filepath.Base(name), started: started, cpuAt: g.now()}
		if err != nil {
			r.cpuErr = err.Error()
		} else {
			r.cpu = times.User + times.System
		}
		if detailed {
			mem, e := p.MemoryInfoWithContext(ctx)
			if e != nil {
				r.rss = metric(0, e)
			} else {
				r.rss = metric(mem.RSS, nil)
			}
			r.footprint = metric(footprint.Physical(p.Pid))
			io, e := p.IOCountersWithContext(ctx)
			if e != nil {
				r.read, r.write = metric(0, e), metric(0, e)
			} else {
				// Darwin's meaningful counters are explicitly Disk*, not generic ReadBytes/WriteBytes.
				r.read, r.write = metric(io.DiskReadBytes, nil), metric(io.DiskWriteBytes, nil)
			}
		}
		out[p.Pid] = r
	}
	return out, diagnostics
}

type Collector struct {
	Source processSource
	Now    func() time.Time
	Sleep  func(time.Duration)
}

func New() *Collector {
	return &Collector{Source: gopsutilSource{now: time.Now}, Now: time.Now, Sleep: time.Sleep}
}

// Collect samples every process and emits Herdr roots plus recognizable outside
// launchers. mappedKinds is authoritative for launchers such as Pi whose kernel
// name is the generic "node".
func (c *Collector) Collect(ctx context.Context, sampleWindow time.Duration, mappedKinds map[int32]string) model.Snapshot {
	start := c.Now()
	before, d1 := c.Source.List(ctx, false)
	if sampleWindow > 0 {
		c.Sleep(sampleWindow)
	}
	after, d2 := c.Source.List(ctx, true)
	end := c.Now()
	elapsed := end.Sub(start)
	s := model.Snapshot{GeneratedAt: end, SampleWindow: elapsed, Diagnostics: uniqueDiagnostics(d1, d2)}
	children := make(map[int32][]int32)
	for pid, p := range after {
		children[p.ppid] = append(children[p.ppid], pid)
	}
	cpu := make(map[int32]model.FloatValue, len(after))
	for pid, p := range after {
		b, ok := before[pid]
		switch {
		case p.cpuErr != "":
			cpu[pid] = model.FloatValue{Error: p.cpuErr}
		case ok && b.cpuErr != "":
			cpu[pid] = model.FloatValue{Error: b.cpuErr}
		case !ok || b.started != p.started:
			cpu[pid] = model.FloatValue{Error: "no matching first sample"}
		case p.cpuAt.IsZero() || b.cpuAt.IsZero() || !p.cpuAt.After(b.cpuAt):
			cpu[pid] = model.FloatValue{Error: "process CPU observation interval is unavailable"}
		case p.cpu < b.cpu:
			cpu[pid] = model.FloatValue{Error: "cumulative CPU time decreased"}
		default:
			cpu[pid] = model.FloatValue{Value: (p.cpu - b.cpu) / p.cpuAt.Sub(b.cpuAt).Seconds() * 100, Valid: true}
		}
	}
	mappedDescendants := map[int32]bool{}
	for root := range mappedKinds {
		ids, _ := descendants(root, children)
		for _, id := range ids[1:] {
			mappedDescendants[id] = true
		}
		if _, ok := after[root]; !ok {
			s.Diagnostics = append(s.Diagnostics, fmt.Sprintf("mapped agent pid %d is no longer available", root))
		}
	}
	for pid, p := range after {
		kind, mapped := mappedKinds[pid]
		if !mapped {
			if mappedDescendants[pid] {
				continue
			}
			kind = processKind(p.name)
			if kind == "" {
				continue
			}
		}
		started := time.UnixMilli(p.started)
		r := model.Record{PID: pid, Kind: kind, StartedAt: started, Age: end.Sub(started), CPUPercent: cpu[pid], RSSBytes: p.rss, FootprintBytes: p.footprint, DiskReadBytes: p.read, DiskWriteBytes: p.write}
		ids, cycle := descendants(pid, children)
		if cycle {
			r.Diagnostics = append(r.Diagnostics, "cycle detected in process tree")
		}
		r.TreeCPUPercent = sumFloat(ids, func(id int32) model.FloatValue { return cpu[id] })
		r.TreeRSSBytes = sumValue(ids, func(id int32) model.Value { return after[id].rss })
		r.TreeFootprintBytes = sumValue(ids, func(id int32) model.Value { return after[id].footprint })
		r.TreeDiskBytes = sumValue(ids, func(id int32) model.Value {
			p := after[id]
			if !p.read.Valid {
				return p.read
			}
			if !p.write.Valid {
				return p.write
			}
			return model.Value{Value: p.read.Value + p.write.Value, Valid: true}
		})
		s.Records = append(s.Records, r)
	}
	sort.Slice(s.Records, func(i, j int) bool { return s.Records[i].PID < s.Records[j].PID })
	return s
}

func uniqueDiagnostics(groups ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, group := range groups {
		for _, diagnostic := range group {
			if !seen[diagnostic] {
				seen[diagnostic] = true
				out = append(out, diagnostic)
			}
		}
	}
	return out
}

func processKind(name string) string {
	name = strings.ToLower(filepath.Base(strings.TrimSpace(name)))
	name = strings.TrimSuffix(name, ".exe")
	if agentKinds[name] {
		return name
	}
	return ""
}

func descendants(root int32, children map[int32][]int32) ([]int32, bool) {
	out := []int32{}
	stack := []int32{root}
	seen := map[int32]bool{}
	cycle := false
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[id] {
			cycle = true
			continue
		}
		seen[id] = true
		out = append(out, id)
		stack = append(stack, children[id]...)
	}
	return out, cycle
}
func sumValue(ids []int32, get func(int32) model.Value) model.Value {
	var n uint64
	for _, id := range ids {
		v := get(id)
		if !v.Valid {
			return model.Value{Error: fmt.Sprintf("tree metric incomplete: pid %d: %s", id, v.Error)}
		}
		n += v.Value
	}
	return model.Value{Value: n, Valid: true}
}
func sumFloat(ids []int32, get func(int32) model.FloatValue) model.FloatValue {
	var n float64
	for _, id := range ids {
		v := get(id)
		if !v.Valid {
			return model.FloatValue{Error: fmt.Sprintf("tree metric incomplete: pid %d: %s", id, v.Error)}
		}
		n += v.Value
	}
	return model.FloatValue{Value: n, Valid: true}
}
