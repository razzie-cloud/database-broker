package dragonfly

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/logging"
	"github.com/razzie-cloud/database-broker/internal/util"

	"github.com/redis/go-redis/v9"
)

type dragonflyAdapter struct {
	client *redis.Client
	host   string
	port   int
}

func New(dragonflyURI string) (adapter.Interface, error) {
	redis.SetLogger(logging.RedisLogger{})
	host, port, err := util.GetURIHostPort(dragonflyURI, 5432)
	if err != nil {
		return nil, fmt.Errorf("parse dragonfly uri: %w", err)
	}
	opts, err := redis.ParseURL(dragonflyURI)
	if err != nil {
		return nil, err
	}

	// Honor probe deadlines for socket reads/writes as well as connection setup.
	opts.ContextTimeoutEnabled = true
	client := redis.NewClient(opts)
	return &dragonflyAdapter{
		client: client,
		host:   host,
		port:   port,
	}, nil
}

func (d *dragonflyAdapter) Ping(ctx context.Context) error {
	return d.client.Ping(ctx).Err()
}

func (d *dragonflyAdapter) GetInstances(ctx context.Context) ([]string, error) {
	iter := d.client.Scan(ctx, 0, "instance:*", 100).Iterator()
	var instances []string
	for iter.Next(ctx) {
		key := iter.Val()
		parts := strings.SplitN(key, ":", 2)
		if len(parts) == 2 {
			instances = append(instances, parts[1])
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}
	return instances, nil
}

func (d *dragonflyAdapter) GetOrCreateInstance(ctx context.Context, instanceName string) (result adapter.Instance, err error) {
	logging.Fields(ctx, "adapter", "dragonfly", "instance_name", instanceName[:min(len(instanceName), 128)])
	start := time.Now()
	stage := "lookup_instance"
	defer func() {
		if err != nil {
			err = logging.Wrap(stage, err)
		}
	}()
	instance, err := d.getInstance(ctx, instanceName)
	if err != nil {
		return nil, err
	}
	if instance != nil {
		return instance, nil
	}
	user := "user_" + instanceName
	pass := util.RandPassword()
	ns := "ns_" + instanceName
	instance = &Instance{
		Instance:  instanceName,
		Host:      d.host,
		Port:      d.port,
		Username:  user,
		Password:  pass,
		Namespace: ns,
		CreatedAt: time.Now().UTC(),
	}
	stage = "encode_metadata"
	data, err := json.Marshal(instance)
	if err != nil {
		return nil, fmt.Errorf("marshal instance data: %w", err)
	}
	// Reserve canonical credentials before any ACL mutation. Keep this reservation
	// even after publication: a paused/crashed replica must never resume using a
	// newly generated password. SET NX acts as a shared, durable first-writer
	// election without an expiring lock that could admit stale provisioning work.
	stage = "reserve_credentials"
	reservation := "instance-pending:" + instanceName
	if err := d.client.SetNX(ctx, reservation, string(data), 0).Err(); err != nil {
		return nil, fmt.Errorf("reserve instance data: %w", err)
	}
	reserved, err := d.client.Get(ctx, reservation).Result()
	if err != nil {
		return nil, fmt.Errorf("load reserved instance data: %w", err)
	}
	instance, err = d.unmarshalInstance(instanceName, reserved)
	if err != nil {
		return nil, err
	}
	// A peer may have completed provisioning while we reserved/read credentials.
	stage = "reload_metadata"
	if published, err := d.getInstance(ctx, instanceName); err != nil {
		return nil, err
	} else if published != nil {
		return published, nil
	}
	stage = "create_acl_user"
	if err := d.createUser(ctx, instance.Username, instance.Password, instance.Namespace); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	stage = "save_metadata"
	ok, err := d.client.SetNX(ctx, "instance:"+instanceName, reserved, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("save instance data: %w", err)
	}
	if !ok {
		stage = "reload_metadata"
		return d.getInstance(ctx, instanceName)
	}
	logging.For(ctx, "adapter", "dragonfly", "instance_name", instanceName[:min(len(instanceName), 128)]).Info("Instance provisioned", "event", "instance.provisioned", "duration_ms", time.Since(start).Milliseconds())
	return instance, nil
}

func (d *dragonflyAdapter) Close() error {
	return d.client.Close()
}

func (d *dragonflyAdapter) getInstance(ctx context.Context, instanceName string) (*Instance, error) {
	data, err := d.client.Get(ctx, "instance:"+instanceName).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get instance data: %w", err)
	}
	return d.unmarshalInstance(instanceName, data)
}

func (d *dragonflyAdapter) unmarshalInstance(instanceName, data string) (*Instance, error) {
	var instance Instance
	instance.Instance = instanceName
	instance.Host = d.host
	instance.Port = d.port
	if err := json.Unmarshal([]byte(data), &instance); err != nil {
		return nil, fmt.Errorf("unmarshal instance data: %w", err)
	}
	return &instance, nil
}

func (d *dragonflyAdapter) createUser(ctx context.Context, username, password, namespace string) error {
	return d.client.Do(ctx, "ACL",
		"SETUSER", username,
		"NAMESPACE:"+namespace,
		"ON",
		">"+password,
		"+@all", "-@admin", "-ACL", "-CONFIG", "-MODULE", "-CLUSTER",
		"~*",
	).Err()
}
