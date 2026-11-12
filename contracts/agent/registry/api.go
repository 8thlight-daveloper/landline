package registry

import (
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Registry interface {
	Register(agents []model.Agent, entry model.Agent, now time.Time) ([]model.Agent, error)
	Find(agents []model.Agent, name string) (model.Agent, bool)
	Resolve(all, live []model.Agent, query string) (model.Agent, []model.Agent)
}
