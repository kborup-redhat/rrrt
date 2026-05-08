package collector_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kborup-redhat/rrrt/internal/collector"
	"github.com/stretchr/testify/assert"
)

func TestDiscoverOVRO_HealthProbeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ok := collector.ProbeHealth(context.Background(), server.URL)
	assert.True(t, ok)
}

func TestDiscoverOVRO_HealthProbeFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ok := collector.ProbeHealth(context.Background(), server.URL)
	assert.False(t, ok)
}

func TestDiscoverOVRO_HealthProbeUnreachable(t *testing.T) {
	ok := collector.ProbeHealth(context.Background(), "http://127.0.0.1:1")
	assert.False(t, ok)
}
