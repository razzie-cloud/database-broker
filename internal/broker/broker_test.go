package broker_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/broker"

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

func assertErrorStatusCode(t *testing.T, err error, statusCode int) {
	assert.Error(t, err)
	if errWithStatus, ok := err.(interface{ StatusCode() int }); ok {
		assert.Equal(t, statusCode, errWithStatus.StatusCode())
	} else {
		t.Errorf("error does not implement StatusCode method")
	}
}
