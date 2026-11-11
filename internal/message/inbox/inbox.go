package inbox

import (
	"cmp"
	"slices"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Inbox struct{}

func New() Inbox { return Inbox{} }

func (Inbox) Select(log []model.Envelope, agent string, cursor int64, threads []string, limit int) model.Page {
	matches := []model.Envelope{}
	for _, each := range log {
		if each.Seq > cursor && forAgent(each, agent) && inThreads(each, threads) {
			matches = append(matches, each)
		}
	}
	slices.SortFunc(matches, func(a, b model.Envelope) int { return cmp.Compare(a.Seq, b.Seq) })
	remaining := 0
	if limit > 0 && len(matches) > limit {
		remaining = len(matches) - limit
		matches = matches[:limit]
	}
	next := cursor
	if len(matches) > 0 {
		next = matches[len(matches)-1].Seq
	}
	return model.Page{Messages: matches, Next: next, Remaining: remaining}
}

func forAgent(each model.Envelope, agent string) bool {
	if each.To == agent {
		return true
	}
	return each.To == model.Broadcast && each.From != agent
}

func inThreads(each model.Envelope, threads []string) bool {
	return len(threads) == 0 || slices.Contains(threads, each.Thread)
}
