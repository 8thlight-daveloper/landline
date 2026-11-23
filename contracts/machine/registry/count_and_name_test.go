package registry

import (
	"errors"
	"reflect"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func TestFiveFailedRedeemsWithNoInviteWaitingStartTheCountAgainSoALaterInviteIsCancelledByFiveMore(t *testing.T) {
	t.Parallel()
	state := failures(t, model.Machines{}, 5, t0)
	if state.Failures != 0 || len(state.Invites) != 0 {
		t.Fatalf("after five failures with no invite waiting: %+v, want no invites and a count of 0", state)
	}
	state, _ = invited(t, state, "box", 8, t0)
	state = failures(t, state, 4, t0)
	if state.Failures != 4 || !reflect.DeepEqual(inviteNames(state), []string{"box"}) {
		t.Fatalf("four failures after the invite: %+v, want the invite kept and a count of 4", state)
	}
	state = failures(t, state, 1, t0)
	if len(state.Invites) != 0 || state.Failures != 0 {
		t.Errorf("five failures after the invite did not cancel it: %+v", state)
	}
}

func TestRevokeMatchesThePendingInviteNameExactlyAndRefusesAnotherCase(t *testing.T) {
	t.Parallel()
	state, _ := invited(t, model.Machines{}, "box", 8, t0)
	state, _ = invited(t, state, "linux-box", 16, t0)
	before := snapshot(state)
	for _, name := range []string{"BOX", "Box", "boX", "LINUX-BOX", "Linux-box"} {
		got, err := newImpl().Revoke(state, name)
		if !errors.Is(err, model.ErrUnknownMachine) || !reflect.DeepEqual(snapshot(got), before) {
			t.Errorf("%q: %v, invites %v, want ErrUnknownMachine and the value unchanged", name, err, inviteNames(got))
		}
	}
}

func TestAllIsAnOrdinaryMachineNameThatCanBeInvitedRedeemedAndRevoked(t *testing.T) {
	t.Parallel()
	state, code, err := newImpl().Invite(model.Machines{}, "all", random(8), t0, lifetime)
	if err != nil {
		t.Fatalf("invite all: %v", err)
	}
	state, name, err := newImpl().Redeem(state, code, publicOf(1), t0)
	if err != nil || name != "all" {
		t.Fatalf("redeem the invite for all: %q %v", name, err)
	}
	if got := names(state); !reflect.DeepEqual(got, []string{"all"}) {
		t.Fatalf("active machines %v, want [all]", got)
	}
	state, err = newImpl().Revoke(state, "all")
	if err != nil || len(state.Active) != 0 {
		t.Errorf("revoke all by name: %v, active %v", err, names(state))
	}
}
