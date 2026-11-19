package registry

import (
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Registry interface {
	Invite(state model.Machines, name string, random []byte, now time.Time, lifetime time.Duration) (model.Machines, string, error)
	Redeem(state model.Machines, code string, publicKey []byte, now time.Time) (model.Machines, string, error)
	Revoke(state model.Machines, name string) (model.Machines, error)
	RevokeAll(state model.Machines) model.Machines
	Verify(state model.Machines, request model.SignedRequest, now, started time.Time, window time.Duration) (model.Machines, string, error)
	Fingerprint(publicKey []byte) string
}
