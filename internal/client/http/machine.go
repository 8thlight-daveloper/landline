package http

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Signer struct {
	Name string
	Key  ed25519.PrivateKey
}

func (s Signer) Header(method, path string, body []byte, now time.Time) string {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	encoded := base64.StdEncoding.EncodeToString(nonce)
	sum := sha256.Sum256(body)
	text := strings.Join([]string{"landline-machine-v1", s.Name, strconv.FormatInt(now.Unix(), 10), encoded, method, path, hex.EncodeToString(sum[:])}, "\n")
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(s.Key, []byte(text)))
	return fmt.Sprintf("Landline-Machine name=%s, time=%d, nonce=%s, signature=%s", s.Name, now.Unix(), encoded, signature)
}

type Invite struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type MachineInfo struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	Created     string `json:"created"`
}

type redeemRequest struct {
	Code      string `json:"code"`
	PublicKey string `json:"public_key"`
}

type redeemResponse struct {
	Name string `json:"name"`
}

type inviteRequest struct {
	Name string `json:"name"`
}

func NewSigned(address string, signer Signer) *Client {
	client := New(address, "")
	client.Signer = &signer
	return client
}

func (c *Client) Invite(name string) (Invite, error) {
	var out Invite
	return out, c.do("POST", "/v1/machines", inviteRequest{Name: name}, &out)
}

func (c *Client) Machines() ([]MachineInfo, error) {
	var out []MachineInfo
	return out, c.do("GET", "/v1/machines", nil, &out)
}

func (c *Client) Revoke(name string) error {
	return c.do("DELETE", "/v1/machines/"+url.PathEscape(name), nil, nil)
}

func (c *Client) RevokeAll() error {
	return c.do("DELETE", "/v1/machines", nil, nil)
}

func (c *Client) Redeem(code string, publicKey []byte) (string, error) {
	var out redeemResponse
	request := redeemRequest{Code: code, PublicKey: base64.StdEncoding.EncodeToString(publicKey)}
	return out.Name, c.doWith(false, "PATCH", "/v1/machines", request, &out)
}
