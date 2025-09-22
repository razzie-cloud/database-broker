package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/broker"

	"github.com/razzie/mock"
	"github.com/stretchr/testify/assert"
)

func TestRouter_ListInstances(t *testing.T) {
	b, bmock := mock.Mock[broker.Interface]()
	bmock.On("GetInstances", mock.Anything, "test").Return([]string{"instance1", "instance2"}, nil)

	h := New(b)
	req := httptest.NewRequest("GET", "/v1/instances/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "instance1")
	assert.Contains(t, w.Body.String(), "instance2")
	bmock.AssertExpectations(t)
}

func TestRouter_GetInstance(t *testing.T) {
	i, imock := mock.Mock[adapter.Instance]()
	imock.On("GetJSON").Return(map[string]string{"instance": "instance1"})
	b, bmock := mock.Mock[broker.Interface]()
	bmock.On("GetOrCreateInstance", mock.Anything, "test", "instance1").Return(i, nil)

	h := New(b)
	req := httptest.NewRequest("GET", "/v1/instances/test/instance1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "instance1")
	imock.AssertExpectations(t)
	bmock.AssertExpectations(t)
}

func TestRouter_GetInstanceURI(t *testing.T) {
	i, imock := mock.Mock[adapter.Instance]()
	imock.On("GetURI").Return("mock://uri")
	b, bmock := mock.Mock[broker.Interface]()
	bmock.On("GetOrCreateInstance", mock.Anything, "test", "instance1").Return(i, nil)

	h := New(b)
	req := httptest.NewRequest("GET", "/v1/instances/test/instance1/uri", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "mock://uri", w.Body.String())
	imock.AssertExpectations(t)
	bmock.AssertExpectations(t)
}
