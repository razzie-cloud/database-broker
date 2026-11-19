package adapter

import (
	"context"
	"errors"
)

var ErrInstanceNameTooLong = errors.New("instance name is too long to provision")

type Instance interface {
	GetJSON() any
	GetURI() string
}

type Interface interface {
	Ping(context.Context) error
	GetInstances(ctx context.Context) ([]string, error)
	GetOrCreateInstance(ctx context.Context, instanceName string) (Instance, error)
	Close() error
}
