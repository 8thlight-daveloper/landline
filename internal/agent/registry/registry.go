package registry

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/8thlight-daveloper/landline/internal/model"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type Registry struct{}

func New() Registry { return Registry{} }

func (Registry) Register(agents []model.Agent, entry model.Agent, now time.Time) ([]model.Agent, error) {
	if !idPattern.MatchString(entry.ID) || entry.ID == model.Broadcast {
		return nil, model.ErrBadName
	}
	if strings.TrimSpace(entry.Harness) == "" {
		return nil, model.ErrNoHarness
	}
	for _, field := range []string{entry.Harness, entry.Host, entry.Session, entry.Folder, entry.Branch, entry.Worktree, entry.HarnessVersion, entry.LandlineVersion, entry.LandlineCommit, entry.LandlineDate, entry.Model, entry.Effort, entry.Project, entry.Task} {
		if !plainField(field) {
			return nil, model.ErrBadField
		}
	}
	entry.LastSeen = now
	out := make([]model.Agent, 0, len(agents)+1)
	for _, each := range agents {
		if each.ID != entry.ID {
			out = append(out, each)
		}
	}
	out = append(out, entry)
	sortByID(out)
	return out, nil
}

func (Registry) Find(agents []model.Agent, id string) (model.Agent, bool) {
	for _, each := range agents {
		if each.ID == id {
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

func sortByID(agents []model.Agent) {
	slices.SortFunc(agents, func(a, b model.Agent) int { return strings.Compare(a.ID, b.ID) })
}
