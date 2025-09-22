package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetURIHostPort(t *testing.T) {
	t.Run("host and port in URI", func(t *testing.T) {
		host, port, err := GetURIHostPort("postgres://localhost:5432/db", 1234)
		assert.NoError(t, err)
		assert.Equal(t, "localhost", host)
		assert.Equal(t, 5432, port)
	})

	t.Run("host only, use default port", func(t *testing.T) {
		host, port, err := GetURIHostPort("postgres://localhost/db", 1234)
		assert.NoError(t, err)
		assert.Equal(t, "localhost", host)
		assert.Equal(t, 1234, port)
	})

	t.Run("invalid URI", func(t *testing.T) {
		host, port, err := GetURIHostPort("://bad_uri", 1234)
		assert.Error(t, err)
		assert.Empty(t, host)
		assert.Equal(t, 0, port)
	})

	t.Run("IPv6 host with port", func(t *testing.T) {
		host, port, err := GetURIHostPort("postgres://[::1]:9999/db", 1234)
		assert.NoError(t, err)
		assert.Equal(t, "::1", host)
		assert.Equal(t, 9999, port)
	})

	t.Run("IPv6 host only, use default port", func(t *testing.T) {
		host, port, err := GetURIHostPort("postgres://[::1]/db", 1234)
		assert.NoError(t, err)
		assert.Equal(t, "::1", host)
		assert.Equal(t, 1234, port)
	})
}
