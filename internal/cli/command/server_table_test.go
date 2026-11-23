package command

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const serverTable = "\n[server]\n" +
	"# chosen by hand\n" +
	"log = \"~/landline-logs/log.jsonl\"\n" +
	"keepalive = \"15s\"\n" +
	"delivery-timeout = \"20s\"\n" +
	"grace = \"3m\"\n" +
	"invite-lifetime = \"5m\"\n" +
	"signature-window = \"60s\"\n"

func (r *rotation) appendToConfig(t *testing.T, text string) string {
	t.Helper()
	path := configPath(r.env)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, text...), 0o600); err != nil {
		t.Fatal(err)
	}
	return string(raw) + text
}

func (r *rotation) configText(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(configPath(r.env))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func flagLines(stdout, flag string) []string {
	var found []string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, flag) && strings.Contains(line, "config.toml") {
			found = append(found, line)
		}
	}
	return found
}

func TestUpRestartAndRotateSecretPassOnOnlyTheFlagsGivenAndNeverAValueFromTheServerTable(t *testing.T) {
	address := listening(t)
	r := newRotation(t, "http://"+address)
	r.appendToConfig(t, serverTable)
	for _, command := range [][]string{{"up"}, {"restart"}, {"rotate-secret"}} {
		if code := r.run(command...); code != 0 {
			t.Fatalf("%v with a [server] table: %d %s", command, code, r.err.String())
		}
		if args := strings.Fields(r.recorded(t, "args")); !reflect.DeepEqual(args, []string{"--addr", address}) {
			t.Errorf("%v passed on %v, want only the saved address", command, args)
		}
	}
	if code := r.run("restart", "--keepalive", "200ms"); code != 0 {
		t.Fatalf("restart --keepalive 200ms: %d %s", code, r.err.String())
	}
	if args := strings.Fields(r.recorded(t, "args")); !reflect.DeepEqual(args, []string{"--addr", address, "--keepalive", "200ms"}) {
		t.Errorf("restart --keepalive 200ms passed on %v", args)
	}
	if text := r.configText(t); strings.Contains(text, "200ms") {
		t.Errorf("a flag's value was written to config.toml:\n%s", strings.ReplaceAll(text, r.config().Secret, "<secret>"))
	}
}

func TestABadValueInTheServerTableStopsUpRestartAndRotateSecretBeforeTheyChangeAnything(t *testing.T) {
	for name, each := range map[string]struct{ line, echo string }{
		"a relative log":            {`log = "relative-zebra/log.jsonl"`, "relative-zebra"},
		"a log under another user":  {`log = "~zebra/log.jsonl"`, "~zebra"},
		"an empty log":              {`log = ""`, `log = ""`},
		"a log that is a number":    {`log = 5`, "log = 5"},
		"a keepalive of zero":       {`keepalive = "0s"`, `"0s"`},
		"a negative delivery":       {`delivery-timeout = "-5s"`, "-5s"},
		"a grace that is no time":   {`grace = "zebra"`, "zebra"},
		"an invite lifetime number": {`invite-lifetime = 300`, "300"},
		"an empty signature window": {`signature-window = ""`, `signature-window = ""`},
		"a duration with no unit":   {`keepalive = "15"`, `"15"`},
	} {
		t.Run(name, func(t *testing.T) {
			address := listening(t)
			r := newRotation(t, "http://"+address)
			if code := r.run("up"); code != 0 {
				t.Fatalf("up: %d %s", code, r.err.String())
			}
			before := r.pid(t)
			written := r.appendToConfig(t, "\n[server]\n# chosen by hand\n"+each.line+"\n")
			badLine := regexp.MustCompile(`config\.toml line 7\b`)
			for _, command := range [][]string{{"restart"}, {"rotate-secret"}, {"down"}, {"up"}} {
				code := r.run(command...)
				if command[0] == "down" {
					continue
				}
				combined := r.out.String() + r.err.String()
				if code != 1 || !badLine.MatchString(combined) || strings.Contains(combined, each.echo) {
					t.Errorf("%v: code %d, want 1 and an error at config.toml line 7 that does not repeat the line: %q", command, code, combined)
				}
				if command[0] != "up" && (!r.running() || r.pid(t) != before) {
					t.Errorf("%v stopped or replaced the running switchboard", command)
				}
				if command[0] == "up" && r.running() {
					t.Errorf("up started a switchboard")
				}
				if r.configText(t) != written {
					t.Errorf("%v changed config.toml", command)
				}
			}
		})
	}
}

func TestTheFlagLineIsPrintedOnStandardOutputBeforeTheResultOnlyForAFlagThatDiffersInMeaning(t *testing.T) {
	address := listening(t)
	r := newRotation(t, "http://"+address)
	r.appendToConfig(t, serverTable)
	if code := r.run("up"); code != 0 || strings.Contains(r.out.String(), "config.toml") {
		t.Fatalf("up with no flags: %d %q %s", code, r.out.String(), r.err.String())
	}
	if code := r.run("rotate-secret", "--grace", "3m", "--keepalive", "15000ms", "--invite-lifetime", "300s", "--log", r.env.Home+"/landline-logs/log.jsonl"); code != 0 {
		t.Fatalf("rotate-secret: %d %s", code, r.err.String())
	}
	if r.out.String() != rotatedMessage+"\n" {
		t.Errorf("flags equal in meaning to the table printed more than the result: %q", r.out.String())
	}
	if code := r.run("rotate-secret", "--grace", "4m", "--delivery-timeout", "20s"); code != 0 {
		t.Fatalf("rotate-secret --grace 4m: %d %s", code, r.err.String())
	}
	lines := strings.Split(strings.TrimSuffix(r.out.String(), "\n"), "\n")
	if len(lines) != 2 || lines[1] != rotatedMessage || len(flagLines(lines[0], "--grace")) != 1 || len(flagLines(r.out.String(), "--delivery-timeout")) != 0 {
		t.Errorf("want one line that names --grace and config.toml, then the result: %q", r.out.String())
	}
	if !strings.Contains(lines[0], "restart") || !strings.Contains(lines[0], "rotate-secret") {
		t.Errorf("the line does not say that restart and rotate-secret use the value in config.toml: %q", lines[0])
	}
	if code := r.run("restart", "--signature-window", "1m", "--delivery-timeout", "21s"); code != 0 {
		t.Fatalf("restart: %d %s", code, r.err.String())
	}
	if len(flagLines(r.out.String(), "--delivery-timeout")) != 1 || len(flagLines(r.out.String(), "--signature-window")) != 0 {
		t.Errorf("restart: want one line for --delivery-timeout and none for --signature-window 1m: %q", r.out.String())
	}
	if lines := strings.Split(strings.TrimSuffix(r.out.String(), "\n"), "\n"); !strings.Contains(lines[len(lines)-1], "the switchboard is running") {
		t.Errorf("restart: the result line is not last: %q", r.out.String())
	}
}

func TestTheFlagLineComparesWithTheDefaultWhenTheTableHasNoValue(t *testing.T) {
	address := listening(t)
	r := newRotation(t, "http://"+address)
	if code := r.run("up", "--grace", "120s", "--keepalive", "15s", "--delivery-timeout", "20s", "--invite-lifetime", "5m", "--signature-window", "1m", "--log", r.env.Home+"/.local/share/landline/log.jsonl"); code != 0 {
		t.Fatalf("up: %d %s", code, r.err.String())
	}
	if lines := strings.Split(strings.TrimSuffix(r.out.String(), "\n"), "\n"); len(lines) != 1 {
		t.Errorf("flags equal to the defaults printed a flag line: %q", r.out.String())
	}
	if code := r.run("restart", "--grace", "121s"); code != 0 {
		t.Fatalf("restart: %d %s", code, r.err.String())
	}
	if len(flagLines(r.out.String(), "--grace")) != 1 {
		t.Errorf("a --grace that differs from the default printed no line: %q", r.out.String())
	}
}
