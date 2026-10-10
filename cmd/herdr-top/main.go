package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/abhirup-dev/herdr-top/internal/collector"
	herdrclient "github.com/abhirup-dev/herdr-top/internal/herdr"
	"github.com/abhirup-dev/herdr-top/internal/model"
	"github.com/abhirup-dev/herdr-top/internal/present"
	"github.com/abhirup-dev/herdr-top/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev"

func collect(ctx context.Context, c *collector.Collector, h herdrclient.Client, window time.Duration) model.Snapshot {
	roots, d := h.Locate(ctx)
	kinds := make(map[int32]string, len(roots))
	for pid, root := range roots {
		kinds[pid] = root.Kind
	}
	s := c.Collect(ctx, window, kinds)
	s.Diagnostics = append(s.Diagnostics, d...)
	for i := range s.Records {
		if root, ok := roots[s.Records[i].PID]; ok {
			loc := root.Location
			s.Records[i].Location = &loc
		}
	}
	return s
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("herdr-top", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print one JSON snapshot and exit")
	listOut := fs.Bool("list", false, "print one table snapshot and exit")
	sortName := fs.String("sort", "memory", "sort by memory, cpu, io, or age")
	sample := fs.Duration("sample", 500*time.Millisecond, "CPU sample interval (must be positive)")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: herdr-top [--list|--json] [--sort memory|cpu|io|age] [--sample 500ms]")
		fmt.Fprintln(stderr, "\nInteractive by default. Keys: j/k move, m/c/i/o sort, r refresh, Enter focus, q quit.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return nil
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if *jsonOut && *listOut {
		return errors.New("--json and --list are mutually exclusive")
	}
	by, err := present.ParseSort(*sortName)
	if err != nil {
		return err
	}
	if *sample <= 0 {
		return errors.New("--sample must be positive")
	}
	ctx := context.Background()
	c := collector.New()
	h := herdrclient.Client{Runner: herdrclient.ExecRunner{}}
	snapshot := collect(ctx, c, h, *sample)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(snapshot)
	}
	if *listOut {
		fmt.Fprintln(stdout, present.Header())
		for _, r := range present.Records(snapshot.Records, by) {
			fmt.Fprintln(stdout, present.Row(r))
			if d := present.Diagnostics(r); d != "" {
				fmt.Fprintf(stderr, "pid %d: %s\n", r.PID, d)
			}
		}
		for _, d := range snapshot.Diagnostics {
			fmt.Fprintln(stderr, "warning:", d)
		}
		return nil
	}
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive mode requires a terminal; use --list or --json")
	}
	refresh := func(ctx context.Context) model.Snapshot { return collect(ctx, c, h, *sample) }
	focus := func(ctx context.Context, loc model.Location) error { return h.Focus(ctx, loc) }
	m := ui.New(snapshot, refresh, focus)
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(stdout)).Run()
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-top:", err)
		os.Exit(1)
	}
}
