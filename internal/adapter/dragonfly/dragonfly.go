package dragonfly

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/util"

	"github.com/redis/go-redis/v9"
)

type dragonflyAdapter struct {
	client *redis.Client
	host   string
	port   int
}

func New(dragonflyURI string) (adapter.Interface, error) {
	host, port, err := util.GetURIHostPort(dragonflyURI, 5432)
	if err != nil {
		return nil, fmt.Errorf("parse dragonfly uri: %w", err)
	}
	opts, err := redis.ParseURL(dragonflyURI)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)
	return &dragonflyAdapter{
		client: client,
		host:   host,
		port:   port,
	}, nil
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

func (d *dragonflyAdapter) GetOrCreateInstance(ctx context.Context, instanceName string) (adapter.Instance, error) {
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
	data, err := json.Marshal(instance)
	if err != nil {
		return nil, fmt.Errorf("marshal instance data: %w", err)
	}
	ok, err := d.client.SetNX(ctx, "instance:"+instanceName, string(data), 0).Result()
	if err != nil {
		return nil, fmt.Errorf("save instance data: %w", err)
	}
	if !ok {
		return d.getInstance(ctx, instanceName)
	}
	if err := d.createUser(ctx, user, pass, ns); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
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
		"NAMESPACE", namespace,
		"ON",
		"RESETPASS", ">"+password,
		"+@all", "-@admin", "-ACL", "-CONFIG", "-MODULE", "-CLUSTER",
		"::"+namespace,
	).Err()
}
