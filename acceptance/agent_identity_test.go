package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

const plainUnknownRecipient = "the recipient is not a registered agent"

var fourHex = regexp.MustCompile(`^[0-9a-f]{4}$`)

func decodeObject(t *testing.T, body string) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal([]byte(body), &object); err != nil {
		t.Fatalf("not a JSON object: %q: %v", body, err)
	}
	return object
}

func keysOf(object map[string]any) []string {
	var keys []string
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (w *world) connectedWithProject(id, project string) {
	w.t.Helper()
	if project == "" {
		w.register(id)
	} else {
		w.registerWithProject(id, project)
	}
	w.openStream(id, true)
	w.waitState(id, "connected")
}

func instantOf(t *testing.T, value any) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, fmt.Sprint(value))
	if err != nil {
		t.Fatalf("%v is not an RFC 3339 time: %v", value, err)
	}
	return parsed.Truncate(time.Second)
}

func sameInstant(t *testing.T, a, b any) bool {
	t.Helper()
	return instantOf(t, a).Equal(instantOf(t, b))
}

func (w *world) setTask(id, task string) (int, string) {
	w.t.Helper()
	return w.post("/v1/status", w.token(id), map[string]string{"task": task})
}

func (w *world) listedByObserver() []map[string]any {
	w.t.Helper()
	var listed []map[string]any
	if err := json.Unmarshal([]byte(w.agentsBody(w.registerObserver("tui"))), &listed); err != nil {
		w.t.Fatal(err)
	}
	return listed
}

func (w *world) agentEntry(id string) map[string]any {
	w.t.Helper()
	for _, entry := range w.listedByObserver() {
		if entry["id"] == id {
			return entry
		}
	}
	w.t.Fatalf("%s is not in /v1/agents", id)
	return nil
}

func TestARegistrationBySeedAnswersAnIDOfFourLowercaseHexCharactersThatNoOtherListedAgentHoldsAndTheSeedNeverShapesIt(t *testing.T) {
	t.Parallel()
	w := start(t)
	body := map[string]string{"seed": "landline-menubar", "harness": "claude-code", "host": "mac", "project": "landline", "folder": "landline-menubar"}
	status, text := w.post("/v1/register", secret, body)
	if status != http.StatusOK {
		t.Fatalf("%d %s", status, text)
	}
	answer := decodeObject(t, text)
	if strings.Join(keysOf(answer), ",") != "id,token" {
		t.Fatalf("the answer has the keys %v, want id and token", keysOf(answer))
	}
	if id, _ := answer["id"].(string); !fourHex.MatchString(id) {
		t.Fatalf("the id %q is not 4 lowercase hexadecimal characters", answer["id"])
	}
	const registrations = 2500
	seen := map[string]bool{answer["id"].(string): true}
	sawLetter, sawDescent, previous := false, false, answer["id"].(string)
	for i := 0; i < registrations; i++ {
		status, text := w.post("/v1/register", secret, map[string]string{"seed": fmt.Sprintf("worker-%d", i), "harness": "claude-code", "host": "mac"})
		if status != http.StatusOK {
			t.Fatalf("registration %d: %d %s", i, status, text)
		}
		id, _ := decodeObject(t, text)["id"].(string)
		if !fourHex.MatchString(id) || strings.Contains(id, "worker") {
			t.Fatalf("registration %d got the id %q", i, id)
		}
		if seen[id] {
			t.Fatalf("the id %s was issued twice while its first holder is still listed", id)
		}
		seen[id] = true
		if strings.ContainsAny(id, "abcdef") {
			sawLetter = true
		}
		if id < previous {
			sawDescent = true
		}
		previous = id
	}
	if !sawLetter || !sawDescent {
		t.Fatalf("the ids look neither random nor hexadecimal: a letter was seen %v, a descent was seen %v", sawLetter, sawDescent)
	}
	if listed := w.listedByObserver(); len(listed) != registrations+1 {
		t.Fatalf("%d agents are listed, want %d", len(listed), registrations+1)
	}
}

func TestARegistrationNeedsExactlyOneOfAnIDAndASeedAndRefusesTheOldKeys(t *testing.T) {
	t.Parallel()
	w := start(t)
	base := func(extra map[string]string) map[string]string {
		body := map[string]string{"harness": "claude-code", "host": "mac"}
		for key, value := range extra {
			body[key] = value
		}
		return body
	}
	for name, body := range map[string]map[string]string{
		"an id and a seed":            base(map[string]string{"id": "alpha", "seed": "alpha"}),
		"neither an id nor a seed":    base(nil),
		"the old key name":            base(map[string]string{"name": "alpha"}),
		"an id and the old key name":  base(map[string]string{"id": "alpha", "name": "alpha"}),
		"the old key label":           base(map[string]string{"id": "alpha", "label": "lead"}),
		"a seed and the old key name": base(map[string]string{"seed": "x", "name": "alpha"}),
		"an id with capitals":         base(map[string]string{"id": "Alpha"}),
		"the id all":                  base(map[string]string{"id": "all"}),
		"an id of 33 characters":      base(map[string]string{"id": strings.Repeat("a", 33)}),
	} {
		if status, text := w.post("/v1/register", secret, body); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
	if listed := w.listedByObserver(); len(listed) != 0 {
		t.Fatalf("a refused registration left agents: %v", listed)
	}
	for name, id := range map[string]string{"one character": "a", "32 characters": strings.Repeat("a", 32), "hex shaped": "a3f9", "with dash and underscore": "a-b_c"} {
		if status, text := w.post("/v1/register", secret, base(map[string]string{"id": id})); status != http.StatusOK || decodeObject(t, text)["id"] != id {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
}

func TestAHandRegisteredIDAndProjectAreRecordedAsGivenAndTheRegisterCommandTakesOnlyTheNewFlags(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.must("", "register", "--id", "alpha", "--project", "landline", "--harness", "claude-code", "--host", "mac")
	w.must("", "register", "--id", strings.Repeat("b", 32), "--harness", "claude-code", "--host", "mac")
	entry := w.agentEntry("alpha")
	if entry["project"] != "landline" {
		t.Fatalf("the entry: %v", entry)
	}
	for _, old := range []string{"name", "label", "handle"} {
		if _, ok := entry[old]; ok {
			t.Errorf("the entry still has the key %q: %v", old, entry)
		}
	}
	if out, err := w.run("", "register", "--id", strings.Repeat("c", 33), "--harness", "claude-code", "--host", "mac"); err == nil {
		t.Errorf("an id of 33 characters registered: %q", out)
	}
	for name, args := range map[string][]string{
		"--name":                   {"register", "--name", "echo", "--harness", "claude-code", "--host", "mac"},
		"--label":                  {"register", "--id", "echo", "--label", "lead", "--harness", "claude-code", "--host", "mac"},
		"--name beside --id":       {"register", "--id", "echo", "--name", "echo", "--harness", "claude-code", "--host", "mac"},
		"claude --label":           {"claude", "--label", "lead", "--print"},
		"claude --name":            {"claude", "--name", "lead", "--print"},
		"an unknown flag on agent": {"agents", "--project", "lead"},
	} {
		result := runBin(t, w.bin, w.env, args...)
		if result.code == 0 || strings.TrimSpace(result.stderr) == "" || (args[0] == "claude" && result.stdout != "") {
			t.Errorf("%s: %+v", name, result)
		}
	}
	if status := w.state("echo"); status != "" {
		t.Errorf("a refused registration listed echo as %q", status)
	}
}

func TestTheMCPProcessRefusesTheOldLabelFlagAndIgnoresTheOldEnvironmentVariable(t *testing.T) {
	t.Parallel()
	w := start(t)
	result := runBin(t, mcpOf(w.bin), w.env, "--label", "lead")
	if result.code == 0 || strings.TrimSpace(result.stderr) == "" {
		t.Fatalf("landline-mcp --label: %+v", result)
	}
	dir := filepath.Join(t.TempDir(), "demo-folder")
	os.MkdirAll(dir, 0o755)
	p := w.startMCPWith(withEnv(w.env, "LANDLINE_LABEL=oldlabel"), dir)
	p.handshake("claude-code", "2.1.296")
	claimed := p.tool("landline_register", `{"model":"opus"}`)
	if strings.Contains(claimed, "oldlabel") {
		t.Fatalf("LANDLINE_LABEL still names a project: %q", claimed)
	}
	if listed := p.tool("landline_agents", `{}`); strings.Contains(listed, "oldlabel") {
		t.Fatalf("LANDLINE_LABEL reached the list: %s", listed)
	}
	given := w.startMCPWith(withEnv(w.env, "LANDLINE_PROJECT=from-environment"), dir)
	given.handshake("claude-code", "2.1.296")
	claimed = given.tool("landline_register", `{"model":"opus"}`)
	if !strings.Contains(claimed, "project: from-environment (as you gave it)") {
		t.Fatalf("LANDLINE_PROJECT: %q", claimed)
	}
}

func TestAnOldKeyOnAttachIsRefusedWith400AndRecordsNothing(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha")
	for name, body := range map[string]map[string]string{"label": {"label": "lead"}, "name": {"name": "x"}, "handle": {"handle": "x"}, "label with model": {"model": "m", "label": "lead"}} {
		if status, text := w.post("/v1/attach", w.token("alpha"), body); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
	if entry := w.agentEntry("alpha"); entry["project"] != nil || entry["model"] != nil {
		t.Fatalf("a refused attach recorded something: %v", entry)
	}
	status, text := w.post("/v1/attach", w.token("alpha"), map[string]string{"project": "lead", "model": "m"})
	if status != http.StatusOK {
		t.Fatalf("%d %s", status, text)
	}
	answer := decodeObject(t, text)
	if answer["id"] != "alpha" {
		t.Errorf("the attach answer names %v, want the key id with alpha", answer)
	}
	for _, old := range []string{"name", "label", "handle"} {
		if _, ok := answer[old]; ok {
			t.Errorf("the attach answer keeps the old key %q: %s", old, text)
		}
	}
	if entry := w.agentEntry("alpha"); entry["project"] != "lead" || entry["model"] != "m" {
		t.Fatalf("the entry: %v", entry)
	}
}

func TestAMessageHeaderAndTheLogUseIDsAndTheProjectAndNeverCarryTheTask(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.connectedWithProject("alpha", "web")
	w.register("bravo")
	stream := w.openStream("bravo", true)
	if status, text := w.setTask("alpha", "ship the checkout flow"); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	if status, body := w.sendHTTP("alpha", "bravo", "hello bravo"); status != 200 {
		t.Fatalf("%d %s", status, body)
	}
	delivery := stream.next(t)
	if !strings.HasPrefix(delivery.Text, marker+"\nfrom: alpha (claude-code on mac)") || !strings.Contains(delivery.Text, "project: web") {
		t.Fatalf("the header: %q", delivery.Text)
	}
	for _, text := range []string{delivery.Text, delivery.Message.Text, w.logText(), w.must("", "log")} {
		for _, hidden := range []string{"ship the checkout flow", "task:", "task_at", "label:", "handle:"} {
			if strings.Contains(text, hidden) {
				t.Fatalf("%q reached a header or the log:\n%s", hidden, text)
			}
		}
	}
	raw, err := os.ReadFile(filepath.Join(w.config, "log.jsonl"))
	if err != nil || strings.Contains(string(raw), "ship the checkout") || !strings.Contains(string(raw), `"from":"alpha"`) || !strings.Contains(string(raw), `"to":"bravo"`) {
		t.Fatalf("the log file keeps from and to and holds no task: %v %s", err, raw)
	}
}

func TestASendTakesAnExactIDAndNothingElseAndAProjectGetsTheHintOnlyOnTheUnknownRecipient404(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.connectedWithProject("alpha", "web")
	w.connectedWithProject("bravo", "web")
	w.connectedWithProject("charlie", "web")
	w.registerWithProject("delta", "web")
	w.connectedWithProject("echo", "solo")
	w.registerWithProject("foxtrot", "ghosts")
	w.connectedWithProject("golf", "all")
	w.connectedWithProject("hotel", "")
	const hint = "web is a project, not an agent. Send to one of: bravo, charlie"
	for name, c := range map[string]struct {
		from, to string
		want     int
		hint     string
	}{
		"a project with connected holders other than the sender": {"hotel", "web", 404, "web is a project, not an agent. Send to one of: alpha, bravo, charlie"},
		"the sender is left out of the ids":                      {"alpha", "web", 404, hint},
		"a project held only by the sender":                      {"echo", "solo", 404, ""},
		"a project held only by disconnected agents":             {"alpha", "ghosts", 404, ""},
		"all, even as the project of an agent":                   {"alpha", "all", 404, ""},
		"a prefix of an id":                                      {"alpha", "brav", 404, ""},
		"a prefix of a project":                                  {"hotel", "we", 404, ""},
		"a project in other letters":                             {"hotel", "WEB", 404, ""},
		"an id with a trailing space":                            {"hotel", "bravo ", 404, ""},
		"an unknown word":                                        {"hotel", "ghost", 404, ""},
	} {
		status, body := w.sendHTTP(c.from, c.to, "hello")
		if status != c.want {
			t.Errorf("%s: %d %s", name, status, body)
			continue
		}
		text := unknownRecipientText(t, body)
		if c.hint == "" && text != plainUnknownRecipient {
			t.Errorf("%s: the text is %q, want exactly %q", name, text, plainUnknownRecipient)
		}
		if c.hint != "" && (!strings.HasPrefix(text, plainUnknownRecipient) || !strings.HasSuffix(text, c.hint)) {
			t.Errorf("%s: the text is %q, want the plain text then %q", name, text, c.hint)
		}
	}
	if got := w.logText(); strings.TrimSpace(got) != "" {
		t.Fatalf("a refused send reached the log: %q", got)
	}
	if status, body := w.sendHTTP("hotel", "delta", "to a disconnected id"); status != 409 || !strings.Contains(body, notConnected) {
		t.Fatalf("the id of an agent that is listed and not connected: %d %s", status, body)
	}
	if status, body := w.sendHTTP("hotel", "bravo", "by id"); status != 200 {
		t.Fatalf("an exact id: %d %s", status, body)
	}
	out, err := w.run("hello", "send", "--as", "hotel", "--to", "web", "-")
	if err == nil || !strings.Contains(out, "web is a project, not an agent. Send to one of: alpha, bravo, charlie") {
		t.Fatalf("landline send to a project: %q, %v", out, err)
	}
}

func TestTheHintListsAtMostTenSortedIDsAndSaysAndMoreAfterTheTenthOnlyWhenThereAreOthers(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.connectedWithProject("sender", "elsewhere")
	ids := func(n int) string {
		var out []string
		for i := 1; i <= n; i++ {
			out = append(out, fmt.Sprintf("m%02d", i))
		}
		return strings.Join(out, ", ")
	}
	for i := 10; i >= 1; i-- {
		w.connectedWithProject(fmt.Sprintf("m%02d", i), "crew")
	}
	status, body := w.sendHTTP("sender", "crew", "hi")
	if text, want := unknownRecipientText(t, body), "crew is a project, not an agent. Send to one of: "+ids(10); status != 404 || !strings.HasSuffix(text, want) || strings.Contains(text, "and more") {
		t.Fatalf("with 10 holders: %d %s, want the text to end with %q and not say and more", status, body, want)
	}
	w.connectedWithProject("m12", "crew")
	w.connectedWithProject("m11", "crew")
	status, body = w.sendHTTP("sender", "crew", "hi")
	want := "crew is a project, not an agent. Send to one of: " + ids(10) + ", and more"
	if text := unknownRecipientText(t, body); status != 404 || !strings.HasSuffix(text, want) || strings.Contains(text, "m11") || strings.Contains(text, "m12") {
		t.Fatalf("with 12 holders: %d %s, want the text to end with %q", status, body, want)
	}
}

func TestTheHintLeavesOutHoldersThatHaveDisconnected(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("sender")
	one, oneID := w.selfRegistered("crew", "claude-code")
	_, twoID := w.selfRegistered("crew", "claude-code")
	both := []string{oneID, twoID}
	sort.Strings(both)
	status, body := w.sendHTTP("sender", "crew", "hi")
	if want := "crew is a project, not an agent. Send to one of: " + strings.Join(both, ", "); status != 404 || !strings.HasSuffix(unknownRecipientText(t, body), want) {
		t.Fatalf("two holders: %d %s", status, body)
	}
	one.stop()
	w.waitState(oneID, "disconnected")
	status, body = w.sendHTTP("sender", "crew", "hi")
	if want := "crew is a project, not an agent. Send to one of: " + twoID; status != 404 || !strings.HasSuffix(unknownRecipientText(t, body), want) {
		t.Fatalf("one connected holder and one in its grace: %d %s", status, body)
	}
}

func (w *world) tryStatus(token string, raw string) (int, string) {
	w.t.Helper()
	req, _ := http.NewRequest("POST", w.url+"/v1/status", strings.NewReader(raw))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.String()
}

func TestStatusNeedsAnAgentTokenAndANonEmptyBodyWithOneStringFieldNamedTask(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha")
	observer := w.registerObserver("tui")
	for name, token := range map[string]string{"no token": "", "a wrong token": "unknown-token-unknown-token", "the secret": secret} {
		if status, text := w.tryStatus(token, `{"task":"x"}`); status != http.StatusUnauthorized {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
	if status, text := w.tryStatus(observer, `{"task":"x"}`); status != http.StatusForbidden || !strings.Contains(text, "error") {
		t.Errorf("an observer token: %d %s", status, text)
	}
	if entry := w.agentEntry("alpha"); entry["task"] != nil {
		t.Fatalf("a refused request set a task: %v", entry)
	}
	token := w.token("alpha")
	for name, raw := range map[string]string{
		"an empty body":                ``,
		"not JSON":                     `task=x`,
		"an empty object":              `{}`,
		"a missing task":               `{"other":"x"}`,
		"null":                         `{"task":null}`,
		"a number":                     `{"task":5}`,
		"a boolean":                    `{"task":true}`,
		"an array":                     `{"task":["x"]}`,
		"an object":                    `{"task":{"a":"b"}}`,
		"an unknown field":             `{"task":"x","extra":1}`,
		"the task time from agent":     `{"task":"x","task_at":"2026-01-01T00:00:00Z"}`,
		"the old key label":            `{"task":"x","label":"y"}`,
		"two JSON values":              `{"task":"x"}{"task":"y"}`,
		"an array of objects":          `[{"task":"x"}]`,
		"a bare string":                `"x"`,
		"65 bytes":                     `{"task":"` + strings.Repeat("a", 65) + `"}`,
		"66 bytes of two byte runes":   `{"task":"` + strings.Repeat("é", 33) + `"}`,
		"66 bytes of three byte runes": `{"task":"` + strings.Repeat("日", 22) + `"}`,
	} {
		if status, text := w.tryStatus(token, raw); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
	for name, task := range map[string]string{
		"a line feed":              "a\nb",
		"a tab":                    "a\tb",
		"a carriage return":        "a\rb",
		"an escape":                "a\x1bb",
		"a delete character":       "a\x7fb",
		"a line separator":         "a\u2028b",
		"a paragraph separator":    "a\u2029b",
		"a zero width space":       "a\u200bb",
		"a right to left override": "a\u202eb",
		"a byte order mark":        "a\ufeffb",
	} {
		if status, text := w.setTask("alpha", task); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
	if entry := w.agentEntry("alpha"); entry["task"] != nil || entry["task_at"] != nil {
		t.Fatalf("a refused request set a task: %v", entry)
	}
	for name, task := range map[string]string{
		"64 bytes":                     strings.Repeat("a", 64),
		"64 bytes of two byte runes":   strings.Repeat("é", 32),
		"63 bytes of three byte runes": strings.Repeat("日", 21),
		"words and punctuation":        "menu bar app for ADR 0140",
	} {
		if status, text := w.setTask("alpha", task); status != http.StatusOK || decodeObject(t, text)["task"] != task {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
}

func TestStatusSetsTheTaskAnswersItWithTheTimeTheSwitchboardStampedAndListsItOnlyWhileItIsSet(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha", "bravo")
	before := time.Now().Add(-2 * time.Second)
	status, text := w.setTask("alpha", "menu bar app for ADR 0140")
	after := time.Now().Add(2 * time.Second)
	if status != http.StatusOK {
		t.Fatalf("%d %s", status, text)
	}
	answer := decodeObject(t, text)
	if strings.Join(keysOf(answer), ",") != "task,task_at" || answer["task"] != "menu bar app for ADR 0140" {
		t.Fatalf("the answer: %s", text)
	}
	first, err := time.Parse(time.RFC3339, fmt.Sprint(answer["task_at"]))
	if err != nil || first.Before(before) || first.After(after) {
		t.Fatalf("task_at %v is not an RFC 3339 time near now: %v", answer["task_at"], err)
	}
	entry := w.agentEntry("alpha")
	if entry["task"] != "menu bar app for ADR 0140" || entry["task_at"] == nil || !sameInstant(t, entry["task_at"], answer["task_at"]) {
		t.Fatalf("the list entry: %v", entry)
	}
	if other := w.agentEntry("bravo"); other["task"] != nil || other["task_at"] != nil {
		t.Fatalf("another agent has a task: %v", other)
	}
	time.Sleep(1100 * time.Millisecond)
	status, text = w.setTask("alpha", "menu bar app for ADR 0140")
	again := decodeObject(t, text)
	if status != http.StatusOK || !sameInstant(t, again["task_at"], answer["task_at"]) || strings.Join(keysOf(again), ",") != "task,task_at" {
		t.Fatalf("the same text is not a change and keeps the time: %d %s, first %s", status, text, answer["task_at"])
	}
	if entry := w.agentEntry("alpha"); !sameInstant(t, entry["task_at"], answer["task_at"]) {
		t.Fatalf("the same text moved task_at in the list: %v", entry)
	}
	status, text = w.setTask("alpha", "menu bar app for ADR 0140 and the tui")
	changed := decodeObject(t, text)
	if status != http.StatusOK {
		t.Fatalf("%d %s", status, text)
	}
	next, err := time.Parse(time.RFC3339, fmt.Sprint(changed["task_at"]))
	if err != nil || !next.After(first) {
		t.Fatalf("a change keeps the old time: first %v, next %v, %v", first, changed["task_at"], err)
	}
	if entry := w.agentEntry("alpha"); entry["task"] != "menu bar app for ADR 0140 and the tui" || entry["task_at"] == nil || !sameInstant(t, entry["task_at"], changed["task_at"]) {
		t.Fatalf("the list entry after a change: %v", entry)
	}
	status, text = w.setTask("alpha", "")
	cleared := decodeObject(t, text)
	if status != http.StatusOK || strings.Join(keysOf(cleared), ",") != "task" || cleared["task"] != "" {
		t.Fatalf("a clear answers task empty and no task_at: %d %s", status, text)
	}
	if entry := w.agentEntry("alpha"); entry["task"] != nil || entry["task_at"] != nil {
		t.Fatalf("a cleared task is still listed: %v", entry)
	}
	if status, text = w.setTask("alpha", ""); status != http.StatusOK || strings.Join(keysOf(decodeObject(t, text)), ",") != "task" {
		t.Fatalf("clearing a clear task: %d %s", status, text)
	}
}

func TestARegistrationByIDClearsTheTaskAndTheSwitchboardForgetsItWithTheAgent(t *testing.T) {
	t.Parallel()
	w := startHub(t, build(t), t.TempDir(), "", "--grace", "5s")
	w.register("alpha", "bravo")
	if status, text := w.setTask("alpha", "first goal"); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	w.register("alpha")
	if entry := w.agentEntry("alpha"); entry["task"] != nil || entry["task_at"] != nil {
		t.Fatalf("a registration by id kept the task: %v", entry)
	}
	if status, text := w.setTask("alpha", "second goal"); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	if status, text := w.setTask("bravo", "other goal"); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	w.register("alpha")
	if other := w.agentEntry("bravo"); other["task"] != "other goal" {
		t.Fatalf("registering alpha changed bravo: %v", other)
	}
	if status, text := w.setTask("alpha", "third goal"); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	eventually(t, "alpha to be forgotten", func() bool { return w.state("alpha") == "" })
	w.register("alpha")
	if entry := w.agentEntry("alpha"); entry["task"] != nil || entry["task_at"] != nil {
		t.Fatalf("a forgotten agent's task came back: %v", entry)
	}
}

func TestAHubRestartForgetsEveryTaskAndTheRunningProcessRegistersAgainWithNone(t *testing.T) {
	t.Parallel()
	bin, config := build(t), t.TempDir()
	first := startHub(t, bin, config, "")
	first.register("alpha")
	dir := filepath.Join(t.TempDir(), "demo-folder")
	os.MkdirAll(dir, 0o755)
	p := first.startMCP(dir, "--channel", "--project", "tutor")
	p.handshake("claude-code", "2.1.296")
	p.tool("landline_register", `{"model":"opus"}`)
	if got := p.tool("landline_status", `{"task":"before the restart"}`); got != "task: before the restart" {
		t.Fatalf("landline_status: %q", got)
	}
	first.stop()
	second := startHub(t, bin, config, first.addr)
	second.register("alpha", "watcher")
	p.tool("landline_agents", `{}`)
	newID := second.idWithProject("tutor")
	second.waitState(newID, "connected")
	if entry := second.agentEntry(newID); entry["task"] != nil || entry["task_at"] != nil {
		t.Fatalf("the restarted switchboard holds a task: %v", entry)
	}
	if claimed := p.tool("landline_register", `{"model":"opus"}`); !strings.Contains(claimed, "task: none.") || !strings.HasPrefix(claimed, "you are "+newID+". ") {
		t.Fatalf("the register line after the restart: %q", claimed)
	}
	if got := p.tool("landline_status", `{"task":"after the restart"}`); got != "task: after the restart" {
		t.Fatalf("landline_status: %q", got)
	}
	if entry := second.agentEntry(newID); entry["task"] != "after the restart" {
		t.Fatalf("the new task is not listed: %v", entry)
	}
}

func TestTheTaskShowsAfterTheProjectInTheAttributeTextAndInTheJSONRowsOfLandlineAgentsAndTheTool(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.registerWithProject("alpha", "landline")
	w.register("bravo")
	w.register("charlie")
	if status, text := w.setTask("alpha", "menu bar app"); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	if status, text := w.setTask("bravo", "docs only"); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	if line := w.agentLine("alpha"); !strings.Contains(line, "project: landline  task: menu bar app") {
		t.Fatalf("the attribute text: %q", line)
	}
	if line := w.agentLine("bravo"); !strings.HasSuffix(line, "\ttask: docs only") || strings.Contains(line, "project:") {
		t.Fatalf("an agent with a task and no project: %q", line)
	}
	if line := w.agentLine("charlie"); strings.Contains(line, "task") {
		t.Fatalf("an agent with no task: %q", line)
	}
	result := runBin(t, w.bin, w.env, "agents", "--json")
	var snapshot struct{ Agents []map[string]any }
	if err := json.Unmarshal([]byte(result.stdout), &snapshot); err != nil || result.code != 0 {
		t.Fatalf("%v %+v", err, result)
	}
	for _, row := range snapshot.Agents {
		switch row["id"] {
		case "alpha":
			if row["task"] != "menu bar app" || row["task_at"] == nil || !sameInstant(t, row["task_at"], w.agentEntry("alpha")["task_at"]) || row["project"] != "landline" {
				t.Errorf("alpha: %v", row)
			}
		case "charlie":
			if _, ok := row["task"]; ok {
				t.Errorf("charlie has a task key: %v", row)
			}
			if _, ok := row["task_at"]; ok {
				t.Errorf("charlie has a task_at key: %v", row)
			}
		}
	}
	p := w.startMCP(t.TempDir(), "--as", "bravo")
	p.handshake("claude-code", "2.1.296")
	if listed := p.tool("landline_agents", `{}`); !strings.Contains(listed, "project: landline  task: menu bar app") || !strings.Contains(listed, "task: docs only") {
		t.Fatalf("landline_agents: %s", listed)
	}
}

func TestAnAttachAnswersTheNumberOfOtherConnectedAgentsWithTheCallersExactProjectEvenWhenZero(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.connectedWithProject("alpha", "p1")
	w.connectedWithProject("bravo", "p1")
	w.registerWithProject("charlie", "p1")
	w.connectedWithProject("delta", "p2")
	w.connectedWithProject("echo", "")
	w.connectedWithProject("foxtrot", "P1")
	peers := func(id string, body map[string]string) (any, map[string]any) {
		t.Helper()
		status, text := w.post("/v1/attach", w.token(id), body)
		if status != http.StatusOK {
			t.Fatalf("%s %v: %d %s", id, body, status, text)
		}
		answer := decodeObject(t, text)
		value, present := answer["project_peers"]
		if !present {
			t.Fatalf("%s: project_peers is missing from %s", id, text)
		}
		return value, answer
	}
	for _, c := range []struct {
		name string
		id   string
		body map[string]string
		want float64
	}{
		{"the caller and a disconnected holder are not counted", "alpha", map[string]string{"model": "m"}, 1},
		{"a lone holder gets zero, written", "delta", map[string]string{"model": "m"}, 0},
		{"no project gets zero, written", "echo", map[string]string{"model": "m"}, 0},
		{"another case of the project is another project", "foxtrot", map[string]string{"model": "m"}, 0},
		{"an empty claim still answers the number", "bravo", map[string]string{}, 1},
		{"the count follows the project the call just set", "echo", map[string]string{"project": "p2"}, 1},
		{"a project nobody else holds", "echo", map[string]string{"project": "nobody"}, 0},
		{"back to a shared project", "echo", map[string]string{"project": "p1"}, 2},
	} {
		if got, answer := peers(c.id, c.body); got != c.want {
			t.Errorf("%s: project_peers is %v, want %v in %v", c.name, got, c.want, answer)
		}
	}
}

func TestTheCodexChallengeAnswerCarriesProjectPeersToo(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.connectedWithProject("alpha", "p1")
	w.connectedWithProject("bravo", "p1")
	status, text := w.post("/v1/attach", w.token("alpha"), map[string]string{"codex_thread": threadOne})
	if status != http.StatusOK {
		t.Fatalf("%d %s", status, text)
	}
	answer := decodeObject(t, text)
	if answer["challenge"] != true || answer["project_peers"] != float64(1) || answer["id"] != "alpha" {
		t.Fatalf("the challenge answer: %s", text)
	}
}

func TestAnAttachAfterAPeerDisconnectsDoesNotCountItAndTheGraceDoesNotCountEither(t *testing.T) {
	t.Parallel()
	w := start(t)
	_, oneID := w.selfRegistered("crew", "claude-code")
	two, twoID := w.selfRegistered("crew", "claude-code")
	two.stop()
	w.waitState(twoID, "disconnected")
	status, text := w.post("/v1/attach", w.token(oneID), map[string]string{"model": "m"})
	if status != 200 || decodeObject(t, text)["project_peers"] != float64(0) {
		t.Fatalf("a peer in its grace: %d %s", status, text)
	}
}

func TestTheRegisterToolSaysWhatItRecordedWhereTheProjectCameFromHowManyShareItAndHowToSetATask(t *testing.T) {
	t.Parallel()
	w := start(t)
	line := func(id, project, origin, shared string) string {
		return "you are " + id + ". project: " + project + " (" + origin + "), shared with " + shared + ". task: none. Set one with landline_status when your goal is clear."
	}
	dir := func(name string) string {
		d := filepath.Join(t.TempDir(), name)
		os.MkdirAll(d, 0o755)
		return d
	}
	first := w.startMCP(dir("crew-a"), "--project", "crew")
	first.handshake("claude-code", "2.1.296")
	claimed := first.tool("landline_register", `{"model":"opus"}`)
	firstID := idInRegisterResult(t, claimed)
	if want := line(firstID, "crew", "as you gave it", "no other agent currently connected"); claimed != want {
		t.Fatalf("one agent:\n%q\nwant\n%q", claimed, want)
	}
	w.waitState(firstID, "connected")
	second := w.startMCP(dir("crew-b"), "--project", "crew")
	second.handshake("claude-code", "2.1.296")
	claimed = second.tool("landline_register", `{"model":"opus"}`)
	secondID := idInRegisterResult(t, claimed)
	if want := line(secondID, "crew", "as you gave it", "1 other agent currently connected"); claimed != want {
		t.Fatalf("two agents:\n%q\nwant\n%q", claimed, want)
	}
	w.waitState(secondID, "connected")
	third := w.startMCP(dir("crew-c"), "--project", "crew")
	third.handshake("claude-code", "2.1.296")
	claimed = third.tool("landline_register", `{"model":"opus"}`)
	thirdID := idInRegisterResult(t, claimed)
	if want := line(thirdID, "crew", "as you gave it", "2 other agents currently connected"); claimed != want {
		t.Fatalf("three agents:\n%q\nwant\n%q", claimed, want)
	}
	w.waitState(thirdID, "connected")
	changed := third.tool("landline_register", `{"project":"solo"}`)
	if want := line(thirdID, "solo", "as you gave it", "no other agent currently connected"); changed != want {
		t.Fatalf("a changed project:\n%q\nwant\n%q", changed, want)
	}
	remembered := third.tool("landline_register", `{"model":"opus"}`)
	if want := line(thirdID, "solo", "as you gave it", "no other agent currently connected"); remembered != want {
		t.Fatalf("the project the process already holds:\n%q\nwant\n%q", remembered, want)
	}
	if entry := w.agentEntry(thirdID); entry["project"] != "solo" {
		t.Fatalf("the entry: %v", entry)
	}
	none := w.startMCP(string(filepath.Separator), "--channel")
	none.handshake("claude-code", "2.1.296")
	claimed = none.tool("landline_register", `{"model":"opus"}`)
	noneID := idInRegisterResult(t, claimed)
	checkNoProject := func(claimed string) {
		t.Helper()
		if !strings.Contains(claimed, "no project, so you are in no group") || !strings.Contains(claimed, "task: none") || strings.Contains(claimed, "label") {
			t.Fatalf("no project: %q", claimed)
		}
	}
	checkNoProject(claimed)
	w.waitState(noneID, "connected")
	other := w.startMCP(string(filepath.Separator), "--channel")
	other.handshake("claude-code", "2.1.296")
	otherClaimed := other.tool("landline_register", `{"model":"opus"}`)
	otherID := idInRegisterResult(t, otherClaimed)
	w.waitState(otherID, "connected")
	checkNoProject(otherClaimed)
	checkNoProject(none.tool("landline_register", `{"model":"opus"}`))
	if status, text := w.post("/v1/attach", w.token(noneID), map[string]string{"model": "m"}); status != 200 || decodeObject(t, text)["project_peers"] != float64(0) {
		t.Fatalf("two agents with no project are not a group: %d %s", status, text)
	}
	for _, old := range []string{"label", "handle", "recorded"} {
		if strings.Contains(claimed, old) {
			t.Errorf("the register result keeps the word %q: %q", old, claimed)
		}
	}
}

func TestARepositoryGivesTheProjectItsNameAndAFlagOrAnArgumentOverridesIt(t *testing.T) {
	t.Parallel()
	w := start(t)
	repo := newGitRepos(t)
	for name, c := range map[string]struct{ dir, want string }{
		"a normal repository":                     {repo.main, "foo"},
		"a subfolder of it":                       {repo.sub, "foo"},
		"a linked worktree of it":                 {repo.linked, "foo"},
		"a subfolder of the linked worktree":      {repo.linkedSub, "foo"},
		"a bare repository named foo.git":         {repo.bare, "foo"},
		"a worktree of the bare repository":       {repo.bareLinked, "foo"},
		"a repository with a separate git folder": {repo.separate, "store"},
		"a folder that is no repository":          {repo.plain, "plain-folder"},
	} {
		p := w.startMCP(c.dir)
		p.handshake("claude-code", "2.1.296")
		claimed := p.tool("landline_register", `{"model":"opus"}`)
		id := idInRegisterResult(t, claimed)
		origin := "(from your repository)"
		if !strings.Contains(claimed, "project: "+c.want+" "+origin) {
			t.Errorf("%s: %q, want the project %s %s", name, claimed, c.want, origin)
			continue
		}
		w.waitState(id, "connected")
		if got := w.agentEntry(id)["project"]; got != c.want {
			t.Errorf("%s: the list shows the project %v, want %s", name, got, c.want)
		}
	}
	flagged := w.startMCP(repo.linked, "--project", "chosen")
	flagged.handshake("claude-code", "2.1.296")
	if claimed := flagged.tool("landline_register", `{"model":"opus"}`); !strings.Contains(claimed, "project: chosen (as you gave it)") {
		t.Errorf("--project: %q", claimed)
	}
	given := w.startMCP(repo.linked)
	given.handshake("claude-code", "2.1.296")
	if claimed := given.tool("landline_register", `{"project":"argued"}`); !strings.Contains(claimed, "project: argued (as you gave it)") {
		t.Errorf("the register argument: %q", claimed)
	}
	root := w.startMCP(string(filepath.Separator))
	root.handshake("claude-code", "2.1.296")
	if claimed := root.tool("landline_register", `{"model":"opus"}`); !strings.Contains(claimed, "no project, so you are in no group") {
		t.Errorf("the root folder: %q", claimed)
	}
}

type gitRepos struct{ main, sub, linked, linkedSub, bare, bareLinked, separate, plain string }

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(isolatedEnv(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.invalid", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func newGitRepos(t *testing.T) gitRepos {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("the git binary is not available, so the default project of a repository cannot be checked")
	}
	root := t.TempDir()
	mk := func(parts ...string) string {
		path := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	var r gitRepos
	r.main = mk("foo")
	gitIn(t, r.main, "init", "-q", "-b", "main")
	gitIn(t, r.main, "commit", "-q", "--allow-empty", "-m", "first")
	r.sub = mk("foo", "src", "deep")
	r.linked = filepath.Join(root, "feature-wt")
	gitIn(t, r.main, "worktree", "add", "-q", "-b", "feature", r.linked)
	r.linkedSub = mk("feature-wt", "inner")
	r.bare = mk("bare-parent", "foo.git")
	gitIn(t, r.bare, "init", "-q", "--bare", "-b", "main")
	clone := filepath.Join(root, "seed-clone")
	gitIn(t, root, "clone", "-q", r.bare, clone)
	gitIn(t, clone, "symbolic-ref", "HEAD", "refs/heads/main")
	gitIn(t, clone, "commit", "-q", "--allow-empty", "-m", "first")
	gitIn(t, clone, "push", "-q", "origin", "main")
	r.bareLinked = filepath.Join(root, "bare-wt")
	gitIn(t, r.bare, "worktree", "add", "-q", r.bareLinked, "main")
	r.separate = mk("elsewhere")
	gitIn(t, r.separate, "init", "-q", "-b", "main", "--separate-git-dir", filepath.Join(root, "store.git"))
	r.plain = mk("plain-folder")
	return r
}

func TestTheStatusToolSetsAndClearsTheTaskAndReportsExactlyWhatItDid(t *testing.T) {
	t.Parallel()
	w := start(t)
	p, id := w.selfRegistered("demo-folder", "claude-code")
	if got := p.tool("landline_status", `{"task":"menu bar app for ADR 0140"}`); got != "task: menu bar app for ADR 0140" {
		t.Fatalf("set: %q", got)
	}
	if entry := w.agentEntry(id); entry["task"] != "menu bar app for ADR 0140" || entry["task_at"] == nil {
		t.Fatalf("the list entry: %v", entry)
	}
	if got := p.tool("landline_status", `{"task":""}`); got != "task cleared" {
		t.Fatalf("clear: %q", got)
	}
	if entry := w.agentEntry(id); entry["task"] != nil {
		t.Fatalf("the list entry after a clear: %v", entry)
	}
	for name, arguments := range map[string]string{"65 bytes": `{"task":"` + strings.Repeat("a", 65) + `"}`, "a line break": `{"task":"a\nb"}`} {
		reply := p.request("tools/call", `{"name":"landline_status","arguments":`+arguments+`}`)
		if !toolFailed(reply) {
			t.Errorf("%s was accepted: %v", name, reply)
		}
	}
	for name, arguments := range map[string]string{"no task": `{}`, "a number": `{"task":5}`, "null": `{"task":null}`} {
		reply := p.request("tools/call", `{"name":"landline_status","arguments":`+arguments+`}`)
		if !toolFailed(reply) {
			t.Errorf("%s was accepted: %v", name, reply)
		}
	}
	if entry := w.agentEntry(id); entry["task"] != nil {
		t.Fatalf("a refused call set a task: %v", entry)
	}
}

func toolFailed(reply map[string]any) bool {
	if reply["error"] != nil {
		return true
	}
	result, _ := reply["result"].(map[string]any)
	return result != nil && result["isError"] == true
}

func toolTexts(t *testing.T, reply map[string]any) []string {
	t.Helper()
	result, _ := reply["result"].(map[string]any)
	if result == nil {
		t.Fatalf("no result: %v", reply)
	}
	var texts []string
	for _, item := range result["content"].([]any) {
		texts = append(texts, item.(map[string]any)["text"].(string))
	}
	return texts
}

func TestTheUnregisteredReminderNamesTheStatusToolAndNoStatusCallCountsAsTheIntroduction(t *testing.T) {
	t.Parallel()
	w := start(t)
	dir := filepath.Join(t.TempDir(), "demo-folder")
	os.MkdirAll(dir, 0o755)
	p := w.startMCP(dir)
	p.handshake("claude-code", "2.1.296")
	reminded := func(reply map[string]any) bool {
		texts := toolTexts(t, reply)
		last := texts[len(texts)-1]
		return strings.Contains(last, "You have not told Landline about yourself yet") && strings.Contains(last, "landline_status") && strings.Contains(last, "project") && !strings.Contains(last, "label")
	}
	status := p.request("tools/call", `{"name":"landline_status","arguments":{"task":"first goal"}}`)
	if texts := toolTexts(t, status); texts[0] != "task: first goal" || !reminded(status) {
		t.Fatalf("a status call before the introduction: %v", texts)
	}
	if agents := p.request("tools/call", `{"name":"landline_agents","arguments":{}}`); !reminded(agents) {
		t.Fatalf("a status call counted as the introduction: %v", toolTexts(t, agents))
	}
	p.tool("landline_register", `{"model":"opus"}`)
	if agents := p.request("tools/call", `{"name":"landline_agents","arguments":{}}`); strings.Contains(strings.Join(toolTexts(t, agents), " "), "You have not told") {
		t.Fatalf("a registered process is still reminded: %v", toolTexts(t, agents))
	}
	again := p.request("tools/call", `{"name":"landline_status","arguments":{"task":"second goal"}}`)
	if texts := toolTexts(t, again); len(texts) != 1 || texts[0] != "task: second goal" {
		t.Fatalf("a status call after the introduction: %v", texts)
	}
}

func TestTheToolDescriptionsAndTheInstructionsUseTheNewWordsAndTheStatusToolIsDescribed(t *testing.T) {
	t.Parallel()
	w := start(t)
	p := w.startMCP(t.TempDir())
	handshake := p.handshake("claude-code", "2.1.296")
	var initialized struct {
		Result struct {
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(handshake), &initialized)
	for _, want := range []string{"landline_status", "project"} {
		if !strings.Contains(initialized.Result.Instructions, want) {
			t.Errorf("the server instructions lack %q:\n%s", want, initialized.Result.Instructions)
		}
	}
	for _, old := range []string{"label", "handle"} {
		if strings.Contains(strings.ToLower(initialized.Result.Instructions), old) {
			t.Errorf("the server instructions keep the word %q", old)
		}
	}
	reply := p.request("tools/list", `{}`)
	tools := map[string]map[string]any{}
	for _, each := range reply["result"].(map[string]any)["tools"].([]any) {
		tool := each.(map[string]any)
		tools[tool["name"].(string)] = tool
	}
	properties := func(name string) map[string]any {
		schema, _ := tools[name]["inputSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		return props
	}
	describe := func(name, property string) string {
		if property == "" {
			text, _ := tools[name]["description"].(string)
			return text
		}
		item, _ := properties(name)[property].(map[string]any)
		text, _ := item["description"].(string)
		return text
	}
	var registerKeys []string
	for key := range properties("landline_register") {
		registerKeys = append(registerKeys, key)
	}
	sort.Strings(registerKeys)
	if strings.Join(registerKeys, ",") != "code,codex_thread,effort,model,project" {
		t.Errorf("the arguments of landline_register: %v", registerKeys)
	}
	wantProject := "The name of the project you are working on, shared with the other agents on that project, for example `landline`. " +
		"Agents with the same project are colleagues: they may exchange read-only requests, and other agents and your user see the project in the agent list. " +
		"Do not put a task or a role here. If you give none, Landline uses the name of your project's repository folder."
	if got := describe("landline_register", "project"); got != wantProject {
		t.Errorf("the project argument:\n%q\nwant\n%q", got, wantProject)
	}
	if text := strings.ToLower(describe("landline_register", "")); !strings.Contains(text, "project") || strings.Contains(text, "label") {
		t.Errorf("the register description: %q", text)
	}
	send := strings.ToLower(describe("landline_send", ""))
	to := strings.ToLower(describe("landline_send", "to"))
	for name, text := range map[string]string{"the send description": send, "the to argument": to} {
		if !regexp.MustCompile(`\bid\b`).MatchString(text) || strings.Contains(text, "label") {
			t.Errorf("%s does not say id and only id: %q", name, text)
		}
	}
	status, ok := tools["landline_status"]
	if !ok {
		t.Fatal("there is no landline_status tool")
	}
	var statusKeys []string
	for key, each := range properties("landline_status") {
		statusKeys = append(statusKeys, key)
		if kind, _ := each.(map[string]any)["type"].(string); kind != "string" {
			t.Errorf("the type of %s is %q", key, kind)
		}
	}
	if strings.Join(statusKeys, ",") != "task" {
		t.Errorf("the arguments of landline_status: %v", statusKeys)
	}
	required, _ := status["inputSchema"].(map[string]any)["required"].([]any)
	if len(required) != 1 || required[0] != "task" {
		t.Errorf("landline_status must require task: %v", required)
	}
	text := describe("landline_status", "") + " " + describe("landline_status", "task")
	for _, phrase := range []string{
		"The outcome you are working toward", "in a short phrase you would say to a teammate", "menu bar app for ADR 0140",
		"Set it when you start work and change it when your goal changes, not when a step ends",
		"if it would change when one test passes, it is too low", "It is not the project, which is your `project`, and not a plan",
		"Everyone connected and your user can read it", "no secrets, no client names, and no personal data", "At most 64 bytes", "An empty task clears it",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("the status description lacks %q:\n%s", phrase, text)
		}
	}
}

func TestTheAgentListAndTheRegisterAndAttachAnswersCarryOnlyTheNewKeys(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.registerWithProject("alpha", "landline")
	status, text := w.post("/v1/register", secret, map[string]string{"id": "bravo", "harness": "claude-code", "host": "mac", "project": "landline"})
	if status != 200 || strings.Join(keysOf(decodeObject(t, text)), ",") != "id,token" {
		t.Fatalf("the register answer: %d %s", status, text)
	}
	if _, text := w.post("/v1/register", secret, map[string]string{"seed": "x", "harness": "claude-code", "host": "mac"}); strings.Join(keysOf(decodeObject(t, text)), ",") != "id,token" {
		t.Fatalf("the register answer for a seed: %s", text)
	}
	w.waitState("alpha", "disconnected")
	for _, entry := range w.agentsJSON() {
		for _, old := range []string{"name", "label", "handle"} {
			if _, ok := entry[old]; ok {
				t.Errorf("the entry keeps the old key %q: %v", old, entry)
			}
		}
		if entry["id"] == nil {
			t.Errorf("the entry has no id: %v", entry)
		}
	}
	status, text = w.post("/v1/attach", w.token("alpha"), map[string]string{"model": "m"})
	if status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	if keys := keysOf(decodeObject(t, text)); fmt.Sprint(keys) != "[attached challenge id project_peers updated]" {
		t.Fatalf("the attach answer has the keys %v", keys)
	}
}

func TestARepeatedRegisterCallInARepositoryKeepsSayingTheProjectCameFromTheRepository(t *testing.T) {
	t.Parallel()
	w := start(t)
	repo := newGitRepos(t)
	p := w.startMCP(repo.sub)
	p.handshake("claude-code", "2.1.296")
	for i := 0; i < 2; i++ {
		claimed := p.tool("landline_register", `{"model":"opus"}`)
		if !strings.Contains(claimed, "project: foo (from your repository)") {
			t.Fatalf("call %d: %q", i+1, claimed)
		}
	}
}

func TestARegisterCallDoesNotClearOrChangeTheTask(t *testing.T) {
	t.Parallel()
	w := start(t)
	p, id := w.selfRegistered("demo-folder", "claude-code")
	if got := p.tool("landline_status", `{"task":"keep this goal"}`); got != "task: keep this goal" {
		t.Fatalf("%q", got)
	}
	before := w.agentEntry(id)
	p.tool("landline_register", `{"model":"opus","effort":"high"}`)
	if status, text := w.post("/v1/attach", w.token(id), map[string]string{"project": "other"}); status != 200 {
		t.Fatalf("%d %s", status, text)
	}
	after := w.agentEntry(id)
	if after["task"] != "keep this goal" || after["task_at"] != before["task_at"] {
		t.Fatalf("an attach changed the task: before %v, after %v", before, after)
	}
}

func TestTheRegisterLineShowsTheTaskTheProcessSetAndSaysNoneAgainAfterAClear(t *testing.T) {
	t.Parallel()
	w := start(t)
	p, id := w.selfRegistered("demo-folder", "claude-code")
	if got := p.tool("landline_status", `{"task":"x y"}`); got != "task: x y" {
		t.Fatalf("%q", got)
	}
	claimed := p.tool("landline_register", `{"model":"opus"}`)
	if !strings.HasPrefix(claimed, "you are "+id+". ") || !strings.Contains(claimed, "task: x y") || strings.Contains(claimed, "task: none") {
		t.Fatalf("after a task was set: %q", claimed)
	}
	if got := p.tool("landline_status", `{"task":""}`); got != "task cleared" {
		t.Fatalf("%q", got)
	}
	claimed = p.tool("landline_register", `{"model":"opus"}`)
	if !strings.Contains(claimed, "task: none") || strings.Contains(claimed, "task: x y") {
		t.Fatalf("after a clear: %q", claimed)
	}
}
