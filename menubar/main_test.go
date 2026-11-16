package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseArgsAcceptsTheIntervalBeforeAndAfterMenuText(t *testing.T) {
	for _, args := range [][]string{
		{"--interval", "7s", "menu", "--text"}, {"menu", "--text", "--interval", "7s"},
		{"menu", "--interval=7s", "--text"}, {"--text", "--interval", "7s", "menu"},
	} {
		opts, problem := parseArgs(args)
		if problem != "" || !opts.menu || !opts.text || opts.interval != 7*time.Second {
			t.Errorf("%v: %+v %q", args, opts, problem)
		}
	}
}

func TestParseArgsDefaultsToTheFiveSecondGraphicalApp(t *testing.T) {
	opts, problem := parseArgs(nil)
	if problem != "" || opts.menu || opts.text || opts.version || opts.interval != 5*time.Second {
		t.Fatalf("%+v %q", opts, problem)
	}
}

func TestParseArgsRefusesWhatItDoesNotKnow(t *testing.T) {
	for _, args := range [][]string{
		{"bogus"}, {"agents"}, {"--bogus"}, {"menu"}, {"--text"}, {"menu", "--bogus"}, {"menu", "menu", "--text"},
		{"version", "--interval", "7s"}, {"version", "menu"}, {"menu", "--text=1"},
	} {
		if _, problem := parseArgs(args); problem != usage {
			t.Errorf("%v: %q", args, problem)
		}
	}
}

func TestParseArgsRefusesABadIntervalWithTheUsage(t *testing.T) {
	for _, args := range [][]string{{"--interval"}, {"--interval", "soon"}, {"menu", "--text", "--interval=x"}} {
		if _, problem := parseArgs(args); !strings.Contains(problem, "interval") || !strings.Contains(problem, "usage:") {
			t.Errorf("%v: %q", args, problem)
		}
	}
}

func TestHelpWinsOverEverythingAfterIt(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--help", "--bogus"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "menu --text") || errOut.Len() != 0 {
		t.Fatalf("%d %q %q", code, out.String(), errOut.String())
	}
}

func TestRunRefusesAFloorViolationWithExitTwoAndNothingOnStandardOutput(t *testing.T) {
	for _, d := range []string{"100ms", "499ms", "0s", "-5s"} {
		var out, errOut bytes.Buffer
		if code := run([]string{"menu", "--text", "--interval", d}, &out, &errOut); code != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "interval") {
			t.Errorf("%s: %d %q %q", d, code, out.String(), errOut.String())
		}
	}
}

func TestVersionPrintsOneLineNamedForTheApp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 || out.String() != "landline-menubar dev\n" {
		t.Fatalf("%d %q", code, out.String())
	}
}
