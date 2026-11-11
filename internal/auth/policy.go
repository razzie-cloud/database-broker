// Package auth implements the v1 scoped broker bearer-token protocol.
package auth

import (
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"

	"github.com/razzie-cloud/database-broker/internal/names"
	"gopkg.in/yaml.v3"
)

const MaxPolicyBytes = 1 << 20

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var publicID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func ValidClient(s string) bool { return identifier.MatchString(s) }
func ValidID(s string) bool     { return publicID.MatchString(s) }

type Grant struct {
	Adapter  string `yaml:"adapter" json:"adapter"`
	Instance string `yaml:"instance" json:"instance"`
}
type Client struct {
	Generation          uint64   `yaml:"generation" json:"generation"`
	AcceptedGenerations []uint64 `yaml:"accepted_generations,omitempty" json:"accepted_generations,omitempty"`
	Enabled             *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Allow               []Grant  `yaml:"allow" json:"allow"`
}
type Policy struct {
	Version        int               `yaml:"version" json:"version"`
	DeploymentID   string            `yaml:"deployment_id" json:"deployment_id"`
	KeyID          string            `yaml:"key_id" json:"key_id"`
	AcceptedKeyIDs []string          `yaml:"accepted_key_ids,omitempty" json:"accepted_key_ids,omitempty"`
	Clients        map[string]Client `yaml:"clients" json:"clients"`
}

// Parse requires exactly one strict YAML document. Diagnostics never echo input.
func Parse(r io.Reader, deployment string) (*Policy, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxPolicyBytes+1))
	if err != nil || len(b) > MaxPolicyBytes {
		return nil, errors.New("cannot read bounded auth policy")
	}
	// yaml.v3 rejects duplicate mapping keys; KnownFields rejects unknown fields.
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	var p Policy
	if d.Decode(&p) != nil {
		return nil, errors.New("invalid auth policy YAML")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("auth policy must contain one document")
	}
	if err := p.validate(deployment); err != nil {
		return nil, err
	}
	return &p, nil
}
func LoadPolicy(path, deployment string) (*Policy, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open auth policy")
	}
	defer f.Close()
	return Parse(f, deployment)
}
func (p *Policy) validate(deployment string) error {
	bad := errors.New("invalid auth policy values")
	if p.Version != 1 || !ValidID(deployment) || p.DeploymentID != deployment || !ValidID(p.KeyID) || len(p.Clients) == 0 || len(p.Clients) > 256 {
		return bad
	}
	if len(p.AcceptedKeyIDs) == 0 {
		p.AcceptedKeyIDs = []string{p.KeyID}
	}
	if len(p.AcceptedKeyIDs) > 4 {
		return bad
	}
	keys := map[string]bool{}
	for _, id := range p.AcceptedKeyIDs {
		if !ValidID(id) || keys[id] {
			return bad
		}
		keys[id] = true
	}
	if !keys[p.KeyID] {
		return bad
	}
	for id, c := range p.Clients {
		if !ValidClient(id) || c.Generation == 0 || c.Generation > 9007199254740991 || len(c.Allow) > 256 {
			return bad
		}
		if len(c.AcceptedGenerations) == 0 {
			c.AcceptedGenerations = []uint64{c.Generation}
		}
		if len(c.AcceptedGenerations) > 4 {
			return bad
		}
		gens := map[uint64]bool{}
		for _, g := range c.AcceptedGenerations {
			if g == 0 || g > 9007199254740991 || gens[g] {
				return bad
			}
			gens[g] = true
		}
		if !gens[c.Generation] {
			return bad
		}
		grants := map[Grant]bool{}
		for i, g := range c.Allow {
			g.Adapter = names.Adapter(g.Adapter)
			g.Instance = names.Instance(g.Instance)
			if (g.Adapter != "postgres" && g.Adapter != "dragonfly" && g.Adapter != "valkey") || !names.ValidInstance(g.Instance) || len(g.Instance) > 128 || grants[g] {
				return bad
			}
			grants[g] = true
			c.Allow[i] = g
		}
		p.Clients[id] = c
	}
	return nil
}
func (p *Policy) Allows(client, adapter, instance string) bool {
	c, ok := p.Clients[client]
	if !ok || !c.IsEnabled() {
		return false
	}
	grant := Grant{names.Adapter(adapter), names.Instance(instance)}
	for _, g := range c.Allow {
		if g == grant {
			return true
		}
	}
	return false
}
func (c Client) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }
