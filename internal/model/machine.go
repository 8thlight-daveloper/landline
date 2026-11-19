package model

import "time"

type Machine struct {
	Name      string    `json:"name"`
	PublicKey []byte    `json:"public_key"`
	Created   time.Time `json:"created"`
}

type Invite struct {
	Name    string
	Code    string
	Expires time.Time
}

type SeenSignature struct {
	Signature []byte
	Until     time.Time
}

type Machines struct {
	Active   []Machine
	Invites  []Invite
	Failures int
	Seen     []SeenSignature
}

type SignedRequest struct {
	Name      string
	Time      int64
	Nonce     string
	Signature []byte
	Method    string
	Path      string
	Body      []byte
}
