package screen

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/attach"
	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	tea "github.com/charmbracelet/bubbletea"
)

func pressKey(m Model, kind tea.KeyType) (Model, tea.Cmd) {
	return send(m, tea.KeyMsg{Type: kind})
}

func typedText(m Model, text string) Model {
	for _, r := range text {
		if r == ' ' {
			m, _ = send(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		m, _ = send(m, key(string(r)))
	}
	return m
}

func settleCommands(m Model, cmd tea.Cmd) Model {
	pending := []tea.Cmd{cmd}
	for steps := 0; len(pending) > 0 && steps < 30; steps++ {
		next := pending[0]
		pending = pending[1:]
		if next == nil {
			continue
		}
		answer := make(chan tea.Msg, 1)
		go func() { answer <- next() }()
		var msg tea.Msg
		select {
		case msg = <-answer:
		case <-time.After(400 * time.Millisecond):
			continue
		}
		if msg == nil {
			continue
		}
		if value := reflect.ValueOf(msg); value.Kind() == reflect.Slice && value.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
			for i := 0; i < value.Len(); i++ {
				if sub, ok := value.Index(i).Interface().(tea.Cmd); ok {
					pending = append(pending, sub)
				}
			}
			continue
		}
		var follow tea.Cmd
		m, follow = send(m, msg)
		pending = append(pending, follow)
	}
	return m
}

type recordedSender struct {
	mu    sync.Mutex
	calls [][2]string
	line  string
	err   error
}

func (r *recordedSender) send(to, body string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, [2]string{to, body})
	return r.line, r.err
}

func (r *recordedSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func loadedModel(agents []hubclient.Agent) Model {
	m := newModel()
	m, _ = send(m, tea.WindowSizeMsg{Width: 200, Height: 24})
	m, _ = send(m, snapshotMsg{agents: agents, at: clock, took: 10 * time.Millisecond})
	return m
}

func loadedWithSender(agents []hubclient.Agent, r *recordedSender) Model {
	return loadedModel(agents).WithSend(r.send)
}

func quits(m Model) bool {
	_, cmd := send(m, key("q"))
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestTheSelectionStartsOnTheFirstRowAndUpDownJAndKMoveItWithoutWrapping(t *testing.T) {
	m := loadedModel(sample)
	if got := m.SelectedHandle(); got != "alpha" {
		t.Fatalf("selected %q", got)
	}
	steps := []struct {
		move tea.Msg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyDown}, "bravo"},
		{tea.KeyMsg{Type: tea.KeyDown}, "bravo"},
		{tea.KeyMsg{Type: tea.KeyUp}, "alpha"},
		{tea.KeyMsg{Type: tea.KeyUp}, "alpha"},
		{key("j"), "bravo"},
		{key("j"), "bravo"},
		{key("k"), "alpha"},
		{key("k"), "alpha"},
	}
	for i, step := range steps {
		m, _ = send(m, step.move)
		if got := m.SelectedHandle(); got != step.want {
			t.Fatalf("step %d: selected %q, want %q", i, got, step.want)
		}
	}
}

func TestMovingTheSelectionDoesNotQuitOrFetchOrSendAnything(t *testing.T) {
	r := &recordedSender{}
	m := loadedWithSender(sample, r)
	for _, move := range []tea.Msg{tea.KeyMsg{Type: tea.KeyDown}, key("j"), key("k"), tea.KeyMsg{Type: tea.KeyUp}} {
		var cmd tea.Cmd
		m, cmd = send(m, move)
		if cmd != nil {
			t.Fatalf("%v returned a command", move)
		}
	}
	if r.count() != 0 {
		t.Fatal("a move sent a message")
	}
}

func TestTheSelectionFollowsTheHandleWhenARefreshReordersTheList(t *testing.T) {
	m := loadedModel(sample)
	m, _ = send(m, key("j"))
	if m.SelectedHandle() != "bravo" {
		t.Fatalf("selected %q", m.SelectedHandle())
	}
	reordered := []hubclient.Agent{
		{Name: "aaa", Harness: "codex", Host: "mac", Connected: true, Label: "first"},
		{Name: "alpha", Harness: "claude", Host: "mac", Connected: true, Label: "second"},
		{Name: "bravo", Harness: "codex", Host: "mac", Connected: true, Label: "third"},
	}
	m, _ = send(m, snapshotMsg{agents: reordered, at: clock.Add(time.Second)})
	if got := m.SelectedHandle(); got != "bravo" {
		t.Fatalf("after a refresh that moved the row: selected %q", got)
	}
	m, _ = send(m, snapshotMsg{agents: reordered[1:], at: clock.Add(2 * time.Second)})
	if got := m.SelectedHandle(); got != "bravo" {
		t.Fatalf("after a refresh that removed another row: selected %q", got)
	}
}

func TestTheSelectionStaysOnTheAgentThroughAFailedPoll(t *testing.T) {
	m := loadedModel(sample)
	m, _ = send(m, key("j"))
	m, _ = send(m, snapshotMsg{err: errors.New("cannot reach the switchboard"), at: clock.Add(time.Second)})
	if got := m.SelectedHandle(); got != "bravo" {
		t.Fatalf("selected %q", got)
	}
}

func TestWhenTheSelectedAgentIsGoneTheSelectionIsAnAgentThatIsStillListedOrNone(t *testing.T) {
	m := loadedModel(sample)
	m, _ = send(m, key("j"))
	m, _ = send(m, snapshotMsg{agents: sample[:1], at: clock.Add(time.Second)})
	if got := m.SelectedHandle(); got != "alpha" && got != "" {
		t.Fatalf("selected %q", got)
	}
	m, _ = send(m, snapshotMsg{agents: nil, at: clock.Add(2 * time.Second)})
	if got := m.SelectedHandle(); got != "" {
		t.Fatalf("selected %q in an empty list", got)
	}
	m, _ = send(m, snapshotMsg{agents: sample, at: clock.Add(3 * time.Second)})
	if got := m.SelectedHandle(); got != "alpha" && got != "bravo" {
		t.Fatalf("selected %q", got)
	}
}

func TestNothingIsSelectedBeforeTheFirstPollOrInAnEmptyListAndTheKeysDoNothing(t *testing.T) {
	r := &recordedSender{}
	for name, m := range map[string]Model{
		"before the first poll": newModel().WithSend(r.send),
		"an empty list":         loadedModel(nil).WithSend(r.send),
	} {
		if got := m.SelectedHandle(); got != "" {
			t.Errorf("%s: selected %q", name, got)
		}
		for _, k := range []tea.Msg{key("j"), key("k"), tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyUp}, key("s"), key("a")} {
			next, _ := send(m, k)
			if next.SelectedHandle() != "" {
				t.Errorf("%s: %v selected %q", name, k, next.SelectedHandle())
			}
			next = typedText(next, "hello")
			next, _ = pressKey(next, tea.KeyEnter)
		}
	}
	if r.count() != 0 {
		t.Fatalf("a message was sent with nobody to send it to: %v", r.calls)
	}
}

func TestSOpensAPromptThatTakesTypedTextWithoutRunningTheHotkeys(t *testing.T) {
	r := &recordedSender{line: "sent m-1 (seq 1) to alpha"}
	m := loadedWithSender(sample, r)
	m, _ = send(m, key("s"))
	m = typedText(m, "quit r s a j k")
	if !strings.Contains(m.View(), "quit r s a j k") {
		t.Fatalf("the typed text is not on the screen:\n%s", m.View())
	}
	m, cmd := send(m, key("q"))
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("q quit while the prompt was open")
		}
	}
	if m.SelectedHandle() != "alpha" {
		t.Fatalf("typing j moved the selection to %q", m.SelectedHandle())
	}
	if r.count() != 0 {
		t.Fatal("typing sent a message")
	}
}

func TestEnterSendsTheTypedTextToTheSelectedHandleAndShowsTheResultLine(t *testing.T) {
	r := &recordedSender{line: "sent m-1 (seq 7) to bravo"}
	m := loadedWithSender(sample, r)
	m, _ = send(m, key("j"))
	m, _ = send(m, key("s"))
	m = typedText(m, "please check the build")
	m, cmd := pressKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("Enter returned no command")
	}
	m = settleCommands(m, cmd)
	if r.count() != 1 || r.calls[0] != [2]string{"bravo", "please check the build"} {
		t.Fatalf("calls %v", r.calls)
	}
	view := m.View()
	if !strings.Contains(view, "sent m-1 (seq 7) to bravo") {
		t.Fatalf("no result line in\n%s", view)
	}
	if strings.Contains(view, "please check the build") {
		t.Fatalf("the prompt is still open:\n%s", view)
	}
	if !quits(m) {
		t.Fatal("q does not quit after the prompt closed")
	}
}

func TestAPromptOnADisconnectedAgentStillSendsAndTheSwitchboardsAnswerIsShown(t *testing.T) {
	r := &recordedSender{err: errors.New("not connected")}
	m := loadedWithSender(sample, r)
	m, _ = send(m, key("j"))
	m, _ = send(m, key("s"))
	m = typedText(m, "hello")
	m, cmd := pressKey(m, tea.KeyEnter)
	m = settleCommands(m, cmd)
	if r.count() != 1 || r.calls[0][0] != "bravo" {
		t.Fatalf("calls %v", r.calls)
	}
	if view := m.View(); !strings.Contains(view, "not connected") {
		t.Fatalf("the failure is not shown:\n%s", view)
	}
}

func TestEscapeCancelsThePromptAndNothingIsSent(t *testing.T) {
	r := &recordedSender{line: "sent m-1 (seq 1) to alpha"}
	m := loadedWithSender(sample, r)
	m, _ = send(m, key("s"))
	m = typedText(m, "never mind")
	m, cmd := pressKey(m, tea.KeyEsc)
	m = settleCommands(m, cmd)
	if r.count() != 0 {
		t.Fatalf("calls %v", r.calls)
	}
	if strings.Contains(m.View(), "never mind") {
		t.Fatalf("the prompt is still open:\n%s", m.View())
	}
	if !quits(m) {
		t.Fatal("q does not quit after the prompt closed")
	}
	m, _ = send(m, key("s"))
	if strings.Contains(m.View(), "never mind") {
		t.Fatalf("a new prompt kept the cancelled text:\n%s", m.View())
	}
}

func TestBackspaceEditsThePromptAndASpaceIsPartOfTheMessage(t *testing.T) {
	r := &recordedSender{line: "sent m-1 (seq 1) to alpha"}
	m := loadedWithSender(sample, r)
	m, _ = send(m, key("s"))
	m = typedText(m, "hellp")
	m, _ = pressKey(m, tea.KeyBackspace)
	m = typedText(m, "o world")
	m, cmd := pressKey(m, tea.KeyEnter)
	settleCommands(m, cmd)
	if r.count() != 1 || r.calls[0] != [2]string{"alpha", "hello world"} {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestThePollGoesOnWhileThePromptIsOpenAndTheTypedTextAndTheTargetSurvive(t *testing.T) {
	r := &recordedSender{line: "sent m-1 (seq 1) to bravo"}
	m := loadedWithSender(sample, r)
	m, _ = send(m, key("j"))
	m, _ = send(m, key("s"))
	m = typedText(m, "draft")
	m, cmd := send(m, snapshotMsg{agents: sample, at: clock.Add(time.Second), took: 10 * time.Millisecond})
	if cmd == nil {
		t.Fatal("a poll that finished while the prompt was open scheduled nothing")
	}
	generation := m.generation
	m, cmd = send(m, tickMsg{generation: generation})
	if cmd == nil {
		t.Fatal("the tick did not fetch while the prompt was open")
	}
	if !strings.Contains(m.View(), "draft") {
		t.Fatalf("the draft is gone:\n%s", m.View())
	}
	reordered := []hubclient.Agent{
		{Name: "bravo", Harness: "codex", Host: "mac", Connected: true},
		{Name: "alpha", Harness: "claude", Host: "mac", Connected: true},
	}
	m, _ = send(m, snapshotMsg{agents: reordered, at: clock.Add(2 * time.Second)})
	m = typedText(m, "ing")
	m, cmd = pressKey(m, tea.KeyEnter)
	settleCommands(m, cmd)
	if r.count() != 1 || r.calls[0] != [2]string{"bravo", "drafting"} {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestARefreshKeyTypedInThePromptIsTextAndNotARefresh(t *testing.T) {
	m := loadedModel(sample)
	m, _ = send(m, snapshotMsg{agents: sample, at: clock})
	m, _ = send(m, key("s"))
	_, cmd := send(m, key("r"))
	if cmd != nil {
		t.Fatal("r fetched while the prompt was open")
	}
}

func TestTheResultLineAndTheErrorOfASendCarryNoControlRune(t *testing.T) {
	for name, r := range map[string]*recordedSender{
		"a result": {line: "sent \x1b]0;pwned\x07 m-1\u202e (seq 1)\r\nto bravo"},
		"an error": {err: errors.New("bad \x1b[2J \u0085 reply\nline2 \x9b31m")},
	} {
		m := loadedWithSender(sample, r)
		m, _ = send(m, key("s"))
		m = typedText(m, "x")
		m, cmd := pressKey(m, tea.KeyEnter)
		m = settleCommands(m, cmd)
		view := m.View()
		for _, bad := range []string{"\x1b", "\x07", "\r", "\u202e", "\u0085", "\x9b"} {
			if strings.Contains(view, bad) {
				t.Errorf("%s: %q in %q", name, bad, view)
			}
		}
		if !strings.Contains(view, "pwned") && !strings.Contains(view, "reply") {
			t.Errorf("%s: the text is missing from\n%s", name, view)
		}
	}
}

func TestTheScreenKeepsItsLinesWithinTheWindowWhileThePromptIsOpen(t *testing.T) {
	m := loadedModel(agentsNamed(40))
	for height := 4; height <= 30; height++ {
		next, _ := send(m, tea.WindowSizeMsg{Width: 60, Height: height})
		next, _ = send(next, key("s"))
		next = typedText(next, strings.Repeat("long ", 30))
		lines := strings.Split(next.View(), "\n")
		if len(lines) > height {
			t.Fatalf("height %d: %d lines", height, len(lines))
		}
		for _, line := range lines {
			if lipglossWidth(line) > 60 {
				t.Fatalf("height %d: %q is %d wide", height, line, lipglossWidth(line))
			}
		}
	}
}

type tmuxRecorder struct {
	mu       sync.Mutex
	outputs  []attach.Command
	answers  map[string]string
	failures map[string]error
	handed   []attach.Command
}

func tmuxSubcommand(c attach.Command) string {
	for _, name := range []string{"display-message", "select-window", "select-pane", "attach-session", "switch-client"} {
		if slices.Contains(c.Args, name) {
			return name
		}
	}
	return ""
}

func (r *tmuxRecorder) output(c attach.Command) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outputs = append(r.outputs, c)
	return r.answers[tmuxSubcommand(c)], r.failures[tmuxSubcommand(c)]
}

func (r *tmuxRecorder) handoff(c attach.Command) tea.Cmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handed = append(r.handed, c)
	return func() tea.Msg { return nil }
}

func (r *tmuxRecorder) ran() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.outputs {
		out = append(out, tmuxSubcommand(c))
	}
	return out
}

func attachableModel(r *tmuxRecorder, environ ...string) Model {
	agents := []hubclient.Agent{
		{Name: "alpha", Harness: "claude", Host: "mac", Connected: true, TmuxSocket: "default", TmuxPane: "%3"},
		{Name: "bravo", Harness: "codex", Host: "buildbox", Connected: true, TmuxSocket: "default", TmuxPane: "%5"},
		{Name: "charlie", Harness: "codex", Host: "mac", Connected: true},
		{Name: "delta", Harness: "codex", Host: "mac", Connected: true, TmuxSocket: "default", TmuxPane: "%x"},
		{Name: "echo", Harness: "codex", Host: "mac", Connected: true, TmuxSocket: "a b", TmuxPane: "%2"},
		{Name: "foxtrot", Harness: "codex", Host: "-oProxyCommand=id", Connected: true, TmuxSocket: "default", TmuxPane: "%2"},
	}
	m := loadedModel(agents).WithAttach(Attach{Host: "mac", Environ: append([]string{"PATH=/usr/bin", "HOME=/home/dev"}, environ...), Output: r.output, Handoff: r.handoff})
	return m
}

func selectingHandle(m Model, handle string) Model {
	for range 10 {
		if m.SelectedHandle() == handle {
			return m
		}
		m, _ = send(m, key("j"))
	}
	return m
}

func attachingTo(m Model, handle string) Model {
	m = selectingHandle(m, handle)
	m, cmd := send(m, key("a"))
	return settleCommands(m, cmd)
}

func TestAttachOnAnAgentWithNoTmuxAttributesOrABadValueSaysItCannotAttachAndRunsNothing(t *testing.T) {
	for _, handle := range []string{"charlie", "delta", "echo", "foxtrot"} {
		r := &tmuxRecorder{}
		m := attachingTo(attachableModel(r), handle)
		view := strings.ToLower(m.View())
		if !strings.Contains(view, "cannot attach") {
			t.Errorf("%s: no explanation in\n%s", handle, m.View())
		}
		if strings.Contains(view, "ssh -t") {
			t.Errorf("%s: an ssh line in\n%s", handle, m.View())
		}
		if len(r.outputs)+len(r.handed) != 0 {
			t.Errorf("%s: something ran: %v %v", handle, r.outputs, r.handed)
		}
	}
}

func TestAttachOnAnAgentOnAnotherHostShowsTheSSHLineAndRunsNothing(t *testing.T) {
	r := &tmuxRecorder{}
	m := attachingTo(attachableModel(r), "bravo")
	if want := "ssh -t buildbox tmux -L default attach-session -t '%5'"; !strings.Contains(m.View(), want) {
		t.Fatalf("no %q in\n%s", want, m.View())
	}
	if len(r.outputs)+len(r.handed) != 0 {
		t.Fatalf("something ran: %v %v", r.outputs, r.handed)
	}
	m, cmd := send(m, key("y"))
	settleCommands(m, cmd)
	if len(r.outputs)+len(r.handed) != 0 {
		t.Fatalf("y ran something: %v %v", r.outputs, r.handed)
	}
}

func TestAttachOnThisHostAsksTmuxFirstAndShowsItsAnswerBeforeAnythingElseRuns(t *testing.T) {
	r := &tmuxRecorder{answers: map[string]string{"display-message": "work vim /home/dev/app\n"}}
	m := attachingTo(attachableModel(r), "alpha")
	if got := r.ran(); !slices.Equal(got, []string{"display-message"}) {
		t.Fatalf("ran %v", got)
	}
	if want := []string{"-L", "default", "display-message", "-p", "-t", "%3", "#{session_name} #{pane_current_command} #{pane_current_path}"}; !slices.Equal(r.outputs[0].Args, want) {
		t.Fatalf("args %q", r.outputs[0].Args)
	}
	view := m.View()
	for _, want := range []string{"work vim /home/dev/app", "%3"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
	if !strings.Contains(strings.ToLower(view), "tmux") {
		t.Errorf("the answer is not marked as tmux's:\n%s", view)
	}
	if len(r.handed) != 0 {
		t.Fatal("the terminal was handed over before the confirmation")
	}
}

func TestYSelectsTheWindowAndThePaneInOrderThenHandsTheTerminalToTheAttachCommand(t *testing.T) {
	r := &tmuxRecorder{answers: map[string]string{"display-message": "work vim /home/dev/app"}}
	m := attachingTo(attachableModel(r, "TMUX_TMPDIR=/elsewhere"), "alpha")
	m, cmd := send(m, key("y"))
	m = settleCommands(m, cmd)
	if got := r.ran(); !slices.Equal(got, []string{"display-message", "select-window", "select-pane"}) {
		t.Fatalf("ran %v", got)
	}
	if !slices.Equal(r.outputs[1].Args, []string{"-L", "default", "select-window", "-t", "%3"}) || !slices.Equal(r.outputs[2].Args, []string{"-L", "default", "select-pane", "-t", "%3"}) {
		t.Fatalf("args %q %q", r.outputs[1].Args, r.outputs[2].Args)
	}
	if len(r.handed) != 1 || !slices.Equal(r.handed[0].Args, []string{"-L", "default", "attach-session", "-t", "%3"}) {
		t.Fatalf("handed %+v", r.handed)
	}
	for _, c := range append(slices.Clone(r.outputs), r.handed...) {
		if slices.ContainsFunc(c.Env, func(pair string) bool { return strings.HasPrefix(pair, "TMUX_TMPDIR=") }) {
			t.Errorf("%q runs with TMUX_TMPDIR", c.Args)
		}
	}
}

func TestInsideTheSameTmuxSocketYSwitchesTheClientAndInsideAnotherItAttachesWithTMUXUnset(t *testing.T) {
	same := &tmuxRecorder{answers: map[string]string{"display-message": "work vim /p"}}
	m := attachingTo(attachableModel(same, "TMUX=/tmp/tmux-501/default,1234,0"), "alpha")
	m, cmd := send(m, key("y"))
	settleCommands(m, cmd)
	if len(same.handed) != 1 || !slices.Equal(same.handed[0].Args, []string{"-L", "default", "switch-client", "-t", "%3"}) {
		t.Fatalf("same socket: %+v", same.handed)
	}
	other := &tmuxRecorder{answers: map[string]string{"display-message": "work vim /p"}}
	m = attachingTo(attachableModel(other, "TMUX=/tmp/tmux-501/other,1234,0"), "alpha")
	m, cmd = send(m, key("y"))
	settleCommands(m, cmd)
	if len(other.handed) != 1 || !slices.Equal(other.handed[0].Args, []string{"-L", "default", "attach-session", "-t", "%3"}) {
		t.Fatalf("another socket: %+v", other.handed)
	}
	if slices.ContainsFunc(other.handed[0].Env, func(pair string) bool { return strings.HasPrefix(pair, "TMUX=") }) {
		t.Fatalf("TMUX is set for the child: %q", other.handed[0].Env)
	}
}

func TestAnyKeyButYCancelsTheConfirmationAndNothingMoreRuns(t *testing.T) {
	for _, cancel := range []tea.Msg{key("n"), key("Y"), key("x"), tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEnter}} {
		r := &tmuxRecorder{answers: map[string]string{"display-message": "work vim /p"}}
		m := attachingTo(attachableModel(r), "alpha")
		m, cmd := send(m, cancel)
		m = settleCommands(m, cmd)
		if got := r.ran(); !slices.Equal(got, []string{"display-message"}) || len(r.handed) != 0 {
			t.Errorf("%v: ran %v handed %v", cancel, got, r.handed)
		}
		if strings.Contains(m.View(), "work vim /p") {
			t.Errorf("%v: the confirmation is still shown:\n%s", cancel, m.View())
		}
		m, cmd = send(m, key("y"))
		settleCommands(m, cmd)
		if len(r.ran()) != 1 || len(r.handed) != 0 {
			t.Errorf("%v: a later y ran something: %v %v", cancel, r.ran(), r.handed)
		}
	}
}

func TestAnEmptyOrWhitespaceAnswerFromTmuxStopsTheAttachWithAMessage(t *testing.T) {
	for _, answer := range []string{"", "\n", "  \t \n"} {
		r := &tmuxRecorder{answers: map[string]string{"display-message": answer}}
		m := attachingTo(attachableModel(r), "alpha")
		m, cmd := send(m, key("y"))
		m = settleCommands(m, cmd)
		if got := r.ran(); !slices.Equal(got, []string{"display-message"}) || len(r.handed) != 0 {
			t.Errorf("answer %q: ran %v handed %v", answer, got, r.handed)
		}
		if !strings.Contains(strings.ToLower(m.View()), "tmux") {
			t.Errorf("answer %q: no message in\n%s", answer, m.View())
		}
	}
}

func TestAFailingInspectCommandStopsTheAttachWithAMessage(t *testing.T) {
	r := &tmuxRecorder{failures: map[string]error{"display-message": errors.New("no server running on /tmp/tmux-501/default")}}
	m := attachingTo(attachableModel(r), "alpha")
	m, cmd := send(m, key("y"))
	m = settleCommands(m, cmd)
	if got := r.ran(); !slices.Equal(got, []string{"display-message"}) || len(r.handed) != 0 {
		t.Fatalf("ran %v handed %v", got, r.handed)
	}
	if !strings.Contains(m.View(), "no server running") {
		t.Fatalf("the failure is not shown:\n%s", m.View())
	}
}

func TestAFailingSelectCommandStopsBeforeTheNextOneAndBeforeTheAttach(t *testing.T) {
	for _, failing := range []string{"select-window", "select-pane"} {
		r := &tmuxRecorder{
			answers:  map[string]string{"display-message": "work vim /p"},
			failures: map[string]error{failing: errors.New("can't find window")},
		}
		m := attachingTo(attachableModel(r), "alpha")
		m, cmd := send(m, key("y"))
		m = settleCommands(m, cmd)
		want := []string{"display-message", "select-window"}
		if failing == "select-pane" {
			want = append(want, "select-pane")
		}
		if got := r.ran(); !slices.Equal(got, want) || len(r.handed) != 0 {
			t.Errorf("%s: ran %v handed %v", failing, got, r.handed)
		}
		if !strings.Contains(m.View(), "can't find window") {
			t.Errorf("%s: the failure is not shown:\n%s", failing, m.View())
		}
	}
}

func TestTheAnswerOfTmuxCarriesNoControlRuneOnTheScreen(t *testing.T) {
	r := &tmuxRecorder{answers: map[string]string{"display-message": "s\x1b]0;pwned\x07 cmd /p\u202eath\u2028x\r\n"}}
	m := attachingTo(attachableModel(r), "alpha")
	view := m.View()
	for _, bad := range []string{"\x1b", "\x07", "\r", "\u202e", "\u2028"} {
		if strings.Contains(view, bad) {
			t.Errorf("%q in %q", bad, view)
		}
	}
	if !strings.Contains(view, "pwned") {
		t.Fatalf("the answer is missing:\n%s", view)
	}
}

func TestThePollGoesOnWhileTheConfirmationIsOpen(t *testing.T) {
	r := &tmuxRecorder{answers: map[string]string{"display-message": "work vim /p"}}
	m := attachingTo(attachableModel(r), "alpha")
	m, cmd := send(m, snapshotMsg{agents: sample, at: clock.Add(time.Second), took: 10 * time.Millisecond})
	if cmd == nil {
		t.Fatal("a poll that finished while the confirmation was open scheduled nothing")
	}
	if _, cmd := send(m, tickMsg{generation: m.generation}); cmd == nil {
		t.Fatal("the tick did not fetch while the confirmation was open")
	}
}

func TestAnAInThePromptIsTextAndDoesNotAttach(t *testing.T) {
	r := &tmuxRecorder{answers: map[string]string{"display-message": "work vim /p"}}
	m := attachableModel(r)
	m, _ = send(m, key("s"))
	m = typedText(m, "a")
	settleCommands(m, nil)
	if len(r.outputs) != 0 || len(r.handed) != 0 {
		t.Fatal("a typed a in the prompt attached")
	}
}
