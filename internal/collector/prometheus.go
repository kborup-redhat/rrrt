package collector

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// MetricSample holds a single metric series returned from Prometheus.
type MetricSample struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Values    []float64
}

// PrometheusClient queries a Prometheus-compatible API.
type PrometheusClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewPrometheusClient creates a client that authenticates with a bearer token.
// If token is empty it falls back to the in-cluster service-account token.
func NewPrometheusClient(baseURL, token string) *PrometheusClient {
	if token == "" {
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
			token = strings.TrimSpace(string(data))
		}
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	pool := x509.NewCertPool()
	loaded := false
	if caCert, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"); err == nil {
		pool.AppendCertsFromPEM(caCert)
		loaded = true
	}
	if caCert, err := os.ReadFile("/etc/pki/tls/serving-ca/service-ca.crt"); err == nil {
		pool.AppendCertsFromPEM(caCert)
		loaded = true
	}
	if loaded {
		tlsCfg.RootCAs = pool
	}

	return &PrometheusClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:   tlsCfg,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

// NewPrometheusClientWithHTTP creates a client using the supplied http.Client
// (useful for testing with httptest.Server).
func NewPrometheusClientWithHTTP(baseURL string, httpClient *http.Client) *PrometheusClient {
	return &PrometheusClient{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// Query executes an instant PromQL query and returns the parsed samples.
func (c *PrometheusClient) Query(ctx context.Context, query string) ([]MetricSample, error) {
	params := url.Values{}
	params.Set("query", query)

	reqURL := fmt.Sprintf("%s/api/v1/query?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	var promResp prometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var samples []MetricSample
	for _, result := range promResp.Data.Result {
		s := MetricSample{
			Name:      result.Metric["name"],
			Namespace: result.Metric["namespace"],
			Labels:    result.Metric,
		}
		if len(result.Value) >= 2 && len(result.Values) == 0 {
			if valStr, ok := result.Value[1].(string); ok {
				if val, err := strconv.ParseFloat(valStr, 64); err == nil {
					s.Values = append(s.Values, val)
				}
			}
		}
		for _, v := range result.Values {
			if len(v) >= 2 {
				valStr, ok := v[1].(string)
				if !ok {
					continue
				}
				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					continue
				}
				s.Values = append(s.Values, val)
			}
		}
		samples = append(samples, s)
	}

	return samples, nil
}
