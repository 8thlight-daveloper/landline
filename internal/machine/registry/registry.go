package registry

import (
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Registry struct{}

func New() Registry { return Registry{} }

func (Registry) Invite(state model.Machines, name string, random []byte, now time.Time, lifetime time.Duration) (model.Machines, string, error) {
	return model.Machines{}, "", nil
}

func (Registry) Redeem(state model.Machines, code string, publicKey []byte, now time.Time) (model.Machines, string, error) {
	return model.Machines{}, "", nil
}

func (Registry) Revoke(state model.Machines, name string) (model.Machines, error) {
	return model.Machines{}, nil
}

func (Registry) RevokeAll(state model.Machines) model.Machines { return model.Machines{} }

func (Registry) Verify(state model.Machines, request model.SignedRequest, now, started time.Time, window time.Duration) (model.Machines, string, error) {
	return model.Machines{}, "", nil
}

func (Registry) Fingerprint(publicKey []byte) string { return "" }
