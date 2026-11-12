package registry

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/8thlight-daveloper/landline/internal/model"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type Registry struct{}

func New() Registry { return Registry{} }

func (Registry) Register(agents []model.Agent, entry model.Agent, now time.Time) ([]model.Agent, error) {
	if !namePattern.MatchString(entry.Name) || entry.Name == model.Broadcast {
		return nil, model.ErrBadName
	}
	if strings.TrimSpace(entry.Harness) == "" {
		return nil, model.ErrNoHarness
	}
	for _, field := range []string{entry.Harness, entry.Host, entry.Session, entry.Folder, entry.Branch, entry.Worktree, entry.HarnessVersion, entry.Model, entry.Effort, entry.Label} {
		if !plainField(field) {
			return nil, model.ErrBadField
		}
	}
	entry.LastSeen = now
	out := make([]model.Agent, 0, len(agents)+1)
	for _, each := range agents {
		if each.Name != entry.Name {
			out = append(out, each)
		}
	}
	out = append(out, entry)
	sortByName(out)
	return out, nil
}

func (Registry) Find(agents []model.Agent, name string) (model.Agent, bool) {
	for _, each := range agents {
		if each.Name == name {
			return each, true
		}
	}
	return model.Agent{}, false
}

func plainField(text string) bool {
	if len(text) > 64 {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return false
		}
	}
	return true
}

func (Registry) Resolve(all, live []model.Agent, query string) (model.Agent, []model.Agent) {
	if query == "" {
		return model.Agent{}, nil
	}
	for _, each := range all {
		if each.Name == query {
			return each, nil
		}
	}
	pick := func(matches []model.Agent) (model.Agent, []model.Agent, bool) {
		switch len(matches) {
		case 0:
			return model.Agent{}, nil, false
		case 1:
			return matches[0], nil, true
		}
		sortByName(matches)
		return model.Agent{}, matches, true
	}
	var labelled, prefixed []model.Agent
	for _, each := range live {
		if each.Label == query {
			labelled = append(labelled, each)
		}
		if strings.HasPrefix(each.Name, query) || (each.Label != "" && strings.HasPrefix(each.Label, query)) {
			prefixed = append(prefixed, each)
		}
	}
	if match, candidates, found := pick(labelled); found {
		return match, candidates
	}
	match, candidates, _ := pick(prefixed)
	return match, candidates
}

func sortByName(agents []model.Agent) {
	slices.SortFunc(agents, func(a, b model.Agent) int { return strings.Compare(a.Name, b.Name) })
}
