package hc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/abhirup-dev/herdr-top/internal/model"
)

func fakeRunner(t *testing.T, outputs map[string]string) (Runner, *[]string) {
	t.Helper()
	var calls []string
	return func(_ context.Context, bin string, args ...string) ([]byte, error) {
		key := bin + " " + strings.Join(args, " ")
		calls = append(calls, key)
		out, ok := outputs[key]
		if !ok {
			return nil, errors.New("unexpected: " + key)
		}
		if strings.HasPrefix(out, "ERR ") {
			return nil, errors.New(out[4:])
		}
		return []byte(out), nil
	}, &calls
}

const ls = `{"terminals":[
 {"address":"t8","state":"live"},
 {"address":"t9","state":"live"},
 {"address":"t10","state":"live"},
 {"address":"t11","state":"live"},
 {"address":"t3","state":"finished"},
 {"address":"vpsme/t1","state":"live"}]}`

func TestLocateMapsForegroundLeadersOfLocalLiveAgents(t *testing.T) {
	run, calls := fakeRunner(t, map[string]string{
		"hc ls --json": ls,
		// A reported agent whose OS name carries .exe; name is a user label.
		"hc get t8 --json": `{"address":"t8","name":"reviewer","title":"Fix login","cwd":"/home","labels":{"where":"dotfiles / 1","herdr.tab":"tb1"},
		  "foreground":{"pid":31593,"name":"claude.exe","argv":["claude"],"cwd":"/repo"},"status":{"app":"claude-code","state":"done"}}`,
		// A plain shell: not an agent, no diagnostic.
		"hc get t9 --json": `{"address":"t9","foreground":{"pid":40,"name":"zsh","argv":["/bin/zsh"],"cwd":"/x"}}`,
		// A node launcher: the kind comes from the agent's own report.
		"hc get t10 --json": `{"address":"t10","foreground":{"pid":50,"name":"node","argv":["node","/opt/pi"],"cwd":"/y"},"status":{"app":"pi","state":"working"}}`,
		// Reported but nothing identifiable: a diagnostic.
		"hc get t11 --json": `{"address":"t11","foreground":{"pid":60,"name":"python3","argv":["python3"],"cwd":"/z"},"status":{"app":"mystery","state":"idle"}}`,
	})
	roots, diags := Client{Run: run}.Locate(context.Background())
	if len(roots) != 2 {
		t.Fatalf("roots = %v", roots)
	}
	r := roots[31593]
	if r.Kind != "claude" || r.Location.Session != "t8" || r.Location.Workspace != "dotfiles / 1" || r.Location.Tab != "tb1" ||
		r.Location.PaneLabel != "reviewer" || r.Location.PaneTitle != "Fix login" || r.Location.State != "done" || r.Location.CWD != "/repo" {
		t.Fatalf("t8 root = %+v", r)
	}
	if roots[50].Kind != "pi" || roots[50].Location.State != "working" {
		t.Fatalf("t10 root = %+v", roots[50])
	}
	if len(diags) != 1 || !strings.Contains(diags[0], "t11") {
		t.Fatalf("diagnostics = %v", diags)
	}
	for _, c := range *calls {
		if strings.Contains(c, "t3") || strings.Contains(c, "vpsme") {
			t.Fatalf("queried a finished or remote terminal: %s", c)
		}
	}
}

func TestLocateRejectsOnePIDInTwoTerminals(t *testing.T) {
	run, _ := fakeRunner(t, map[string]string{
		"hc ls --json":     `{"terminals":[{"address":"t1","state":"live"},{"address":"t2","state":"live"}]}`,
		"hc get t1 --json": `{"address":"t1","foreground":{"pid":7,"name":"codex","argv":["codex"],"cwd":"/a"}}`,
		"hc get t2 --json": `{"address":"t2","foreground":{"pid":7,"name":"codex","argv":["codex"],"cwd":"/a"}}`,
	})
	roots, diags := Client{Run: run}.Locate(context.Background())
	if len(roots) != 0 || len(diags) != 1 {
		t.Fatalf("roots = %v diags = %v", roots, diags)
	}
}

func TestLocateReportsUnavailableHC(t *testing.T) {
	run, _ := fakeRunner(t, map[string]string{"hc ls --json": "ERR hc: not running"})
	roots, diags := Client{Run: run}.Locate(context.Background())
	if len(roots) != 0 || len(diags) != 1 || !strings.Contains(diags[0], "not running") {
		t.Fatalf("roots = %v diags = %v", roots, diags)
	}
}

func TestFocusUsesTerminalAddress(t *testing.T) {
	run, calls := fakeRunner(t, map[string]string{"herdr-app do agent.focus --terminal t8": `{"ok":true}`})
	c := Client{Run: run}
	if err := c.Focus(context.Background(), roots(t)["t8"]); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != "herdr-app do agent.focus --terminal t8" {
		t.Fatalf("calls = %v", *calls)
	}
	if err := c.Focus(context.Background(), roots(t)["none"]); err == nil {
		t.Fatal("expected an error without an address")
	}
	run, _ = fakeRunner(t, map[string]string{"herdr-app do agent.focus --terminal t8": "ERR no such terminal"})
	if err := (Client{Run: run}).Focus(context.Background(), roots(t)["t8"]); err == nil || !strings.Contains(err.Error(), "no such terminal") {
		t.Fatalf("err = %v", err)
	}
}

func roots(t *testing.T) map[string]model.Location {
	t.Helper()
	return map[string]model.Location{"t8": {Session: "t8"}, "none": {}}
}
