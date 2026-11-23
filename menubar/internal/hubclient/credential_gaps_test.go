package hubclient

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
)

var signedFields = regexp.MustCompile(`time=(\d+), nonce=([^,]+),`)

type observerRegistrations struct {
	mu      sync.Mutex
	headers []string
}

func (o *observerRegistrations) switchboard(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/observers":
			o.mu.Lock()
			o.headers = append(o.headers, r.Header.Get("Authorization"))
			o.mu.Unlock()
			w.Write([]byte(`{"token":"observer-token"}`))
		case "/v1/agents":
			w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (o *observerRegistrations) last(t *testing.T) (string, string) {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.headers) == 0 {
		t.Fatal("no observer registration reached the switchboard")
	}
	found := signedFields.FindStringSubmatch(o.headers[len(o.headers)-1])
	if found == nil {
		t.Fatalf("the registration was not signed: %q", o.headers[len(o.headers)-1])
	}
	return found[1], found[2]
}

func TestTwoSignedRegistrationsInTheSameSecondCarryDifferentSixteenByteNonces(t *testing.T) {
	t.Parallel()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	var seen observerRegistrations
	server := seen.switchboard(t)
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := NewWith(server.URL, Credential{Machine: "linux-box", Key: key}).Agents(); err != nil {
			t.Fatal(err)
		}
		firstTime, firstNonce := seen.last(t)
		if _, err := NewWith(server.URL, Credential{Machine: "linux-box", Key: key}).Agents(); err != nil {
			t.Fatal(err)
		}
		secondTime, secondNonce := seen.last(t)
		if firstTime != secondTime {
			continue
		}
		if firstNonce == secondNonce {
			t.Errorf("two signed registrations at time %s carried the same nonce %s", firstTime, firstNonce)
		}
		for _, nonce := range []string{firstNonce, secondNonce} {
			if raw, err := base64.StdEncoding.DecodeString(nonce); err != nil || len(raw) != 16 {
				t.Errorf("the nonce %q is not 16 bytes in standard base64: %v", nonce, err)
			}
		}
		return
	}
	t.Fatal("no two registrations fell in the same second in 5 attempts")
}

func TestTheKeyReaderTakesOnlyTheFirstLineOfMachineKey(t *testing.T) {
	t.Parallel()
	seed := bytes.Repeat([]byte{9}, ed25519.SeedSize)
	firstLine := base64.StdEncoding.EncodeToString(seed) + "\n"
	for name, rest := range map[string]string{
		"a second line that is not base64": "written by hand, not a key\n",
		"a second line that is base64":     "AAAA\n",
		"a second key":                     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, ed25519.SeedSize)) + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := homeWith(t, map[string]string{
				"config.toml": "host = \"127.0.0.1\"\nport = 8787\nmachine = \"linux-box\"\n",
				"machine.key": firstLine + rest,
			})
			_, credential, err := DiscoverCredential(env(nil), dir)
			if err != nil {
				t.Fatalf("a key file with a good first line was refused: %v", err)
			}
			if !bytes.Equal(credential.Key, ed25519.NewKeyFromSeed(seed)) || credential.Machine != "linux-box" {
				t.Errorf("the key read is not the key on the first line")
			}
		})
	}
}
