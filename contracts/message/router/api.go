package router

import (
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Router interface {
	Route(known []model.Agent, from string, request model.SendRequest, id string, seq int64, now time.Time) (model.Decision, error)
}
