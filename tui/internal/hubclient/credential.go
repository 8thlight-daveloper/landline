package hubclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Credential struct {
	Secret  string
	Machine string
	Key     ed25519.PrivateKey
}

func (c Credential) empty() bool { return c.Secret == "" && c.Key == nil }

func signedHeader(machine string, key ed25519.PrivateKey, method, path string, body []byte, now time.Time) string {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	encoded := base64.StdEncoding.EncodeToString(nonce)
	sum := sha256.Sum256(body)
	text := strings.Join([]string{"landline-machine-v1", machine, strconv.FormatInt(now.Unix(), 10), encoded, method, path, hex.EncodeToString(sum[:])}, "\n")
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(text)))
	return fmt.Sprintf("Landline-Machine name=%s, time=%d, nonce=%s, signature=%s", machine, now.Unix(), encoded, signature)
}

func readCredential(directory string, getenv func(string) string, config saved) (Credential, error) {
	if secret := getenv("LANDLINE_SECRET"); secret != "" {
		return Credential{Secret: secret}, nil
	}
	path := filepath.Join(directory, "machine.key")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credential{Secret: config.Secret}, nil
	}
	if err != nil {
		return Credential{}, fmt.Errorf("%s cannot be read: %v", path, err)
	}
	first, _, _ := strings.Cut(string(raw), "\n")
	seed, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(first))
	if decodeErr != nil || len(seed) != ed25519.SeedSize {
		return Credential{}, fmt.Errorf("%s does not hold a 32 byte machine key in base64 on its first line", path)
	}
	if config.Machine == "" {
		return Credential{}, fmt.Errorf("%s is saved but config.toml names no machine. Run landline server join again", path)
	}
	return Credential{Machine: config.Machine, Key: ed25519.NewKeyFromSeed(seed)}, nil
}
