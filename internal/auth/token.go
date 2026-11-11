package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

const MaxTokenBytes = 128

var invalidToken = errors.New("invalid broker authentication")

type Envelope struct {
	KeyID      string
	Generation uint64
	MAC        []byte
}

func ParseToken(token string) (Envelope, error) {
	var e Envelope
	if len(token) > MaxTokenBytes {
		return e, invalidToken
	}
	s := strings.Split(token, ".")
	if len(s) != 4 || s[0] != "v1" || !ValidID(s[1]) {
		return e, invalidToken
	}
	g, err := strconv.ParseUint(s[2], 10, 64)
	if err != nil || g == 0 || g > 9007199254740991 || strconv.FormatUint(g, 10) != s[2] {
		return e, invalidToken
	}
	mac, err := base64.RawURLEncoding.Strict().DecodeString(s[3])
	if err != nil || len(mac) != sha256.Size {
		return e, invalidToken
	}
	return Envelope{s[1], g, mac}, nil
}
func ReadKey(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open auth key")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(b) != 32 {
		return nil, errors.New("auth key must contain exactly 32 raw bytes")
	}
	return b, nil
}
func derive(p *Policy, key []byte, client, keyID string, generation uint64) []byte {
	// Fixed JSON array, canonical decimal integers, no whitespace or trailing newline.
	b, _ := json.Marshal([]any{"razcloud-broker", 1, p.DeploymentID, keyID, client, generation})
	h := hmac.New(sha256.New, key)
	h.Write(b)
	return h.Sum(nil)
}
func Generate(p *Policy, key []byte, client string) (string, error) {
	c, ok := p.Clients[client]
	if !ok || !c.IsEnabled() || len(key) != 32 {
		return "", errors.New("cannot generate token for client")
	}
	return "v1." + p.KeyID + "." + strconv.FormatUint(c.Generation, 10) + "." + base64.RawURLEncoding.EncodeToString(derive(p, key, client, p.KeyID, c.Generation)), nil
}

type Verifier struct {
	policy *Policy
	keys   map[string][]byte
}

func NewVerifier(p *Policy, keys map[string][]byte) (*Verifier, error) {
	if p == nil {
		return nil, errors.New("missing auth policy")
	}
	if len(keys) != len(p.AcceptedKeyIDs) {
		return nil, errors.New("auth keys must match accepted key IDs")
	}
	for _, id := range p.AcceptedKeyIDs {
		if len(keys[id]) != 32 {
			return nil, errors.New("missing or invalid accepted auth key")
		}
	}
	return &Verifier{p, keys}, nil
}
func (v *Verifier) Authenticate(client, token string) bool {
	if !ValidClient(client) {
		return false
	}
	e, err := ParseToken(token)
	if err != nil {
		return false
	}
	c, ok := v.policy.Clients[client]
	if !ok || !c.IsEnabled() {
		return false
	}
	key, ok := v.keys[e.KeyID]
	if !ok {
		return false
	}
	accepted := false
	for _, g := range c.AcceptedGenerations {
		if g == e.Generation {
			accepted = true
		}
	}
	if !accepted {
		return false
	}
	return hmac.Equal(derive(v.policy, key, client, e.KeyID, e.Generation), e.MAC)
}
func (v *Verifier) Allows(client, adapter, instance string) bool {
	return v.policy.Allows(client, adapter, instance)
}
