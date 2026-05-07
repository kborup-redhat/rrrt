---
title: "Chapter 2: Prometheus Client"
order: 2
---

# Chapter 2: Prometheus Client

## Introduction

The Prometheus client is RRRT's window into cluster metrics. Think of it as a translator between PromQL (the query language for Prometheus) and Go data structures. It handles the HTTP connection, TLS certificates, authentication, and response parsing so that the rest of the codebase can simply ask "give me the CPU utilization for this VM" and get back a slice of floats.

## How It Works

The client lives in `internal/collector/prometheus.go` and exposes a single public method: `Query`. Everything else — TLS configuration, bearer token loading, HTTP request construction, JSON response parsing — is internal.

### Creating a Client

```go
func NewPrometheusClient(baseURL, token string) *PrometheusClient {
    if token == "" {
        if data, err := os.ReadFile(
            "/var/run/secrets/kubernetes.io/serviceaccount/token",
        ); err == nil {
            token = strings.TrimSpace(string(data))
        }
    }
    tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
    if caCert, err := os.ReadFile(
        "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
    ); err == nil {
        pool := x509.NewCertPool()
        pool.AppendCertsFromPEM(caCert)
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
```

When the analyzer runs inside a Kubernetes Pod, it automatically picks up the service account token and CA certificate from the standard mount paths. This is the standard pattern for in-cluster authentication — no manual credential configuration needed.

The TLS configuration enforces TLS 1.2 as a minimum version and loads the cluster's CA certificate so the client can verify the Thanos/Prometheus server's identity. `MaxIdleConnsPerHost: 10` allows connection reuse across the many queries the collector makes.

For testing, there's a simpler constructor:

```go
func NewPrometheusClientWithHTTP(baseURL string, httpClient *http.Client) *PrometheusClient
```

This accepts an `*http.Client` directly, which makes it easy to point at an `httptest.Server` in tests.

### Executing Queries

```go
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
    // ...
}
```

The method uses the standard Prometheus HTTP API (`/api/v1/query`) with the PromQL expression passed as a query parameter. Context propagation ensures that if the parent operation is cancelled (e.g., timeout or signal), the HTTP request is also cancelled.

### Response Parsing

The Prometheus API returns JSON with a specific structure. The client parses it into `MetricSample` structs:

```go
type MetricSample struct {
    Name      string
    Namespace string
    Labels    map[string]string
    Values    []float64
}
```

Each `MetricSample` represents one time-series (one VM or container). The `Values` slice contains the raw utilization data points that the calculator will later analyze. The parsing loop extracts string values from the `[timestamp, value]` pairs that Prometheus returns:

```go
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
```

Malformed data points are silently skipped rather than causing the entire query to fail — this is intentional, since partial data is still useful for analysis.

### PromQL Query Builders

The companion file `queries.go` builds the actual PromQL expressions with proper input sanitization:

```go
func vmCPUQuery(vmName, namespace, lookback string) string {
    return fmt.Sprintf(
        `rate(kubevirt_vmi_cpu_usage_seconds_total{name="%s",namespace="%s"}[5m])[%s:1m]`,
        SanitizeLabelValue(vmName), SanitizeLabelValue(namespace), lookback,
    )
}
```

Two sanitization functions protect against injection:

- `SanitizeLabelValue` escapes backslashes, double quotes, and strips newlines for exact-match (`=`) selectors
- `SanitizeRegexValue` additionally applies `regexp.QuoteMeta` for regex-match (`=~`) selectors, escaping characters like `.`, `+`, and `*` that have special meaning in regular expressions

Container queries use regex matching (`pod=~"workload-.*"`) because a Deployment's pods have generated suffixes. VM queries use exact matching because KubeVirt VMI names match the VM name directly.

## Relationships

- The **Collector** calls `Query` for every resource it analyzes, passing in PromQL built by the query builder functions
- **MetricSample.Values** flows directly into the **Calculator** for percentile computation
- The **query builders** depend on input from the Collector (VM name, namespace, lookback period)

## Key Takeaways

- In-cluster authentication uses the standard service account token and CA certificate mount paths
- The client enforces TLS 1.2 minimum and validates the server's certificate against the cluster CA
- PromQL queries are parameterized with sanitized inputs to prevent injection
- Container workloads use regex matching (escaped with `regexp.QuoteMeta`), VMs use exact matching
- The `NewPrometheusClientWithHTTP` constructor enables clean unit testing with `httptest.Server`

Next, we'll see how the Calculator takes these raw metric values and turns them into actionable rightsizing recommendations.
