package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheTallyLeavesOutNotViableMutantsAndAcceptedSurvivors(t *testing.T) {
	results := []result{
		{Status: statusKilled}, {Status: statusKilled}, {Status: statusKilled},
		{Status: statusSurvived}, {Status: statusSurvived},
		{Status: statusNotViable}, {Status: statusNotViable},
		{Status: statusTimedOut}, {Status: statusError},
	}
	survivors := []survivor{{key: "aaaa"}, {key: "bbbb"}}
	counts, unreviewed := tallyOf(results, survivors, map[string]bool{"aaaa": true})
	if counts.killed != 3 || counts.survived != 1 || counts.accepted != 1 || counts.notViable != 2 || counts.timedOut != 1 || counts.errored != 1 {
		t.Fatalf("unexpected tally %+v", counts)
	}
	if len(unreviewed) != 1 || unreviewed[0].key != "bbbb" {
		t.Fatalf("unexpected survivors %+v", unreviewed)
	}
	if got := counts.score(); got != 0.75 {
		t.Fatalf("score is %v, want 0.75", got)
	}
}

func TestAScoreWithNothingJudgedIsOne(t *testing.T) {
	if got := (tally{notViable: 4}).score(); got != 1 {
		t.Fatalf("score is %v", got)
	}
}

func TestARunWhereNoMutantWasKilledOrSurvivedJudgedNothing(t *testing.T) {
	cases := map[string]struct {
		counts tally
		want   bool
	}{
		"only not viable":          {tally{notViable: 4}, true},
		"not viable and timed out": {tally{notViable: 4, timedOut: 2}, true},
		"only timed out":           {tally{timedOut: 6}, true},
		"only errors":              {tally{errored: 3}, true},
		"timed out, one killed":    {tally{killed: 1, timedOut: 6}, false},
		"one killed":               {tally{killed: 1, notViable: 4}, false},
		"one survivor":             {tally{survived: 1, notViable: 4}, false},
		"every survivor accepted":  {tally{accepted: 2, notViable: 4}, false},
		"no mutants at all":        {tally{}, false},
	}
	for name, each := range cases {
		if got := each.counts.judgedNothing(); got != each.want {
			t.Errorf("%s: judgedNothing = %v, want %v", name, got, each.want)
		}
	}
}

func TestARunThatJudgedNothingIsAProblemNotAPerfectScore(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "internal", "noun", "role", "a.go")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("package role\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{statusNotViable, statusTimedOut, statusError} {
		results := []result{
			{Mutant: mutant{FilePath: source, Line: 1}, Status: status},
			{Mutant: mutant{FilePath: source, Line: 1, Column: 2}, Status: status},
		}
		if code := summarize(root, defaultLayout(), "noun/role", results); code != exitProblem {
			t.Errorf("all %s: exit code is %d, want %d", status, code, exitProblem)
		}
	}
}

func TestTheScoreLineSaysMutationScore(t *testing.T) {
	var output bytes.Buffer
	printScore(&output, tally{killed: 3, survived: 1, accepted: 2, notViable: 5})
	want := "The mutation score is 0.750000 (3 passed, 1 failed, 2 accepted, 5 not viable, 0 timed out, 0 errors, total is 11)\n"
	if output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func TestASurvivorIsPrintedAsTheLineAPersonAddsToTheAcceptedList(t *testing.T) {
	var output bytes.Buffer
	printSurvivors(&output, []survivor{{key: "0123456789abcdef", file: "internal/x/a.go", line: 4, kind: "conditional_binary", original: "==", mutated: "<=", source: "return x == 1"}})
	want := "0123456789abcdef internal/x/a.go:4 conditional_binary: == -> <=\n    return x == 1\n"
	if output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func TestTheAcceptedListReadsTheFirstWordOfEachLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "joiner.txt")
	text := "# reviewed on 2026-10-21\n\n0123456789abcdef internal/x/a.go:4 conditional_binary: == -> <= :: the size hint only\nfedcba9876543210 :: another\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	accepted, err := readAccepted(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted) != 2 || !accepted["0123456789abcdef"] || !accepted["fedcba9876543210"] {
		t.Fatalf("unexpected accepted keys %v", accepted)
	}
}

func TestAMissingAcceptedListAcceptsNothing(t *testing.T) {
	accepted, err := readAccepted(filepath.Join(t.TempDir(), "none.txt"))
	if err != nil || len(accepted) != 0 {
		t.Fatalf("got %v, %v", accepted, err)
	}
}

func TestTheContractFolderIsFoundFromTheComponentFolder(t *testing.T) {
	cases := map[string]string{
		"record/joiner": "../../../contracts/record/joiner/",
		"secrets":       "../../contracts/secrets/",
	}
	for component, want := range cases {
		if got := contractPathFrom(defaultLayout(), component); got != want {
			t.Errorf("contractPathFrom(%q) = %q, want %q", component, got, want)
		}
	}
}

func TestGoFlagsKeepAnExistingLimitAndAddOneOtherwise(t *testing.T) {
	cases := map[string]string{
		"":               "-p=2",
		"-race":          "-race -p=2",
		"-p=4 -race":     "-p=4 -race",
		"-race -p 4":     "-race -p 4",
		"-tags=mutation": "-tags=mutation -p=2",
	}
	for existing, want := range cases {
		if got := limitedGoFlags(existing); got != want {
			t.Errorf("limitedGoFlags(%q) = %q, want %q", existing, got, want)
		}
	}
}

func TestAReportThatIsNotJSONIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mutation-report.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readReport(path); err == nil || !strings.Contains(err.Error(), "not a gomu report") {
		t.Fatalf("got %v", err)
	}
}

func TestTheTestBinaryGetsItsOwnTimeoutSoAHungMutantCannotOutliveTheRun(t *testing.T) {
	arguments := gomuArguments(".direnv/bin/gomu", defaultLayout(), "./internal/record/joiner", "record/joiner", 1, 30, false)
	want := []string{"--", "-count=1", "-failfast", "-timeout=30s", "../../../contracts/record/joiner/"}
	tail := arguments[len(arguments)-len(want):]
	if strings.Join(tail, " ") != strings.Join(want, " ") {
		t.Fatalf("test flags are %v, want %v", tail, want)
	}
}

func TestGomuWaitsLongerThanTheTestBinaryDoes(t *testing.T) {
	arguments := gomuArguments(".direnv/bin/gomu", defaultLayout(), "./internal/secrets", "secrets", 2, 30, false)
	joined := strings.Join(arguments, " ")
	for _, want := range []string{"--timeout 40", "--workers 2", "./internal/secrets", "--incremental=false", "--output json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("arguments %q lack %q", joined, want)
		}
	}
}

func TestShortRunsPassShortToTheTestBinaryBeforeThePackage(t *testing.T) {
	arguments := gomuArguments(".direnv/bin/gomu", defaultLayout(), "./internal/secrets", "secrets", 1, 30, true)
	want := []string{"--", "-count=1", "-failfast", "-timeout=30s", "-short", "../../contracts/secrets/"}
	tail := arguments[len(arguments)-len(want):]
	if strings.Join(tail, " ") != strings.Join(want, " ") {
		t.Fatalf("test flags are %v, want %v", tail, want)
	}
}

func TestAnotherLayoutMovesTheContractFolderAndTheTarget(t *testing.T) {
	place := layout{components: "src/parts", contracts: "specs", accepted: "mutation/accepted", gomuVersion: "v0.3.0"}
	if got, want := contractPathFrom(place, "record/joiner"), "../../../../specs/record/joiner/"; got != want {
		t.Errorf("contractPathFrom = %q, want %q", got, want)
	}
	if got, want := componentTarget(place, "record/joiner"), "./src/parts/record/joiner"; got != want {
		t.Errorf("componentTarget = %q, want %q", got, want)
	}
}

func TestFolderFlagsAreCleanedBeforeTheContractPathIsBuilt(t *testing.T) {
	for _, components := range []string{"internal", "./internal", "internal/", "./internal/"} {
		place, err := layout{components: components, contracts: "./contracts/", accepted: "tools/mutation/accepted"}.normalized()
		if err != nil {
			t.Fatalf("%q: %v", components, err)
		}
		if got, want := contractPathFrom(place, "record/joiner"), "../../../contracts/record/joiner/"; got != want {
			t.Errorf("components %q: contractPathFrom = %q, want %q", components, got, want)
		}
		if got, want := componentTarget(place, "record/joiner"), "./internal/record/joiner"; got != want {
			t.Errorf("components %q: componentTarget = %q, want %q", components, got, want)
		}
	}
}

func TestAFolderFlagOutsideTheProjectIsRefused(t *testing.T) {
	for _, bad := range []string{"/abs/internal", "../internal", ".."} {
		if _, err := (layout{components: bad, contracts: "contracts", accepted: "accepted"}).normalized(); err == nil {
			t.Errorf("--components %q was accepted", bad)
		}
	}
}

func TestAComponentOutsideTheComponentsFolderIsRefused(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"..", "../x", "/abs", "."} {
		if err := checkComponent(root, defaultLayout(), bad); err == nil || !strings.Contains(err.Error(), "is not a component folder") {
			t.Errorf("component %q: got %v", bad, err)
		}
	}
}

func TestACachedResultDoesNotCrossAGomuVersion(t *testing.T) {
	if cacheKey([]byte("source"), "suite", "v1", false) == cacheKey([]byte("source"), "suite", "v2", false) {
		t.Fatal("two gomu versions share a cache key")
	}
}
