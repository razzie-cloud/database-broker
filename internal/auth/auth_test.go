package auth

import (
	"bytes"
	"strings"
	"testing"
)

const fixture = `version: 1
deployment_id: test-deploy
key_id: k1
clients:
  files:
    generation: 1
    allow:
      - adapter: REDIS
        instance: Files
`

func policy(t *testing.T, s string) *Policy {
	t.Helper()
	p, e := Parse(strings.NewReader(s), "test-deploy")
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestVectorAndCanonicalPolicy(t *testing.T) {
	p := policy(t, fixture)
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	token, e := Generate(p, k, "files")
	if e != nil {
		t.Fatal(e)
	}
	const expected = "v1.k1.1.pB2pEf2Wd0_28pAo-a40bxoqufdsL3tAFStUVXEAU7Y"
	if token != expected {
		t.Fatal("fixed vector mismatch")
	}
	v, e := NewVerifier(p, map[string][]byte{"k1": k})
	if e != nil {
		t.Fatal(e)
	}
	if !v.Authenticate("files", token) || !v.Allows("files", "ReDiS", "FILES") || !v.Allows("files", "dragonfly", "files") || v.Allows("files", "valkey", "files") {
		t.Fatal("canonical authorization")
	}
	p2 := policy(t, strings.ReplaceAll(fixture, "generation: 1", "generation: 1 # formatting"))
	token2, _ := Generate(p2, k, "files")
	if token2 != token {
		t.Fatal("format changed token")
	}
	for _, s := range []string{
		strings.Replace(fixture, "generation: 1", "generation: 2", 1),
		strings.Replace(fixture, "key_id: k1", "key_id: k2", 1),
		strings.ReplaceAll(fixture, "files:", "other:"),
	} {
		q := policy(t, s)
		client := "files"
		if q.Clients[client].Generation == 0 {
			client = "other"
		}
		x, _ := Generate(q, k, client)
		if x == token {
			t.Fatal("input not bound")
		}
	}
	q := *p
	q.DeploymentID = "other"
	x, _ := Generate(&q, k, "files")
	if x == token {
		t.Fatal("deployment not bound")
	}
	p.Clients["files"] = Client{Generation: 1, AcceptedGenerations: []uint64{1}, Allow: []Grant{{"postgres", "other"}}}
	if !v.Authenticate("files", token) || v.Allows("files", "redis", "files") {
		t.Fatal("permissions should not bind token")
	}
}
func TestPolicyRejections(t *testing.T) {
	for _, s := range []string{"", fixture + "---\n", fixture + "unknown: 1\n", fixture + "version: 1\n", strings.Replace(fixture, "test-deploy", "other", 1), strings.Replace(fixture, "generation: 1", "generation: 0", 1), strings.Replace(fixture, "key_id: k1", "key_id: k.1", 1), strings.Replace(fixture, "generation: 1", "generation: 1\n    unknown: true", 1), strings.Replace(fixture, "instance: Files", "instance: '*'", 1), strings.Replace(fixture, "generation: 1", "generation: 1\n    accepted_generations: [1, 1]", 1), strings.Replace(fixture, "key_id: k1", "key_id: k1\naccepted_key_ids: [k2]", 1), strings.Replace(fixture, "instance: Files", "instance: Files\n      - adapter: dragonfly\n        instance: files", 1)} {
		if _, e := Parse(strings.NewReader(s), "test-deploy"); e == nil {
			t.Fatal("accepted invalid policy")
		}
	}
	if _, e := Parse(bytes.NewReader(make([]byte, MaxPolicyBytes+1)), "test-deploy"); e == nil {
		t.Fatal("oversize")
	}
}
func TestEnvelopeAndRotation(t *testing.T) {
	p := policy(t, fixture)
	key := bytes.Repeat([]byte{3}, 32)
	token, _ := Generate(p, key, "files")
	v, _ := NewVerifier(p, map[string][]byte{"k1": key})
	for _, s := range []string{"", token + "=", strings.Replace(token, ".1.", ".01.", 1), strings.Replace(token, "v1.", "v2.", 1), strings.Repeat("x", 129), token[:len(token)-1], "v1.k1.1." + strings.Repeat("A", 43)} {
		if v.Authenticate("files", s) {
			t.Fatal("accepted malformed token")
		}
	}
	if v.Authenticate("other", token) || v.Authenticate(strings.Repeat("a", 65), token) {
		t.Fatal("client")
	}
	rotated := policy(t, strings.Replace(strings.Replace(fixture, "key_id: k1", "key_id: k2\naccepted_key_ids: [k1, k2]", 1), "generation: 1", "generation: 2\n    accepted_generations: [1, 2]", 1))
	v, e := NewVerifier(rotated, map[string][]byte{"k1": key, "k2": bytes.Repeat([]byte{4}, 32)})
	if e != nil || !v.Authenticate("files", token) {
		t.Fatal("rotation overlap")
	}
	current, _ := Generate(rotated, bytes.Repeat([]byte{4}, 32), "files")
	if !v.Authenticate("files", current) || !strings.HasPrefix(current, "v1.k2.2.") {
		t.Fatal("active generator")
	}
	rotated = policy(t, strings.Replace(fixture, "generation: 1", "generation: 2", 1))
	v, _ = NewVerifier(rotated, map[string][]byte{"k1": key})
	if v.Authenticate("files", token) {
		t.Fatal("revocation")
	}
}
