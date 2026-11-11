package registry

import (
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Registry interface {
	Register(agents []model.Agent, entry model.Agent, now time.Time) ([]model.Agent, error)
	Online(agents []model.Agent, now time.Time, window time.Duration) []model.Agent
	Find(agents []model.Agent, name string) (model.Agent, bool)
}
