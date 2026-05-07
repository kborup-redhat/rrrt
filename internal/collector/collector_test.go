package collector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kborup-redhat/rrrt/internal/collector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusClient_QueryRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "matrix",
				"result": []map[string]interface{}{
					{
						"metric": map[string]string{"name": "test-vm", "namespace": "default"},
						"values": [][]interface{}{
							{1620000000.0, "0.25"},
							{1620000060.0, "0.30"},
							{1620000120.0, "0.28"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := collector.NewPrometheusClientWithHTTP(server.URL, server.Client())
	samples, err := client.Query(context.Background(), "test_query")
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, "test-vm", samples[0].Name)
	assert.Len(t, samples[0].Values, 3)
	assert.InDelta(t, 0.25, samples[0].Values[0], 0.01)
}

func TestSanitizeLabelValue(t *testing.T) {
	assert.Equal(t, `test\"quote`, collector.SanitizeLabelValue(`test"quote`))
	assert.Equal(t, `test\\slash`, collector.SanitizeLabelValue(`test\slash`))
}

func TestSanitizeRegexValue(t *testing.T) {
	assert.Equal(t, `my-app\.v2`, collector.SanitizeRegexValue("my-app.v2"))
	assert.Equal(t, `simple`, collector.SanitizeRegexValue("simple"))
	assert.Equal(t, `has\+plus`, collector.SanitizeRegexValue("has+plus"))
}
