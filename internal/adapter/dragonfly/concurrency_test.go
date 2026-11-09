package dragonfly

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestConcurrentProvisioning(t *testing.T) {
	stop, uri, _ := startDragonflyContainer(t)
	defer stop()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	const replicas = 16
	type result struct {
		instance adapter.Instance
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, replicas)
	var first *dragonflyAdapter
	for range replicas {
		peer, err := New(uri)
		require.NoError(t, err)
		defer peer.Close()
		first = peer.(*dragonflyAdapter)
		go func() {
			<-start
			instance, err := peer.GetOrCreateInstance(ctx, "concurrent")
			results <- result{instance, err}
		}()
	}
	close(start)
	var canonical adapter.Instance
	for range replicas {
		result := <-results
		require.NoError(t, result.err)
		if canonical == nil {
			canonical = result.instance
		}
		require.Equal(t, canonical.GetURI(), result.instance.GetURI())
	}
	assertCredentials(t, canonical)
	acl, err := first.client.Do(ctx, "ACL", "LIST").StringSlice()
	require.NoError(t, err)
	found := false
	for _, user := range acl {
		if strings.HasPrefix(user, "user user_concurrent ") {
			found = true
			require.Equal(t, 1, strings.Count(user, "#"), "concurrent creation must not add losing passwords")
		}
	}
	require.True(t, found, "published namespace must have an ACL user")
	instances, err := first.GetInstances(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"concurrent"}, instances, "reservations must not appear as published instances")
}

func assertCredentials(t *testing.T, instance adapter.Instance) {
	t.Helper()
	opts, err := redis.ParseURL(instance.GetURI())
	require.NoError(t, err)
	client := redis.NewClient(opts)
	defer client.Close()
	require.NoError(t, client.Ping(t.Context()).Err())
	// Instance users must retain namespace access and lose administrative access.
	require.NoError(t, client.Set(t.Context(), "proof", "value", 0).Err())
	require.Error(t, client.Do(t.Context(), "ACL", "LIST").Err())
}

var errProvision = errors.New("injected provisioning failure")

type failProvisioning struct{ acl bool }

func (failProvisioning) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}
func (failure failProvisioning) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if failure.acl && cmd.Name() == "acl" {
			return errProvision
		}
		if !failure.acl && (cmd.Name() == "set" || cmd.Name() == "setnx") && strings.HasPrefix(cmd.Args()[1].(string), "instance:") {
			return errProvision
		}
		return next(ctx, cmd)
	}
}
func (failProvisioning) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestProvisioningResumesAfterFailure(t *testing.T) {
	for _, stage := range []string{"acl", "publication"} {
		t.Run(stage, func(t *testing.T) { provisioningResumesAfterFailure(t, stage == "acl") })
	}
}

func provisioningResumesAfterFailure(t *testing.T, atACL bool) {
	stop, uri, _ := startDragonflyContainer(t)
	defer stop()
	failed, err := New(uri)
	require.NoError(t, err)
	defer failed.Close()
	failed.(*dragonflyAdapter).client.AddHook(failProvisioning{acl: atACL})
	_, err = failed.GetOrCreateInstance(t.Context(), "resumed")
	require.ErrorIs(t, err, errProvision)
	peer, err := New(uri)
	require.NoError(t, err)
	defer peer.Close()
	d := peer.(*dragonflyAdapter)
	data, err := d.client.Get(t.Context(), "instance-pending:resumed").Result()
	require.NoError(t, err)
	reserved, err := d.unmarshalInstance("resumed", data)
	require.NoError(t, err)
	instances, err := d.GetInstances(t.Context())
	require.NoError(t, err)
	require.Empty(t, instances, "failed provisioning must not be published")
	instance, err := peer.GetOrCreateInstance(t.Context(), "resumed")
	require.NoError(t, err)
	require.Equal(t, reserved.GetURI(), instance.GetURI())
	assertCredentials(t, instance)
	// A restart/retry keeps published credentials and the durable reservation.
	again, err := failed.GetOrCreateInstance(t.Context(), "resumed")
	require.NoError(t, err)
	require.Equal(t, instance.GetURI(), again.GetURI())
}
