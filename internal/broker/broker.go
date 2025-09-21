package broker

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/razzie-cloud/database-broker/internal/adapter"
)

var validInstanceName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type Interface interface {
	RegisterAdapter(name string, adapter adapter.Interface)
	UnregisterAdapter(name string)
	GetInstances(ctx context.Context, adapterName string) ([]string, error)
	GetOrCreateInstance(ctx context.Context, adapterName, instanceName string) (adapter.Instance, error)
}

type broker struct {
	mu       sync.RWMutex
	adapters map[string]adapter.Interface
}

func New() Interface {
	return &broker{
		adapters: map[string]adapter.Interface{},
	}
}

func (b *broker) RegisterAdapter(name string, adapter adapter.Interface) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.adapters[name] = adapter
}

func (b *broker) UnregisterAdapter(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.adapters, name)
}

func (b *broker) GetInstances(ctx context.Context, adapterName string) ([]string, error) {
	b.mu.RLock()
	a, ok := b.adapters[adapterName]
	b.mu.RUnlock()
	if !ok {
		return nil, newError("adapter not found: %s", adapterName).WithStatusCode(http.StatusNotFound)
	}
	return a.GetInstances(ctx)
}

func (b *broker) GetOrCreateInstance(ctx context.Context, adapterName, instanceName string) (adapter.Instance, error) {
	instanceName = strings.ToLower(instanceName)
	if !validInstanceName.MatchString(instanceName) {
		return nil, newError("invalid instance name: %s", instanceName).WithStatusCode(http.StatusUnprocessableEntity)
	}
	b.mu.RLock()
	a, ok := b.adapters[adapterName]
	b.mu.RUnlock()
	if !ok {
		return nil, newError("adapter not found: %s", adapterName).WithStatusCode(http.StatusNotFound)
	}
	return a.GetOrCreateInstance(ctx, instanceName)
}
