package herdr

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/abhirup-dev/herdr-top/internal/model"
)

type fakeRunner struct {
	calls     [][]string
	responses map[string]string
	failures  map[string]error
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	k := strings.Join(args, " ")
	if err := f.failures[k]; err != nil {
		return nil, err
	}
	return []byte(f.responses[k]), nil
}
func TestLocateExplicitlySelectsEverySessionAndScopesSamePaneID(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{
		"session list --json":                              `{"sessions":[{"name":"default","running":true},{"name":"team","running":true}]}`,
		"--session default api snapshot":                   `{"result":{"snapshot":{"workspaces":[{"workspace_id":"w1","label":"alpha"}],"agents":[{"workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1","agent":"pi","agent_status":"idle"}]}}}`,
		"--session team api snapshot":                      `{"result":{"snapshot":{"workspaces":[{"workspace_id":"w1","label":"beta"}],"panes":[{"pane_id":"w1:p1","label":"reviewer"}],"agents":[{"workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1","agent":"claude","agent_status":"working","name":"rev-bot","title":"PR review"}]}}}`,
		"--session default pane process-info --pane w1:p1": `{"result":{"process_info":{"pane_id":"w1:p1","foreground_process_group_id":101,"foreground_processes":[{"pid":101,"name":"node"},{"pid":102,"name":"bun"}]}}}`,
		"--session team pane process-info --pane w1:p1":    `{"result":{"process_info":{"pane_id":"w1:p1","foreground_process_group_id":202,"foreground_processes":[{"pid":202,"name":"claude.exe"},{"pid":203,"name":"node"}]}}}`,
	}}
	got, d := (Client{Runner: f}).Locate(context.Background())
	if len(d) != 0 {
		t.Fatal(d)
	}
	if got[101].Location.Session != "default" || got[101].Kind != "pi" || got[202].Location.Session != "team" || got[202].Kind != "claude" {
		t.Fatalf("roots=%+v", got)
	}
	if l := got[202].Location; l.PaneLabel != "reviewer" || l.PaneTitle != "PR review" || l.AgentName != "rev-bot" {
		t.Fatalf("names=%+v", l)
	}
	if l := got[101].Location; l.PaneLabel != "" || l.AgentName != "" {
		t.Fatalf("unnamed pane got names %+v", l)
	}
	if _, helper := got[102]; helper {
		t.Fatal("Pi bun helper was classified as another agent root")
	}
	if _, helper := got[203]; helper {
		t.Fatal("Claude node helper was classified as another agent root")
	}
	want := []string{"--session", "default", "api", "snapshot"}
	found := false
	for _, c := range f.calls {
		if reflect.DeepEqual(c, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("default was not explicitly selected: %v", f.calls)
	}
}
func TestLocateRejectsAmbiguousPIDAcrossSessions(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{"session list --json": `{"sessions":[{"name":"default","running":true},{"name":"other","running":true}]}`, "--session default api snapshot": `{"result":{"snapshot":{"agents":[{"pane_id":"p1","agent":"codex"}]}}}`, "--session other api snapshot": `{"result":{"snapshot":{"agents":[{"pane_id":"p1","agent":"codex"}]}}}`, "--session default pane process-info --pane p1": `{"result":{"process_info":{"pane_id":"p1","foreground_process_group_id":7,"foreground_processes":[{"pid":7,"name":"codex"}]}}}`, "--session other pane process-info --pane p1": `{"result":{"process_info":{"pane_id":"p1","foreground_process_group_id":7,"foreground_processes":[{"pid":7,"name":"codex"}]}}}`}}
	got, d := (Client{Runner: f}).Locate(context.Background())
	if _, ok := got[7]; ok || len(d) == 0 {
		t.Fatalf("got=%v diagnostics=%v", got, d)
	}
}
func TestEnvironmentIsolation(t *testing.T) {
	got := IsolatedEnvironment([]string{"PATH=/bin", "HERDR_SESSION=wrong-session", "HERDR_SOCKET_PATH=/synthetic/socket", "SAFE=yes"})
	want := []string{"PATH=/bin", "SAFE=yes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
func TestFocusAwaitsFailureAndVerifiesExactPane(t *testing.T) {
	ctx := context.Background()
	loc := model.Location{Session: "s", Pane: "p"}
	f := &fakeRunner{responses: map[string]string{
		"--session s agent focus p": `{"result":{"pane_id":"p"}}`,
		"--session s api snapshot":  `{"result":{"snapshot":{"agents":[{"pane_id":"p","focused":true}]}}}`,
	}}
	if err := (Client{Runner: f}).Focus(ctx, loc); err != nil {
		t.Fatal(err)
	}
	// An echoed target in the mutation response is not success without focused read-back.
	f.responses["--session s api snapshot"] = `{"result":{"snapshot":{"agents":[{"pane_id":"other","focused":true},{"pane_id":"p","focused":false}]}}}`
	if err := (Client{Runner: f}).Focus(ctx, loc); err == nil || !strings.Contains(err.Error(), "read-back") {
		t.Fatalf("expected read-back failure, got %v", err)
	}
	f.failures = map[string]error{"--session s agent focus p": errors.New("server rejected focus")}
	if err := (Client{Runner: f}).Focus(ctx, loc); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("err=%v", err)
	}
}

func TestFocusSwitchesTabAndRevealsSessionWindow(t *testing.T) {
	ctx := context.Background()
	loc := model.Location{Session: "s", Tab: "stale", Pane: "p"}
	var marker string
	reveal := func(_ context.Context, m string) (string, error) { marker = m; return "focused", nil }
	f := &fakeRunner{responses: map[string]string{
		"--session s agent focus p":        `{"result":{"agent":{"pane_id":"p","tab_id":"t"}}}`,
		"--session s api snapshot":         `{"result":{"snapshot":{"agents":[{"pane_id":"p","focused":true}]}}}`,
		"--session s terminal title clear": `{"result":{"changed":true}}`,
	}}
	runner := &markerRunner{fakeRunner: f, title: `{"result":{"changed":true,"reason":"set"}}`}
	if err := (Client{Runner: runner, RevealWindow: reveal}).Focus(ctx, loc); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range f.calls {
		got = append(got, strings.Join(c, " "))
	}
	want := []string{"--session s agent focus p", "--session s tab focus t", "--session s api snapshot", "--session s terminal title set " + marker, "--session s terminal title clear"}
	if !strings.HasPrefix(marker, "herdr-top-") || !reflect.DeepEqual(got, want) {
		t.Fatalf("marker=%q calls=%q", marker, got)
	}

	// No attached client: report it, still clear the title.
	f.calls = nil
	runner.title = `{"result":{"changed":false,"reason":"no_foreground_client"}}`
	err := (Client{Runner: runner, RevealWindow: reveal}).Focus(ctx, loc)
	if err == nil || !strings.Contains(err.Error(), "no_foreground_client") {
		t.Fatalf("err=%v", err)
	}
	if last := strings.Join(f.calls[len(f.calls)-1], " "); last != "--session s terminal title clear" {
		t.Fatalf("title not cleared, last call %q", last)
	}
}

// markerRunner answers "terminal title set <random marker>" without knowing the marker up front.
type markerRunner struct {
	*fakeRunner
	title string
}

func (m *markerRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	b, err := m.fakeRunner.Run(ctx, args...)
	if len(args) == 6 && args[2] == "terminal" && args[4] == "set" {
		return []byte(m.title), err
	}
	return b, err
}
