package acceptance

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const menubarVersion = "0.2.0"

func TestMenubarVersionPrintsOneLineWithItsOwnNameAndNoBanner(t *testing.T) {
	cases := map[string]struct{ flags, want string }{
		"clean":                {flagsFor(menubarVersion, stampCommit, stampDate), "landline-menubar 0.2.0 (commit " + stampCommit + ", built " + stampDate + ")"},
		"dirty":                {flagsFor(menubarVersion, stampCommit+routeDirtyTag, ""), "landline-menubar 0.2.0 (commit " + stampCommit + routeDirtyTag + ")"},
		"dirty with a date":    {flagsFor(menubarVersion, stampCommit+routeDirtyTag, stampDate), "landline-menubar 0.2.0 (commit " + stampCommit + routeDirtyTag + ")"},
		"no commit":            {flagsFor(menubarVersion, "", ""), "landline-menubar 0.2.0"},
		"a date and no commit": {flagsFor(menubarVersion, "", stampDate), "landline-menubar 0.2.0"},
		"a plain go build":     {"", "landline-menubar dev"},
	}
	for name, c := range cases {
		rig := newMenubarRig(t, c.flags, "", landlineMissing)
		result := rig.run(t, "version")
		if result.code != 0 || result.stdout != c.want+"\n" || result.stderr != "" {
			t.Errorf("%s: %+v, want stdout %q", name, result, c.want+"\n")
		}
	}
}

func TestMenubarVersionNeedsNoSwitchboardNoSecretAndNoConfigFile(t *testing.T) {
	rig := newMenubarRig(t, flagsFor(menubarVersion, stampCommit, stampDate), "", landlineMissing)
	result := rig.run(t, "version")
	if result.code != 0 || !strings.HasPrefix(result.stdout, "landline-menubar "+menubarVersion) {
		t.Fatalf("%+v", result)
	}
}

func TestMenubarHelpPrintsTheUsageOnStandardOutputAndExitsZero(t *testing.T) {
	rig := newMenubarRig(t, defaultFlags(t), "", landlineMissing)
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		result := rig.run(t, args...)
		if result.code != 0 || result.stderr != "" {
			t.Errorf("%v: %+v", args, result)
		}
		for _, want := range []string{"landline-menubar", "--interval", "menu --text", "version"} {
			if !strings.Contains(result.stdout, want) {
				t.Errorf("%v: the usage lacks %q:\n%s", args, want, result.stdout)
			}
		}
	}
}

func TestMenubarRefusesAnUnknownCommandAndUnknownFlagsWithExitTwoAndTheUsageOnStandardError(t *testing.T) {
	rig := newMenubarRig(t, defaultFlags(t), deadAddress(t), landlineMissing)
	for _, args := range [][]string{{"bogus"}, {"menu", "--bogus"}, {"--bogus"}, {"agents"}} {
		result := rig.run(t, args...)
		if result.code != 2 || result.stdout != "" || !strings.Contains(result.stderr, "landline-menubar") || !strings.Contains(result.stderr, "--interval") {
			t.Errorf("%v: %+v", args, result)
		}
	}
}

func TestMenubarIntervalHasAFloorOfFiveHundredMillisecondsAndMustBeADuration(t *testing.T) {
	rig := newMenubarRig(t, defaultFlags(t), deadAddress(t), landlineMissing)
	for _, args := range [][]string{
		{"--interval", "100ms", "menu", "--text"},
		{"menu", "--text", "--interval", "100ms"},
		{"menu", "--text", "--interval", "499ms"},
		{"menu", "--text", "--interval", "0s"},
		{"menu", "--text", "--interval", "-5s"},
	} {
		result := rig.run(t, args...)
		if result.code != 2 || result.stdout != "" || !strings.Contains(result.stderr, "interval") {
			t.Errorf("%v: %+v", args, result)
		}
	}
	for _, args := range [][]string{{"--interval", "soon", "menu", "--text"}, {"menu", "--text", "--interval", "soon"}, {"menu", "--text", "--interval"}} {
		result := rig.run(t, args...)
		if result.code != 2 || result.stdout != "" || !strings.Contains(result.stderr, "interval") {
			t.Errorf("%v: %+v", args, result)
		}
	}
	plain := rig.run(t, "menu", "--text")
	for _, args := range [][]string{
		{"--interval", "500ms", "menu", "--text"},
		{"menu", "--text", "--interval", "500ms"},
		{"menu", "--text", "--interval", "7s"},
	} {
		result := rig.run(t, args...)
		if result.code != 0 || result.stdout != plain.stdout || result.stderr != "" {
			t.Errorf("%v: %+v, want the output of menu --text", args, result)
		}
	}
}

func TestMenuTextWithNoSwitchboardListeningIsAStateAndNotAnError(t *testing.T) {
	address := deadAddress(t)
	rig := newMenubarRig(t, defaultFlags(t), address, landlineNextToBinary)
	result := rig.run(t, "menu", "--text")
	want := expectedMenu("jack dimmed", unreachableHead(), localSwitchboard(address, false, "Switchboard (build unknown)"), menubarBuildLine(releaseVersion(t), ""))
	if result.code != 0 || result.stderr != "" || result.stdout != want {
		t.Fatalf("%+v\nwant:\n%s", result, want)
	}
}

func TestMenuTextAgainstARealSwitchboardWithNoAgentsShowsTheMarkWithNoCountAndTheBuildsOfBothSides(t *testing.T) {
	flags := flagsFor(menubarVersion, stampCommit, stampDate)
	w := startHub(t, buildStamped(t, menubarVersion, stampCommit, stampDate), t.TempDir(), "")
	rig := newMenubarRig(t, flags, w.url, landlineNextToBinary)
	want := expectedMenu("jack", nil, localSwitchboard(w.url, true, switchboardBuildLine(menubarVersion, stampCommit)), menubarBuildLine(menubarVersion, stampCommit))
	if got := rig.menu(t); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMenuTextShowsTheVersionOnlyBuildsOfAReleaseAndDevForAnUnstampedSwitchboardAndApp(t *testing.T) {
	released := startHub(t, build(t), t.TempDir(), "")
	rig := newMenubarRig(t, defaultFlags(t), released.url, landlineNextToBinary)
	want := expectedMenu("jack", nil, localSwitchboard(released.url, true, switchboardBuildLine(releaseVersion(t), "")), menubarBuildLine(releaseVersion(t), ""))
	if got := rig.menu(t); got != want {
		t.Errorf("a release with no commit:\n%s\nwant:\n%s", got, want)
	}
	plain := startHub(t, buildAllWith(t, ""), t.TempDir(), "")
	rig = newMenubarRig(t, "", plain.url, landlineNextToBinary)
	want = expectedMenu("jack", nil, localSwitchboard(plain.url, true, "Switchboard dev"), "Menubar dev")
	if got := rig.menu(t); got != want {
		t.Errorf("an unstamped build:\n%s\nwant:\n%s", got, want)
	}
}

func TestMenuTextSaysTheSwitchboardDiffersFromTheAppWhenTheCommitsDifferAndOnlyThen(t *testing.T) {
	otherCommit := strings.Repeat("f", 40)
	w := startHub(t, buildStamped(t, menubarVersion, stampCommit, stampDate), t.TempDir(), "")
	rig := newMenubarRig(t, flagsFor(menubarVersion, otherCommit, stampDate), w.url, landlineNextToBinary)
	want := expectedMenu("jack", nil, localSwitchboard(w.url, true, switchboardBuildLine(menubarVersion, stampCommit)+" (differs from this app)"), menubarBuildLine(menubarVersion, otherCommit))
	if got := rig.menu(t); got != want {
		t.Errorf("different commits:\n%s\nwant:\n%s", got, want)
	}
	same := newMenubarRig(t, flagsFor(menubarVersion, stampCommit, stampDate), w.url, landlineNextToBinary)
	if got := same.menu(t); strings.Contains(got, "differs") {
		t.Errorf("the same commit differs:\n%s", got)
	}
}

func TestMenuTextAgainstAFakeSwitchboardShowsDirtyCommitsDevAnd404AsUnknown(t *testing.T) {
	flags := flagsFor(menubarVersion, stampCommit, stampDate)
	cases := map[string]struct {
		status int
		body   any
		want   string
	}{
		"a dirty switchboard":       {200, fakeBuild(menubarVersion, stampCommit+routeDirtyTag, ""), "Switchboard 0.2.0 (a1b2c3d-dirty) (differs from this app)"},
		"the same commit":           {200, fakeBuild(menubarVersion, stampCommit, stampDate), "Switchboard 0.2.0 (a1b2c3d)"},
		"another version":           {200, fakeBuild("0.3.0", stampCommit, stampDate), "Switchboard 0.3.0 (a1b2c3d)"},
		"a switchboard from before": {404, map[string]string{"error": "not found"}, "Switchboard (build unknown)"},
	}
	for name, c := range cases {
		fake := newFakeSwitchboard(t, "127.0.0.1", []any{}, c.status, c.body)
		rig := newMenubarRig(t, flags, fake.url, landlineNextToBinary)
		got := rig.menu(t)
		if !strings.Contains(got, "\n  "+c.want+disabledMark+"\n") {
			t.Errorf("%s: the Switchboard submenu lacks %q:\n%s", name, c.want, got)
		}
		if !strings.HasPrefix(got, "icon: jack\n") || !strings.Contains(got, "\n  Running at "+fake.url+disabledMark+"\n") {
			t.Errorf("%s: an unknown or odd build must still show a reachable switchboard:\n%s", name, got)
		}
	}
}

func TestMenuTextTreatsAnAnswerOfNoBuildAsUnknownAndKeepsTheRestOfTheMenuExactly(t *testing.T) {
	fake := newFakeSwitchboard(t, "127.0.0.1", []any{}, http.StatusNotFound, map[string]string{"error": "not found"})
	rig := newMenubarRig(t, flagsFor(menubarVersion, stampCommit, stampDate), fake.url, landlineNextToBinary)
	want := expectedMenu("jack", nil, localSwitchboard(fake.url, true, "Switchboard (build unknown)"), menubarBuildLine(menubarVersion, stampCommit))
	if got := rig.menu(t); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMenuTextSanitizesBuildTextFromAHostileSwitchboardAndCutsTheCommitAfterCleaning(t *testing.T) {
	version := "0.2.0\x1b[2J\nFAKE"
	commit := "a1\u2028b\x1b3d4e5f60718293a4b5c6d7e8f9012345678-dirty"
	fake := newFakeSwitchboard(t, "127.0.0.1", []any{}, 200, fakeBuild(version, commit, "2026\n-11\x1b[2J"))
	rig := newMenubarRig(t, flagsFor(menubarVersion, stampCommit, stampDate), fake.url, landlineNextToBinary)
	got := rig.menu(t)
	short := string([]rune(cleaned(commit))[:7]) + routeDirtyTag
	if short != "a1 b 3d-dirty" {
		t.Fatalf("the fixture is wrong: %q", short)
	}
	want := expectedMenu("jack", nil, localSwitchboard(fake.url, true, "Switchboard "+cleaned(version)+" ("+short+") (differs from this app)"), menubarBuildLine(menubarVersion, stampCommit))
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	if strings.ContainsAny(got, "\x1b\r\t\a") {
		t.Errorf("a control character reached the menu: %q", got)
	}
}

func fixedAgents() []map[string]any {
	return []map[string]any{
		fakeAgent("tango", true, map[string]any{"harness": "codex", "host": "mac-2"}),
		fakeAgent("bravo", true, map[string]any{
			"project": "zed-review", "model": "opus", "effort": "high", "folder": "/work/secret-repo", "branch": "feature/private",
			"landline_version": "0.2.0", "landline_commit": stampCommit,
		}),
		fakeAgent("alpha", true, map[string]any{"project": "amy-docs", "task": "menu bar app for ADR 0140", "task_at": "2026-11-16T09:15:30Z", "folder": "/work/docs", "branch": "main"}),
		fakeAgent("echo", false, map[string]any{"project": "yankee"}),
		fakeAgent("delta", false, nil),
		fakeAgent("charlie", true, nil),
	}
}

func TestMenuTextListsAgentRowsInTheScreensOrderWithTheirFieldsAndNoFolderOrBranchOnTheRow(t *testing.T) {
	fake := newFakeSwitchboard(t, "127.0.0.1", fixedAgents(), 200, fakeBuild(menubarVersion, stampCommit, stampDate))
	rig := newMenubarRig(t, flagsFor(menubarVersion, stampCommit, stampDate), fake.url, landlineNextToBinary)
	var head []string
	head = append(head, agentRow("amy-docs alpha", liveMarker, "ID: alpha", "Project: amy-docs", "Task: menu bar app for ADR 0140", "Harness: claude-code", "Host: mac", "Folder: /work/docs", "Branch: main")...)
	head = append(head, agentRow("zed-review bravo", liveMarker, "ID: bravo", "Project: zed-review", "Harness: claude-code", "Host: mac", "Model: opus", "Effort: high", "Folder: /work/secret-repo", "Branch: feature/private", "Landline version: 0.2.0")...)
	head = append(head, agentRow("charlie", liveMarker, "ID: charlie", "Harness: claude-code", "Host: mac")...)
	head = append(head, agentRow("tango", liveMarker, "ID: tango", "Harness: codex", "Host: mac-2")...)
	head = append(head, agentRow("yankee echo", "disconnected", "ID: echo", "Project: yankee", "Harness: claude-code", "Host: mac")...)
	head = append(head, agentRow("delta", "disconnected", "ID: delta", "Harness: claude-code", "Host: mac")...)
	want := expectedMenu("jack 4", head, localSwitchboard(fake.url, true, switchboardBuildLine(menubarVersion, stampCommit)), menubarBuildLine(menubarVersion, stampCommit))
	got := rig.menu(t)
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	for _, line := range menuLines(got) {
		if !strings.HasPrefix(line, " ") && (strings.Contains(line, "secret-repo") || strings.Contains(line, "feature/private") || strings.Contains(line, "/work")) {
			t.Errorf("a folder or branch is on a top level line: %q", line)
		}
	}
}

func TestMenuTextCountsOnlyConnectedAgentsAndShowsNoCountWhenNoneIsConnected(t *testing.T) {
	flags := defaultFlags(t)
	none := newFakeSwitchboard(t, "127.0.0.1", []map[string]any{fakeAgent("echo", false, nil), fakeAgent("delta", false, nil)}, 200, fakeBuild(menubarVersion, "", ""))
	if got := newMenubarRig(t, flags, none.url, landlineNextToBinary).menu(t); !strings.HasPrefix(got, "icon: jack\n") {
		t.Errorf("no agent is connected but the icon is %q", menuLines(got)[0])
	}
	one := newFakeSwitchboard(t, "127.0.0.1", []map[string]any{fakeAgent("echo", false, nil), fakeAgent("delta", true, nil)}, 200, fakeBuild(menubarVersion, "", ""))
	if got := newMenubarRig(t, flags, one.url, landlineNextToBinary).menu(t); !strings.HasPrefix(got, "icon: jack 1\n") {
		t.Errorf("one agent is connected but the icon is %q", menuLines(got)[0])
	}
	many := make([]map[string]any, 0, 12)
	for i := 0; i < 12; i++ {
		many = append(many, fakeAgent("agent-"+string(rune('a'+i)), true, nil))
	}
	big := newFakeSwitchboard(t, "127.0.0.1", many, 200, fakeBuild(menubarVersion, "", ""))
	if got := newMenubarRig(t, flags, big.url, landlineNextToBinary).menu(t); !strings.HasPrefix(got, "icon: jack 12\n") {
		t.Errorf("twelve agents are connected but the icon is %q", menuLines(got)[0])
	}
}

func TestMenuTextSanitizesEveryFieldOfAHostileAgentTheWayTheScreenDoes(t *testing.T) {
	hostile := func(name string) string { return name + "\x1b[31m\nX\r\t\u2028\u2029\u202e\u200b\x07\x00end" }
	fields := map[string]any{
		"harness": hostile("harness"), "host": hostile("host"), "model": hostile("model"), "effort": hostile("effort"),
		"folder": hostile("folder"), "branch": hostile("branch"), "project": hostile("project"), "task": hostile("task"), "landline_version": hostile("version"),
	}
	agent := fakeAgent(hostile("id"), true, fields)
	fake := newFakeSwitchboard(t, "127.0.0.1", []map[string]any{agent}, 200, fakeBuild(menubarVersion, "", ""))
	rig := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary)
	row := agentRow(cleaned(hostile("project"))+" "+cleaned(hostile("id")), liveMarker,
		"ID: "+cleaned(hostile("id")), "Project: "+cleaned(hostile("project")), "Task: "+cleaned(hostile("task")), "Harness: "+cleaned(hostile("harness")), "Host: "+cleaned(hostile("host")),
		"Model: "+cleaned(hostile("model")), "Effort: "+cleaned(hostile("effort")), "Folder: "+cleaned(hostile("folder")),
		"Branch: "+cleaned(hostile("branch")), "Landline version: "+cleaned(hostile("version")))
	want := expectedMenu("jack 1", row, localSwitchboard(fake.url, true, switchboardBuildLine(menubarVersion, "")), menubarBuildLine(releaseVersion(t), ""))
	got := rig.menu(t)
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	if strings.ContainsAny(got, "\x1b\r\t\a\x00\u2028\u2029\u202e\u200b") || len(menuLines(got)) != len(menuLines(want)) {
		t.Errorf("a control character or an extra line reached the menu: %q", got)
	}
}

func TestMenuTextKeepsEveryAgentOnItsOwnLinesWhateverTheLengthOfItsValues(t *testing.T) {
	long := strings.Repeat("x", 5000)
	agents := []map[string]any{fakeAgent("alpha", true, map[string]any{"project": long, "task": long, "folder": "/" + long, "model": long, "branch": long})}
	fake := newFakeSwitchboard(t, "127.0.0.1", agents, 200, fakeBuild(menubarVersion, "", ""))
	rig := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary)
	got := rig.menu(t)
	shape := agentRow("x", liveMarker, "ID: alpha", "Project: x", "Task: x", "Harness: claude-code", "Host: mac", "Model: x", "Folder: x", "Branch: x")
	want := expectedMenu("jack 1", shape, localSwitchboard(fake.url, true, switchboardBuildLine(menubarVersion, "")), menubarBuildLine(releaseVersion(t), ""))
	if len(menuLines(got)) != len(menuLines(want)) {
		t.Fatalf("a long value changed the number of lines: %d, want %d", len(menuLines(got)), len(menuLines(want)))
	}
	if !strings.Contains(got, "ID: alpha"+disabledMark+"\n") {
		t.Errorf("the id line is missing:\n%.400s", got)
	}
}

func TestMenuTextRegistersAnObserverOfKindMenubarAndPollsWithItsTokenAndNeverTheSecret(t *testing.T) {
	fake := newFakeSwitchboard(t, "127.0.0.1", fixedAgents(), 200, fakeBuild(menubarVersion, stampCommit, stampDate))
	rig := newMenubarRig(t, flagsFor(menubarVersion, stampCommit, stampDate), fake.url, landlineNextToBinary)
	result := rig.run(t, "menu", "--text")
	if result.code != 0 {
		t.Fatalf("%+v", result)
	}
	for _, leaked := range []string{secret, "observer-token-1"} {
		if strings.Contains(result.stdout, leaked) || strings.Contains(result.stderr, leaked) {
			t.Errorf("the output shows %q", leaked)
		}
	}
	assertNoCredentialsOnDisk(t, rig)
	if fake.count("POST", "/v1/observers") != 1 || fake.count("GET", "/v1/agents") != 1 {
		t.Errorf("menu --text must register once and poll once: %+v", fake.seen())
	}
	for _, each := range fake.seen() {
		switch {
		case each.method == "POST" && each.path == "/v1/observers":
			var body map[string]any
			if err := json.Unmarshal([]byte(each.body), &body); err != nil || len(body) != 1 || body["kind"] != "menubar" {
				t.Errorf("the observer body is %q, want exactly {\"kind\":\"menubar\"}", each.body)
			}
			if each.authorization != "Bearer "+secret {
				t.Errorf("registration is authorised by %q, want the secret", each.authorization)
			}
		case each.path == "/v1/agents" || each.path == "/v1/build":
			if each.authorization != "Bearer observer-token-1" {
				t.Errorf("%s %s carries %q, want the observer token and never the secret", each.method, each.path, each.authorization)
			}
		default:
			t.Errorf("the menu bar app called %s %s", each.method, each.path)
		}
	}
}

func TestMenuTextRegistersAgainOnA401AndShowsTheListFromTheNewToken(t *testing.T) {
	fake := newFakeSwitchboard(t, "127.0.0.1", []map[string]any{fakeAgent("alpha", true, nil)}, 200, fakeBuild(menubarVersion, "", ""))
	fake.staleFirst = true
	rig := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary)
	got := rig.menu(t)
	if !strings.HasPrefix(got, "icon: jack 1\n") || !strings.Contains(got, "\n\u25cf alpha\n") {
		t.Fatalf("after a 401 the menu is:\n%s", got)
	}
	if fake.count("POST", "/v1/observers") < 2 {
		t.Errorf("the app must register again after a 401: %+v", fake.seen())
	}
	listedWithFreshToken := false
	for _, each := range fake.seen() {
		if each.path == "/v1/observers" {
			continue
		}
		if each.authorization == "Bearer "+secret {
			t.Errorf("%s %s carries the secret", each.method, each.path)
		}
		if each.path == "/v1/agents" && each.authorization != "Bearer observer-token-1" && strings.HasPrefix(each.authorization, "Bearer observer-token-") {
			listedWithFreshToken = true
		}
	}
	if !listedWithFreshToken {
		t.Errorf("the agent list was never read with a token other than the stale one: %+v", fake.seen())
	}
}

func TestMenuTextReadsTheConfigFileWhenTheEnvironmentHasNoAddressAndNoSecretAndTheEnvironmentWinsOtherwise(t *testing.T) {
	w := start(t)
	w.register("alpha")
	flags := defaultFlags(t)
	writeConfig := func(rig menubarRig, url, fileSecret string) {
		dir := filepath.Join(rig.home, ".config", "landline")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]string{"url": url, "secret": fileSecret})
		if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fromFile := newMenubarRig(t, flags, "", landlineNextToBinary)
	writeConfig(fromFile, w.url, secret)
	if got := fromFile.menu(t); !strings.HasPrefix(got, "icon: jack\n") || !strings.Contains(got, "\n\u25cb alpha\n") || !strings.Contains(got, "Running at "+w.url) {
		t.Errorf("the config file was not used:\n%s", got)
	}
	envWins := newMenubarRig(t, flags, w.url, landlineNextToBinary)
	writeConfig(envWins, deadAddress(t), "a-different-secret-0123456789")
	if got := envWins.menu(t); !strings.Contains(got, "Running at "+w.url) {
		t.Errorf("the environment did not win over the file:\n%s", got)
	}
	deadEnv := newMenubarRig(t, flags, deadAddress(t), landlineNextToBinary)
	writeConfig(deadEnv, w.url, secret)
	if got := deadEnv.menu(t); !strings.HasPrefix(got, "icon: jack dimmed\n") {
		t.Errorf("an unreachable address in the environment fell back to the file:\n%s", got)
	}
	secretFromFile := newMenubarRig(t, flags, w.url, landlineNextToBinary)
	secretFromFile.env = withEnv(secretFromFile.env, "LANDLINE_SECRET=")
	writeConfig(secretFromFile, "", secret)
	if got := secretFromFile.menu(t); !strings.HasPrefix(got, "icon: jack\n") {
		t.Errorf("the secret was not read from the config file when the environment has none:\n%s", got)
	}
}

func unreachableMenu(t *testing.T, url string) string {
	return expectedMenu("jack dimmed", unreachableHead(), localSwitchboard(url, false, "Switchboard (build unknown)"), menubarBuildLine(releaseVersion(t), ""))
}

func TestMenuTextTreatsEveryFailureToGetTheAgentListAsTheUnreachableState(t *testing.T) {
	type failure struct {
		prepare func(f *fakeSwitchboard)
		secret  string
	}
	cases := map[string]failure{
		"a 401 even after registering again": {func(f *fakeSwitchboard) { f.rejectAll = true }, secret},
		"a 500 on the list":                  {func(f *fakeSwitchboard) { f.agentsCode = 500 }, secret},
		"a 503 on the list":                  {func(f *fakeSwitchboard) { f.agentsCode = 503 }, secret},
		"malformed JSON":                     {func(f *fakeSwitchboard) { f.agentsRaw = "{not json" }, secret},
		"JSON of the wrong shape":            {func(f *fakeSwitchboard) { f.agentsRaw = `{"agents": 7}` }, secret},
		"an empty body":                      {func(f *fakeSwitchboard) { f.agentsRaw = " " }, secret},
		"a wrong secret":                     {func(f *fakeSwitchboard) {}, "wrong-secret-0123456789"},
	}
	for name, c := range cases {
		fake := newFakeSwitchboard(t, "127.0.0.1", []map[string]any{fakeAgent("alpha", true, nil)}, 200, fakeBuild(menubarVersion, "", ""))
		c.prepare(fake)
		rig := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary)
		rig.env = withEnv(rig.env, "LANDLINE_SECRET="+c.secret)
		result := rig.run(t, "menu", "--text")
		if result.code != 0 || result.stderr != "" || result.stdout != unreachableMenu(t, fake.url) {
			t.Errorf("%s: %+v\nwant:\n%s", name, result, unreachableMenu(t, fake.url))
		}
		for _, leaked := range []string{secret, c.secret, "observer-token-"} {
			if strings.Contains(result.stdout+result.stderr, leaked) {
				t.Errorf("%s: the output shows %q", name, leaked)
			}
		}
		assertNoCredentialsOnDisk(t, rig)
	}
}

func TestMenuTextWithNoSecretAtAllIsTheUnreachableState(t *testing.T) {
	fake := newFakeSwitchboard(t, "127.0.0.1", []map[string]any{fakeAgent("alpha", true, nil)}, 200, fakeBuild(menubarVersion, "", ""))
	rig := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary)
	rig.env = withEnv(rig.env, "LANDLINE_SECRET=")
	result := rig.run(t, "menu", "--text")
	if result.code != 0 || result.stderr != "" || result.stdout != unreachableMenu(t, fake.url) {
		t.Errorf("%+v\nwant:\n%s", result, unreachableMenu(t, fake.url))
	}
}

func TestMenuTextTreatsAFailureOfTheBuildRouteAloneAsAnUnknownBuildAndChangesNothingElse(t *testing.T) {
	agents := []map[string]any{fakeAgent("alpha", true, nil), fakeAgent("echo", false, nil)}
	cases := map[string]struct {
		status int
		body   any
		raw    string
	}{
		"404":                   {404, map[string]string{"error": "not found"}, ""},
		"500":                   {500, map[string]string{"error": "boom"}, ""},
		"503":                   {503, map[string]string{"error": "later"}, ""},
		"malformed JSON":        {200, nil, "{not json"},
		"a body that is a list": {200, nil, "[1, 2]"},
		"an empty body":         {200, nil, " "},
	}
	var head []string
	head = append(head, agentRow("alpha", liveMarker, "ID: alpha", "Harness: claude-code", "Host: mac")...)
	head = append(head, agentRow("echo", "disconnected", "ID: echo", "Harness: claude-code", "Host: mac")...)
	for name, c := range cases {
		fake := newFakeSwitchboard(t, "127.0.0.1", agents, c.status, c.body)
		fake.buildRaw = c.raw
		rig := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary)
		want := expectedMenu("jack 1", head, localSwitchboard(fake.url, true, "Switchboard (build unknown)"), menubarBuildLine(releaseVersion(t), ""))
		if got := rig.menu(t); got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got, want)
		}
	}
}

func TestMenuTextShowsDiffersOnlyWhenBothCommitsAreNonEmptyAndDiffer(t *testing.T) {
	other := strings.Repeat("e", 40)
	cases := map[string]struct {
		appFlags string
		build    map[string]string
		line     string
	}{
		"both set and different":         {flagsFor(menubarVersion, stampCommit, stampDate), fakeBuild(menubarVersion, other, ""), "Switchboard 0.2.0 (eeeeeee) (differs from this app)"},
		"both set and equal":             {flagsFor(menubarVersion, stampCommit, stampDate), fakeBuild(menubarVersion, stampCommit, ""), "Switchboard 0.2.0 (a1b2c3d)"},
		"a switchboard with no commit":   {flagsFor(menubarVersion, stampCommit, stampDate), fakeBuild(menubarVersion, "", ""), "Switchboard 0.2.0"},
		"a dev switchboard, stamped app": {flagsFor(menubarVersion, stampCommit, stampDate), fakeBuild("dev", "", ""), "Switchboard dev"},
		"an app with no commit":          {flagsFor(menubarVersion, "", ""), fakeBuild(menubarVersion, other, ""), "Switchboard 0.2.0 (eeeeeee)"},
		"a dev app":                      {"", fakeBuild(menubarVersion, other, ""), "Switchboard 0.2.0 (eeeeeee)"},
		"neither has a commit":           {flagsFor(menubarVersion, "", ""), fakeBuild(menubarVersion, "", ""), "Switchboard 0.2.0"},
		"dev on both sides":              {"", fakeBuild("dev", "", ""), "Switchboard dev"},
	}
	for name, c := range cases {
		fake := newFakeSwitchboard(t, "127.0.0.1", []any{}, 200, c.build)
		got := newMenubarRig(t, c.appFlags, fake.url, landlineNextToBinary).menu(t)
		if !strings.Contains(got, "\n  "+c.line+disabledMark+"\n") {
			t.Errorf("%s: the Switchboard submenu lacks %q:\n%s", name, c.line, got)
		}
	}
}

func TestMenuTextAgainstARealSwitchboardListsConnectedFirstThenProjectThenIDWithMarkersAndTheCount(t *testing.T) {
	menubarBinary(t, defaultFlags(t))
	w := start(t)
	w.registerWithProject("delta", "yankee")
	w.registerWithProject("foxtrot", "bravo-l")
	w.register("tango", "alpha")
	if status, body := w.post("/v1/register", secret, map[string]string{
		"id": "kilo", "harness": "codex", "host": "mac", "project": "mike", "model": "gpt-x", "effort": "low", "folder": "/srv/kilo-work", "branch": "release",
	}); status != http.StatusOK {
		t.Fatalf("register kilo: %d %s", status, body)
	}
	projectLate := w.selfRegisteredWithProject("a-x", "zed")
	projectEarly := w.selfRegisteredWithProject("b-x", "amy")
	plainLate := w.selfRegisteredWithProject("d-x", "")
	plainEarly := w.selfRegisteredWithProject("c-x", "")
	if status, body := w.post("/v1/status", w.token(projectLate), map[string]string{"task": "menu bar app for ADR 0140"}); status != http.StatusOK {
		t.Fatalf("set the task: %d %s", status, body)
	}
	connected := map[string]bool{projectLate: true, projectEarly: true, plainLate: true, plainEarly: true}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(w.agentsBody(w.registerObserver("tui"))), &listed); err != nil {
		t.Fatal(err)
	}
	var agents []listedAgent
	for _, entry := range listed {
		state := "disconnected"
		if entry["connected"] == true {
			state = "connected"
		}
		project, _ := entry["project"].(string)
		agents = append(agents, listedAgent{id: entry["id"].(string), state: state, project: project})
	}
	if len(agents) != 9 {
		t.Fatalf("the fixture has %d agents", len(agents))
	}
	rig := newMenubarRig(t, defaultFlags(t), w.url, landlineNextToBinary)
	lines := menuLines(rig.menu(t))
	if lines[0] != "icon: jack 4" {
		t.Errorf("the icon line is %q, want 4 connected agents", lines[0])
	}
	var rows []string
	for _, line := range lines[1:] {
		if line == menuSeparator {
			break
		}
		if !strings.HasPrefix(line, " ") {
			rows = append(rows, line)
		}
	}
	order := expectedListOrder(agents)
	if len(rows) != len(order) {
		t.Fatalf("%d rows, want %d:\n%s", len(rows), len(order), strings.Join(lines, "\n"))
	}
	for i, agent := range order {
		marker := "disconnected"
		if connected[agent.id] {
			marker = liveMarker
		}
		if want := shapeFor(marker) + " " + titleOf(agent.id, agents); rows[i] != want {
			t.Errorf("row %d is %q, want %q", i, rows[i], want)
		}
	}
	block := rowBlock(lines, "\u25cb mike kilo")
	want := agentRow("mike kilo", "disconnected", "ID: kilo", "Project: mike", "Harness: codex", "Host: mac", "Model: gpt-x", "Effort: low", "Folder: /srv/kilo-work", "Branch: release")
	if strings.Join(block, "\n") != strings.Join(want, "\n") {
		t.Errorf("the submenu of kilo is:\n%s\nwant:\n%s", strings.Join(block, "\n"), strings.Join(want, "\n"))
	}
	late := rowBlock(lines, "\u25cf zed "+projectLate)
	if len(late) < 5 || late[1] != "  State: connected"+disabledMark || late[2] != "  ID: "+projectLate+disabledMark ||
		late[3] != "  Project: zed"+disabledMark || late[4] != "  Task: menu bar app for ADR 0140"+disabledMark || late[len(late)-1] != "  Copy id" {
		t.Errorf("the submenu with a project and a task is:\n%s", strings.Join(late, "\n"))
	}
	early := strings.Join(rowBlock(lines, "\u25cf amy "+projectEarly), "\n")
	if strings.Contains(early, "Task:") || !strings.Contains(early, "  Project: amy"+disabledMark+"\n") {
		t.Errorf("the submenu of an agent with a project and no task is:\n%s", early)
	}
	for _, id := range []string{plainLate, plainEarly} {
		submenu := strings.Join(rowBlock(lines, "\u25cf "+id), "\n")
		if !strings.Contains(submenu, "  ID: "+id+disabledMark+"\n") || strings.Contains(submenu, "Project:") || strings.Contains(submenu, "Task:") || !strings.HasSuffix(submenu, "  Copy id") {
			t.Errorf("the submenu of %s, which has no project, is:\n%s", id, submenu)
		}
	}
	if strings.Contains(strings.Join(lines, "\n"), "Handle") || strings.Contains(strings.Join(lines, "\n"), "Label") {
		t.Errorf("an old word is in the menu:\n%s", strings.Join(lines, "\n"))
	}
}

func TestMenuTextShowsAClearedTaskAsNoTaskLineAndAChangedTaskAsTheNewText(t *testing.T) {
	menubarBinary(t, defaultFlags(t))
	w := start(t)
	id := w.selfRegisteredWithProject("task-x", "demo")
	rig := newMenubarRig(t, defaultFlags(t), w.url, landlineNextToBinary)
	for task, want := range map[string]string{"first goal": "  Task: first goal" + disabledMark + "\n", "second goal": "  Task: second goal" + disabledMark + "\n"} {
		if status, body := w.post("/v1/status", w.token(id), map[string]string{"task": task}); status != http.StatusOK {
			t.Fatalf("%d %s", status, body)
		}
		if got := rig.menu(t); !strings.Contains(got, want) {
			t.Errorf("the menu lacks %q:\n%s", want, got)
		}
	}
	if status, body := w.post("/v1/status", w.token(id), map[string]string{"task": ""}); status != http.StatusOK {
		t.Fatalf("%d %s", status, body)
	}
	if got := rig.menu(t); strings.Contains(got, "Task:") {
		t.Errorf("a cleared task still shows:\n%s", got)
	}
}

func titleOf(id string, agents []listedAgent) string {
	for _, agent := range agents {
		if agent.id == id && agent.project != "" {
			return agent.project + " " + id
		}
	}
	return id
}

func rowBlock(lines []string, title string) []string {
	for i, line := range lines {
		if line != title {
			continue
		}
		block := []string{line}
		for _, next := range lines[i+1:] {
			if !strings.HasPrefix(next, " ") {
				break
			}
			block = append(block, next)
		}
		return block
	}
	return nil
}

func TestMenuTextTwiceAgainstTheSameStatePrintsIdenticalBytes(t *testing.T) {
	menubarBinary(t, defaultFlags(t))
	w := start(t)
	w.registerWithProject("delta", "yankee")
	w.selfRegisteredWithProject("a-x", "zed")
	w.selfRegisteredWithProject("b-x", "")
	rig := newMenubarRig(t, defaultFlags(t), w.url, landlineNextToBinary)
	first := rig.run(t, "menu", "--text")
	second := rig.run(t, "menu", "--text")
	if first.code != 0 || second.code != 0 || first.stdout != second.stdout || first.stdout == "" {
		t.Fatalf("the two runs differ:\n%q\n%q", first.stdout, second.stdout)
	}
	dead := newMenubarRig(t, defaultFlags(t), deadAddress(t), landlineNextToBinary)
	if a, b := dead.run(t, "menu", "--text"), dead.run(t, "menu", "--text"); a.stdout != b.stdout || a.stdout == "" {
		t.Errorf("the unreachable menu differs between runs:\n%q\n%q", a.stdout, b.stdout)
	}
}

func TestMenuTextEnablesStartAloneWhenUnreachableAndStopAndRestartWhenReachableWhereverLandlineIsFound(t *testing.T) {
	w := start(t)
	flags := defaultFlags(t)
	dead := deadAddress(t)
	for name, where := range map[string]landlineAt{"next to the binary": landlineNextToBinary, "on the path": landlineOnPath} {
		reachable := newMenubarRig(t, flags, w.url, where).menu(t)
		want := expectedMenu("jack", nil, localSwitchboard(w.url, true, switchboardBuildLine(releaseVersion(t), "")), menubarBuildLine(releaseVersion(t), ""))
		if reachable != want {
			t.Errorf("%s, reachable:\n%s\nwant:\n%s", name, reachable, want)
		}
		unreachable := newMenubarRig(t, flags, dead, where).menu(t)
		want = expectedMenu("jack dimmed", unreachableHead(), localSwitchboard(dead, false, "Switchboard (build unknown)"), menubarBuildLine(releaseVersion(t), ""))
		if unreachable != want {
			t.Errorf("%s, unreachable:\n%s\nwant:\n%s", name, unreachable, want)
		}
	}
}

func TestMenuTextDisablesStartStopAndRestartAndSaysToRunJustInstallWhenLandlineIsNeitherNextToTheBinaryNorOnThePath(t *testing.T) {
	w := start(t)
	flags := defaultFlags(t)
	dead := deadAddress(t)
	missing := switchboardItems{first: notFoundLine(), build: switchboardBuildLine(releaseVersion(t), "")}
	want := expectedMenu("jack", nil, missing, menubarBuildLine(releaseVersion(t), ""))
	if got := newMenubarRig(t, flags, w.url, landlineMissing).menu(t); got != want {
		t.Errorf("reachable:\n%s\nwant:\n%s", got, want)
	}
	missing.build = "Switchboard (build unknown)"
	want = expectedMenu("jack dimmed", unreachableHead(), missing, menubarBuildLine(releaseVersion(t), ""))
	if got := newMenubarRig(t, flags, dead, landlineMissing).menu(t); got != want {
		t.Errorf("unreachable:\n%s\nwant:\n%s", got, want)
	}
}

func remoteItemsFor(address string) switchboardItems {
	return switchboardItems{first: "Remote switchboard at " + address, build: "Switchboard (build unknown)"}
}

func TestMenuTextTreatsAnAddressThatIsNotLoopbackAsARemoteSwitchboardWithAllThreeServerItemsDisabled(t *testing.T) {
	flags := defaultFlags(t)
	var closed []string
	if ip := nonLoopbackAddress(); ip != "" {
		closed = append(closed, "http://"+net.JoinHostPort(ip, freePort(t)))
	}
	literal := "http://[2001:db8::10]:8787"
	for _, remote := range closed {
		want := expectedMenu("jack dimmed", unreachableHead(), remoteItemsFor(remote), menubarBuildLine(releaseVersion(t), ""))
		for _, where := range []landlineAt{landlineNextToBinary, landlineOnPath, landlineMissing} {
			if got := newMenubarRig(t, flags, remote, where).menu(t); got != want {
				t.Errorf("%s, landline at %d:\n%s\nwant:\n%s", remote, where, got, want)
			}
		}
	}
	want := expectedMenu("jack dimmed", unreachableHead(), remoteItemsFor(literal), menubarBuildLine(releaseVersion(t), ""))
	if got := newMenubarRig(t, flags, literal, landlineNextToBinary).menu(t); got != want {
		t.Errorf("%s:\n%s\nwant:\n%s", literal, got, want)
	}
	if len(closed) == 0 {
		t.Log("this machine has no non-loopback IPv4 address, so only the documentation-range literal ran")
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	_, port, _ := net.SplitHostPort(freeAddr(t))
	return port
}

func TestMenuTextTreatsLocalhostAndIPv6LoopbackAndATrailingSlashAsLocal(t *testing.T) {
	flags := defaultFlags(t)
	for _, address := range []string{"http://localhost:" + freePort(t), "http://[::1]:" + freePort(t), "http://127.0.0.2:" + freePort(t)} {
		got := newMenubarRig(t, flags, address, landlineNextToBinary).menu(t)
		want := expectedMenu("jack dimmed", unreachableHead(), localSwitchboard(address, false, "Switchboard (build unknown)"), menubarBuildLine(releaseVersion(t), ""))
		if got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", address, got, want)
		}
	}
	fake := newFakeSwitchboard(t, "127.0.0.1", []any{}, 200, fakeBuild(menubarVersion, "", ""))
	got := newMenubarRig(t, flags, fake.url+"/", landlineNextToBinary).menu(t)
	hostPort := strings.TrimPrefix(fake.url, "http://")
	if !strings.HasPrefix(got, "icon: jack\n") || !strings.Contains(got, "\n  Running at http://"+hostPort) ||
		!strings.Contains(got, "\n  Start"+disabledMark+"\n  Stop\n  Restart\n") {
		t.Errorf("a trailing slash made the switchboard look remote or unreachable:\n%s", got)
	}
}

func TestMenuTextFindsLandlineOnThePathThatTheLoginShellPrintsWhenItsOwnPathIsEmpty(t *testing.T) {
	dead := deadAddress(t)
	flags := defaultFlags(t)
	withLandline := t.TempDir()
	if err := os.WriteFile(filepath.Join(withLandline, "landline"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		printed string
		want    switchboardItems
	}{
		"a login shell path with landline":    {withLandline, localSwitchboard(dead, false, "Switchboard (build unknown)")},
		"a login shell path without landline": {t.TempDir(), switchboardItems{first: notFoundLine(), build: "Switchboard (build unknown)"}},
	}
	for name, c := range cases {
		source := menubarBinary(t, flags)
		dir := t.TempDir()
		bin := copyBinary(t, source, dir)
		shell := filepath.Join(t.TempDir(), "shell")
		if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf %s '"+c.printed+"'\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		home := t.TempDir()
		env := withEnv(isolatedEnv(home), "PATH=", "SHELL="+shell, "LANDLINE_URL="+dead, "LANDLINE_SECRET="+secret)
		rig := menubarRig{bin: bin, env: env, home: home}
		want := expectedMenu("jack dimmed", unreachableHead(), c.want, menubarBuildLine(releaseVersion(t), ""))
		if got := rig.menu(t); got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got, want)
		}
	}
}

func TestMenuTextShowsOnlyTheMenubarBuildFormsOfTheDecisionRecords(t *testing.T) {
	dead := deadAddress(t)
	cases := map[string]struct{ flags, line string }{
		"clean":       {flagsFor(menubarVersion, stampCommit, stampDate), "Menubar 0.2.0 (a1b2c3d)"},
		"dirty":       {flagsFor(menubarVersion, stampCommit+routeDirtyTag, ""), "Menubar 0.2.0 (a1b2c3d-dirty)"},
		"no commit":   {flagsFor(menubarVersion, "", ""), "Menubar 0.2.0"},
		"plain build": {"", "Menubar dev"},
	}
	for name, c := range cases {
		got := newMenubarRig(t, c.flags, dead, landlineNextToBinary).menu(t)
		if !strings.Contains(got, "\n"+c.line+disabledMark+"\nAbout Landline\nRefresh\n") {
			t.Errorf("%s: the build line is not %q:\n%s", name, c.line, got)
		}
		if strings.Contains(got, stampDate) {
			t.Errorf("%s: the menu shows a date:\n%s", name, got)
		}
	}
}

func TestMenuTextRowShapesAreExactlyTheSolidAndTheHollowCircleAndTheTitleHasNoStateWord(t *testing.T) {
	agents := []map[string]any{fakeAgent("alpha", true, nil), fakeAgent("bravo", false, nil)}
	fake := newFakeSwitchboard(t, "127.0.0.1", agents, 200, fakeBuild(menubarVersion, "", ""))
	got := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary).menu(t)
	lines := menuLines(got)
	if lines[1] != "\u25cf alpha" {
		t.Errorf("the connected row is %q, want the solid circle U+25CF, a space, and the name", lines[1])
	}
	hollow := rowBlock(lines, "○ bravo")
	if hollow == nil {
		t.Fatalf("no row \"○ bravo\" (hollow circle U+25CB) in:\n%s", got)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, " ") && (strings.Contains(line, "connected") || strings.Contains(line, "[")) && !strings.HasPrefix(line, "icon:") {
			t.Errorf("a top level line carries a state word or a bracket: %q", line)
		}
	}
}

func TestMenuTextShowsAnAgentWithATaskAndNoProjectAsItsIDAloneWithAStateIDAndTaskSubmenu(t *testing.T) {
	agents := []map[string]any{fakeAgent("alpha", true, map[string]any{"task": "docs only"})}
	fake := newFakeSwitchboard(t, "127.0.0.1", agents, 200, fakeBuild(menubarVersion, "", ""))
	got := newMenubarRig(t, defaultFlags(t), fake.url, landlineNextToBinary).menu(t)
	row := agentRow("alpha", liveMarker, "ID: alpha", "Task: docs only", "Harness: claude-code", "Host: mac")
	want := expectedMenu("jack 1", row, localSwitchboard(fake.url, true, switchboardBuildLine(menubarVersion, "")), menubarBuildLine(releaseVersion(t), ""))
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "Project:") {
		t.Errorf("an agent with no project shows a Project line:\n%s", got)
	}
}
