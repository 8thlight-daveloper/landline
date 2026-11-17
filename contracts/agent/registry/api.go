package registry

import (
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Registry interface {
	Register(agents []model.Agent, entry model.Agent, now time.Time) ([]model.Agent, error)
	Find(agents []model.Agent, id string) (model.Agent, bool)
}
