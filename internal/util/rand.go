package util

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

func RandPassword() string {
	return strings.ToLower(RandToken(16))
}

func RandToken(nbytes int) string {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	// base32 gives alnum without special chars; strip padding.
	return strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), "=")
}
