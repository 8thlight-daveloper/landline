package agentsview

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
)

var at = time.Date(2026, 11, 12, 10, 30, 0, 0, time.UTC)

func ids(view View) []string {
	out := []string{}
	for _, row := range view.Agents {
		out = append(out, row.ID)
	}
	return out
}

func TestBuildSortsConnectedFirstThenWithAProjectThenProjectThenID(t *testing.T) {
	view := Build("http://hub", at, []hubclient.Agent{
		{ID: "zulu", Project: "a", Connected: false},
		{ID: "yankee", Project: "b", Connected: true},
		{ID: "xray", Project: "a", Connected: true},
		{ID: "whiskey", Project: "a", Connected: true},
		{ID: "victor", Connected: true},
	})
	want := "whiskey,xray,yankee,victor,zulu"
	if got := strings.Join(ids(view), ","); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if view.Agents[4].State != Disconnected || view.Agents[0].State != Connected {
		t.Fatalf("states %+v", view.Agents)
	}
}

func TestBuildDoesNotChangeItsInputOrder(t *testing.T) {
	input := []hubclient.Agent{{ID: "b"}, {ID: "a", Connected: true}}
	Build("h", at, input)
	if input[0].ID != "b" {
		t.Fatal("input reordered")
	}
}

func TestJSONShapeIsPinned(t *testing.T) {
	view := Build("http://127.0.0.1:8787", at, []hubclient.Agent{{
		ID: "alpha", Harness: "claude", Host: "mac", Connected: true, Project: "lead",
		Task: "menu bar app", TaskAt: time.Date(2026, 11, 16, 9, 15, 0, 0, time.UTC), LandlineVersion: "0.9", HarnessVersion: "2.1", Folder: "/w", Branch: "main", Worktree: "wt", Model: "m", Effort: "high",
	}, {ID: "bravo", Harness: "codex", Host: "mac"}})
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"screen":"agents","fetched_at":"2026-11-12T10:30:00Z","hub":"http://127.0.0.1:8787","agents":[` +
		`{"id":"alpha","project":"lead","task":"menu bar app","task_at":"2026-11-16T09:15:00Z","harness":"claude","state":"connected","landline_version":"0.9","harness_version":"2.1","folder":"/w","branch":"main","worktree":"wt","model":"m","effort":"high","host":"mac"},` +
		`{"id":"bravo","harness":"codex","state":"disconnected","host":"mac"}]}`
	if string(raw) != want {
		t.Fatalf("got\n%s\nwant\n%s", raw, want)
	}
}

func TestJSONForNoAgentsIsAnEmptyArray(t *testing.T) {
	raw, _ := json.Marshal(Build("h", at, nil))
	if !strings.Contains(string(raw), `"agents":[]`) {
		t.Fatalf("got %s", raw)
	}
}

func TestJSONCarriesNoSecretOrToken(t *testing.T) {
	view := Build("http://hub", at, []hubclient.Agent{{ID: "alpha", Harness: "claude", Host: "mac", Session: "session-xyz"}})
	raw, _ := json.Marshal(view)
	for _, forbidden := range []string{"secret", "token", "bearer", "session"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("%q found in %s", forbidden, raw)
		}
	}
}

func hostile() hubclient.Agent {
	return hubclient.Agent{
		ID: "al\x1bpha", Harness: "c\u200dx", Host: "h\u0085ost", Connected: true,
		Project: "\x1b]0;pwned\x07\x1b[2J", Task: "t\x1b[31mask\nx\u202ey", Folder: "x\ny\u202eevil", Branch: "b\rc",
		Model: "\u009b31m", Effort: "e\u2028f", Worktree: "w\tt", LandlineVersion: "\u00adv", HarnessVersion: "\ufeffv",
	}
}

func fields(row Row) []string {
	return []string{row.ID, row.Project, row.Task, row.Harness, row.State, row.LandlineVersion, row.HarnessVersion,
		row.Folder, row.Branch, row.Worktree, row.Model, row.Effort, row.Host}
}

func TestBuildReplacesEveryControlFormatAndSeparatorRuneWithASpace(t *testing.T) {
	row := Build("h", at, []hubclient.Agent{hostile()}).Agents[0]
	want := map[string]string{
		"id": "al pha", "project": " ]0;pwned  [2J", "task": "t [31mask x y", "harness": "c x", "host": "h ost", "folder": "x y evil",
		"branch": "b c", "model": " 31m", "effort": "e f", "worktree": "w t", "landline": " v", "harness version": " v",
	}
	got := map[string]string{
		"id": row.ID, "project": row.Project, "task": row.Task, "harness": row.Harness, "host": row.Host, "folder": row.Folder,
		"branch": row.Branch, "model": row.Model, "effort": row.Effort, "worktree": row.Worktree,
		"landline": row.LandlineVersion, "harness version": row.HarnessVersion,
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s: got %q want %q", name, got[name], w)
		}
	}
}

func TestBuildReplacesInvalidUTF8WithTheReplacementRune(t *testing.T) {
	row := Build("h", at, []hubclient.Agent{{ID: "a\x9bb", Model: "\x9b31m"}}).Agents[0]
	if strings.ContainsRune(row.ID+row.Model, 0x9b) || strings.Contains(row.ID+row.Model, "\x9b") {
		t.Fatalf("%q %q", row.ID, row.Model)
	}
}

func TestHostileValuesLeaveNoControlRuneInTheJSONOrItsDecodedValues(t *testing.T) {
	raw, err := json.Marshal(Build("h", at, []hubclient.Agent{hostile()}))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range string(raw) {
		if unicode.IsControl(r) {
			t.Fatalf("raw control %U in %s", r, raw)
		}
	}
	var decoded View
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, value := range fields(decoded.Agents[0]) {
		for _, r := range value {
			if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
				t.Fatalf("decoded value %q holds %U", value, r)
			}
		}
	}
	if strings.Contains(string(raw), `\u001b`) || strings.Contains(string(raw), `\u009b`) {
		t.Fatalf("escaped control survived: %s", raw)
	}
}

func TestPlainTextAndOtherUnicodeAreKept(t *testing.T) {
	row := Build("h", at, []hubclient.Agent{{ID: "alpha", Project: "r\u00e9sum\u00e9 \u65e5\u672c  x"}}).Agents[0]
	if row.Project != "r\u00e9sum\u00e9 \u65e5\u672c  x" {
		t.Fatalf("got %q", row.Project)
	}
}

func TestProjectOrderBeatsIDOrderAfterConnectedFirst(t *testing.T) {
	view := Build("http://hub", at, []hubclient.Agent{
		{ID: "alpha", Project: "zeta", Connected: true},
		{ID: "bravo", Project: "beta", Connected: true},
		{ID: "charlie", Project: "alpha", Connected: false},
		{ID: "delta", Project: "", Connected: false},
	})
	if got, want := strings.Join(ids(view), ","), "bravo,alpha,charlie,delta"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestBuildCleansAHubAddressPassedInDirectly(t *testing.T) {
	view := Build("http://h\x1b[2J\u0085‮\n:1", at, nil)
	for _, r := range view.Hub {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			t.Fatalf("hub %q holds %U", view.Hub, r)
		}
	}
	if view.Hub != "http://h [2J   :1" {
		t.Fatalf("got %q", view.Hub)
	}
}

func TestARowBuiltFromTheOldKeysNameAndLabelHasNoIDAndNoProject(t *testing.T) {
	var agent hubclient.Agent
	if err := json.Unmarshal([]byte(`{"name":"x","label":"y","connected":true}`), &agent); err != nil {
		t.Fatal(err)
	}
	row := Build("h", at, []hubclient.Agent{agent}).Agents[0]
	if row.ID != "" || row.Project != "" {
		t.Fatalf("an old key reached the row: %+v", row)
	}
}

func TestARowWithATaskAndNoProjectKeepsTheTaskAndOmitsTheProjectKey(t *testing.T) {
	view := Build("h", at, []hubclient.Agent{{ID: "a3f9", Task: "docs only", Connected: true}})
	raw, err := json.Marshal(view.Agents[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"task":"docs only"`) || strings.Contains(string(raw), `"project"`) {
		t.Fatalf("%s", raw)
	}
}
