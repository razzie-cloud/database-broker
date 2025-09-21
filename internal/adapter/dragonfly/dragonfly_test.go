package dragonfly

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startDragonflyContainer(t *testing.T) (testcontainers.Container, string, int) {
	req := testcontainers.ContainerRequest{
		Image:        "docker.dragonflydb.io/dragonflydb/dragonfly:latest",
		ExposedPorts: []string{"6379/tcp"},
		Env: map[string]string{
			"REQUIREPASS": "adminpass",
		},
		WaitingFor: wait.ForListeningPort("6379/tcp"),
	}
	container, err := testcontainers.GenericContainer(context.Background(), testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(context.Background())
	require.NoError(t, err)
	portStr, err := container.MappedPort(context.Background(), "6379")
	require.NoError(t, err)
	port := portStr.Int()
	uri := fmt.Sprintf("redis://:adminpass@%s:%d/0", host, port)
	return container, uri, port
}

func TestDragonflyAdapterIntegration(t *testing.T) {
	ctx := context.Background()
	container, uri, _ := startDragonflyContainer(t)
	defer container.Terminate(ctx)

	adapter, err := New(uri)
	require.NoError(t, err)

	foo, err := adapter.GetOrCreateInstance(ctx, "foo")
	require.NoError(t, err)
	bar, err := adapter.GetOrCreateInstance(ctx, "bar")
	require.NoError(t, err)

	instances, err := adapter.GetInstances(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"foo", "bar"}, instances)

	fooClientOpts, _ := redis.ParseURL(foo.GetURI())
	fooClient := redis.NewClient(fooClientOpts)
	defer fooClient.Close()
	barClientOpts, _ := redis.ParseURL(bar.GetURI())
	barClient := redis.NewClient(barClientOpts)
	defer barClient.Close()

	require.NoError(t, fooClient.Ping(ctx).Err())
	require.NoError(t, barClient.Ping(ctx).Err())

	// Set key in foo, check not in bar
	require.NoError(t, fooClient.Set(ctx, "foo-key", "foo-value", 0).Err())
	val, err := fooClient.Get(ctx, "foo-key").Result()
	require.NoError(t, err)
	require.Equal(t, "foo-value", val)

	_, err = barClient.Get(ctx, "foo-key").Result()
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "nil"))

	// Set key in bar, check not in foo
	require.NoError(t, barClient.Set(ctx, "bar-key", "bar-value", 0).Err())
	val, err = barClient.Get(ctx, "bar-key").Result()
	require.NoError(t, err)
	require.Equal(t, "bar-value", val)

	_, err = fooClient.Get(ctx, "bar-key").Result()
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "nil"))
}
