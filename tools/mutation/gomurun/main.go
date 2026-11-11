package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

const (
	exitSuccess = 0
	exitProblem = 1
	exitUsage   = 2

	gomuGraceSeconds = 10

	reportFile  = "mutation-report.json"
	historyFile = ".gomu_history.json"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	options := flag.NewFlagSet("gomurun", flag.ContinueOnError)
	workers := options.Int("workers", 1, "mutants tested at once")
	timeout := options.Int("timeout", 30, "seconds a test run may take for one mutant")
	gomu := options.String("gomu", ".direnv/bin/gomu", "path of the gomu program")
	short := options.Bool("short", false, "run the tests with -short, which skips the slow ones")
	cached := options.Bool("cached", false, "reuse the results of source files whose text and contract suite have not changed")
	defaults := defaultLayout()
	place := layout{}
	options.StringVar(&place.components, "components", defaults.components, "folder that holds the components")
	options.StringVar(&place.contracts, "contracts", defaults.contracts, "folder that holds the contract packages")
	options.StringVar(&place.accepted, "accepted", defaults.accepted, "folder that holds the accepted survivor lists")
	options.StringVar(&place.gomuVersion, "gomu-version", defaults.gomuVersion, "gomu version, which is part of the cache key")
	if options.Parse(args) != nil || options.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: gomurun [--workers N] [--timeout S] [--short] [--cached] [--gomu PATH] [--components DIR] [--contracts DIR] [--accepted DIR] [--gomu-version V] <noun>/<role>")
		return exitUsage
	}
	place, err := place.normalized()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	component := path.Clean(filepath.ToSlash(options.Arg(0)))
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitProblem
	}
	if problem := checkComponent(root, place, component); problem != nil {
		fmt.Fprintln(os.Stderr, problem)
		return exitUsage
	}
	defer os.Remove(filepath.Join(root, reportFile))
	defer os.Remove(filepath.Join(root, historyFile))
	var results []result
	if *cached {
		results, err = cachedResults(root, place, component, func(target string) error {
			return runGomu(*gomu, place, target, component, *workers, *timeout, *short)
		}, *short)
	} else {
		err = runGomu(*gomu, place, componentTarget(place, component), component, *workers, *timeout, *short)
		if err == nil {
			results, err = freshResults(root)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitProblem
	}
	code := summarize(root, place, component, results)
	if *short {
		fmt.Fprintln(os.Stdout, "Short run: a mutant that only a skipped test kills is listed as a survivor. Check each survivor against the full suite before you accept or report it.")
	}
	return code
}

func freshResults(root string) ([]result, error) {
	parsed, err := readReport(filepath.Join(root, reportFile))
	if err != nil {
		return nil, err
	}
	return parsed.Results, nil
}

func cachedResults(root string, place layout, component string, mutate func(target string) error, short bool) ([]result, error) {
	names, err := sourceFiles(root, place, component)
	if err != nil {
		return nil, err
	}
	suite, err := suiteDigest(root, place, component)
	if err != nil {
		return nil, err
	}
	sources, err := sourceDigest(root, place, component, names)
	if err != nil {
		return nil, err
	}
	key := cacheKey([]byte(sources), suite, place.gomuVersion, short)
	var all []result
	for _, name := range names {
		path := cachePath(root, component, name, key)
		if results, ok := readCached(path); ok {
			fmt.Fprintf(os.Stderr, "%s: reused %d results\n", name, len(results))
			all = append(all, results...)
			continue
		}
		fmt.Fprintf(os.Stderr, "%s: mutating\n", name)
		if err := mutate(componentTarget(place, component) + "/" + name); err != nil {
			return nil, err
		}
		results, err := freshResults(root)
		if err != nil {
			return nil, err
		}
		os.Remove(filepath.Join(root, reportFile))
		if err := writeCached(path, results); err != nil {
			return nil, err
		}
		forgetStale(root, component, name, path)
		all = append(all, results...)
	}
	return all, nil
}

func checkComponent(root string, place layout, component string) error {
	if !insideProject(component) || component == "." {
		return fmt.Errorf("%q is not a component folder", component)
	}
	for _, folder := range []string{place.components, place.contracts} {
		if info, err := os.Stat(filepath.Join(root, folder, component)); err != nil || !info.IsDir() {
			return fmt.Errorf("%s/%s is not a folder", folder, component)
		}
	}
	return nil
}

func componentTarget(place layout, component string) string {
	return "./" + filepath.ToSlash(filepath.Join(place.components, component))
}

func contractPathFrom(place layout, component string) string {
	depth := segments(place.components) + segments(component)
	return strings.Repeat("../", depth) + place.contracts + "/" + component + "/"
}

func gomuArguments(gomu string, place layout, target string, component string, workers int, timeout int, short bool) []string {
	testFlags := []string{"-count=1", "-failfast", fmt.Sprintf("-timeout=%ds", timeout)}
	if short {
		testFlags = append(testFlags, "-short")
	}
	arguments := []string{
		"-n", "10", gomu, "run", target,
		"--incremental=false", "--threshold", "0", "--output", "json",
		"--workers", fmt.Sprint(workers), "--timeout", fmt.Sprint(timeout + gomuGraceSeconds),
		"--",
	}
	arguments = append(arguments, testFlags...)
	return append(arguments, contractPathFrom(place, component))
}

func runGomu(gomu string, place layout, target string, component string, workers int, timeout int, short bool) error {
	command := exec.Command("nice", gomuArguments(gomu, place, target, component, workers, timeout, short)...)
	command.Env = append(os.Environ(), "GOFLAGS="+limitedGoFlags(os.Getenv("GOFLAGS")))
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	return command.Run()
}

func limitedGoFlags(existing string) string {
	for _, flag := range strings.Fields(existing) {
		if flag == "-p" || strings.HasPrefix(flag, "-p=") {
			return existing
		}
	}
	return strings.TrimSpace(existing + " -p=2")
}

func summarize(root string, place layout, component string, results []result) int {
	survivors, err := survivorsOf(results, root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitProblem
	}
	accepted, err := readAccepted(filepath.Join(root, place.accepted, component+".txt"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitProblem
	}
	counts, unreviewed := tallyOf(results, survivors, accepted)
	if counts.judgedNothing() {
		fmt.Fprintf(os.Stderr, "Every one of the %d mutants was not viable, timed out, or errored (%d not viable, %d timed out, %d errors), so the run judged nothing. The component or its contract package may not build, or the tests may need a longer --timeout.\n", counts.unjudged(), counts.notViable, counts.timedOut, counts.errored)
		return exitProblem
	}
	printSurvivors(os.Stdout, unreviewed)
	printScore(os.Stdout, counts)
	return exitSuccess
}
