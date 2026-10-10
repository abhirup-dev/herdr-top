package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/abhirup-dev/herdr-top/internal/model"
)

type Runner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type ExecRunner struct {
	Binary  string
	Environ []string
}

var routingVars = map[string]bool{"HERDR_SESSION": true, "HERDR_SOCKET": true, "HERDR_SOCKET_PATH": true, "HERDR_API_SOCKET": true, "HERDR_WORKSPACE_ID": true, "HERDR_TAB_ID": true, "HERDR_PANE_ID": true}

func IsolatedEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, v := range env {
		k, _, _ := strings.Cut(v, "=")
		if !routingVars[k] {
			out = append(out, v)
		}
	}
	return out
}

func (r ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	bin := r.Binary
	if bin == "" {
		bin = "herdr"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	env := r.Environ
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = IsolatedEnvironment(env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("herdr %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

type sessionsResponse struct {
	Sessions []struct {
		Name    string `json:"name"`
		Running bool   `json:"running"`
	} `json:"sessions"`
}
type snapshotEnvelope struct {
	Result struct {
		Snapshot struct {
			Workspaces []struct {
				ID    string `json:"workspace_id"`
				Label string `json:"label"`
			} `json:"workspaces"`
			Agents []struct {
				Workspace     string `json:"workspace_id"`
				Tab           string `json:"tab_id"`
				Pane          string `json:"pane_id"`
				Agent         string `json:"agent"`
				DisplayAgent  string `json:"display_agent"`
				Status        string `json:"agent_status"`
				Focused       bool   `json:"focused"`
				CWD           string `json:"cwd"`
				ForegroundCWD string `json:"foreground_cwd"`
			} `json:"agents"`
		} `json:"snapshot"`
	} `json:"result"`
}
type processInfoEnvelope struct {
	Result struct {
		ProcessInfo struct {
			Pane                     string `json:"pane_id"`
			ShellPID                 int32  `json:"shell_pid"`
			ForegroundProcessGroupID *int32 `json:"foreground_process_group_id"`
			Foreground               []struct {
				PID   int32  `json:"pid"`
				Name  string `json:"name"`
				Argv0 string `json:"argv0"`
			} `json:"foreground_processes"`
		} `json:"process_info"`
	} `json:"result"`
}

type Client struct{ Runner Runner }

func (c Client) Locate(ctx context.Context) (map[int32]model.AgentRoot, []string) {
	var sl sessionsResponse
	b, err := c.Runner.Run(ctx, "session", "list", "--json")
	if err != nil {
		return map[int32]model.AgentRoot{}, []string{err.Error()}
	}
	if err = json.Unmarshal(b, &sl); err != nil {
		return map[int32]model.AgentRoot{}, []string{"decode session list: " + err.Error()}
	}
	roots := map[int32]model.AgentRoot{}
	ambiguous := map[int32]bool{}
	var diagnostics []string
	for _, s := range sl.Sessions {
		if !s.Running {
			continue
		}
		if s.Name == "" {
			diagnostics = append(diagnostics, "ignored running session with empty name")
			continue
		}
		// --session is intentional even for "default": inherited selectors must never retarget it.
		b, err := c.Runner.Run(ctx, "--session", s.Name, "api", "snapshot")
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("session %s: %v", s.Name, err))
			continue
		}
		var snap snapshotEnvelope
		if err = json.Unmarshal(b, &snap); err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("session %s snapshot: %v", s.Name, err))
			continue
		}
		labels := map[string]string{}
		for _, w := range snap.Result.Snapshot.Workspaces {
			labels[w.ID] = w.Label
		}
		for _, a := range snap.Result.Snapshot.Agents {
			if a.Pane == "" {
				diagnostics = append(diagnostics, fmt.Sprintf("session %s agent missing pane", s.Name))
				continue
			}
			pb, err := c.Runner.Run(ctx, "--session", s.Name, "pane", "process-info", "--pane", a.Pane)
			if err != nil {
				diagnostics = append(diagnostics, fmt.Sprintf("session %s pane %s: %v", s.Name, a.Pane, err))
				continue
			}
			var pi processInfoEnvelope
			if err = json.Unmarshal(pb, &pi); err != nil {
				diagnostics = append(diagnostics, fmt.Sprintf("session %s pane %s process-info: %v", s.Name, a.Pane, err))
				continue
			}
			if pi.Result.ProcessInfo.Pane != "" && pi.Result.ProcessInfo.Pane != a.Pane {
				diagnostics = append(diagnostics, fmt.Sprintf("session %s pane %s process-info returned pane %s", s.Name, a.Pane, pi.Result.ProcessInfo.Pane))
				continue
			}
			cwd := a.CWD
			if cwd == "" {
				cwd = a.ForegroundCWD
			}
			loc := model.Location{Session: s.Name, Workspace: labels[a.Workspace], Tab: a.Tab, Pane: a.Pane, State: a.Status, CWD: cwd}
			kind := normalizeAgentKind(a.Agent)
			if kind == "" {
				kind = normalizeAgentKind(a.DisplayAgent)
			}
			pid, ok := launcherRoot(pi.Result.ProcessInfo.ForegroundProcessGroupID, pi.Result.ProcessInfo.Foreground, kind)
			if !ok {
				diagnostics = append(diagnostics, fmt.Sprintf("session %s pane %s: cannot identify agent launcher root", s.Name, a.Pane))
				continue
			}
			if kind == "" {
				for _, p := range pi.Result.ProcessInfo.Foreground {
					if p.PID == pid {
						kind = normalizeAgentKind(p.Name)
						if kind == "" {
							kind = normalizeAgentKind(p.Argv0)
						}
					}
				}
			}
			if kind == "" {
				diagnostics = append(diagnostics, fmt.Sprintf("session %s pane %s: agent kind unavailable", s.Name, a.Pane))
				continue
			}
			if old, exists := roots[pid]; exists && (old.Location.Session != loc.Session || old.Location.Pane != loc.Pane) {
				ambiguous[pid] = true
				delete(roots, pid)
				diagnostics = append(diagnostics, fmt.Sprintf("pid %d maps to multiple session/panes", pid))
				continue
			}
			if !ambiguous[pid] {
				roots[pid] = model.AgentRoot{PID: pid, Kind: kind, Location: loc}
			}
		}
	}
	return roots, diagnostics
}

func launcherRoot(groupID *int32, processes []struct {
	PID   int32  `json:"pid"`
	Name  string `json:"name"`
	Argv0 string `json:"argv0"`
}, kind string) (int32, bool) {
	if groupID != nil && *groupID > 0 {
		for _, p := range processes {
			if p.PID == *groupID {
				return p.PID, true
			}
		}
	}
	if kind == "" {
		return 0, false
	}
	var match int32
	for _, p := range processes {
		if normalizeAgentKind(p.Name) != kind && normalizeAgentKind(p.Argv0) != kind {
			continue
		}
		if match != 0 {
			return 0, false
		}
		match = p.PID
	}
	return match, match > 0
}

func normalizeAgentKind(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.LastIndexAny(name, `/\\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "aider", "amp", "claude", "cline", "codex", "copilot", "cursor", "droid", "gemini", "grok", "kilo", "kimi", "omp", "opencode", "pi", "qwen":
		return name
	default:
		return ""
	}
}

func (c Client) Focus(ctx context.Context, loc model.Location) error {
	if loc.Session == "" || loc.Pane == "" {
		return fmt.Errorf("cannot focus without an exact session and pane")
	}
	if _, err := c.Runner.Run(ctx, "--session", loc.Session, "agent", "focus", loc.Pane); err != nil {
		return err
	}
	// The focus response can echo the target without proving state changed. Read
	// the same explicitly selected server and require its focused agent pane.
	b, err := c.Runner.Run(ctx, "--session", loc.Session, "api", "snapshot")
	if err != nil {
		return fmt.Errorf("verify focused pane: %w", err)
	}
	var snap snapshotEnvelope
	if err := json.Unmarshal(b, &snap); err != nil {
		return fmt.Errorf("verify focused pane: decode snapshot: %w", err)
	}
	for _, agent := range snap.Result.Snapshot.Agents {
		if agent.Pane == loc.Pane && agent.Focused {
			return nil
		}
	}
	return fmt.Errorf("focus read-back did not show pane %s focused", loc.Pane)
}
