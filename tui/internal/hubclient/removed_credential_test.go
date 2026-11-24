package hubclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAfterEveryCredentialIsRemovedTheNextRegistrationIsNotSignedWithTheCachedKeyAndReportsNoCredential(t *testing.T) {
	var signed atomic.Int32
	var expired atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/observers":
			if strings.HasPrefix(r.Header.Get("Authorization"), "Landline-Machine ") {
				signed.Add(1)
			}
			w.Write([]byte(`{"token":"observer-token"}`))
		case expired.Load():
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unknown token"}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(server.Close)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	held := Credential{Machine: "linux-box", Key: key}
	var removed atomic.Bool
	client := NewWith(server.URL, held)
	client.RereadCredentialWith(func() (string, Credential, error) {
		if removed.Load() {
			return server.URL, Credential{}, nil
		}
		return server.URL, held, nil
	})
	if _, err := client.Agents(); err != nil || signed.Load() != 1 {
		t.Fatalf("with the key: %v, %d signed registrations", err, signed.Load())
	}

	removed.Store(true)
	expired.Store(true)
	_, err = client.Agents()
	if got := signed.Load(); got != 1 {
		t.Errorf("after the key and every other credential were removed, the client signed %d more registrations with the key it had cached", got-1)
	}
	if err == nil || !strings.Contains(err.Error(), "there is no switchboard credential") {
		t.Errorf("after every credential was removed: %v, want the result for no credential", err)
	}
}
