// Package hc locates agents through herdr-core (`hc`) and focuses them through
// the Herdr desktop app (`herdr-app`), replacing the old herdr TUI socket.
package hc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	herdrclient "github.com/abhirup-dev/herdr-top/internal/herdr"
	"github.com/abhirup-dev/herdr-top/internal/model"
)

// Runner executes bin with args via direct argv (never a shell) and returns stdout.
type Runner func(ctx context.Context, bin string, args ...string) ([]byte, error)

// Exec is the Runner used outside tests.
func Exec(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = herdrclient.IsolatedEnvironment(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// herdr-app do prints {"error": ...} on stdout and exits 1.
		return nil, fmt.Errorf("%s %s: %w: %s", bin, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()+" "+stdout.String()))
	}
	return stdout.Bytes(), nil
}

type Client struct{ Run Runner }

type listResponse struct {
	Terminals []struct {
		Address string `json:"address"`
		State   string `json:"state"`
	} `json:"terminals"`
}

type terminal struct {
	Address    string            `json:"address"`
	State      string            `json:"state"`
	Name       string            `json:"name"`
	Title      string            `json:"title"`
	CWD        string            `json:"cwd"`
	Labels     map[string]string `json:"labels"`
	Foreground *struct {
		PID  int32    `json:"pid"`
		Name string   `json:"name"`
		Argv []string `json:"argv"`
		CWD  string   `json:"cwd"`
	} `json:"foreground"`
	Status *struct {
		App   string `json:"app"`
		State string `json:"state"`
	} `json:"status"`
}

// Locate maps each live local agent terminal's foreground process-group leader
// to an AgentRoot. Location.Session carries the hc address (`t8`): it is the
// TUI's SESSION column and the focus target. Remote terminals (`vps/t1`) have
// no local PIDs and are skipped.
func (c Client) Locate(ctx context.Context) (map[int32]model.AgentRoot, []string) {
	roots := map[int32]model.AgentRoot{}
	b, err := c.Run(ctx, "hc", "ls", "--json")
	if err != nil {
		return roots, []string{err.Error()}
	}
	var ls listResponse
	if err := json.Unmarshal(b, &ls); err != nil {
		return roots, []string{"decode hc ls: " + err.Error()}
	}
	ambiguous := map[int32]bool{}
	var diagnostics []string
	for _, entry := range ls.Terminals {
		if entry.State != "live" || strings.Contains(entry.Address, "/") {
			continue
		}
		b, err := c.Run(ctx, "hc", "get", entry.Address, "--json")
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("terminal %s: %v", entry.Address, err))
			continue
		}
		var t terminal
		if err := json.Unmarshal(b, &t); err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("terminal %s: decode: %v", entry.Address, err))
			continue
		}
		if t.Address != "" && t.Address != entry.Address {
			diagnostics = append(diagnostics, fmt.Sprintf("terminal %s: hc get returned %s", entry.Address, t.Address))
			continue
		}
		if t.Foreground == nil || t.Foreground.PID <= 0 {
			if t.Status != nil {
				diagnostics = append(diagnostics, fmt.Sprintf("terminal %s: no foreground process", entry.Address))
			}
			continue
		}
		kind := kindOf(t)
		if kind == "" {
			if t.Status != nil {
				diagnostics = append(diagnostics, fmt.Sprintf("terminal %s: agent kind unavailable", entry.Address))
			}
			continue
		}
		pid := t.Foreground.PID
		if old, exists := roots[pid]; exists && old.Location.Session != entry.Address {
			ambiguous[pid] = true
			delete(roots, pid)
			diagnostics = append(diagnostics, fmt.Sprintf("pid %d maps to multiple terminals", pid))
			continue
		}
		if ambiguous[pid] {
			continue
		}
		cwd := t.Foreground.CWD
		if cwd == "" {
			cwd = t.CWD
		}
		loc := model.Location{Session: entry.Address, Workspace: t.Labels["where"], Tab: t.Labels["herdr.tab"], PaneLabel: t.Name, PaneTitle: t.Title, CWD: cwd}
		if t.Status != nil {
			loc.State = t.Status.State
		}
		roots[pid] = model.AgentRoot{PID: pid, Kind: kind, Location: loc}
	}
	return roots, diagnostics
}

// kindOf prefers the OS process name, then argv0, then the agent's own report
// (`claude-code` -> claude) for launchers like `node`.
func kindOf(t terminal) string {
	if k := herdrclient.NormalizeAgentKind(t.Foreground.Name); k != "" {
		return k
	}
	if len(t.Foreground.Argv) > 0 {
		if k := herdrclient.NormalizeAgentKind(t.Foreground.Argv[0]); k != "" {
			return k
		}
	}
	if t.Status != nil {
		return herdrclient.NormalizeAgentKind(strings.TrimSuffix(t.Status.App, "-code"))
	}
	return ""
}

// Focus shows the agent's terminal in the Herdr desktop app.
func (c Client) Focus(ctx context.Context, loc model.Location) error {
	if loc.Session == "" {
		return fmt.Errorf("cannot focus without a terminal address")
	}
	_, err := c.Run(ctx, "herdr-app", "do", "agent.focus", "--terminal", loc.Session)
	return err
}
