package dragonfly

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func localDragonfly(t *testing.T) (string, int, func()) {
	t.Helper()
	raw := os.Getenv("TEST_DRAGONFLY_URL")
	if raw == "" {
		return "", 0, nil
	}
	uri, err := url.Parse(raw)
	require.NoError(t, err)
	ip := net.ParseIP(uri.Hostname())
	require.True(t, ip != nil && ip.IsLoopback(), "TEST_DRAGONFLY_URL must be a disposable loopback backend")
	port, err := strconv.Atoi(uri.Port())
	require.NoError(t, err)
	opts, err := redis.ParseURL(raw)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	for _, pattern := range []string{"instance:*", "instance-pending:*"} {
		keys, err := client.Keys(t.Context(), pattern).Result()
		require.NoError(t, err)
		require.Empty(t, keys, "TEST_DRAGONFLY_URL must have an empty broker registry")
	}
	return raw, port, func() {
		ctx := context.Background()
		for _, pattern := range []string{"instance:*", "instance-pending:*"} {
			keys, _ := client.Keys(ctx, pattern).Result()
			for _, key := range keys {
				data, _ := client.Get(ctx, key).Result()
				var instance Instance
				if json.Unmarshal([]byte(data), &instance) == nil && instance.Username != "" {
					_ = client.Do(ctx, "ACL", "DELUSER", instance.Username).Err()
				}
				_ = client.Del(ctx, key).Err()
			}
		}
		_ = client.Close()
	}
}
