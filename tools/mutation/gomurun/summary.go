package main

import (
	"fmt"
	"io"
)

type tally struct {
	killed    int
	survived  int
	accepted  int
	notViable int
	timedOut  int
	errored   int
}

func tallyOf(results []result, survivors []survivor, accepted map[string]bool) (tally, []survivor) {
	var counts tally
	for _, each := range results {
		switch each.Status {
		case statusKilled:
			counts.killed++
		case statusNotViable:
			counts.notViable++
		case statusTimedOut:
			counts.timedOut++
		case statusError:
			counts.errored++
		}
	}
	var unreviewed []survivor
	for _, each := range survivors {
		if accepted[each.key] {
			counts.accepted++
			continue
		}
		counts.survived++
		unreviewed = append(unreviewed, each)
	}
	return counts, unreviewed
}

func (counts tally) judgedNothing() bool {
	return counts.killed+counts.survived+counts.accepted == 0 && counts.unjudged() > 0
}

func (counts tally) unjudged() int {
	return counts.notViable + counts.timedOut + counts.errored
}

func (counts tally) score() float64 {
	judged := counts.killed + counts.survived
	if judged == 0 {
		return 1
	}
	return float64(counts.killed) / float64(judged)
}

func printSurvivors(output io.Writer, unreviewed []survivor) {
	for _, each := range unreviewed {
		fmt.Fprintf(output, "%s %s:%d %s: %s -> %s\n", each.key, each.file, each.line, each.kind, each.original, each.mutated)
		fmt.Fprintf(output, "    %s\n", each.source)
	}
}

func printScore(output io.Writer, counts tally) {
	total := counts.killed + counts.survived + counts.accepted + counts.notViable + counts.timedOut + counts.errored
	fmt.Fprintf(output, "The mutation score is %.6f (%d passed, %d failed, %d accepted, %d not viable, %d timed out, %d errors, total is %d)\n",
		counts.score(), counts.killed, counts.survived, counts.accepted, counts.notViable, counts.timedOut, counts.errored, total)
}
