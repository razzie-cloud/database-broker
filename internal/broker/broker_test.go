package broker_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/logging"

	"github.com/razzie/mock"
	"github.com/stretchr/testify/assert"
)

func TestRegisterAndUnregisterAdapter(t *testing.T) {
	expectedInstances := []string{"instance1", "instance2"}
	b := broker.New()

	a, m := mock.Mock[adapter.Interface]()
	m.On("GetInstances", mock.Anything).Return(expectedInstances, nil)
	b.RegisterAdapter("test", a)
	instances, err := b.GetInstances(context.Background(), "test")
	assert.NoError(t, err)
	assert.ElementsMatch(t, expectedInstances, instances)
	m.AssertExpectations(t)

	b.UnregisterAdapter("test")
	_, err = b.GetInstances(context.Background(), "test")
	assertErrorStatusCode(t, err, http.StatusNotFound)
}

func TestGetOrCreateInstance_InvalidName(t *testing.T) {
	b := broker.New()
	a, m := mock.Mock[adapter.Interface]()
	b.RegisterAdapter("test", a)
	_, err := b.GetOrCreateInstance(context.Background(), "test", "invalid-name!")
	assertErrorStatusCode(t, err, http.StatusUnprocessableEntity)
	m.AssertExpectations(t)
}

func TestGetOrCreateInstance_AdapterNotFound(t *testing.T) {
	b := broker.New()
	_, err := b.GetOrCreateInstance(context.Background(), "missing", "validname")
	assertErrorStatusCode(t, err, http.StatusNotFound)
}

func TestGetOrCreateInstance_NameTooLong(t *testing.T) {
	for _, err := range []error{
		adapter.ErrInstanceNameTooLong,
		logging.Wrap("validate_instance_name", fmt.Errorf("private backend detail: %w", adapter.ErrInstanceNameTooLong)),
	} {
		b := broker.New()
		a, m := mock.Mock[adapter.Interface]()
		name := strings.Repeat("a", 51)
		m.On("GetOrCreateInstance", mock.Anything, name).Return(nil, err)
		b.RegisterAdapter("postgres", a)
		instance, got := b.GetOrCreateInstance(context.Background(), "postgres", name)
		assert.Nil(t, instance)
		assertErrorStatusCode(t, got, http.StatusUnprocessableEntity)
		assert.EqualError(t, got, adapter.ErrInstanceNameTooLong.Error())
		m.AssertExpectations(t)
	}
}

func TestGetOrCreateInstance_PreservesOtherAdapterResults(t *testing.T) {
	for _, backendErr := range []error{nil, errors.New("backend unavailable")} {
		b := broker.New()
		a, m := mock.Mock[adapter.Interface]()
		i, _ := mock.Mock[adapter.Instance]()
		name := strings.Repeat("a", 128)
		m.On("GetOrCreateInstance", mock.Anything, name).Return(i, backendErr)
		b.RegisterAdapter("dragonfly", a)
		instance, err := b.GetOrCreateInstance(context.Background(), "dragonfly", strings.ToUpper(name))
		assert.Equal(t, i, instance)
		assert.Equal(t, backendErr, err)
		m.AssertExpectations(t)
	}
}

func assertErrorStatusCode(t *testing.T, err error, statusCode int) {
	assert.Error(t, err)
	if errWithStatus, ok := err.(interface{ StatusCode() int }); ok {
		assert.Equal(t, statusCode, errWithStatus.StatusCode())
	} else {
		t.Errorf("error does not implement StatusCode method")
	}
}
