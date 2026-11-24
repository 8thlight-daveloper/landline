package registry

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const alphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

var dummyKey = ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)

var machineName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

type Registry struct{}

func New() Registry { return Registry{} }

func cloneState(state model.Machines) model.Machines {
	state.Active = slices.Clone(state.Active)
	for i := range state.Active {
		state.Active[i].PublicKey = slices.Clone(state.Active[i].PublicKey)
	}
	state.Invites = slices.Clone(state.Invites)
	state.Seen = slices.Clone(state.Seen)
	for i := range state.Seen {
		state.Seen[i].Signature = slices.Clone(state.Seen[i].Signature)
	}
	return state
}

func activeIndex(state model.Machines, name string) int {
	found := -1
	for i, machine := range state.Active {
		if machine.Name == name && found < 0 {
			found = i
		}
	}
	return found
}

func (Registry) Invite(state model.Machines, name string, random []byte, now time.Time, lifetime time.Duration) (model.Machines, string, error) {
	state = cloneState(state)
	if !machineName.MatchString(name) {
		return state, "", model.ErrBadMachineName
	}
	var characters [8]byte
	if len(random) < len(characters) {
		return state, "", model.ErrNotEnoughRandom
	}
	for i := range characters {
		characters[i] = alphabet[int(random[i])%len(alphabet)]
	}
	code := string(characters[:])
	if activeIndex(state, name) >= 0 {
		return state, "", model.ErrMachineHeld
	}
	for _, invite := range state.Invites {
		if invite.Name == name && invite.Expires.After(now) {
			return state, "", model.ErrMachineHeld
		}
	}
	for _, invite := range state.Invites {
		if invite.Code == code && invite.Expires.After(now) {
			return state, "", model.ErrCodeInUse
		}
	}
	state.Invites = slices.DeleteFunc(state.Invites, func(invite model.Invite) bool { return invite.Name == name })
	state.Invites = append(state.Invites, model.Invite{Name: name, Code: code, Expires: now.Add(lifetime)})
	slices.SortFunc(state.Invites, func(a, b model.Invite) int { return strings.Compare(a.Name, b.Name) })
	return state, code[:4] + "-" + code[4:], nil
}

func normalizeCode(code string) string {
	var normalized strings.Builder
	for i := 0; i < len(code); i++ {
		c := code[i]
		if c == '-' {
			continue
		}
		if 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		normalized.WriteByte(c)
	}
	return normalized.String()
}

func (Registry) Redeem(state model.Machines, code string, publicKey []byte, now time.Time) (model.Machines, string, error) {
	state = cloneState(state)
	code = normalizeCode(code)
	i := slices.IndexFunc(state.Invites, func(invite model.Invite) bool {
		return invite.Code == code && invite.Expires.After(now)
	})
	if i == -1 {
		state.Failures++
		if state.Failures == 5 {
			state.Invites = nil
			state.Failures = 0
		}
		return state, "", model.ErrInvalidCode
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return state, "", model.ErrBadPublicKey
	}
	name := state.Invites[i].Name
	state.Invites = slices.Delete(state.Invites, i, i+1)
	state.Active = append(state.Active, model.Machine{Name: name, PublicKey: slices.Clone(publicKey), Created: now})
	slices.SortFunc(state.Active, func(a, b model.Machine) int { return strings.Compare(a.Name, b.Name) })
	state.Failures = 0
	return state, name, nil
}

func (Registry) Revoke(state model.Machines, name string) (model.Machines, error) {
	state = cloneState(state)
	if i := activeIndex(state, name); i >= 0 {
		state.Active = slices.Delete(state.Active, i, i+1)
		return state, nil
	}
	i := slices.IndexFunc(state.Invites, func(invite model.Invite) bool { return invite.Name == name })
	if i < 0 {
		return state, model.ErrUnknownMachine
	}
	state.Invites = slices.Delete(state.Invites, i, i+1)
	return state, nil
}

func (Registry) RevokeAll(state model.Machines) model.Machines {
	state.Active = nil
	state.Invites = nil
	return cloneState(state)
}

func clockDifference(request, now int64) int64 {
	if now > 0 && request < math.MinInt64+now {
		return math.MinInt64
	}
	if now < 0 && request > math.MaxInt64+now {
		return math.MaxInt64
	}
	return request - now
}

const startPeriodSeconds = 5

func inStartPeriod(requestTime int64, started time.Time) bool {
	if started.Unix() > math.MaxInt64-startPeriodSeconds {
		return true
	}
	return requestTime <= started.Unix()+startPeriodSeconds
}

func signedText(request model.SignedRequest) []byte {
	digest := sha256.Sum256(request.Body)
	return []byte(strings.Join([]string{
		"landline-machine-v1", request.Name, strconv.FormatInt(request.Time, 10),
		request.Nonce, request.Method, request.Path, hex.EncodeToString(digest[:]),
	}, "\n"))
}

func (Registry) Verify(state model.Machines, request model.SignedRequest, now, started time.Time, window time.Duration) (model.Machines, string, error) {
	state = cloneState(state)
	seconds := int64(window / time.Second)
	difference := clockDifference(request.Time, now.Unix())
	if difference < -seconds || difference > seconds {
		return state, "", &model.ClockSkewError{Seconds: difference}
	}
	i := activeIndex(state, request.Name)
	key, trusted := dummyKey, false
	if i >= 0 && len(state.Active[i].PublicKey) == ed25519.PublicKeySize {
		key, trusted = state.Active[i].PublicKey, true
	}
	verified := ed25519.Verify(key, signedText(request), request.Signature) && trusted
	switch {
	case i < 0:
		return state, "", model.ErrUnknownMachine
	case inStartPeriod(request.Time, started):
		return state, "", model.ErrBeforeStart
	case !verified:
		return state, "", model.ErrBadSignature
	}
	for _, seen := range state.Seen {
		if seen.Until.After(now) && slices.Equal(seen.Signature, request.Signature) {
			return state, "", model.ErrReplayed
		}
	}
	state.Seen = slices.DeleteFunc(state.Seen, func(seen model.SeenSignature) bool { return !seen.Until.After(now) })
	until := time.Unix(request.Time, 0).UTC().Add(window.Truncate(time.Second)).Add(time.Second)
	state.Seen = append(state.Seen, model.SeenSignature{Signature: slices.Clone(request.Signature), Until: until})
	return state, request.Name, nil
}

func (Registry) Fingerprint(publicKey []byte) string {
	digest := sha256.Sum256(publicKey)
	return hex.EncodeToString(digest[:6])
}
