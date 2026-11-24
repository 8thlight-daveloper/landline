package registry

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

const (
	lifetime = 5 * time.Minute
	window   = 60 * time.Second
)

func random(first byte) []byte {
	out := make([]byte, 16)
	for i := range out {
		out[i] = first + byte(i)
	}
	return out
}

func keyOf(n byte) ed25519.PrivateKey { return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{n}, 32)) }

func publicOf(n byte) []byte { return keyOf(n).Public().(ed25519.PublicKey) }

func invited(t *testing.T, state model.Machines, name string, first byte, now time.Time) (model.Machines, string) {
	t.Helper()
	next, code, err := newImpl().Invite(state, name, random(first), now, lifetime)
	if err != nil {
		t.Fatalf("invite %s: %v", name, err)
	}
	return next, code
}

func joined(t *testing.T, state model.Machines, name string, first, key byte, now time.Time) model.Machines {
	t.Helper()
	state, code := invited(t, state, name, first, now)
	next, got, err := newImpl().Redeem(state, code, publicOf(key), now)
	if err != nil || got != name {
		t.Fatalf("redeem %s: %q %v", name, got, err)
	}
	return next
}

func failures(t *testing.T, state model.Machines, count int, now time.Time) model.Machines {
	t.Helper()
	for i := 0; i < count; i++ {
		var err error
		if state, _, err = newImpl().Redeem(state, "WRONGONE", publicOf(1), now); !errors.Is(err, model.ErrInvalidCode) {
			t.Fatalf("failure %d: %v", i+1, err)
		}
	}
	return state
}

func snapshot(state model.Machines) model.Machines {
	out := model.Machines{Failures: state.Failures}
	for _, m := range state.Active {
		out.Active = append(out.Active, model.Machine{Name: m.Name, PublicKey: append([]byte(nil), m.PublicKey...), Created: m.Created})
	}
	out.Invites = append(out.Invites, state.Invites...)
	for _, s := range state.Seen {
		out.Seen = append(out.Seen, model.SeenSignature{Signature: append([]byte(nil), s.Signature...), Until: s.Until})
	}
	return out
}

func names(state model.Machines) []string {
	out := []string{}
	for _, m := range state.Active {
		out = append(out, m.Name)
	}
	return out
}

func sortedSignatures(signatures ...[]byte) []string {
	out := []string{}
	for _, signature := range signatures {
		out = append(out, string(signature))
	}
	slices.Sort(out)
	return out
}

func seenSignatures(state model.Machines) []string {
	signatures := [][]byte{}
	for _, seen := range state.Seen {
		signatures = append(signatures, seen.Signature)
	}
	return sortedSignatures(signatures...)
}

func inviteNames(state model.Machines) []string {
	out := []string{}
	for _, i := range state.Invites {
		out = append(out, i.Name)
	}
	return out
}

func TestInviteMakesAnEightCharacterCodeFromTheFirstEightRandomBytesModuloThirtyOne(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		bytes   []byte
		display string
	}{
		{[]byte{0, 1, 2, 3, 4, 5, 6, 7}, "2345-6789"},
		{[]byte{8, 9, 10, 11, 12, 13, 14, 15}, "ABCD-EFGH"},
		{[]byte{31, 62, 93, 124, 155, 186, 217, 248}, "2222-2222"},
		{[]byte{255, 254, 253, 252, 251, 250, 249, 248}, "9876-5432"},
		{[]byte{30, 29, 28, 27, 26, 25, 24, 23}, "ZYXW-VUTS"},
	} {
		_, code, err := newImpl().Invite(model.Machines{}, "box", append(c.bytes, 200, 201, 202), t0, lifetime)
		if err != nil || code != c.display {
			t.Errorf("random %v: %q %v, want %q", c.bytes, code, err, c.display)
		}
	}
}

func TestInviteKeepsThePendingInviteWithTheNameTheCodeAsEightUpperCaseCharactersAndTheExpiry(t *testing.T) {
	t.Parallel()
	got, code, err := newImpl().Invite(model.Machines{}, "linux-box", random(8), t0, lifetime)
	if err != nil || code != "ABCD-EFGH" {
		t.Fatalf("%q %v", code, err)
	}
	want := []model.Invite{{Name: "linux-box", Code: "ABCDEFGH", Expires: t0.Add(lifetime)}}
	if !reflect.DeepEqual(got.Invites, want) {
		t.Fatalf("invites %+v, want %+v", got.Invites, want)
	}
	if len(got.Active) != 0 {
		t.Errorf("an invite made a machine: %+v", got.Active)
	}
}

func TestInviteAcceptsAMachineNameOfOneToThirtyTwoLowercaseLettersDigitsAndHyphensStartingWithALetter(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"a", "linux-box", "box-1", "a1-b2-c3", strings.Repeat("a", 32), "a--b", "a-"} {
		if _, _, err := newImpl().Invite(model.Machines{}, name, random(0), t0, lifetime); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"", "Linux", "1box", "-box", "has space", "under_score", "box.local", strings.Repeat("a", 33), "bóx", "box\n", "all\x00"} {
		if _, _, err := newImpl().Invite(model.Machines{}, name, random(0), t0, lifetime); !errors.Is(err, model.ErrBadMachineName) {
			t.Errorf("%q: %v, want ErrBadMachineName", name, err)
		}
	}
}

func TestInviteWithFewerThanEightRandomBytesIsRefused(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 1, 7} {
		if _, _, err := newImpl().Invite(model.Machines{}, "box", random(0)[:n], t0, lifetime); !errors.Is(err, model.ErrNotEnoughRandom) {
			t.Errorf("%d bytes: %v", n, err)
		}
	}
}

func TestInviteIsRefusedForANameAnActiveMachineHolds(t *testing.T) {
	t.Parallel()
	state := joined(t, model.Machines{}, "linux-box", 8, 1, t0)
	before := snapshot(state)
	got, code, err := newImpl().Invite(state, "linux-box", random(16), t0, lifetime)
	if !errors.Is(err, model.ErrMachineHeld) || code != "" {
		t.Fatalf("%q %v", code, err)
	}
	if !reflect.DeepEqual(snapshot(got), before) {
		t.Error("a refused invite changed the state")
	}
}

func TestInviteIsRefusedForANameAPendingInviteHoldsUntilItExpires(t *testing.T) {
	t.Parallel()
	state, first := invited(t, model.Machines{}, "linux-box", 8, t0)
	if _, _, err := newImpl().Invite(state, "linux-box", random(16), t0.Add(lifetime-time.Nanosecond), lifetime); !errors.Is(err, model.ErrMachineHeld) {
		t.Fatalf("one nanosecond before expiry: %v", err)
	}
	later := t0.Add(lifetime)
	next, second, err := newImpl().Invite(state, "linux-box", random(16), later, lifetime)
	if err != nil || second == first {
		t.Fatalf("at expiry: %q %v", second, err)
	}
	if len(next.Invites) != 1 || next.Invites[0].Code != strings.ReplaceAll(second, "-", "") || !next.Invites[0].Expires.Equal(later.Add(lifetime)) {
		t.Errorf("invites after the expired one was replaced: %+v", next.Invites)
	}
}

func TestInviteKeepsInvitesAndMachinesSortedByNameAndLeavesTheFailureCountAlone(t *testing.T) {
	t.Parallel()
	state := model.Machines{Failures: 3}
	for i, name := range []string{"charlie", "alpha", "bravo"} {
		state, _ = invited(t, state, name, byte(i*8), t0)
	}
	if want := []string{"alpha", "bravo", "charlie"}; !reflect.DeepEqual(inviteNames(state), want) {
		t.Errorf("invites %v, want %v", inviteNames(state), want)
	}
	if state.Failures != 3 {
		t.Errorf("an invite changed the failure count to %d", state.Failures)
	}
}

func TestRedeemMakesTheMachineWithItsKeyAndTheTimeAndUsesUpTheInvite(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "linux-box", 8, t0)
	later := t0.Add(time.Minute)
	got, name, err := newImpl().Redeem(state, code, publicOf(1), later)
	if err != nil || name != "linux-box" {
		t.Fatalf("%q %v", name, err)
	}
	want := []model.Machine{{Name: "linux-box", PublicKey: publicOf(1), Created: later}}
	if !reflect.DeepEqual(got.Active, want) || len(got.Invites) != 0 {
		t.Fatalf("got %+v, want machines %+v and no invites", got, want)
	}
	if _, _, err := newImpl().Redeem(got, code, publicOf(2), later); !errors.Is(err, model.ErrInvalidCode) {
		t.Errorf("the same code a second time: %v", err)
	}
}

func TestRedeemReadsTheCodeInAnyCaseWithOrWithoutTheHyphen(t *testing.T) {
	t.Parallel()
	for _, typed := range []string{"ABCD-EFGH", "abcd-efgh", "ABCDEFGH", "abcdefgh", "AbCd-eFgH"} {
		state, _ := invited(t, model.Machines{}, "box", 8, t0)
		if _, name, err := newImpl().Redeem(state, typed, publicOf(1), t0); err != nil || name != "box" {
			t.Errorf("%q: %q %v", typed, name, err)
		}
	}
}

func TestRedeemWithAnyBadCodeIsInvalidCountsAsOneFailureAndChangesNothingElse(t *testing.T) {
	t.Parallel()
	for _, typed := range []string{"", "-", "WRONGONE", "ABCD-EFG", "ABCD-EFGHJ", "ABCD-EFGI", "ABCD-EFGL", "ABCD-EFGO", "ABCD-EFG0", "ABCD-EFG1", "ABCD EFGH", "ÅBCD-EFGH", "ABCD-EFGH\n"} {
		state, _ := invited(t, model.Machines{}, "box", 8, t0)
		before := snapshot(state)
		got, name, err := newImpl().Redeem(state, typed, publicOf(1), t0)
		if !errors.Is(err, model.ErrInvalidCode) || name != "" {
			t.Errorf("%q: %q %v", typed, name, err)
			continue
		}
		if got.Failures != 1 || !reflect.DeepEqual(got.Invites, before.Invites) || len(got.Active) != 0 {
			t.Errorf("%q: %+v", typed, got)
		}
	}
}

func TestRedeemOfAnExpiredCodeIsInvalidAtTheExpiryAndValidOneNanosecondBefore(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "box", 8, t0)
	if _, _, err := newImpl().Redeem(state, code, publicOf(1), t0.Add(lifetime-time.Nanosecond)); err != nil {
		t.Errorf("one nanosecond before expiry: %v", err)
	}
	got, _, err := newImpl().Redeem(state, code, publicOf(1), t0.Add(lifetime))
	if !errors.Is(err, model.ErrInvalidCode) || got.Failures != 1 || len(got.Active) != 0 {
		t.Errorf("at expiry: %v %+v", err, got)
	}
}

func TestRedeemWithAValidCodeAndAMalformedKeyIsRefusedWithoutUsingUpTheCodeOrCountingAFailure(t *testing.T) {
	t.Parallel()
	for _, key := range [][]byte{nil, {}, bytes.Repeat([]byte{1}, 31), bytes.Repeat([]byte{1}, 33)} {
		state, code := invited(t, model.Machines{Failures: 2}, "box", 8, t0)
		before := snapshot(state)
		got, name, err := newImpl().Redeem(state, code, key, t0)
		if !errors.Is(err, model.ErrBadPublicKey) || name != "" {
			t.Errorf("%d bytes: %q %v", len(key), name, err)
		}
		if !reflect.DeepEqual(snapshot(got), before) {
			t.Errorf("%d bytes: the state changed: %+v", len(key), got)
		}
		if _, _, err := newImpl().Redeem(got, code, publicOf(1), t0); err != nil {
			t.Errorf("%d bytes: the code was used up: %v", len(key), err)
		}
	}
}

func TestRedeemWithAWrongCodeAndAMalformedKeyIsAnInvalidCodeAndNotABadKey(t *testing.T) {
	t.Parallel()
	state, _ := invited(t, model.Machines{}, "box", 8, t0)
	got, _, err := newImpl().Redeem(state, "WRONGONE", []byte{1}, t0)
	if !errors.Is(err, model.ErrInvalidCode) || errors.Is(err, model.ErrBadPublicKey) || got.Failures != 1 {
		t.Errorf("%v %+v", err, got)
	}
}

func TestTheFifthFailedRedeemCancelsEveryWaitingInviteAndTheCountStartsAgainAtZero(t *testing.T) {
	t.Parallel()
	state, _ := invited(t, model.Machines{}, "one", 8, t0)
	state, _ = invited(t, state, "two", 16, t0)
	state = failures(t, state, 4, t0)
	if state.Failures != 4 || len(state.Invites) != 2 {
		t.Fatalf("after four failures: %+v", state)
	}
	state = failures(t, state, 1, t0)
	if state.Failures != 0 || len(state.Invites) != 0 {
		t.Fatalf("after five failures: %+v", state)
	}
}

func TestAfterACancelTheNextInviteSurvivesFourFailures(t *testing.T) {
	t.Parallel()
	state, _ := invited(t, model.Machines{}, "one", 8, t0)
	state = failures(t, state, 5, t0)
	state, code := invited(t, state, "two", 16, t0)
	state = failures(t, state, 4, t0)
	if _, name, err := newImpl().Redeem(state, code, publicOf(1), t0); err != nil || name != "two" {
		t.Fatalf("%q %v", name, err)
	}
}

func TestAGoodRedeemStartsTheFailureCountAgainAtZero(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "one", 8, t0)
	state = failures(t, state, 4, t0)
	state, _, err := newImpl().Redeem(state, code, publicOf(1), t0)
	if err != nil || state.Failures != 0 {
		t.Fatalf("%v %+v", err, state)
	}
	state, code = invited(t, state, "two", 16, t0)
	state = failures(t, state, 4, t0)
	if _, _, err := newImpl().Redeem(state, code, publicOf(2), t0); err != nil {
		t.Errorf("four failures after a good redeem cancelled the invite: %v", err)
	}
}

func TestAnInviteMadeBetweenFailuresDoesNotRestartTheCount(t *testing.T) {
	t.Parallel()
	state, _ := invited(t, model.Machines{}, "one", 8, t0)
	state = failures(t, state, 3, t0)
	state, _ = invited(t, state, "two", 16, t0)
	state = failures(t, state, 2, t0)
	if len(state.Invites) != 0 {
		t.Errorf("five failures with an invite between them left %v", inviteNames(state))
	}
}

func TestRedeemKeepsMachinesSortedByName(t *testing.T) {
	t.Parallel()
	state := model.Machines{}
	for i, name := range []string{"charlie", "alpha", "bravo"} {
		state = joined(t, state, name, byte(i*8), byte(i+1), t0)
	}
	if want := []string{"alpha", "bravo", "charlie"}; !reflect.DeepEqual(names(state), want) {
		t.Errorf("machines %v, want %v", names(state), want)
	}
}

func TestRevokeRemovesAnActiveMachineAndLeavesTheOthers(t *testing.T) {
	t.Parallel()
	state := joined(t, model.Machines{}, "one", 8, 1, t0)
	state = joined(t, state, "two", 16, 2, t0)
	got, err := newImpl().Revoke(state, "one")
	if err != nil || !reflect.DeepEqual(names(got), []string{"two"}) {
		t.Fatalf("%v %v", err, names(got))
	}
}

func TestRevokeRemovesAPendingInviteSoItsCodeStopsWorkingAndItsNameIsFree(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "one", 8, t0)
	state, _ = invited(t, state, "two", 16, t0)
	got, err := newImpl().Revoke(state, "one")
	if err != nil || !reflect.DeepEqual(inviteNames(got), []string{"two"}) {
		t.Fatalf("%v %v", err, inviteNames(got))
	}
	if _, _, err := newImpl().Redeem(got, code, publicOf(1), t0); !errors.Is(err, model.ErrInvalidCode) {
		t.Errorf("the revoked code: %v", err)
	}
	if _, _, err := newImpl().Invite(got, "one", random(24), t0, lifetime); err != nil {
		t.Errorf("the name after a revoke: %v", err)
	}
}

func TestRevokeOfAnUnknownNameIsRefusedAndChangesNothing(t *testing.T) {
	t.Parallel()
	state := joined(t, model.Machines{}, "one", 8, 1, t0)
	state, _ = invited(t, state, "two", 16, t0)
	before := snapshot(state)
	for _, name := range []string{"three", "on", "", "ONE"} {
		got, err := newImpl().Revoke(state, name)
		if !errors.Is(err, model.ErrUnknownMachine) || !reflect.DeepEqual(snapshot(got), before) {
			t.Errorf("%q: %v %+v", name, err, got)
		}
	}
}

func TestRevokeAllRemovesEveryMachineAndEveryPendingInviteAndLeavesTheFailureCountAndTheSeenSignatures(t *testing.T) {
	t.Parallel()
	state := joined(t, model.Machines{}, "one", 8, 1, t0)
	state, _ = invited(t, state, "two", 16, t0)
	state.Failures = 2
	state.Seen = []model.SeenSignature{{Signature: []byte{1}, Until: t0.Add(time.Hour)}}
	got := newImpl().RevokeAll(state)
	if len(got.Active) != 0 || len(got.Invites) != 0 || got.Failures != 2 || len(got.Seen) != 1 {
		t.Errorf("%+v", got)
	}
}

func TestFingerprintIsTheFirstTwelveHexCharactersOfTheSHA256OfTheRawKeyBytes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		key  []byte
		want string
	}{
		{make([]byte, 32), "66687aadf862"},
		{func() []byte {
			out := make([]byte, 32)
			for i := range out {
				out[i] = byte(i)
			}
			return out
		}(), "630dcd2966c4"},
	} {
		if got := newImpl().Fingerprint(c.key); got != c.want {
			t.Errorf("%x: %q, want %q", c.key[:2], got, c.want)
		}
	}
	key := publicOf(7)
	sum := sha256.Sum256(key)
	if got := newImpl().Fingerprint(key); got != hex.EncodeToString(sum[:])[:12] {
		t.Errorf("a real key: %q", got)
	}
}

type signing struct {
	name      string
	key       ed25519.PrivateKey
	unix      int64
	nonce     string
	method    string
	path      string
	body      []byte
	signature []byte
}

func signedBy(name string, key ed25519.PrivateKey, unix int64) signing {
	return signing{name: name, key: key, unix: unix, nonce: "AAAAAAAAAAAAAAAAAAAAAA==", method: "POST", path: "/v1/register", body: []byte(`{"id":"alpha"}`)}
}

func (s signing) text() []byte {
	sum := sha256.Sum256(s.body)
	return []byte(strings.Join([]string{"landline-machine-v1", s.name, strconv.FormatInt(s.unix, 10), s.nonce, s.method, s.path, hex.EncodeToString(sum[:])}, "\n"))
}

func (s signing) request() model.SignedRequest {
	signature := s.signature
	if signature == nil {
		signature = ed25519.Sign(s.key, s.text())
	}
	return model.SignedRequest{Name: s.name, Time: s.unix, Nonce: s.nonce, Signature: signature, Method: s.method, Path: s.path, Body: s.body}
}

func machineState(t *testing.T) model.Machines {
	t.Helper()
	return joined(t, model.Machines{}, "linux-box", 8, 1, t0.Add(-time.Hour))
}

func verify(state model.Machines, s signing, now time.Time) (model.Machines, string, error) {
	return newImpl().Verify(state, s.request(), now, t0.Add(-time.Hour), window)
}

func TestVerifyAcceptsAGoodSignatureAndNamesTheMachine(t *testing.T) {
	t.Parallel()
	_, name, err := verify(machineState(t), signedBy("linux-box", keyOf(1), t0.Unix()), t0)
	if err != nil || name != "linux-box" {
		t.Fatalf("%q %v", name, err)
	}
}

func TestVerifyRefusesAMachineThatIsNotActive(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	if _, _, err := verify(state, signedBy("other-box", keyOf(1), t0.Unix()), t0); !errors.Is(err, model.ErrUnknownMachine) {
		t.Errorf("an unknown name: %v", err)
	}
	pending, _ := invited(t, state, "pending-box", 16, t0)
	if _, _, err := verify(pending, signedBy("pending-box", keyOf(1), t0.Unix()), t0); !errors.Is(err, model.ErrUnknownMachine) {
		t.Errorf("a pending name: %v", err)
	}
	revoked, _ := newImpl().Revoke(state, "linux-box")
	if _, _, err := verify(revoked, signedBy("linux-box", keyOf(1), t0.Unix()), t0); !errors.Is(err, model.ErrUnknownMachine) {
		t.Errorf("a revoked name: %v", err)
	}
}

func TestVerifyRefusesASignatureThatCoversAnyOtherNameTimeNonceMethodPathOrBodyOrCameFromAnotherKey(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	good := signedBy("linux-box", keyOf(1), t0.Unix())
	for label, mutate := range map[string]func(*model.SignedRequest){
		"another time":      func(r *model.SignedRequest) { r.Time++ },
		"another nonce":     func(r *model.SignedRequest) { r.Nonce = "AQEBAQEBAQEBAQEBAQEBAQ==" },
		"another method":    func(r *model.SignedRequest) { r.Method = "GET" },
		"another path":      func(r *model.SignedRequest) { r.Path = "/v1/observers" },
		"a query":           func(r *model.SignedRequest) { r.Path += "?after=0" },
		"another body":      func(r *model.SignedRequest) { r.Body = []byte(`{"id":"bravo"}`) },
		"no body":           func(r *model.SignedRequest) { r.Body = nil },
		"a flipped byte":    func(r *model.SignedRequest) { r.Signature = append([]byte{r.Signature[0] ^ 1}, r.Signature[1:]...) },
		"a short signature": func(r *model.SignedRequest) { r.Signature = r.Signature[:63] },
		"no signature":      func(r *model.SignedRequest) { r.Signature = nil },
	} {
		request := good.request()
		mutate(&request)
		if _, _, err := newImpl().Verify(state, request, t0, t0.Add(-time.Hour), window); !errors.Is(err, model.ErrBadSignature) {
			t.Errorf("%s: %v", label, err)
		}
	}
	if _, _, err := verify(state, signedBy("linux-box", keyOf(2), t0.Unix()), t0); !errors.Is(err, model.ErrBadSignature) {
		t.Errorf("another key: %v", err)
	}
}

func TestVerifySignsTheQueryAsPartOfThePath(t *testing.T) {
	t.Parallel()
	s := signedBy("linux-box", keyOf(1), t0.Unix())
	s.method, s.path, s.body = "GET", "/v1/log?after=7", nil
	if _, _, err := verify(machineState(t), s, t0); err != nil {
		t.Errorf("a signature over the path and its query: %v", err)
	}
}

func TestVerifyAcceptsATimeExactlyTheWindowFromNowInEitherDirectionAndRefusesOneSecondMore(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	for _, offset := range []int64{60, -60} {
		if _, _, err := verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+offset), t0); err != nil {
			t.Errorf("%d seconds: %v", offset, err)
		}
	}
	for _, offset := range []int64{61, -61, 3600, -3600} {
		_, _, err := verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+offset), t0)
		var skew *model.ClockSkewError
		if !errors.As(err, &skew) || skew.Seconds != offset || !strings.Contains(err.Error(), strconv.FormatInt(offset, 10)) || !strings.Contains(strings.ToLower(err.Error()), "clock") {
			t.Errorf("%d seconds: %v", offset, err)
		}
	}
}

func TestVerifyUsesTheWindowItIsGiven(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	request := signedBy("linux-box", keyOf(1), t0.Unix()+5).request()
	if _, _, err := newImpl().Verify(state, request, t0, t0.Add(-time.Hour), 5*time.Second); err != nil {
		t.Errorf("five seconds ahead with a window of five seconds: %v", err)
	}
	var skew *model.ClockSkewError
	if _, _, err := newImpl().Verify(state, request, t0, t0.Add(-time.Hour), 4*time.Second); !errors.As(err, &skew) || skew.Seconds != 5 {
		t.Errorf("five seconds ahead with a window of four seconds: %v", err)
	}
}

func TestVerifyChecksTheWindowBeforeTheStartRule(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	started := t0.Add(-10 * time.Second)
	var skew *model.ClockSkewError
	if _, _, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()-90).request(), t0, started, window); !errors.As(err, &skew) {
		t.Errorf("a time 90 seconds behind and before the start: %v", err)
	}
}

func TestVerifyRefusesEveryTimeUpToFiveSecondsAfterTheStartSecondAndAcceptsTheNextWhateverTheWindow(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	for _, started := range []time.Time{t0, t0.Add(700 * time.Millisecond), t0.Add(999 * time.Millisecond)} {
		for _, window := range []time.Duration{time.Second, time.Second + 900*time.Millisecond, 5 * time.Second, 5*time.Second + 900*time.Millisecond, 60 * time.Second} {
			at := fmt.Sprintf("start %s, window %v", started.Format("15:04:05.000"), window)
			for offset := int64(-1); offset <= 5; offset++ {
				now := t0.Add(time.Duration(offset)*time.Second + 300*time.Millisecond)
				got, name, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+offset).request(), now, started, window)
				if !errors.Is(err, model.ErrBeforeStart) || name != "" || !reflect.DeepEqual(snapshot(got), snapshot(state)) {
					t.Errorf("%s: a time %d seconds after the start second, arriving with it: %q %v, want model.ErrBeforeStart and the value unchanged", at, offset, name, err)
				}
			}
			late := t0.Add(6*time.Second + 300*time.Millisecond)
			if _, _, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+5).request(), late, started, window); !errors.Is(err, model.ErrBeforeStart) {
				t.Errorf("%s: a time 5 seconds after the start second, arriving one second after the period: %v, want model.ErrBeforeStart, because the rule reads the time in the request", at, err)
			}
			if _, name, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+6).request(), late, started, window); err != nil || name != "linux-box" {
				t.Errorf("%s: a time 6 seconds after the start second: %q %v, want it accepted", at, name, err)
			}
		}
	}
}

func TestVerifyReadsTheStartPeriodFromTheTimeInTheRequestAndNotFromTheMomentItArrives(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	started := t0.Add(700 * time.Millisecond)
	window := 5 * time.Second
	if _, _, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+5).request(), t0.Add(10*time.Second+300*time.Millisecond), started, window); !errors.Is(err, model.ErrBeforeStart) {
		t.Errorf("a time in the start period arriving 5 seconds after it: %v, want model.ErrBeforeStart", err)
	}
	if _, name, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+6).request(), t0.Add(1300*time.Millisecond), started, window); err != nil || name != "linux-box" {
		t.Errorf("a time after the start period from a clock 5 seconds ahead, arriving inside the period: %q %v, want it accepted", name, err)
	}
}

func TestVerifyRefusesACopyThatTheProcessBeforeARestartAcceptedFromAMachineWhoseClockRunsAhead(t *testing.T) {
	t.Parallel()
	window := 5 * time.Second
	fresh := machineState(t)
	oldStart := t0.Add(-time.Hour)
	oldNow := t0.Add(500 * time.Millisecond)
	restarted := t0.Add(800 * time.Millisecond)
	copyArrives := t0.Add(time.Second)
	for ahead := int64(1); ahead <= 5; ahead++ {
		request := signedBy("linux-box", keyOf(1), t0.Unix()+ahead).request()
		if _, _, err := newImpl().Verify(fresh, request, oldNow, oldStart, window); err != nil {
			t.Fatalf("the process before the restart refused a clock %d seconds ahead: %v", ahead, err)
		}
		if _, name, err := newImpl().Verify(fresh, request, copyArrives, restarted, window); !errors.Is(err, model.ErrBeforeStart) {
			t.Errorf("a copy of a request %d seconds ahead, which the process before the restart accepted, reached a process with an empty memory: %q %v, want model.ErrBeforeStart", ahead, name, err)
		}
	}
}

func TestVerifyKeepsTheOrderOfItsErrorsForATimeInTheStartPeriodAfterTheStartSecond(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	now := t0.Add(3300 * time.Millisecond)
	if _, _, err := newImpl().Verify(state, signedBy("ghost-box", keyOf(1), t0.Unix()+3).request(), now, t0, 5*time.Second); !errors.Is(err, model.ErrUnknownMachine) {
		t.Errorf("an unknown name with a time in the start period: %v, want model.ErrUnknownMachine", err)
	}
	if _, _, err := newImpl().Verify(state, signedBy("linux-box", keyOf(2), t0.Unix()+3).request(), now, t0, 5*time.Second); !errors.Is(err, model.ErrBeforeStart) {
		t.Errorf("a bad signature with a time in the start period: %v, want model.ErrBeforeStart", err)
	}
}

func TestVerifyRefusesEveryTimeWhenTheStartPeriodRunsPastTheLargestTimeWithoutWrapping(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	for _, startSecond := range []int64{math.MaxInt64 - 4, math.MaxInt64 - 2, math.MaxInt64} {
		started := time.Unix(startSecond, 0)
		now := time.Unix(math.MaxInt64, 0)
		if started.Unix() != startSecond || now.Unix() != math.MaxInt64 {
			t.Fatalf("fixture error: a time with %d seconds reads back as %d", startSecond, started.Unix())
		}
		for _, request := range []int64{math.MaxInt64 - 1, math.MaxInt64} {
			got, name, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), request).request(), now, started, 5*time.Second)
			if !errors.Is(err, model.ErrBeforeStart) || name != "" || !reflect.DeepEqual(snapshot(got), snapshot(state)) {
				t.Errorf("start %d, time %d: %q %v, want model.ErrBeforeStart, because the time is not later than the start plus 5 seconds", startSecond, request, name, err)
			}
		}
	}
}

func TestVerifyRefusesTheSameSignatureASecondTimeAndAcceptsAnotherNonce(t *testing.T) {
	t.Parallel()
	first := signedBy("linux-box", keyOf(1), t0.Unix())
	state, _, err := verify(machineState(t), first, t0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := verify(state, first, t0); !errors.Is(err, model.ErrReplayed) {
		t.Errorf("the same request again: %v", err)
	}
	if _, _, err := verify(state, first, t0.Add(time.Second)); !errors.Is(err, model.ErrReplayed) {
		t.Errorf("the same request a second later: %v", err)
	}
	second := first
	second.nonce = "AgICAgICAgICAgICAgICAg=="
	if _, _, err := verify(state, second, t0); err != nil {
		t.Errorf("the same request with another nonce: %v", err)
	}
}

func TestVerifyDropsRememberedSignaturesThatHaveRunOutWhenItAcceptsAnother(t *testing.T) {
	t.Parallel()
	state, _, err := verify(machineState(t), signedBy("linux-box", keyOf(1), t0.Unix()), t0)
	if err != nil {
		t.Fatal(err)
	}
	later := t0.Add(2 * window)
	second := signedBy("linux-box", keyOf(1), later.Unix())
	second.nonce = "AwMDAwMDAwMDAwMDAwMDAw=="
	got, _, err := verify(state, second, later)
	if err != nil || len(got.Seen) != 1 {
		t.Fatalf("%v seen %+v", err, got.Seen)
	}
}

func TestAFailedVerifyReturnsTheStateItWasGivenUnchanged(t *testing.T) {
	t.Parallel()
	good := signedBy("linux-box", keyOf(1), t0.Unix())
	state, _, _ := verify(machineState(t), good, t0)
	before := snapshot(state)
	for label, s := range map[string]signing{
		"a replay":       good,
		"a bad key":      signedBy("linux-box", keyOf(2), t0.Unix()),
		"a clock error":  signedBy("linux-box", keyOf(1), t0.Unix()+999),
		"an unknown one": signedBy("other-box", keyOf(1), t0.Unix()),
	} {
		got, name, err := verify(state, s, t0)
		if err == nil || name != "" || !reflect.DeepEqual(snapshot(got), before) {
			t.Errorf("%s: %q %v %+v", label, name, err, got)
		}
	}
}

func TestInviteUsesTheLifetimeItIsGiven(t *testing.T) {
	t.Parallel()
	state, code, err := newImpl().Invite(model.Machines{}, "box", random(8), t0, 90*time.Second)
	if err != nil || len(state.Invites) != 1 || !state.Invites[0].Expires.Equal(t0.Add(90*time.Second)) {
		t.Fatalf("%v %+v", err, state.Invites)
	}
	if _, _, err := newImpl().Redeem(state, code, publicOf(1), t0.Add(90*time.Second-time.Nanosecond)); err != nil {
		t.Errorf("one nanosecond before a 90 second expiry: %v", err)
	}
	if _, _, err := newImpl().Redeem(state, code, publicOf(1), t0.Add(90*time.Second)); !errors.Is(err, model.ErrInvalidCode) {
		t.Errorf("at a 90 second expiry: %v", err)
	}
}

func TestInviteRefusesACodeThatAnotherPendingInviteAlreadyHoldsAndKeepsTheFirstInvite(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "one", 8, t0)
	before := snapshot(state)
	got, second, err := newImpl().Invite(state, "two", random(8), t0, lifetime)
	if !errors.Is(err, model.ErrCodeInUse) || second != "" || !reflect.DeepEqual(snapshot(got), before) {
		t.Fatalf("%q %v %+v", second, err, got)
	}
	if _, name, err := newImpl().Redeem(got, code, publicOf(1), t0); err != nil || name != "one" {
		t.Errorf("the first invite after the refusal: %q %v", name, err)
	}
	if _, _, err := newImpl().Invite(state, "two", random(9), t0, lifetime); err != nil {
		t.Errorf("another random value: %v", err)
	}
	if _, _, err := newImpl().Invite(state, "two", random(8), t0.Add(lifetime), lifetime); err != nil {
		t.Errorf("the same code once the first invite has expired: %v", err)
	}
}

func richState(t *testing.T) (model.Machines, []string) {
	t.Helper()
	state := joined(t, model.Machines{}, "alpha-box", 8, 1, t0.Add(-time.Hour))
	state = joined(t, state, "bravo-box", 16, 2, t0.Add(-time.Hour))
	var codes []string
	for i, name := range []string{"one-box", "two-box", "three-box"} {
		var code string
		state, code = invited(t, state, name, byte(40+8*i), t0)
		codes = append(codes, code)
	}
	state.Failures = 2
	state.Seen = []model.SeenSignature{{Signature: []byte{1, 1}, Until: t0.Add(time.Hour)}, {Signature: []byte{2, 2}, Until: t0.Add(2 * time.Hour)}}
	return state, codes
}

func TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone(t *testing.T) {
	t.Parallel()
	state, codes := richState(t)
	before := snapshot(state)
	r := newImpl()

	got, _ := r.Revoke(state, "alpha-box")
	if !reflect.DeepEqual(names(got), []string{"bravo-box"}) || !reflect.DeepEqual(got.Invites, before.Invites) || !reflect.DeepEqual(got.Seen, before.Seen) || got.Failures != 2 {
		t.Errorf("Revoke of a machine: %+v", got)
	}
	got, _ = r.Revoke(state, "two-box")
	if !reflect.DeepEqual(inviteNames(got), []string{"one-box", "three-box"}) || !reflect.DeepEqual(got.Active, before.Active) || !reflect.DeepEqual(got.Seen, before.Seen) || got.Failures != 2 {
		t.Errorf("Revoke of an invite: %+v", got)
	}

	got, name, _ := r.Redeem(state, codes[1], publicOf(3), t0)
	if name != "two-box" || !reflect.DeepEqual(names(got), []string{"alpha-box", "bravo-box", "two-box"}) || !reflect.DeepEqual(inviteNames(got), []string{"one-box", "three-box"}) || !reflect.DeepEqual(got.Seen, before.Seen) || got.Failures != 0 {
		t.Errorf("a good Redeem: %q %+v", name, got)
	}
	got, _, _ = r.Redeem(state, "WRONGONE", publicOf(3), t0)
	if !reflect.DeepEqual(got.Active, before.Active) || !reflect.DeepEqual(got.Invites, before.Invites) || !reflect.DeepEqual(got.Seen, before.Seen) || got.Failures != 3 {
		t.Errorf("a failed Redeem: %+v", got)
	}
	state.Failures = 4
	got, _, _ = r.Redeem(state, "WRONGONE", publicOf(3), t0)
	if !reflect.DeepEqual(got.Active, before.Active) || len(got.Invites) != 0 || !reflect.DeepEqual(got.Seen, before.Seen) || got.Failures != 0 {
		t.Errorf("the fifth failed Redeem: %+v", got)
	}
	state.Failures = 2

	got, _, _ = r.Invite(state, "four-box", random(72), t0, lifetime)
	if !reflect.DeepEqual(got.Active, before.Active) || !reflect.DeepEqual(got.Seen, before.Seen) || got.Failures != 2 || !reflect.DeepEqual(inviteNames(got), []string{"four-box", "one-box", "three-box", "two-box"}) {
		t.Errorf("Invite: %+v", got)
	}

	got, name, err := r.Verify(state, signedBy("alpha-box", keyOf(1), t0.Unix()).request(), t0, t0.Add(-time.Hour), window)
	if err != nil || name != "alpha-box" || !reflect.DeepEqual(got.Active, before.Active) || !reflect.DeepEqual(got.Invites, before.Invites) || got.Failures != 2 || len(got.Seen) != 3 {
		t.Errorf("Verify: %q %v %+v", name, err, got)
	}
}

func TestNoFunctionChangesTheStateOrTheBytesItWasGiven(t *testing.T) {
	t.Parallel()
	state, codes := richState(t)
	before := snapshot(state)
	key := publicOf(3)
	keyBefore := append([]byte(nil), key...)
	request := signedBy("alpha-box", keyOf(1), t0.Unix()).request()
	bodyBefore := append([]byte(nil), request.Body...)
	signatureBefore := append([]byte(nil), request.Signature...)
	randomBytes := random(100)
	randomBefore := append([]byte(nil), randomBytes...)
	r := newImpl()
	r.Invite(state, "four-box", randomBytes, t0, lifetime)
	r.Redeem(state, codes[0], key, t0)
	r.Redeem(state, "WRONGONE", key, t0)
	r.Revoke(state, "alpha-box")
	r.Revoke(state, "two-box")
	r.RevokeAll(state)
	r.Verify(state, request, t0, t0.Add(-time.Hour), window)
	later := signedBy("alpha-box", keyOf(1), t0.Add(3*time.Hour).Unix()).request()
	r.Verify(state, later, t0.Add(3*time.Hour), t0.Add(-time.Hour), window)
	if !reflect.DeepEqual(snapshot(state), before) {
		t.Error("a function changed the value it was given")
	}
	if !bytes.Equal(key, keyBefore) || !bytes.Equal(request.Body, bodyBefore) || !bytes.Equal(request.Signature, signatureBefore) || !bytes.Equal(randomBytes, randomBefore) {
		t.Error("a function changed a byte slice it was given")
	}
}

func TestRedeemReadsEveryHyphenAndOnlyASCIILettersAsUpperCase(t *testing.T) {
	t.Parallel()
	for _, typed := range []string{"ABCDE-FGH", "ABCDEFGH-", "-ABCDEFGH", "A-BCDEFGH", "ABCD--EFGH"} {
		state, _ := invited(t, model.Machines{}, "box", 8, t0)
		if _, name, err := newImpl().Redeem(state, typed, publicOf(1), t0); err != nil || name != "box" {
			t.Errorf("%q: %q %v", typed, name, err)
		}
	}
	state, _ := invited(t, model.Machines{}, "box", 16, t0)
	for _, typed := range []string{"JKMNPQR\u017f", "jkmnpqr\u017f", "J\u212aMNPQRS", "\uff2a\uff2b\uff2d\uff2e\uff30\uff31\uff32\uff33"} {
		if _, _, err := newImpl().Redeem(state, typed, publicOf(1), t0); !errors.Is(err, model.ErrInvalidCode) {
			t.Errorf("%q: %v", typed, err)
		}
	}
}

func TestRedeemOfAnExpiredCodeWithAMalformedKeyIsAnInvalidCode(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "box", 8, t0)
	got, _, err := newImpl().Redeem(state, code, []byte{1}, t0.Add(lifetime))
	if !errors.Is(err, model.ErrInvalidCode) || got.Failures != 1 {
		t.Errorf("%v %+v", err, got)
	}
}

func TestTheInvalidCodeErrorSaysInvalidOrExpiredInviteCode(t *testing.T) {
	t.Parallel()
	if got := model.ErrInvalidCode.Error(); got != "invalid or expired invite code" {
		t.Errorf("%q", got)
	}
}

func notActiveStates(t *testing.T) map[string]model.Machines {
	t.Helper()
	known := machineState(t)
	pending, _ := invited(t, known, "pending-box", 16, t0)
	revoked, err := newImpl().Revoke(known, "linux-box")
	if err != nil {
		t.Fatal(err)
	}
	return map[string]model.Machines{"a known machine": known, "a pending name": pending, "a revoked name": revoked}
}

func notActiveNames() map[string]string {
	return map[string]string{"a known machine": "linux-box", "a pending name": "pending-box", "a revoked name": "linux-box"}
}

func TestVerifyReportsAClockErrorForAnyNameWhenTheTimeIsOutsideTheWindow(t *testing.T) {
	t.Parallel()
	held := notActiveNames()
	for label, state := range notActiveStates(t) {
		for _, name := range []string{held[label], "ghost-box"} {
			for _, offset := range []int64{61, -61, 999, -999, 3600} {
				got, who, err := verify(state, signedBy(name, keyOf(1), t0.Unix()+offset), t0)
				var skew *model.ClockSkewError
				if !errors.As(err, &skew) || errors.Is(err, model.ErrUnknownMachine) || skew.Seconds != offset || who != "" {
					t.Errorf("%s, name %s, %d seconds: %q %v", label, name, offset, who, err)
				}
				if !reflect.DeepEqual(snapshot(got), snapshot(state)) {
					t.Errorf("%s, name %s, %d seconds: the state changed", label, name, offset)
				}
			}
		}
	}
}

func TestVerifyGivesTheSameClockErrorWhetherOrNotTheNameBelongsToAMachine(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	for _, offset := range []int64{61, -61, 5000} {
		_, _, known := verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+offset), t0)
		_, _, unknown := verify(state, signedBy("ghost-box", keyOf(9), t0.Unix()+offset), t0)
		if known == nil || unknown == nil || known.Error() != unknown.Error() {
			t.Errorf("%d seconds: a known name gets %v and an unknown name gets %v", offset, known, unknown)
		}
	}
}

func TestVerifyReportsAnUnknownMachineForAnyNameThatIsNotActiveWhenTheTimeIsInsideTheWindow(t *testing.T) {
	t.Parallel()
	for label, state := range notActiveStates(t) {
		if label == "a known machine" {
			continue
		}
		name := notActiveNames()[label]
		for _, offset := range []int64{0, 60, -60} {
			_, who, err := verify(state, signedBy(name, keyOf(1), t0.Unix()+offset), t0)
			var skew *model.ClockSkewError
			if !errors.Is(err, model.ErrUnknownMachine) || errors.As(err, &skew) || who != "" {
				t.Errorf("%s, %d seconds: %q %v", label, offset, who, err)
			}
		}
	}
	for _, offset := range []int64{0, 60, -60} {
		if _, _, err := verify(machineState(t), signedBy("ghost-box", keyOf(1), t0.Unix()+offset), t0); !errors.Is(err, model.ErrUnknownMachine) {
			t.Errorf("a name that never existed, %d seconds: %v", offset, err)
		}
	}
}

func TestVerifyReportsAnUnknownMachineBeforeATimeBeforeTheStart(t *testing.T) {
	t.Parallel()
	started := t0.Add(-5 * time.Second)
	request := signedBy("ghost-box", keyOf(1), t0.Unix()-10).request()
	if _, _, err := newImpl().Verify(machineState(t), request, t0, started, window); !errors.Is(err, model.ErrUnknownMachine) {
		t.Errorf("an unknown name with a time inside the window and before the start: %v", err)
	}
}

func TestVerifyReportsATimeBeforeTheStartBeforeABadSignature(t *testing.T) {
	t.Parallel()
	started := t0.Add(-5 * time.Second)
	_, _, err := newImpl().Verify(machineState(t), signedBy("linux-box", keyOf(2), t0.Unix()-10).request(), t0, started, window)
	if !errors.Is(err, model.ErrBeforeStart) {
		t.Errorf("%v", err)
	}
}

func TestVerifyCountsTheWindowInWholeSecondsOfNowWhateverTheFraction(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	now := t0.Add(900 * time.Millisecond)
	for _, offset := range []int64{60, -60} {
		if _, _, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+offset).request(), now, t0.Add(-time.Hour), window); err != nil {
			t.Errorf("%d seconds with a fraction of a second in now: %v", offset, err)
		}
	}
	for _, offset := range []int64{61, -61} {
		if _, _, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+offset).request(), now, t0.Add(-time.Hour), window); err == nil {
			t.Errorf("%d seconds was accepted", offset)
		}
	}
}

func TestVerifyRemembersEverySignatureItAcceptsForTheirWholeMemory(t *testing.T) {
	t.Parallel()
	first := signedBy("linux-box", keyOf(1), t0.Unix())
	second := first
	second.nonce = "AgICAgICAgICAgICAgICAg=="
	state, _, err := verify(machineState(t), first, t0)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = verify(state, second, t0.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for label, s := range map[string]signing{"the first": first, "the second": second} {
		if _, _, err := verify(state, s, t0.Add(2*time.Second)); !errors.Is(err, model.ErrReplayed) {
			t.Errorf("a replay of %s: %v", label, err)
		}
	}
}

func TestVerifyRemembersASignatureUntilOneSecondAfterItsTimePlusTheWindow(t *testing.T) {
	t.Parallel()
	short := 4 * time.Second
	for offset, want := range map[int64]int64{3: 8, 0: 5, -4: 1} {
		request := signedBy("linux-box", keyOf(1), t0.Unix()+offset).request()
		state, _, err := newImpl().Verify(machineState(t), request, t0, t0.Add(-time.Hour), short)
		if err != nil {
			t.Fatalf("time %+d: %v", offset, err)
		}
		if len(state.Seen) != 1 || !state.Seen[0].Until.Equal(time.Unix(t0.Unix()+want, 0)) {
			t.Errorf("time %+d: seen %+v, want one entry that lasts until %d seconds after t0", offset, state.Seen, want)
		}
	}
}

func TestVerifyRefusesACopyUntilItsMemoryRunsOutAndNeverAcceptsItAfter(t *testing.T) {
	t.Parallel()
	short := 4 * time.Second
	request := signedBy("linux-box", keyOf(1), t0.Unix()+4).request()
	state, _, err := newImpl().Verify(machineState(t), request, t0, t0.Add(-time.Hour), short)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Unix(t0.Unix()+9, 0)
	if _, _, err := newImpl().Verify(state, request, until.Add(-time.Nanosecond), t0.Add(-time.Hour), short); !errors.Is(err, model.ErrReplayed) {
		t.Errorf("one nanosecond before the memory runs out: %v", err)
	}
	var skew *model.ClockSkewError
	if _, _, err := newImpl().Verify(state, request, until, t0.Add(-time.Hour), short); !errors.As(err, &skew) {
		t.Errorf("at the moment the memory runs out the copy must be refused as out of the window: %v", err)
	}
}

func TestEveryFunctionGivesTheSameResultForTheSameArgumentsOnOneInstanceWhateverRanBetween(t *testing.T) {
	t.Parallel()
	state, codes := richState(t)
	request := signedBy("alpha-box", keyOf(1), t0.Unix()).request()
	r := newImpl()
	calls := []func() []any{
		func() []any { a, b, c := r.Invite(state, "four-box", random(72), t0, lifetime); return []any{a, b, c} },
		func() []any { a, b, c := r.Redeem(state, codes[0], publicOf(3), t0); return []any{a, b, c} },
		func() []any { a, b := r.Revoke(state, "one-box"); return []any{a, b} },
		func() []any { return []any{r.RevokeAll(state)} },
		func() []any {
			a, b, c := r.Verify(state, request, t0, t0.Add(-time.Hour), window)
			return []any{a, b, c}
		},
		func() []any { return []any{r.Fingerprint(publicOf(1))} },
	}
	for i, call := range calls {
		first := call()
		r.Redeem(state, "WRONGONE", publicOf(1), t0)
		r.Verify(state, request, t0, t0.Add(-time.Hour), window)
		r.Invite(state, "other-box", random(88), t0, lifetime)
		if second := call(); !reflect.DeepEqual(first, second) {
			t.Errorf("call %d: two identical calls differ: %+v and %+v", i, first, second)
		}
	}
}

func TestARedeemedKeyIsACopyOfTheBytesTheCallerPassed(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "box", 8, t0)
	key := append([]byte(nil), publicOf(1)...)
	got, _, err := newImpl().Redeem(state, code, key, t0)
	if err != nil || len(got.Active) != 1 {
		t.Fatalf("%v %+v", err, got.Active)
	}
	key[0] ^= 0xff
	if !bytes.Equal(got.Active[0].PublicKey, publicOf(1)) {
		t.Error("the stored key changed when the caller changed its slice")
	}
}

func TestARememberedSignatureIsACopyOfTheBytesTheCallerPassed(t *testing.T) {
	t.Parallel()
	request := signedBy("linux-box", keyOf(1), t0.Unix()).request()
	original := append([]byte(nil), request.Signature...)
	state, _, err := newImpl().Verify(machineState(t), request, t0, t0.Add(-time.Hour), window)
	if err != nil {
		t.Fatal(err)
	}
	request.Signature[0] ^= 0xff
	again := signedBy("linux-box", keyOf(1), t0.Unix()).request()
	if !bytes.Equal(again.Signature, original) {
		t.Fatal("fixture error")
	}
	if _, _, err := newImpl().Verify(state, again, t0, t0.Add(-time.Hour), window); !errors.Is(err, model.ErrReplayed) {
		t.Errorf("the remembered signature changed with the caller's slice: %v", err)
	}
}

func TestTwoResultsBuiltFromOneStateWithSpareCapacityDoNotShareStorage(t *testing.T) {
	t.Parallel()
	state, codes := richState(t)
	state.Active = append(make([]model.Machine, 0, 20), state.Active...)
	state.Invites = append(make([]model.Invite, 0, 20), state.Invites...)
	state.Seen = append(make([]model.SeenSignature, 0, 20), state.Seen...)
	r := newImpl()
	one, _, _ := r.Redeem(state, codes[0], publicOf(3), t0)
	two, _, _ := r.Redeem(state, codes[1], publicOf(4), t0)
	if !reflect.DeepEqual(names(one), []string{"alpha-box", "bravo-box", "one-box"}) || !reflect.DeepEqual(names(two), []string{"alpha-box", "bravo-box", "two-box"}) {
		t.Errorf("redeems from one state: %v and %v", names(one), names(two))
	}
	a, _, _ := r.Invite(state, "four-box", random(72), t0, lifetime)
	b, _, _ := r.Invite(state, "five-box", random(88), t0, lifetime)
	if !reflect.DeepEqual(inviteNames(a), []string{"four-box", "one-box", "three-box", "two-box"}) || !reflect.DeepEqual(inviteNames(b), []string{"five-box", "one-box", "three-box", "two-box"}) {
		t.Errorf("invites from one state: %v and %v", inviteNames(a), inviteNames(b))
	}
	x, _, _ := r.Verify(state, signedBy("alpha-box", keyOf(1), t0.Unix()).request(), t0, t0.Add(-time.Hour), window)
	second := signedBy("alpha-box", keyOf(1), t0.Unix())
	second.nonce = "AgICAgICAgICAgICAgICAg=="
	y, _, _ := r.Verify(state, second.request(), t0, t0.Add(-time.Hour), window)
	xSignature := signedBy("alpha-box", keyOf(1), t0.Unix()).request().Signature
	ySignature := second.request().Signature
	if got, want := seenSignatures(x), sortedSignatures([]byte{1, 1}, []byte{2, 2}, xSignature); !reflect.DeepEqual(got, want) {
		t.Errorf("the first verify remembers %x, want %x", got, want)
	}
	if got, want := seenSignatures(y), sortedSignatures([]byte{1, 1}, []byte{2, 2}, ySignature); !reflect.DeepEqual(got, want) {
		t.Errorf("the second verify remembers %x, want %x", got, want)
	}
}

func TestVerifyChecksEachMachineWithItsOwnKey(t *testing.T) {
	t.Parallel()
	state := joined(t, model.Machines{}, "alpha-box", 8, 1, t0.Add(-time.Hour))
	state = joined(t, state, "bravo-box", 16, 2, t0.Add(-time.Hour))
	state = joined(t, state, "charlie-box", 24, 3, t0.Add(-time.Hour))
	for name, key := range map[string]byte{"alpha-box": 1, "bravo-box": 2, "charlie-box": 3} {
		if _, got, err := verify(state, signedBy(name, keyOf(key), t0.Unix()), t0); err != nil || got != name {
			t.Errorf("%s with its own key: %q %v", name, got, err)
		}
	}
	if _, _, err := verify(state, signedBy("bravo-box", keyOf(1), t0.Unix()), t0); !errors.Is(err, model.ErrBadSignature) {
		t.Errorf("bravo-box signed with alpha-box's key: %v", err)
	}
}

func TestRedeemTakesTheLiveInviteWhenAnExpiredOneHasTheSameCode(t *testing.T) {
	t.Parallel()
	state, code := invited(t, model.Machines{}, "one", 8, t0)
	state, same, err := newImpl().Invite(state, "two", random(8), t0.Add(lifetime), lifetime)
	if err != nil || same != code {
		t.Fatalf("%q %v", same, err)
	}
	got, name, err := newImpl().Redeem(state, code, publicOf(1), t0.Add(lifetime))
	if err != nil || name != "two" || got.Failures != 0 {
		t.Errorf("%q %v %+v", name, err, got)
	}
}

func TestFailedRedeemsAreCountedWhenNoInviteIsWaiting(t *testing.T) {
	t.Parallel()
	state := failures(t, model.Machines{}, 4, t0)
	if state.Failures != 4 {
		t.Fatalf("after four failures with no invite: %+v", state)
	}
	state, _ = invited(t, state, "box", 8, t0)
	state = failures(t, state, 1, t0)
	if len(state.Invites) != 0 || state.Failures != 0 {
		t.Errorf("the fifth failure did not cancel the invite made after the first four: %+v", state)
	}
}

func TestVerifyRefusesAChangedRequestWithARememberedSignatureAsABadSignatureAndNotAReplay(t *testing.T) {
	t.Parallel()
	first := signedBy("linux-box", keyOf(1), t0.Unix())
	state, _, err := verify(machineState(t), first, t0)
	if err != nil {
		t.Fatal(err)
	}
	changed := first.request()
	changed.Nonce = "AgICAgICAgICAgICAgICAg=="
	if _, _, err := newImpl().Verify(state, changed, t0, t0.Add(-time.Hour), window); !errors.Is(err, model.ErrBadSignature) {
		t.Errorf("%v", err)
	}
}

func TestVerifyTakesTheNonceAsAnOpaqueStringAndSignsItAsGiven(t *testing.T) {
	t.Parallel()
	for _, nonce := range []string{"", "not base64 at all", "\u00e9\u4e16", strings.Repeat("A", 500)} {
		s := signedBy("linux-box", keyOf(1), t0.Unix())
		s.nonce = nonce
		if _, _, err := verify(machineState(t), s, t0); err != nil {
			t.Errorf("nonce %.20q: %v", nonce, err)
		}
	}
}

func TestVerifyNeverVerifiesAgainstAStoredKeyThatIsNotThirtyTwoBytes(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	state.Active[0].PublicKey = []byte{1, 2, 3}
	if _, _, err := verify(state, signedBy("linux-box", keyOf(1), t0.Unix()), t0); !errors.Is(err, model.ErrBadSignature) {
		t.Errorf("%v", err)
	}
	state.Active[0].PublicKey = nil
	if _, _, err := verify(state, signedBy("linux-box", keyOf(1), t0.Unix()), t0); !errors.Is(err, model.ErrBadSignature) {
		t.Errorf("a missing key: %v", err)
	}
}

func TestVerifyRefusesATimeAtTheLimitsOfTheIntegerRangeAsOutOfTheWindowWithoutOverflowing(t *testing.T) {
	t.Parallel()
	for _, unix := range []int64{math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1} {
		var skew *model.ClockSkewError
		if _, _, err := verify(machineState(t), signedBy("linux-box", keyOf(1), unix), t0); !errors.As(err, &skew) {
			t.Errorf("time %d: %v", unix, err)
		}
	}
}

func TestInviteWithExactlyEightRandomBytesIsAcceptedAndMakesTheCodeFromThem(t *testing.T) {
	t.Parallel()
	for _, n := range []int{8, 9} {
		got, code, err := newImpl().Invite(model.Machines{}, "box", random(0)[:n], t0, lifetime)
		if err != nil || code != "2345-6789" {
			t.Errorf("%d bytes: %q %v", n, code, err)
			continue
		}
		want := []model.Invite{{Name: "box", Code: "23456789", Expires: t0.Add(lifetime)}}
		if !reflect.DeepEqual(got.Invites, want) {
			t.Errorf("%d bytes: invites %+v, want %+v", n, got.Invites, want)
		}
	}
}

func TestRedeemRefusesACodeTypedWithAnySeparatorOtherThanAHyphenSuchAsAnUnderscoreOrATab(t *testing.T) {
	t.Parallel()
	var typed []string
	for c := 0; c < 128; c++ {
		switch {
		case c == '-', '0' <= c && c <= '9', 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
			continue
		}
		separator := string(rune(c))
		typed = append(typed, "2345"+separator+"6789", separator+"23456789", "23456789"+separator)
	}
	for _, separator := range []string{"\u00a0", "\u0085", "\u2003", "\u2028", "\u200b", "\ufeff"} {
		typed = append(typed, "2345"+separator+"6789", separator+"23456789", "23456789"+separator)
	}
	typed = append(typed, "2345_6789", "2345\t6789", "2345 6789", " 2345-6789", "2345-6789 ", "2345-6789\t", "2345-6789\r\n", "2345\n6789")
	for _, each := range typed {
		state, _ := invited(t, model.Machines{}, "box", 0, t0)
		got, name, err := newImpl().Redeem(state, each, publicOf(1), t0)
		if !errors.Is(err, model.ErrInvalidCode) || name != "" || got.Failures != 1 || len(got.Active) != 0 || len(got.Invites) != 1 {
			t.Errorf("%q: %q %v %+v", each, name, err, got)
		}
	}
	state, _ := invited(t, model.Machines{}, "box", 0, t0)
	for _, each := range []string{"2345-6789", "23456789", "2345--6789", "-2345-6789-"} {
		if _, name, err := newImpl().Redeem(state, each, publicOf(1), t0); err != nil || name != "box" {
			t.Errorf("control %q: %q %v", each, name, err)
		}
	}
}

func TestVerifyDoesNotTreatASignatureRememberedUntilExactlyNowAsAReplay(t *testing.T) {
	t.Parallel()
	now := t0.Add(500 * time.Millisecond)
	request := signedBy("linux-box", keyOf(1), now.Unix()).request()
	for _, c := range []struct {
		until  time.Duration
		replay bool
	}{
		{-time.Second, false},
		{-time.Nanosecond, false},
		{0, false},
		{time.Nanosecond, true},
		{time.Second, true},
	} {
		state := machineState(t)
		state.Seen = []model.SeenSignature{{Signature: request.Signature, Until: now.Add(c.until)}}
		_, name, err := newImpl().Verify(state, request, now, t0.Add(-time.Hour), window)
		if c.replay && (!errors.Is(err, model.ErrReplayed) || name != "") || !c.replay && (err != nil || name != "linux-box") {
			t.Errorf("remembered until now%+v: %q %v", c.until, name, err)
		}
	}
}

func TestVerifyDropsARememberedSignatureWhoseMemoryEndsExactlyNowAndKeepsOneThatEndsANanosecondLater(t *testing.T) {
	t.Parallel()
	now := t0.Add(500 * time.Millisecond)
	state := machineState(t)
	state.Seen = []model.SeenSignature{
		{Signature: []byte{1, 1}, Until: now.Add(-time.Nanosecond)},
		{Signature: []byte{2, 2}, Until: now},
		{Signature: []byte{3, 3}, Until: now.Add(time.Nanosecond)},
	}
	request := signedBy("linux-box", keyOf(1), now.Unix()).request()
	got, name, err := newImpl().Verify(state, request, now, t0.Add(-time.Hour), window)
	if err != nil || name != "linux-box" {
		t.Fatalf("%q %v", name, err)
	}
	kept := map[string]time.Time{}
	for _, seen := range got.Seen {
		kept[string(seen.Signature)] = seen.Until
	}
	if len(got.Seen) != 2 || len(kept) != 2 {
		t.Fatalf("seen %+v, want the entry that ends a nanosecond after now and the new signature", got.Seen)
	}
	if until, ok := kept[string([]byte{3, 3})]; !ok || !until.Equal(now.Add(time.Nanosecond)) {
		t.Errorf("the entry that ends a nanosecond after now: %v %v", until, ok)
	}
	if until, ok := kept[string(request.Signature)]; !ok || !until.Equal(time.Unix(now.Unix()+61, 0)) {
		t.Errorf("the new signature: %v %v", until, ok)
	}
}

func TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		window time.Duration
		whole  int64
	}{
		{90*time.Second + 500*time.Millisecond, 90},
		{5*time.Second + 900*time.Millisecond, 5},
		{60*time.Second + 999*time.Millisecond, 60},
		{2*time.Second + time.Nanosecond, 2},
		{time.Second + 999*time.Millisecond, 1},
	} {
		state := machineState(t)
		for _, offset := range []int64{c.whole, -c.whole} {
			request := signedBy("linux-box", keyOf(1), t0.Unix()+offset).request()
			if _, _, err := newImpl().Verify(state, request, t0, t0.Add(-time.Hour), c.window); err != nil {
				t.Errorf("window %v, %d seconds: %v", c.window, offset, err)
			}
		}
		for _, offset := range []int64{c.whole + 1, -c.whole - 1} {
			request := signedBy("linux-box", keyOf(1), t0.Unix()+offset).request()
			var skew *model.ClockSkewError
			if _, _, err := newImpl().Verify(state, request, t0, t0.Add(-time.Hour), c.window); !errors.As(err, &skew) || skew.Seconds != offset {
				t.Errorf("window %v, %d seconds: %v", c.window, offset, err)
			}
		}
		got, _, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), t0.Unix()).request(), t0, t0.Add(-time.Hour), c.window)
		if err != nil || len(got.Seen) != 1 || !got.Seen[0].Until.Equal(time.Unix(t0.Unix()+c.whole+1, 0)) {
			t.Errorf("window %v: %v seen %+v, want one entry that lasts until %d seconds after t0", c.window, err, got.Seen, c.whole+1)
		}
	}
}

func TestVerifyRefusesATimeWhoseDistanceFromNowDoesNotFitAnInt64WithoutPanickingOrWrapping(t *testing.T) {
	t.Parallel()
	unbounded := time.Unix(math.MinInt64, 0)
	for _, c := range []struct{ now, request int64 }{
		{math.MaxInt64, math.MinInt64},
		{math.MaxInt64, math.MinInt64 + 1},
		{math.MaxInt64, math.MinInt64 + 59},
		{math.MaxInt64, math.MinInt64 + 60},
		{math.MinInt64, math.MaxInt64},
		{math.MinInt64, math.MaxInt64 - 1},
		{math.MinInt64, math.MaxInt64 - 58},
		{math.MinInt64, math.MaxInt64 - 59},
		{math.MaxInt64 / 2, math.MinInt64},
		{math.MinInt64 / 2, math.MaxInt64},
		{1, math.MinInt64},
		{-1, math.MaxInt64},
	} {
		state := machineState(t)
		now := time.Unix(c.now, 0)
		if now.Unix() != c.now {
			t.Fatalf("fixture error: a time with %d seconds reads back as %d", c.now, now.Unix())
		}
		got, name, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), c.request).request(), now, unbounded, window)
		var skew *model.ClockSkewError
		if !errors.As(err, &skew) || name != "" || !strings.Contains(strings.ToLower(err.Error()), "clock") {
			t.Errorf("now %d, time %d: %q %v", c.now, c.request, name, err)
			continue
		}
		saturated := int64(math.MinInt64)
		if c.request > c.now {
			saturated = math.MaxInt64
		}
		if skew.Seconds != saturated || !strings.Contains(err.Error(), strconv.FormatInt(saturated, 10)) {
			t.Errorf("now %d, time %d: seconds %d, want %d: %v", c.now, c.request, skew.Seconds, saturated, err)
		}
		if !reflect.DeepEqual(snapshot(got), snapshot(state)) {
			t.Errorf("now %d, time %d: the state changed", c.now, c.request)
		}
	}
}

func TestVerifyReportsTheExactDifferenceWhenItJustFitsAnInt64(t *testing.T) {
	t.Parallel()
	unbounded := time.Unix(math.MinInt64, 0)
	for _, c := range []struct{ now, request, seconds int64 }{
		{0, math.MaxInt64, math.MaxInt64},
		{0, math.MinInt64, math.MinInt64},
		{-1, math.MaxInt64 - 1, math.MaxInt64},
		{1, math.MinInt64 + 1, math.MinInt64},
		{math.MinInt64 / 2, math.MaxInt64 / 2, math.MaxInt64},
		{math.MaxInt64 / 2, math.MinInt64 / 2, math.MinInt64 + 1},
	} {
		state := machineState(t)
		now := time.Unix(c.now, 0)
		if now.Unix() != c.now || c.request-c.now != c.seconds {
			t.Fatalf("fixture error: now %d, time %d, seconds %d", c.now, c.request, c.seconds)
		}
		_, name, err := newImpl().Verify(state, signedBy("linux-box", keyOf(1), c.request).request(), now, unbounded, window)
		var skew *model.ClockSkewError
		if !errors.As(err, &skew) || skew.Seconds != c.seconds || name != "" {
			t.Errorf("now %d, time %d: %q %v, want seconds %d", c.now, c.request, name, err, c.seconds)
		}
	}
}

func TestInviteRefusesAMachineNameWithAnUpperCaseLetterAfterItsFirstCharacter(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"linux-Box", "boX", "aB", "a-B-c", "boxZ", "boxA", "a1B", "a" + strings.Repeat("b", 30) + "C"} {
		got, code, err := newImpl().Invite(model.Machines{}, name, random(0), t0, lifetime)
		if !errors.Is(err, model.ErrBadMachineName) || code != "" || len(got.Invites) != 0 {
			t.Errorf("%q: %q %v %+v", name, code, err, got)
		}
	}
}

func TestRedeemReadsALowerCaseZAsTheLetterZ(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		random []byte
		typed  []string
	}{
		{[]byte{8, 9, 10, 11, 27, 28, 29, 30}, []string{"abcd-wxyz", "abcdwxyz", "AbCd-wXyZ"}},
		{[]byte{30, 29, 28, 27, 26, 25, 24, 23}, []string{"zyxw-vuts", "zyxwvuts", "ZyXw-vUtS"}},
		{bytes.Repeat([]byte{30}, 8), []string{"zzzz-zzzz", "zzzzzzzz", "ZZZZ-ZZZZ"}},
	} {
		state, display, err := newImpl().Invite(model.Machines{}, "box", c.random, t0, lifetime)
		if err != nil {
			t.Fatal(err)
		}
		for _, typed := range append(c.typed, strings.ToLower(display)) {
			if _, name, err := newImpl().Redeem(state, typed, publicOf(1), t0); err != nil || name != "box" {
				t.Errorf("code %s typed as %q: %q %v", display, typed, name, err)
			}
		}
	}
}

func TestInviteIsRefusedForANameOfAPendingInviteThatIsNotTheFirstInTheList(t *testing.T) {
	t.Parallel()
	state := model.Machines{}
	for i, name := range []string{"alpha", "bravo", "charlie"} {
		state, _ = invited(t, state, name, byte(i*8), t0)
	}
	before := snapshot(state)
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		got, code, err := newImpl().Invite(state, name, random(100), t0, lifetime)
		if !errors.Is(err, model.ErrMachineHeld) || code != "" || !reflect.DeepEqual(snapshot(got), before) {
			t.Errorf("%s: %q %v %+v", name, code, err, got)
		}
	}
	later := t0.Add(lifetime)
	if _, _, err := newImpl().Invite(state, "charlie", random(100), later.Add(-time.Nanosecond), lifetime); !errors.Is(err, model.ErrMachineHeld) {
		t.Errorf("the last invite a nanosecond before it expires: %v", err)
	}
	if _, _, err := newImpl().Invite(state, "charlie", random(100), later, lifetime); err != nil {
		t.Errorf("the last invite at its expiry: %v", err)
	}
}

func TestInviteAndRedeemKeepTheNanosecondsOfTheTimeTheyAreGiven(t *testing.T) {
	t.Parallel()
	now := t0.Add(123456789 * time.Nanosecond)
	state, code := invited(t, model.Machines{}, "box", 8, now)
	if len(state.Invites) != 1 || !state.Invites[0].Expires.Equal(now.Add(lifetime)) {
		t.Fatalf("invites %+v, want one that expires at %v", state.Invites, now.Add(lifetime))
	}
	later := now.Add(987654321 * time.Nanosecond)
	got, _, err := newImpl().Redeem(state, code, publicOf(1), later)
	if err != nil || len(got.Active) != 1 || !got.Active[0].Created.Equal(later) {
		t.Fatalf("%v machines %+v, want one created at %v", err, got.Active, later)
	}
	if _, _, err := newImpl().Redeem(state, code, publicOf(1), now.Add(lifetime-time.Nanosecond)); err != nil {
		t.Errorf("one nanosecond before the expiry: %v", err)
	}
	if _, _, err := newImpl().Redeem(state, code, publicOf(1), now.Add(lifetime)); !errors.Is(err, model.ErrInvalidCode) {
		t.Errorf("at the expiry: %v", err)
	}
}

func TestVerifyNeverVerifiesAndNeverPanicsAgainstAStoredKeyOfThirtyOneOrThirtyThreeBytes(t *testing.T) {
	t.Parallel()
	good := publicOf(1)
	for label, key := range map[string][]byte{
		"thirty-one bytes":   good[:31],
		"thirty-three bytes": append(append([]byte(nil), good...), 0),
		"sixty-four bytes":   append(append([]byte(nil), good...), good...),
	} {
		state := machineState(t)
		state.Active[0].PublicKey = key
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("%s: the call panicked: %v", label, recovered)
				}
			}()
			got, name, err := verify(state, signedBy("linux-box", keyOf(1), t0.Unix()), t0)
			if !errors.Is(err, model.ErrBadSignature) || name != "" || len(got.Seen) != 0 {
				t.Errorf("%s: %q %v %+v", label, name, err, got.Seen)
			}
		}()
	}
}

func TestFingerprintDoesNotChangeTheKeyItIsGiven(t *testing.T) {
	t.Parallel()
	for _, key := range [][]byte{publicOf(7), make([]byte, 32), {1, 2, 3}} {
		before := append([]byte(nil), key...)
		first := newImpl().Fingerprint(key)
		if !bytes.Equal(key, before) {
			t.Errorf("%d bytes: the key changed from %x to %x", len(key), before, key)
		}
		if second := newImpl().Fingerprint(key); second != first {
			t.Errorf("%d bytes: %q and then %q", len(key), first, second)
		}
	}
}

func TestVerifyReportsTheExactSignedClockDifferenceEvenWhenItIsTooLargeForATimeDuration(t *testing.T) {
	t.Parallel()
	state := machineState(t)
	for _, offset := range []int64{10_000_000_000, -10_000_000_000, 9_223_372_037, -9_223_372_037, 4_000_000_000_000} {
		got, name, err := verify(state, signedBy("linux-box", keyOf(1), t0.Unix()+offset), t0)
		var skew *model.ClockSkewError
		if !errors.As(err, &skew) || skew.Seconds != offset || name != "" {
			t.Errorf("%d seconds: %q %v", offset, name, err)
			continue
		}
		if !strings.Contains(err.Error(), strconv.FormatInt(offset, 10)+" seconds") || !strings.Contains(strings.ToLower(err.Error()), "clock") {
			t.Errorf("%d seconds: the text is %q", offset, err.Error())
		}
		if !reflect.DeepEqual(snapshot(got), snapshot(state)) {
			t.Errorf("%d seconds: the state changed", offset)
		}
	}
}

func TestInviteAcceptsAMachineNameWithTheDigitZeroAnywhereAfterItsFirstLetter(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"a0", "box-0", "box-10", "r0b0t", "a" + strings.Repeat("0", 31)} {
		if _, _, err := newImpl().Invite(model.Machines{}, name, random(0), t0, lifetime); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	if _, _, err := newImpl().Invite(model.Machines{}, "0box", random(0), t0, lifetime); !errors.Is(err, model.ErrBadMachineName) {
		t.Errorf("a name that starts with 0: %v", err)
	}
}

func TestRedeemRefusesACodeTypedWithAUnicodeHyphenOrDashInPlaceOfTheHyphen(t *testing.T) {
	t.Parallel()
	for _, dash := range []string{"\u2010", "\u2011", "\u2012", "\u2013", "\u2014", "\u2015", "\u2212", "\ufe63", "\uff0d", "\u00ad"} {
		for _, typed := range []string{"2345" + dash + "6789", dash + "23456789", "23456789" + dash} {
			state, _ := invited(t, model.Machines{}, "box", 0, t0)
			got, name, err := newImpl().Redeem(state, typed, publicOf(1), t0)
			if !errors.Is(err, model.ErrInvalidCode) || name != "" || got.Failures != 1 || len(got.Active) != 0 || len(got.Invites) != 1 {
				t.Errorf("%+q: %q %v %+v", typed, name, err, got)
			}
		}
	}
}

func TestInviteReplacesOnlyTheExpiredInviteForItsOwnNameAndRevokeStillRemovesAnotherExpiredOne(t *testing.T) {
	t.Parallel()
	state, _ := invited(t, model.Machines{}, "alpha-box", 8, t0)
	state, _ = invited(t, state, "bravo-box", 16, t0)
	state, _ = invited(t, state, "charlie-box", 24, t0)
	later := t0.Add(lifetime)
	next, _, err := newImpl().Invite(state, "bravo-box", random(32), later, lifetime)
	if err != nil {
		t.Fatalf("invite over an expired invite: %v", err)
	}
	if want := []string{"alpha-box", "bravo-box", "charlie-box"}; !reflect.DeepEqual(inviteNames(next), want) {
		t.Fatalf("invites %v, want %v", inviteNames(next), want)
	}
	for _, invite := range next.Invites {
		if invite.Name != "bravo-box" && !invite.Expires.Equal(t0.Add(lifetime)) {
			t.Errorf("the expired invite for %s changed: %+v", invite.Name, invite)
		}
	}
	for _, name := range []string{"alpha-box", "charlie-box"} {
		revoked, err := newImpl().Revoke(next, name)
		if err != nil || slices.Contains(inviteNames(revoked), name) {
			t.Errorf("revoke of the expired pending %s after another name was invited: %v %v", name, err, inviteNames(revoked))
		}
	}
}

func TestEditingAMachineKeyInAResultLeavesThePriorStateAlone(t *testing.T) {
	t.Parallel()
	r := newImpl()
	calls := map[string]func(model.Machines, []string) model.Machines{
		"Invite": func(state model.Machines, _ []string) model.Machines {
			next, _, _ := r.Invite(state, "four-box", random(72), t0, lifetime)
			return next
		},
		"Redeem": func(state model.Machines, codes []string) model.Machines {
			next, _, _ := r.Redeem(state, codes[0], publicOf(3), t0)
			return next
		},
		"Revoke": func(state model.Machines, _ []string) model.Machines {
			next, _ := r.Revoke(state, "one-box")
			return next
		},
		"Verify": func(state model.Machines, _ []string) model.Machines {
			next, _, _ := r.Verify(state, signedBy("alpha-box", keyOf(1), t0.Unix()).request(), t0, t0.Add(-time.Hour), window)
			return next
		},
	}
	for name, call := range calls {
		state, codes := richState(t)
		result := call(state, codes)
		if len(result.Active) < 2 {
			t.Fatalf("%s: %d active machines, want at least 2", name, len(result.Active))
		}
		for i := range result.Active {
			result.Active[i].PublicKey[0] ^= 0xff
		}
		if !bytes.Equal(state.Active[0].PublicKey, publicOf(1)) || !bytes.Equal(state.Active[1].PublicKey, publicOf(2)) {
			t.Errorf("%s: editing the result changed the prior state's public key", name)
		}
	}
}

func TestEditingAnInheritedSeenSignatureInAResultLeavesThePriorStateAlone(t *testing.T) {
	t.Parallel()
	r := newImpl()
	calls := map[string]func(model.Machines, []string) model.Machines{
		"Invite": func(state model.Machines, _ []string) model.Machines {
			next, _, _ := r.Invite(state, "four-box", random(72), t0, lifetime)
			return next
		},
		"Redeem": func(state model.Machines, codes []string) model.Machines {
			next, _, _ := r.Redeem(state, codes[0], publicOf(3), t0)
			return next
		},
		"Revoke": func(state model.Machines, _ []string) model.Machines {
			next, _ := r.Revoke(state, "one-box")
			return next
		},
		"Verify": func(state model.Machines, _ []string) model.Machines {
			next, _, _ := r.Verify(state, signedBy("alpha-box", keyOf(1), t0.Unix()).request(), t0, t0.Add(-time.Hour), window)
			return next
		},
	}
	for name, call := range calls {
		state, codes := richState(t)
		result := call(state, codes)
		edited := 0
		for i := range result.Seen {
			if bytes.Equal(result.Seen[i].Signature, []byte{1, 1}) || bytes.Equal(result.Seen[i].Signature, []byte{2, 2}) {
				result.Seen[i].Signature[0] ^= 0xff
				edited++
			}
		}
		if edited != 2 {
			t.Fatalf("%s: the result keeps %d of the 2 inherited signatures, want both", name, edited)
		}
		if !bytes.Equal(state.Seen[0].Signature, []byte{1, 1}) || !bytes.Equal(state.Seen[1].Signature, []byte{2, 2}) {
			t.Errorf("%s: editing a signature in the result changed the prior state's signature", name)
		}
	}
}

func TestInviteAcceptsEveryDigitAfterTheFirstLetter(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"alpha9", "a0123456789", "b9876543210", "a0-1-2-3-4-5-6-7-8-9", "z" + strings.Repeat("0123456789", 3) + "9"} {
		if _, code, err := newImpl().Invite(model.Machines{}, name, random(8), t0, lifetime); err != nil || code == "" {
			t.Errorf("%q (%d characters): %v, want an invite", name, len(name), err)
		}
	}
}
