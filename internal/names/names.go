// Package names defines the canonical broker namespace used by policy and lookup.
package names

import (
	"regexp"
	"strings"
)

var instance = regexp.MustCompile(`^[a-z0-9_]+$`)

func Adapter(s string) string {
	s = strings.ToLower(s)
	if s == "redis" {
		return "dragonfly"
	}
	return s
}
func Instance(s string) string    { return strings.ToLower(s) }
func ValidInstance(s string) bool { return instance.MatchString(Instance(s)) }
