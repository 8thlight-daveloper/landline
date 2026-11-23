package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"runtime"
	"testing"
)

const stagingAttempts = 50

type gapOutcome int

const (
	gapCaught gapOutcome = iota
	gapMissed
)

func (h *harness) issuedLocked() int { return len(h.hub.tokens) + len(h.hub.observers) }

func (h *harness) replaceMachineKeyLocked(name string, public ed25519.PublicKey) {
	h.t.Helper()
	hub := h.hub
	now := h.now
	next, err := hub.machines.Revoke(hub.state, name)
	if err != nil {
		h.t.Fatalf("revoke %s: %v", name, err)
	}
	hub.state = next
	hub.endMachineLocked(func(machine string) bool { return machine == name })
	random := make([]byte, 16)
	rand.Read(random)
	next, code, err := hub.machines.Invite(hub.state, name, random, now, hub.cfg.InviteLifetime)
	if err != nil {
		h.t.Fatalf("invite %s again: %v", name, err)
	}
	next, _, err = hub.machines.Redeem(next, code, public, now)
	if err != nil {
		h.t.Fatalf("redeem %s with the new key: %v", name, err)
	}
	hub.state = next
}

func (h *harness) replaceKeyAfterVerifyBeforeIssue(seenBefore, issuedBefore int, answered <-chan struct{}, name string, public ed25519.PublicKey) gapOutcome {
	for {
		h.hub.mu.Lock()
		verified := len(h.hub.state.Seen) > seenBefore
		issued := h.issuedLocked() != issuedBefore
		if verified && !issued {
			h.replaceMachineKeyLocked(name, public)
			h.hub.mu.Unlock()
			return gapCaught
		}
		h.hub.mu.Unlock()
		if issued {
			return gapMissed
		}
		select {
		case <-answered:
			return gapMissed
		default:
		}
		runtime.Gosched()
	}
}

func TestARequestSignedByARevokedKeyGetsNoTokenWhenItsNameWasJoinedAgainWithANewKeyWhileTheRequestWasInFlight(t *testing.T) {
	for _, route := range []struct{ path, fields string }{
		{"/v1/register", `"id":"alpha","harness":"claude-code","host":"mac"`},
		{"/v1/observers", `"kind":"tui"`},
	} {
		t.Run(route.path, func(t *testing.T) {
			caught := 0
			for attempt := 0; attempt < stagingAttempts && caught == 0; attempt++ {
				h := start(t)
				h.advance(pastTheStartPeriod)
				oldKey := h.joinMachine("linux-box")
				newPublic, _, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				body := paddedBody(route.fields)
				h.hub.mu.Lock()
				seenBefore, issuedBefore := len(h.hub.state.Seen), h.issuedLocked()
				h.hub.mu.Unlock()
				authorization := machineAuthorization(oldKey, "linux-box", h.now.Unix(), "POST", route.path, body)
				answered := make(chan struct{})
				var status int
				var raw []byte
				go func() {
					status, raw = h.sendWithAuthorization("POST", route.path, authorization, body)
					close(answered)
				}()
				outcome := h.replaceKeyAfterVerifyBeforeIssue(seenBefore, issuedBefore, answered, "linux-box", newPublic)
				<-answered
				if outcome != gapCaught {
					continue
				}
				caught++
				if status != http.StatusUnauthorized {
					t.Errorf("a request signed by the revoked key of linux-box, verified before linux-box was joined again with a new key, got %d %s, want 401 and no token", status, raw)
				}
				h.hub.mu.Lock()
				for token, machine := range h.hub.machineTokens {
					if machine == "linux-box" {
						t.Errorf("a token %s is recorded for the new linux-box though only the revoked key signed a request", token)
					}
				}
				h.hub.mu.Unlock()
			}
			if caught == 0 {
				t.Logf("in %d attempts the request never stopped between its signature check and the issue of a token, so there is no gap to stage", stagingAttempts)
			}
		})
	}
}
