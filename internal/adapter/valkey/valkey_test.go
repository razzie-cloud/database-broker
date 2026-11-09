package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestVersion(t *testing.T) {
	for _, version := range []string{"9.1.0", "9.1.1", "9.10.0", "10.0.0"} {
		require.NoError(t, checkVersion("# Server\r\nredis_version:7.2.4\r\nvalkey_version:"+version+"\r\n"))
	}
	for _, info := range []string{
		"redis_version:99.0.0\r\n", "valkey_version:8.1.9", "valkey_version:9.0.99",
		"valkey_version:9.1", "valkey_version:9.1.0-rc1", "valkey_version:9.1.-1",
		"valkey_version:9.1.x", "valkey_version:99999999999999999999999.1.0", "",
	} {
		require.Error(t, checkVersion(info), info)
	}
}

func TestInstanceURI(t *testing.T) {
	i := Instance{Instance: "example", Host: "::1", Port: 6379, Database: 5, Username: "user_example", Password: "p:@/word", TLS: true}
	u, err := url.Parse(i.GetURI())
	require.NoError(t, err)
	require.Equal(t, "rediss", u.Scheme)
	require.Equal(t, "[::1]:6379", u.Host)
	require.Equal(t, "/5", u.Path)
	password, _ := u.User.Password()
	require.Equal(t, i.Password, password)
	data, err := json.Marshal(i.GetJSON())
	require.NoError(t, err)
	require.Contains(t, string(data), `"database":5`)
	require.Contains(t, string(data), `"uri":"rediss://`)
	i.TLS = false
	require.True(t, strings.HasPrefix(i.GetURI(), "redis://"))
}

// Native tests require an empty, dedicated server; cleanup removes all data and
// test-created users. Never point TEST_VALKEY_URL at a shared backend.
func startValkey(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("TEST_VALKEY_URL")
	if raw == "" {
		container, err := testcontainers.GenericContainer(t.Context(), testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "valkey/valkey:9.1.0", ExposedPorts: []string{"6379/tcp"},
				Cmd:        []string{"sh", "-ec", `umask 077; printf 'user default on >adminpass ~* &* +@all\n' > /data/users.acl; exec valkey-server --aclfile /data/users.acl --databases 16`},
				WaitingFor: wait.ForListeningPort("6379/tcp"),
			}, Started: true,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(context.Background()) })
		host, err := container.Host(t.Context())
		require.NoError(t, err)
		port, err := container.MappedPort(t.Context(), "6379/tcp")
		require.NoError(t, err)
		raw = "redis://:adminpass@" + net.JoinHostPort(host, port.Port()) + "/0"
	} else {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		ip := net.ParseIP(u.Hostname())
		require.True(t, ip != nil && ip.IsLoopback(), "TEST_VALKEY_URL must be disposable and loopback")
	}
	admin := newClient(t, raw)
	info, err := admin.Info(t.Context(), "keyspace").Result()
	require.NoError(t, err)
	for line := range strings.SplitSeq(info, "\n") {
		require.False(t, strings.HasPrefix(line, "db"), "TEST_VALKEY_URL must have no data in any database")
	}
	users, err := admin.Do(t.Context(), "ACL", "USERS").StringSlice()
	require.NoError(t, err)
	require.Equal(t, []string{"default"}, users, "TEST_VALKEY_URL must have only the default admin")
	t.Cleanup(func() {
		ctx := context.Background()
		_ = admin.FlushAll(ctx).Err()
		users, _ := admin.Do(ctx, "ACL", "USERS").StringSlice()
		for _, user := range users {
			if user != "default" {
				_ = admin.ACLDelUser(ctx, user).Err()
			}
		}
	})
	return raw
}

func newClient(t *testing.T, uri string) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(uri)
	require.NoError(t, err)
	opts.ContextTimeoutEnabled = true
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func newAdapter(t *testing.T, uri string) *valkeyAdapter {
	t.Helper()
	a, err := New(uri)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })
	return a.(*valkeyAdapter)
}

func TestValkeyIsolation(t *testing.T) {
	uri := startValkey(t)
	v := newAdapter(t, uri)
	ctx := t.Context()
	require.NoError(t, v.Ping(ctx))
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, v.Ping(cancelled))
	foo, err := v.GetOrCreateInstance(ctx, "foo")
	require.NoError(t, err)
	bar, err := v.GetOrCreateInstance(ctx, "bar")
	require.NoError(t, err)
	f, b := foo.(*Instance), bar.(*Instance)
	require.NotEqual(t, f.Database, b.Database)
	require.NotEqual(t, v.metadataDB, f.Database)
	fc, bc := newClient(t, foo.GetURI()), newClient(t, bar.GetURI())
	require.NoError(t, fc.Set(ctx, "same-key", "foo", 0).Err())
	require.NoError(t, bc.Set(ctx, "same-key", "bar", 0).Err())
	require.Equal(t, "foo", fc.Get(ctx, "same-key").Val())
	require.Equal(t, "bar", bc.Get(ctx, "same-key").Val())
	for _, db := range []int{v.metadataDB, b.Database} {
		for _, cmd := range [][]any{
			{"SELECT", db}, {"MOVE", "same-key", db}, {"COPY", "same-key", "copy", "DB", db},
			{"SWAPDB", f.Database, db},
		} {
			require.ErrorContains(t, fc.Do(ctx, cmd...).Err(), "NOPERM", fmt.Sprint(cmd))
		}
	}
	for _, cmd := range [][]any{
		{"FLUSHALL"}, {"ACL", "LIST"}, {"CONFIG", "GET", "*"}, {"MODULE", "LIST"},
		{"MONITOR"}, {"CLIENT", "LIST"}, {"FUNCTION", "LIST"},
		{"PUBLISH", "channel", "message"}, {"SUBSCRIBE", "channel"}, {"PSUBSCRIBE", "*"},
		{"SPUBLISH", "channel", "message"}, {"SSUBSCRIBE", "channel"}, {"PUBSUB", "CHANNELS"},
		{"UNSUBSCRIBE"}, {"PUNSUBSCRIBE"}, {"SUNSUBSCRIBE"},
	} {
		require.ErrorContains(t, fc.Do(ctx, cmd...).Err(), "NOPERM", fmt.Sprint(cmd))
	}
	// Scripts run with the caller's ACL, including SELECT and PUBLISH checks.
	require.Equal(t, "foo", fc.Eval(ctx, `return redis.call('GET', KEYS[1])`, []string{"same-key"}).Val())
	require.Error(t, fc.Eval(ctx, `return redis.call('SELECT', ARGV[1])`, nil, b.Database).Err())
	require.Error(t, fc.Eval(ctx, `return redis.call('PUBLISH', 'channel', 'message')`, nil).Err())
	// Even authenticating without SELECT cannot read broker metadata from DB 0.
	opts, err := redis.ParseURL(foo.GetURI())
	require.NoError(t, err)
	opts.DB = v.metadataDB
	wrongDB := redis.NewClient(opts)
	defer wrongDB.Close()
	require.ErrorContains(t, wrongDB.Get(ctx, instancePrefix+"foo").Err(), "NOPERM")
	admin := newClient(t, uri)
	require.NoError(t, admin.Publish(ctx, "channel", "admin-message").Err(), "administrator retains pub/sub")
	channels, err := admin.Do(ctx, "ACL", "GETUSER", f.Username).Result()
	require.NoError(t, err)
	require.NotContains(t, fmt.Sprint(channels), "&*")
	again, err := newAdapter(t, uri).GetOrCreateInstance(ctx, "foo")
	require.NoError(t, err)
	require.Equal(t, foo.GetURI(), again.GetURI())
	names, err := v.GetInstances(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, names)
}

func TestConcurrentProvisioning(t *testing.T) {
	uri := startValkey(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	const requests = 24
	peers := []*valkeyAdapter{newAdapter(t, uri), newAdapter(t, uri), newAdapter(t, uri)}
	type result struct {
		name string
		i    adapter.Instance
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, requests)
	for n := range requests {
		go func() {
			<-start
			name := "tenant_" + strconv.Itoa(n%6)
			i, err := peers[n%len(peers)].GetOrCreateInstance(ctx, name)
			results <- result{name, i, err}
		}()
	}
	close(start)
	canonical := map[string]string{}
	owners := map[int]string{}
	for range requests {
		r := <-results
		require.NoError(t, r.err)
		i := r.i.(*Instance)
		if saved := canonical[r.name]; saved != "" {
			require.Equal(t, saved, i.GetURI())
		} else {
			require.Empty(t, owners[i.Database], "different tenants cannot share a DB")
			owners[i.Database] = r.name
			canonical[r.name] = i.GetURI()
		}
	}
	for name, uri := range canonical {
		client := newClient(t, uri)
		require.NoError(t, client.Set(ctx, "proof", name, 0).Err())
		acl, err := peers[0].client.Do(ctx, "ACL", "LIST").StringSlice()
		require.NoError(t, err)
		found := false
		for _, entry := range acl {
			if strings.HasPrefix(entry, "user user_"+name+" ") {
				found = true
				require.Equal(t, 1, strings.Count(entry, "#"))
			}
		}
		require.True(t, found)
	}
	names, err := peers[0].GetInstances(ctx)
	require.NoError(t, err)
	require.Len(t, names, 6)
}

func TestACLPersistence(t *testing.T) {
	uri := startValkey(t)
	v := newAdapter(t, uri)
	if !v.persistACL {
		t.Skip("requires a disposable Valkey server with aclfile configured")
	}
	ctx := t.Context()
	// A failed ACL checkpoint must leave only the canonical reservation so a
	// different replica can retry without returning credentials that vanish.
	failed := newAdapter(t, uri)
	failed.client.AddHook(failACLSave{})
	_, err := failed.GetOrCreateInstance(ctx, "durable")
	require.ErrorIs(t, err, errInjected)
	require.Zero(t, v.client.Exists(ctx, instancePrefix+"durable").Val())
	reserved, err := v.client.Get(ctx, pendingPrefix+"durable").Result()
	require.NoError(t, err)
	canonical, err := v.decode("durable", reserved)
	require.NoError(t, err)
	i, err := v.GetOrCreateInstance(ctx, "durable")
	require.NoError(t, err)
	require.Equal(t, canonical.GetURI(), i.GetURI())

	// Loading the file must restore the same password and database restrictions.
	require.NoError(t, v.client.ACLDelUser(ctx, canonical.Username).Err())
	require.NoError(t, v.client.Do(ctx, "ACL", "LOAD").Err())
	client := newClient(t, i.GetURI())
	require.NoError(t, client.Set(ctx, "proof", "value", 0).Err())
	require.ErrorContains(t, client.Do(ctx, "SELECT", v.metadataDB).Err(), "NOPERM")
	require.ErrorContains(t, client.Publish(ctx, "channel", "value").Err(), "NOPERM")
}

type failACLSave struct{}

func (failACLSave) DialHook(next redis.DialHook) redis.DialHook { return next }
func (failACLSave) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (failACLSave) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "acl" && strings.EqualFold(fmt.Sprint(cmd.Args()[1]), "save") {
			return errInjected
		}
		return next(ctx, cmd)
	}
}

var errInjected = errors.New("injected provisioning failure")

type failProvision struct{ publication bool }

func (failProvision) DialHook(next redis.DialHook) redis.DialHook { return next }
func (failProvision) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (f failProvision) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if !f.publication && cmd.Name() == "acl" || f.publication && (cmd.Name() == "set" || cmd.Name() == "setnx") && strings.HasPrefix(cmd.Args()[1].(string), instancePrefix) {
			return errInjected
		}
		return next(ctx, cmd)
	}
}

func TestProvisioningRecovery(t *testing.T) {
	for _, publication := range []bool{false, true} {
		t.Run(fmt.Sprint(publication), func(t *testing.T) {
			uri := startValkey(t)
			failed, peer := newAdapter(t, uri), newAdapter(t, uri)
			failed.client.AddHook(failProvision{publication})
			_, err := failed.GetOrCreateInstance(t.Context(), "resume")
			require.ErrorIs(t, err, errInjected)
			data, err := peer.client.Get(t.Context(), pendingPrefix+"resume").Result()
			require.NoError(t, err)
			reserved, err := peer.decode("resume", data)
			require.NoError(t, err)
			names, err := peer.GetInstances(t.Context())
			require.NoError(t, err)
			require.Empty(t, names)
			// A resumed SETUSER must replace passwords, channel grants, and extra
			// selectors, rather than add restricted rules beside broad old ones.
			require.NoError(t, peer.client.Do(t.Context(), "ACL", "SETUSER", reserved.Username,
				"on", ">stale-pass", "+@all", "~*", "&*", "alldbs", "(+@all ~* &* alldbs)").Err())
			i, err := peer.GetOrCreateInstance(t.Context(), "resume")
			require.NoError(t, err)
			require.Equal(t, reserved.GetURI(), i.GetURI())
			client := newClient(t, i.GetURI())
			require.NoError(t, client.Set(t.Context(), "proof", "value", 0).Err())
			require.ErrorContains(t, client.Publish(t.Context(), "channel", "message").Err(), "NOPERM")
			require.ErrorContains(t, client.Do(t.Context(), "SELECT", peer.metadataDB).Err(), "NOPERM")
			stale, err := url.Parse(i.GetURI())
			require.NoError(t, err)
			stale.User = url.UserPassword(reserved.Username, "stale-pass")
			require.Error(t, newClient(t, stale.String()).Ping(t.Context()).Err())
			i2, err := failed.GetOrCreateInstance(t.Context(), "resume")
			require.NoError(t, err)
			require.Equal(t, i.GetURI(), i2.GetURI())
		})
	}
}

func TestDatabaseExhaustion(t *testing.T) {
	uri := startValkey(t)
	v := newAdapter(t, uri)
	ctx := t.Context()
	for n := 0; n < v.databases-1; n++ {
		_, err := v.GetOrCreateInstance(ctx, "tenant_"+strconv.Itoa(n))
		require.NoError(t, err)
	}
	_, err := v.GetOrCreateInstance(ctx, "overflow")
	require.ErrorContains(t, err, "no valkey databases available")
	require.Zero(t, v.client.Exists(ctx, pendingPrefix+"overflow", instancePrefix+"overflow").Val())
	users, err := v.client.Do(ctx, "ACL", "USERS").StringSlice()
	require.NoError(t, err)
	require.NotContains(t, users, "user_overflow")
	_, err = v.GetOrCreateInstance(ctx, "tenant_0")
	require.NoError(t, err, "existing credentials remain available at capacity")
}

func TestMetadataDatabaseAndAdminCollision(t *testing.T) {
	uri := startValkey(t)
	u, err := url.Parse(uri)
	require.NoError(t, err)
	u.Path = "/2"
	v := newAdapter(t, u.String())
	i, err := v.GetOrCreateInstance(t.Context(), "zero")
	require.NoError(t, err)
	require.Zero(t, i.(*Instance).Database)
	require.NoError(t, newClient(t, i.GetURI()).Set(t.Context(), "proof", "value", 0).Err())
	// The authenticated administrator must never be reset as a tenant account.
	admin := newClient(t, uri)
	require.NoError(t, admin.Do(t.Context(), "ACL", "SETUSER", "user_admin", "on", ">adminpass", "+@all", "~*", "&*", "alldbs").Err())
	u.User = url.UserPassword("user_admin", "adminpass")
	v2 := newAdapter(t, u.String())
	_, err = v2.GetOrCreateInstance(t.Context(), "admin")
	require.ErrorContains(t, err, "conflicts with administrator")
	require.NoError(t, v2.client.Publish(t.Context(), "channel", "message").Err())
}
