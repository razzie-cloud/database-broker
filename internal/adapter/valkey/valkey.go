// Package valkey provisions isolated numbered databases on Valkey 9.1+.
package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/logging"
	"github.com/razzie-cloud/database-broker/internal/util"
	"github.com/redis/go-redis/v9"
)

const (
	instancePrefix = "valkey:instance:"
	pendingPrefix  = "valkey:instance-pending:"
	ownersKey      = "valkey:database-owners"
)

type valkeyAdapter struct {
	client     *redis.Client
	host       string
	port       int
	tls        bool
	metadataDB int
	databases  int
	admin      string
	persistACL bool
}

func New(uri string) (adapter.Interface, error) {
	redis.SetLogger(logging.RedisLogger{})
	host, port, err := util.GetURIHostPort(uri, 6379)
	if err != nil {
		return nil, fmt.Errorf("parse valkey uri: %w", err)
	}
	opts, err := redis.ParseURL(uri)
	if err != nil {
		return nil, err
	}
	opts.ContextTimeoutEnabled = true
	v := &valkeyAdapter{client: redis.NewClient(opts), host: host, port: port,
		tls: opts.TLSConfig != nil, metadataDB: opts.DB}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := v.initialize(ctx); err != nil {
		_ = v.Close()
		return nil, err
	}
	return v, nil
}

func (v *valkeyAdapter) initialize(ctx context.Context) error {
	info, err := v.client.Info(ctx, "server").Result()
	if err != nil {
		return fmt.Errorf("read valkey version: %w", err)
	}
	if err := checkVersion(info); err != nil {
		return err
	}
	cluster, err := v.client.Info(ctx, "cluster").Result()
	if err != nil {
		return fmt.Errorf("read valkey cluster mode: %w", err)
	}
	if infoField(cluster, "cluster_enabled") != "0" {
		return errors.New("valkey adapter requires standalone mode")
	}
	config, err := v.client.ConfigGet(ctx, "databases").Result()
	if err != nil {
		return fmt.Errorf("read valkey database count: %w", err)
	}
	v.databases, err = strconv.Atoi(config["databases"])
	if err != nil || v.databases < 2 || v.metadataDB < 0 || v.metadataDB >= v.databases {
		return errors.New("valkey requires at least two databases and a valid metadata database")
	}
	config, err = v.client.ConfigGet(ctx, "aclfile").Result()
	if err != nil {
		return fmt.Errorf("read valkey ACL file configuration: %w", err)
	}
	v.persistACL = config["aclfile"] != ""
	v.admin, err = v.client.Do(ctx, "ACL", "WHOAMI").Text()
	if err != nil {
		return fmt.Errorf("read valkey admin identity: %w", err)
	}
	return nil
}

func infoField(info, field string) string {
	for line := range strings.SplitSeq(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && key == field {
			return value
		}
	}
	return ""
}

func checkVersion(info string) error {
	// Redis's compatibility version is unrelated to the Valkey version. Require
	// the Valkey-specific field and reject malformed and prerelease versions.
	version := infoField(info, "valkey_version")
	parts := strings.Split(version, ".")
	var numbers [3]int
	if len(parts) != len(numbers) {
		return errors.New("valkey 9.1.0 or newer is required for database ACLs")
	}
	for i, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return errors.New("valkey 9.1.0 or newer is required for database ACLs")
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return errors.New("invalid valkey version")
		}
		numbers[i] = n
	}
	if numbers[0] < 9 || numbers[0] == 9 && numbers[1] < 1 {
		return errors.New("valkey 9.1.0 or newer is required for database ACLs")
	}
	return nil
}

func (v *valkeyAdapter) Ping(ctx context.Context) error { return v.client.Ping(ctx).Err() }
func (v *valkeyAdapter) Close() error                   { return v.client.Close() }

func (v *valkeyAdapter) GetInstances(ctx context.Context) ([]string, error) {
	iter := v.client.Scan(ctx, 0, instancePrefix+"*", 100).Iterator()
	instances := make(map[string]struct{})
	for iter.Next(ctx) {
		instances[strings.TrimPrefix(iter.Val(), instancePrefix)] = struct{}{}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("iterate valkey instances: %w", err)
	}
	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (v *valkeyAdapter) GetOrCreateInstance(ctx context.Context, name string) (result adapter.Instance, err error) {
	logging.Fields(ctx, "adapter", "valkey", "instance_name", name[:min(len(name), 128)])
	start := time.Now()
	stage := "lookup_instance"
	defer func() {
		if err != nil {
			err = logging.Wrap(stage, err)
		}
	}()
	if instance, err := v.getInstance(ctx, name); err != nil || instance != nil {
		return instance, err
	}
	stage = "reserve_database"
	reserved, err := v.reserve(ctx, name)
	if err != nil {
		return nil, err
	}
	instance, err := v.decode(name, reserved)
	if err != nil {
		return nil, err
	}
	stage = "create_acl_user"
	// Reset all preexisting permissions/selectors/passwords before applying the
	// canonical reservation. Never grant module commands or shared functions.
	// Pub/sub is global across databases: deny the category AND all channels.
	err = v.client.Do(ctx, "ACL", "SETUSER", instance.Username,
		"reset", "on", ">"+instance.Password, "~*", "db="+strconv.Itoa(instance.Database),
		"+@read", "+@write", "+@connection", "+@transaction", "+@scripting",
		"-@dangerous", "-@admin", "-@pubsub", "resetchannels",
		"-function", "-script", "+script|load", "+script|exists",
		"-client", "+client|setname", "+client|getname", "+client|setinfo", "+client|id",
	).Err()
	if err != nil {
		return nil, fmt.Errorf("create valkey user: %w", err)
	}
	// ACLs are separate from RDB/AOF data. Persist them before publishing
	// credentials so a restart cannot leave metadata pointing at a missing user.
	if v.persistACL {
		stage = "save_acl"
		if err := v.client.Do(ctx, "ACL", "SAVE").Err(); err != nil {
			return nil, fmt.Errorf("save valkey ACLs: %w", err)
		}
	}
	stage = "save_metadata"
	ok, err := v.client.SetNX(ctx, instancePrefix+name, reserved, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("save valkey instance: %w", err)
	}
	if !ok {
		return v.getInstance(ctx, name)
	}
	logging.For(ctx, "adapter", "valkey", "instance_name", name[:min(len(name), 128)]).Info("Instance provisioned", "event", "instance.provisioned", "duration_ms", time.Since(start).Milliseconds())
	return instance, nil
}

func (v *valkeyAdapter) getInstance(ctx context.Context, name string) (*Instance, error) {
	data, err := v.client.Get(ctx, instancePrefix+name).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get valkey instance: %w", err)
	}
	return v.decode(name, data)
}

func (v *valkeyAdapter) decode(name, data string) (*Instance, error) {
	var instance Instance
	if err := json.Unmarshal([]byte(data), &instance); err != nil {
		return nil, fmt.Errorf("decode valkey instance: %w", err)
	}
	if instance.Database == v.metadataDB || instance.Database < 0 || instance.Database >= v.databases ||
		instance.Username != "user_"+name || instance.Username == v.admin || instance.Password == "" {
		return nil, errors.New("invalid valkey instance reservation")
	}
	instance.Instance, instance.Host, instance.Port, instance.TLS = name, v.host, v.port, v.tls
	return &instance, nil
}

func (v *valkeyAdapter) reserve(ctx context.Context, name string) (string, error) {
	proposal := Instance{Instance: name, Username: "user_" + name,
		Password: util.RandPassword(), CreatedAt: time.Now().UTC()}
	if proposal.Username == v.admin {
		return "", errors.New("valkey instance username conflicts with administrator")
	}
	pending := pendingPrefix + name
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var reserved string
		// WATCH pins the reads and MULTI/EXEC to one connection. The database
		// owner and credentials become durable together; no leases can expire
		// while a paused replica still holds credentials for an allocated DB.
		err := v.client.Watch(ctx, func(tx *redis.Tx) error {
			var err error
			reserved, err = tx.Get(ctx, pending).Result()
			if err == nil {
				return nil
			}
			if !errors.Is(err, redis.Nil) {
				return err
			}
			owners, err := tx.HGetAll(ctx, ownersKey).Result()
			if err != nil {
				return err
			}
			for db := 0; db < v.databases; db++ {
				id := strconv.Itoa(db)
				if db == v.metadataDB || owners[id] != "" {
					continue
				}
				proposal.Database = db
				data, err := json.Marshal(proposal)
				if err != nil {
					return err
				}
				reserved = string(data)
				_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					pipe.HSet(ctx, ownersKey, id, name)
					pipe.Set(ctx, pending, reserved, 0)
					return nil
				})
				return err
			}
			return errors.New("no valkey databases available")
		}, pending, ownersKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return reserved, err
	}
}
