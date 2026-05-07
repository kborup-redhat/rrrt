# RRRT Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a kubectl/oc CLI plugin that dispatches an analysis Job on-cluster, collects VM and container utilization metrics from Prometheus, runs a rightsizing calculator, and generates a professional PDF report with charts and justification text.

**Architecture:** Two Go binaries in one repo — a thin CLI (`oc-rrrt`) that orchestrates an ephemeral namespace/Job/cleanup lifecycle, and an analyzer container that queries Prometheus, runs the calculator, and generates a PDF. Shared packages for types, calculator logic, and owner resolution.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra` v1.10.x (CLI), `k8s.io/client-go` (Kubernetes API), `github.com/go-pdf/fpdf` v0.9.x (PDF), `github.com/wcharczuk/go-chart/v2` v2.1.x (charts), Red Hat UBI9 minimal base image, GitHub Actions CI/CD, `quay.io/kborup/rrrt` container registry.

---

## File Structure

```
rrrt/
├── .github/
│   └── workflows/
│       ├── ci.yaml
│       ├── release.yaml
│       └── docs.yaml
├── cmd/
│   ├── oc-rrrt/
│   │   └── main.go              # CLI entry point: root + report command
│   └── analyzer/
│       └── main.go              # Analyzer entry point: reads env, runs pipeline, writes PDF
├── internal/
│   ├── types/
│   │   └── types.go             # Shared data structures (ResourceAnalysis, ReportData, etc.)
│   ├── calculator/
│   │   ├── calculator.go        # Percentile + headroom analysis (ported from OVRO)
│   │   └── calculator_test.go
│   ├── owner/
│   │   ├── resolver.go          # Owner label resolution (VM, Deployment, namespace fallback)
│   │   └── resolver_test.go
│   ├── collector/
│   │   ├── prometheus.go        # Prometheus HTTP client with bearer token auth
│   │   ├── queries.go           # Query builders for VM + container metrics
│   │   ├── vm.go                # VM discovery + metric collection
│   │   ├── container.go         # Deployment/StatefulSet discovery + metric collection
│   │   ├── collector.go         # Top-level Collector that orchestrates VM + container collection
│   │   └── collector_test.go
│   ├── pdf/
│   │   ├── report.go            # Top-level report orchestrator (calls sub-renderers)
│   │   ├── cover.go             # Cover page renderer
│   │   ├── summary.go           # Executive summary with charts
│   │   ├── tables.go            # Resource summary tables (VM + container)
│   │   ├── details.go           # Detail cards with utilization charts + justification
│   │   ├── charts.go            # go-chart wrappers (line charts, donut chart)
│   │   ├── insufficient.go      # Insufficient data section
│   │   ├── appendix.go          # Appendix + footer
│   │   └── report_test.go
│   └── cli/
│       ├── run.go               # Main orchestration: permission check, namespace, RBAC, Job, stream, copy, cleanup
│       ├── rbac.go              # ClusterRole, ClusterRoleBinding, ServiceAccount creation
│       ├── job.go               # Job spec builder
│       ├── copy.go              # Pod file copy via tar stream API
│       ├── console.go           # Console URL auto-detection
│       └── cleanup.go           # Signal handler + cleanup logic
├── Containerfile
├── go.mod
├── go.sum
└── README.md
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/oc-rrrt/main.go`
- Create: `cmd/analyzer/main.go`
- Create: `internal/types/types.go`

- [ ] **Step 1: Initialize the Go module**

```bash
cd /home/kborup/ai-code/rrrt
go mod init github.com/kborup-redhat/rrrt
```

- [ ] **Step 2: Create shared types**

Create `internal/types/types.go`:

```go
package types

const (
	LabelOwner      = "rightsizing.redhatconsulting.io/owner"
	AnnotationExclude = "rightsizing.redhatconsulting.io/exclude"

	DefaultPrometheusURL = "https://thanos-querier.openshift-monitoring.svc:9091"
	DefaultLookbackDays  = 14
	DefaultPercentile    = 95
	DefaultHeadroom      = 20
	DefaultUpsizeThreshold = 90

	DefaultVMMinCPUSavings    = 1000    // millicores (1 core)
	DefaultVMMinMemSavings    = 1 << 30 // 1 GiB in bytes

	DefaultContainerMinCPUSavings = 250     // millicores (250m)
	DefaultContainerMinMemSavings = 256 << 20 // 256 Mi in bytes
)

type Direction string

const (
	Downsize Direction = "downsize"
	Upsize   Direction = "upsize"
)

type ResourceKind string

const (
	KindVM          ResourceKind = "VirtualMachine"
	KindDeployment  ResourceKind = "Deployment"
	KindStatefulSet ResourceKind = "StatefulSet"
)

type ResourceAnalysis struct {
	Namespace   string
	Name        string
	Kind        ResourceKind
	Owner       string
	ConsoleURL  string
	Direction   Direction
	CurrentCPU  int64 // millicores
	CurrentMem  int64 // bytes
	RecommendedCPU int64
	RecommendedMem int64
	CPUSavings  int64
	MemSavings  int64
	CPUP95      float64
	MemP95      float64
	CPUMax      float64
	MemMax      float64
	CPUSamples  []float64 // raw time-series for charts
	MemSamples  []float64
	Justification string
}

type InsufficientDataEntry struct {
	Namespace      string
	Name           string
	Kind           ResourceKind
	DataPoints     int
	ExpectedPoints int
}

type ReportData struct {
	ClusterName    string
	GeneratedAt    string
	Scope          string
	LookbackDays   int
	Percentile     int
	HeadroomPct    int
	VMAnalyses     []ResourceAnalysis
	ContainerAnalyses []ResourceAnalysis
	InsufficientData  []InsufficientDataEntry
	CLIVersion     string
	ImageVersion   string
}

type AnalyzerConfig struct {
	Namespaces       []string
	LookbackDays     int
	ConsoleURL       string
	IncludeOpenShift bool
	PrometheusURL    string
}
```

- [ ] **Step 3: Create CLI entry point stub**

Create `cmd/oc-rrrt/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "oc-rrrt",
		Short: "Resource Rightsizing Reporting Tool",
	}

	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a rightsizing report",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("report command not yet implemented")
			return nil
		},
	}

	reportCmd.Flags().StringSliceP("namespace", "n", nil, "Namespaces to analyze (can be repeated)")
	reportCmd.Flags().Bool("all-namespaces", true, "Analyze all accessible namespaces")
	reportCmd.Flags().StringP("output", "o", "", "Output PDF path")
	reportCmd.Flags().Int("lookback-days", 14, "Metrics lookback window in days")
	reportCmd.Flags().String("image", "", "Override analyzer container image")
	reportCmd.Flags().String("console-url", "", "OpenShift Console base URL")
	reportCmd.Flags().Bool("include-openshift", false, "Include openshift-* and kube-* namespaces")
	reportCmd.Flags().Bool("keep-namespace", false, "Don't delete temporary namespace after completion")
	reportCmd.Flags().Duration("timeout", 30*60*1e9, "Maximum time to wait for the Job")

	rootCmd.AddCommand(reportCmd)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Create analyzer entry point stub**

Create `cmd/analyzer/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	fmt.Println("rrrt-analyzer", version)
	fmt.Println("analyzer not yet implemented")
	os.Exit(0)
}
```

- [ ] **Step 5: Install dependencies and verify build**

```bash
cd /home/kborup/ai-code/rrrt
go get github.com/spf13/cobra@v1.10.2
go mod tidy
go build ./...
```

Expected: builds with no errors.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum cmd/ internal/types/
git commit -m "feat: scaffold project with CLI and analyzer stubs"
```

---

### Task 2: Calculator Package

**Files:**
- Create: `internal/calculator/calculator.go`
- Create: `internal/calculator/calculator_test.go`

This is ported from OVRO but adapted for millicore/byte units instead of whole cores/GiB, so it works for both VMs and containers.

- [ ] **Step 1: Write the failing tests**

Create `internal/calculator/calculator_test.go`:

```go
package calculator_test

import (
	"testing"

	"github.com/kborup-redhat/rrrt/internal/calculator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputePercentile_Empty(t *testing.T) {
	assert.Equal(t, 0.0, calculator.ComputePercentile(nil, 95))
}

func TestComputePercentile_Single(t *testing.T) {
	assert.InDelta(t, 42.0, calculator.ComputePercentile([]float64{42.0}, 95), 0.01)
}

func TestComputePercentile_Multiple(t *testing.T) {
	samples := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	p95 := calculator.ComputePercentile(samples, 95)
	assert.InDelta(t, 95.5, p95, 1.0)
}

func TestAnalyze_Downsize(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:       8000, // 8 cores in millicores
		CurrentMem:       16 * 1024 * 1024 * 1024,
		CPUP95Percent:    28.3,
		MemP95Percent:    41.7,
		HeadroomPercent:  20,
		MinCPUSavings:    1000, // 1 core
		MinMemSavings:    1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, calculator.Downsize, result.Direction)
	assert.Less(t, result.RecommendedCPU, input.CurrentCPU)
	assert.Less(t, result.RecommendedMem, input.CurrentMem)
	assert.Greater(t, result.CPUSavings, int64(0))
	assert.Greater(t, result.MemSavings, int64(0))
}

func TestAnalyze_Upsize(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:       4000,
		CurrentMem:       8 * 1024 * 1024 * 1024,
		CPUP95Percent:    94.0,
		MemP95Percent:    92.0,
		HeadroomPercent:  20,
		MinCPUSavings:    1000,
		MinMemSavings:    1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, calculator.Upsize, result.Direction)
	assert.Greater(t, result.RecommendedCPU, input.CurrentCPU)
	assert.Greater(t, result.RecommendedMem, input.CurrentMem)
}

func TestAnalyze_NoRecommendation(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:       4000,
		CurrentMem:       8 * 1024 * 1024 * 1024,
		CPUP95Percent:    75.0,
		MemP95Percent:    75.0,
		HeadroomPercent:  20,
		MinCPUSavings:    1000,
		MinMemSavings:    1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	assert.Nil(t, result)
}

func TestAnalyze_ContainerLowThreshold(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:       1000, // 1 core
		CurrentMem:       1 * 1024 * 1024 * 1024,
		CPUP95Percent:    20.0,
		MemP95Percent:    20.0,
		HeadroomPercent:  20,
		MinCPUSavings:    250,       // 250m
		MinMemSavings:    256 << 20, // 256Mi
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, calculator.Downsize, result.Direction)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/kborup/ai-code/rrrt
go get github.com/stretchr/testify@latest
go test ./internal/calculator/ -v
```

Expected: FAIL — package/functions don't exist yet.

- [ ] **Step 3: Implement the calculator**

Create `internal/calculator/calculator.go`:

```go
package calculator

import (
	"math"
	"sort"
)

type Direction string

const (
	Downsize Direction = "downsize"
	Upsize   Direction = "upsize"
)

type AnalysisInput struct {
	CurrentCPU         int64   // millicores
	CurrentMem         int64   // bytes
	CPUP95Percent      float64
	MemP95Percent      float64
	CPUMaxPercent      float64
	MemMaxPercent      float64
	HeadroomPercent    int
	MinCPUSavings      int64 // millicores
	MinMemSavings      int64 // bytes
	UpsizeThresholdPct int
}

type AnalysisResult struct {
	Direction      Direction
	RecommendedCPU int64
	RecommendedMem int64
	CPUSavings     int64
	MemSavings     int64
}

func ComputePercentile(samples []float64, percentile int) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)

	if len(sorted) == 1 {
		return sorted[0]
	}

	rank := float64(percentile) / 100.0 * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper {
		return sorted[lower]
	}
	fraction := rank - float64(lower)
	return sorted[lower] + fraction*(sorted[upper]-sorted[lower])
}

func Analyze(input AnalysisInput) *AnalysisResult {
	if input.CPUP95Percent >= float64(input.UpsizeThresholdPct) ||
		input.MemP95Percent >= float64(input.UpsizeThresholdPct) {
		return analyzeUpsize(input)
	}
	return analyzeDownsize(input)
}

func analyzeDownsize(input AnalysisInput) *AnalysisResult {
	headroom := 1.0 + float64(input.HeadroomPercent)/100.0

	cpuUsage := float64(input.CurrentCPU) * input.CPUP95Percent / 100.0
	recCPU := int64(math.Ceil(cpuUsage * headroom))

	memUsage := float64(input.CurrentMem) * input.MemP95Percent / 100.0
	recMem := int64(math.Ceil(memUsage * headroom))

	if recCPU < 100 {
		recCPU = 100 // minimum 100m
	}
	if recMem < 64<<20 {
		recMem = 64 << 20 // minimum 64Mi
	}

	if recCPU >= input.CurrentCPU {
		recCPU = input.CurrentCPU
	}
	if recMem >= input.CurrentMem {
		recMem = input.CurrentMem
	}

	cpuSavings := input.CurrentCPU - recCPU
	memSavings := input.CurrentMem - recMem

	if cpuSavings < input.MinCPUSavings && memSavings < input.MinMemSavings {
		return nil
	}

	return &AnalysisResult{
		Direction:      Downsize,
		RecommendedCPU: recCPU,
		RecommendedMem: recMem,
		CPUSavings:     cpuSavings,
		MemSavings:     memSavings,
	}
}

func analyzeUpsize(input AnalysisInput) *AnalysisResult {
	cpuUsage := float64(input.CurrentCPU) * input.CPUP95Percent / 100.0
	memUsage := float64(input.CurrentMem) * input.MemP95Percent / 100.0

	recCPU := int64(math.Ceil(cpuUsage / 0.70))
	recMem := int64(math.Ceil(memUsage / 0.70))

	if recCPU <= input.CurrentCPU {
		recCPU = input.CurrentCPU
	}
	if recMem <= input.CurrentMem {
		recMem = input.CurrentMem
	}

	cpuIncrease := recCPU - input.CurrentCPU
	memIncrease := recMem - input.CurrentMem

	if cpuIncrease < input.MinCPUSavings && memIncrease < input.MinMemSavings {
		return nil
	}

	return &AnalysisResult{
		Direction:      Upsize,
		RecommendedCPU: recCPU,
		RecommendedMem: recMem,
		CPUSavings:     -cpuIncrease,
		MemSavings:     -memIncrease,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/calculator/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/calculator/
git commit -m "feat: add rightsizing calculator with millicore/byte units"
```

---

### Task 3: Owner Resolver

**Files:**
- Create: `internal/owner/resolver.go`
- Create: `internal/owner/resolver_test.go`

Resolves owner from resource labels, falling back to namespace labels. Works for VMs, Deployments, and StatefulSets — not coupled to any specific resource type.

- [ ] **Step 1: Write the failing tests**

Create `internal/owner/resolver_test.go`:

```go
package owner_test

import (
	"context"
	"testing"

	"github.com/kborup-redhat/rrrt/internal/owner"
	"github.com/kborup-redhat/rrrt/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveFromResourceLabels(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	r := owner.NewResolver(c)

	labels := map[string]string{types.LabelOwner: "alice@example.com"}
	result, err := r.ResolveFromLabels(context.Background(), labels, "default")
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", result)
}

func TestResolveFromNamespaceFallback(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "prod",
			Labels: map[string]string{types.LabelOwner: "team-infra@example.com"},
		},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	r := owner.NewResolver(c)
	result, err := r.ResolveFromLabels(context.Background(), nil, "prod")
	require.NoError(t, err)
	assert.Equal(t, "team-infra@example.com", result)
}

func TestResolveNoOwner(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	r := owner.NewResolver(c)
	result, err := r.ResolveFromLabels(context.Background(), nil, "default")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go get sigs.k8s.io/controller-runtime@latest
go get k8s.io/api@latest
go get k8s.io/apimachinery@latest
go test ./internal/owner/ -v
```

Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement the resolver**

Create `internal/owner/resolver.go`:

```go
package owner

import (
	"context"
	"fmt"

	"github.com/kborup-redhat/rrrt/internal/types"
	corev1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Resolver struct {
	client client.Client
}

func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

func (r *Resolver) ResolveFromLabels(ctx context.Context, resourceLabels map[string]string, namespace string) (string, error) {
	if resourceLabels != nil {
		if owner, ok := resourceLabels[types.LabelOwner]; ok {
			return owner, nil
		}
	}

	ns := &corev1.Namespace{}
	if err := r.client.Get(ctx, k8stypes.NamespacedName{Name: namespace}, ns); err != nil {
		return "", fmt.Errorf("fetching namespace %s: %w", namespace, err)
	}

	if ns.Labels != nil {
		if owner, ok := ns.Labels[types.LabelOwner]; ok {
			return owner, nil
		}
	}

	return "", nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/owner/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/owner/
git commit -m "feat: add owner resolver with namespace fallback"
```

---

### Task 4: Prometheus Collector

**Files:**
- Create: `internal/collector/prometheus.go`
- Create: `internal/collector/queries.go`
- Create: `internal/collector/vm.go`
- Create: `internal/collector/container.go`
- Create: `internal/collector/collector.go`
- Create: `internal/collector/collector_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/collector/collector_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/collector/ -v
```

Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement the Prometheus HTTP client**

Create `internal/collector/prometheus.go`:

```go
package collector

import (
	"context"
	"crypto/tls"
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

type MetricSample struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Values    []float64
}

type PrometheusClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewPrometheusClient(baseURL, token string) *PrometheusClient {
	if token == "" {
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
			token = strings.TrimSpace(string(data))
		}
	}
	return &PrometheusClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec
				},
			},
		},
	}
}

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
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

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
	defer resp.Body.Close()

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
```

- [ ] **Step 4: Implement query builders**

Create `internal/collector/queries.go`:

```go
package collector

import (
	"fmt"
	"strings"
)

func SanitizeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func vmCPUQuery(vmName, namespace, lookback string) string {
	return fmt.Sprintf(
		`rate(kubevirt_vmi_cpu_usage_seconds_total{name="%s",namespace="%s"}[5m])[%s:1m]`,
		SanitizeLabelValue(vmName), SanitizeLabelValue(namespace), lookback,
	)
}

func vmMemoryQuery(vmName, namespace, lookback string) string {
	return fmt.Sprintf(
		`kubevirt_vmi_memory_resident_bytes{name="%s",namespace="%s"}[%s]`,
		SanitizeLabelValue(vmName), SanitizeLabelValue(namespace), lookback,
	)
}

func containerCPUQuery(workloadName, namespace, lookback string) string {
	return fmt.Sprintf(
		`rate(container_cpu_usage_seconds_total{namespace="%s",pod=~"%s-.*",container!=""}[5m])[%s:1m]`,
		SanitizeLabelValue(namespace), SanitizeLabelValue(workloadName), lookback,
	)
}

func containerMemoryQuery(workloadName, namespace, lookback string) string {
	return fmt.Sprintf(
		`container_memory_working_set_bytes{namespace="%s",pod=~"%s-.*",container!=""}[%s]`,
		SanitizeLabelValue(namespace), SanitizeLabelValue(workloadName), lookback,
	)
}
```

- [ ] **Step 5: Implement VM collector**

Create `internal/collector/vm.go`:

```go
package collector

import (
	"context"
	"fmt"

	"github.com/kborup-redhat/rrrt/internal/calculator"
	"github.com/kborup-redhat/rrrt/internal/owner"
	"github.com/kborup-redhat/rrrt/internal/types"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var vmGVR = schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}

func (c *Collector) collectVMs(ctx context.Context, namespace string) ([]types.ResourceAnalysis, []types.InsufficientDataEntry) {
	var analyses []types.ResourceAnalysis
	var insufficient []types.InsufficientDataEntry

	vmList := &unstructured.UnstructuredList{}
	vmList.SetGroupVersionKind(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineList"})

	opts := []client.ListOption{client.InNamespace(namespace)}
	if err := c.k8s.List(ctx, vmList, opts...); err != nil {
		c.logProgress("vms", namespace, "", 0, 0, fmt.Sprintf("error listing VMs: %v", err))
		return nil, nil
	}

	for i, vm := range vmList.Items {
		name := vm.GetName()
		ns := vm.GetNamespace()

		annotations := vm.GetAnnotations()
		if annotations[types.AnnotationExclude] == "true" {
			continue
		}

		c.logProgress("vms", ns, name, i+1, len(vmList.Items), "analyzing")

		cores, _, _ := unstructured.NestedInt64(vm.Object, "spec", "template", "spec", "domain", "cpu", "cores")
		memStr, _, _ := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")

		cpuMillis := cores * 1000
		var memBytes int64
		if memStr != "" {
			q, err := resource.ParseQuantity(memStr)
			if err == nil {
				memBytes = q.Value()
			}
		}

		lookback := fmt.Sprintf("%dd", c.lookbackDays)

		cpuSamples, err := c.prom.Query(ctx, vmCPUQuery(name, ns, lookback))
		if err != nil {
			c.logProgress("vms", ns, name, i+1, len(vmList.Items), fmt.Sprintf("error querying CPU: %v", err))
			continue
		}
		memSamples, err := c.prom.Query(ctx, vmMemoryQuery(name, ns, lookback))
		if err != nil {
			c.logProgress("vms", ns, name, i+1, len(vmList.Items), fmt.Sprintf("error querying memory: %v", err))
			continue
		}

		var cpuVals, memVals []float64
		if len(cpuSamples) > 0 {
			cpuVals = cpuSamples[0].Values
		}
		if len(memSamples) > 0 {
			memVals = memSamples[0].Values
		}

		expectedPoints := c.lookbackDays * 24 * 60
		minPoints := expectedPoints / 2
		if len(cpuVals) < minPoints {
			insufficient = append(insufficient, types.InsufficientDataEntry{
				Namespace: ns, Name: name, Kind: types.KindVM,
				DataPoints: len(cpuVals), ExpectedPoints: expectedPoints,
			})
			continue
		}

		cpuP95 := calculator.ComputePercentile(cpuVals, 95) * 100
		memP95 := calculator.ComputePercentile(memVals, 95)
		cpuMax := maxVal(cpuVals) * 100
		memMax := maxVal(memVals)

		if memBytes > 0 {
			memP95 = memP95 / float64(memBytes) * 100
			memMax = memMax / float64(memBytes) * 100
		}

		result := calculator.Analyze(calculator.AnalysisInput{
			CurrentCPU:         cpuMillis,
			CurrentMem:         memBytes,
			CPUP95Percent:      cpuP95,
			MemP95Percent:      memP95,
			HeadroomPercent:    c.headroomPct,
			MinCPUSavings:      types.DefaultVMMinCPUSavings,
			MinMemSavings:      types.DefaultVMMinMemSavings,
			UpsizeThresholdPct: types.DefaultUpsizeThreshold,
		})

		ownerStr, _ := c.owner.ResolveFromLabels(ctx, vm.GetLabels(), ns)

		consoleURL := fmt.Sprintf("%s/k8s/ns/%s/kubevirt.io~v1~VirtualMachine/%s", c.consoleURL, ns, name)

		analysis := types.ResourceAnalysis{
			Namespace: ns, Name: name, Kind: types.KindVM,
			Owner: ownerStr, ConsoleURL: consoleURL,
			CurrentCPU: cpuMillis, CurrentMem: memBytes,
			CPUP95: cpuP95, MemP95: memP95,
			CPUMax: cpuMax, MemMax: memMax,
			CPUSamples: cpuVals, MemSamples: memVals,
		}

		if result != nil {
			analysis.Direction = types.Direction(result.Direction)
			analysis.RecommendedCPU = result.RecommendedCPU
			analysis.RecommendedMem = result.RecommendedMem
			analysis.CPUSavings = result.CPUSavings
			analysis.MemSavings = result.MemSavings
			analysis.Justification = buildJustification(analysis)
		}

		analyses = append(analyses, analysis)
	}

	return analyses, insufficient
}

func maxVal(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func buildJustification(a types.ResourceAnalysis) string {
	cpuCores := float64(a.CurrentCPU) / 1000
	recCPUCores := float64(a.RecommendedCPU) / 1000

	if a.Direction == types.Downsize {
		return fmt.Sprintf(
			"CPU P95 utilization is %.0f%% with %.1f cores allocated. Reducing to %.1f cores provides adequate headroom above P95 and saves %.1f cores. "+
				"Memory P95 utilization is %.0f%% — recommended allocation reduces memory by %s.",
			a.CPUP95, cpuCores, recCPUCores, float64(a.CPUSavings)/1000,
			a.MemP95, formatBytes(a.MemSavings),
		)
	}
	return fmt.Sprintf(
		"CPU P95 utilization is %.0f%% with %.1f cores allocated, indicating resource pressure. Increasing to %.1f cores targets 70%% utilization. "+
			"Memory P95 utilization is %.0f%% — recommended increase of %s.",
		a.CPUP95, cpuCores, recCPUCores,
		a.MemP95, formatBytes(-a.MemSavings),
	)
}

func formatBytes(b int64) string {
	if b < 0 {
		b = -b
	}
	const gi = 1 << 30
	const mi = 1 << 20
	if b >= gi {
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(gi))
	}
	return fmt.Sprintf("%.0f MiB", float64(b)/float64(mi))
}
```

- [ ] **Step 6: Implement container collector**

Create `internal/collector/container.go`:

```go
package collector

import (
	"context"
	"fmt"

	"github.com/kborup-redhat/rrrt/internal/calculator"
	"github.com/kborup-redhat/rrrt/internal/types"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *Collector) collectContainers(ctx context.Context, namespace string) ([]types.ResourceAnalysis, []types.InsufficientDataEntry) {
	var analyses []types.ResourceAnalysis
	var insufficient []types.InsufficientDataEntry

	var deployments appsv1.DeploymentList
	if err := c.k8s.List(ctx, &deployments, client.InNamespace(namespace)); err != nil {
		c.logProgress("containers", namespace, "", 0, 0, fmt.Sprintf("error listing deployments: %v", err))
		return nil, nil
	}

	var statefulSets appsv1.StatefulSetList
	if err := c.k8s.List(ctx, &statefulSets, client.InNamespace(namespace)); err != nil {
		c.logProgress("containers", namespace, "", 0, 0, fmt.Sprintf("error listing statefulsets: %v", err))
	}

	type workload struct {
		name      string
		namespace string
		kind      types.ResourceKind
		labels    map[string]string
		annotations map[string]string
		cpuMillis int64
		memBytes  int64
	}

	var workloads []workload
	for _, d := range deployments.Items {
		w := workload{
			name: d.Name, namespace: d.Namespace,
			kind: types.KindDeployment,
			labels: d.Labels, annotations: d.Annotations,
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			w.cpuMillis += c.Resources.Requests.Cpu().MilliValue()
			w.memBytes += c.Resources.Requests.Memory().Value()
		}
		workloads = append(workloads, w)
	}
	for _, s := range statefulSets.Items {
		w := workload{
			name: s.Name, namespace: s.Namespace,
			kind: types.KindStatefulSet,
			labels: s.Labels, annotations: s.Annotations,
		}
		for _, c := range s.Spec.Template.Spec.Containers {
			w.cpuMillis += c.Resources.Requests.Cpu().MilliValue()
			w.memBytes += c.Resources.Requests.Memory().Value()
		}
		workloads = append(workloads, w)
	}

	for i, w := range workloads {
		if w.annotations[types.AnnotationExclude] == "true" {
			continue
		}
		if w.cpuMillis == 0 && w.memBytes == 0 {
			continue
		}

		c.logProgress("containers", w.namespace, w.name, i+1, len(workloads), "analyzing")

		lookback := fmt.Sprintf("%dd", c.lookbackDays)

		cpuSamples, err := c.prom.Query(ctx, containerCPUQuery(w.name, w.namespace, lookback))
		if err != nil {
			continue
		}
		memSamples, err := c.prom.Query(ctx, containerMemoryQuery(w.name, w.namespace, lookback))
		if err != nil {
			continue
		}

		var cpuVals, memVals []float64
		for _, s := range cpuSamples {
			cpuVals = append(cpuVals, s.Values...)
		}
		for _, s := range memSamples {
			memVals = append(memVals, s.Values...)
		}

		expectedPoints := c.lookbackDays * 24 * 60
		minPoints := expectedPoints / 2
		if len(cpuVals) < minPoints {
			insufficient = append(insufficient, types.InsufficientDataEntry{
				Namespace: w.namespace, Name: w.name, Kind: w.kind,
				DataPoints: len(cpuVals), ExpectedPoints: expectedPoints,
			})
			continue
		}

		cpuP95Raw := calculator.ComputePercentile(cpuVals, 95)
		memP95Raw := calculator.ComputePercentile(memVals, 95)
		cpuMaxRaw := maxVal(cpuVals)
		memMaxRaw := maxVal(memVals)

		cpuP95 := cpuP95Raw / (float64(w.cpuMillis) / 1000) * 100
		memP95 := memP95Raw / float64(w.memBytes) * 100
		cpuMax := cpuMaxRaw / (float64(w.cpuMillis) / 1000) * 100
		memMax := memMaxRaw / float64(w.memBytes) * 100

		result := calculator.Analyze(calculator.AnalysisInput{
			CurrentCPU:         w.cpuMillis,
			CurrentMem:         w.memBytes,
			CPUP95Percent:      cpuP95,
			MemP95Percent:      memP95,
			HeadroomPercent:    c.headroomPct,
			MinCPUSavings:      types.DefaultContainerMinCPUSavings,
			MinMemSavings:      types.DefaultContainerMinMemSavings,
			UpsizeThresholdPct: types.DefaultUpsizeThreshold,
		})

		ownerStr, _ := c.owner.ResolveFromLabels(ctx, w.labels, w.namespace)

		var consoleURL string
		if w.kind == types.KindDeployment {
			consoleURL = fmt.Sprintf("%s/k8s/ns/%s/deployments/%s", c.consoleURL, w.namespace, w.name)
		} else {
			consoleURL = fmt.Sprintf("%s/k8s/ns/%s/statefulsets/%s", c.consoleURL, w.namespace, w.name)
		}

		analysis := types.ResourceAnalysis{
			Namespace: w.namespace, Name: w.name, Kind: w.kind,
			Owner: ownerStr, ConsoleURL: consoleURL,
			CurrentCPU: w.cpuMillis, CurrentMem: w.memBytes,
			CPUP95: cpuP95, MemP95: memP95,
			CPUMax: cpuMax, MemMax: memMax,
			CPUSamples: cpuVals, MemSamples: memVals,
		}

		if result != nil {
			analysis.Direction = types.Direction(result.Direction)
			analysis.RecommendedCPU = result.RecommendedCPU
			analysis.RecommendedMem = result.RecommendedMem
			analysis.CPUSavings = result.CPUSavings
			analysis.MemSavings = result.MemSavings
			analysis.Justification = buildJustification(analysis)
		}

		analyses = append(analyses, analysis)
	}

	return analyses, insufficient
}
```

- [ ] **Step 7: Implement top-level collector orchestrator**

Create `internal/collector/collector.go`:

```go
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kborup-redhat/rrrt/internal/owner"
	"github.com/kborup-redhat/rrrt/internal/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Collector struct {
	k8s          client.Client
	prom         *PrometheusClient
	owner        *owner.Resolver
	consoleURL   string
	lookbackDays int
	headroomPct  int
	includeOS    bool
}

func New(k8s client.Client, prom *PrometheusClient, ownerResolver *owner.Resolver, consoleURL string, lookbackDays, headroomPct int, includeOpenShift bool) *Collector {
	return &Collector{
		k8s: k8s, prom: prom, owner: ownerResolver,
		consoleURL: consoleURL, lookbackDays: lookbackDays,
		headroomPct: headroomPct, includeOS: includeOpenShift,
	}
}

func (c *Collector) Collect(ctx context.Context, namespaces []string) (*types.ReportData, error) {
	if len(namespaces) == 0 {
		var nsList corev1.NamespaceList
		if err := c.k8s.List(ctx, &nsList); err != nil {
			return nil, fmt.Errorf("listing namespaces: %w", err)
		}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	namespaces = c.filterNamespaces(namespaces)

	data := &types.ReportData{
		LookbackDays: c.lookbackDays,
		Percentile:   types.DefaultPercentile,
		HeadroomPct:  c.headroomPct,
	}

	total := len(namespaces)
	for i, ns := range namespaces {
		c.logProgress("scan", ns, "", i+1, total, "scanning namespace")

		vms, vmInsuf := c.collectVMs(ctx, ns)
		data.VMAnalyses = append(data.VMAnalyses, vms...)
		data.InsufficientData = append(data.InsufficientData, vmInsuf...)

		containers, contInsuf := c.collectContainers(ctx, ns)
		data.ContainerAnalyses = append(data.ContainerAnalyses, containers...)
		data.InsufficientData = append(data.InsufficientData, contInsuf...)
	}

	return data, nil
}

func (c *Collector) filterNamespaces(namespaces []string) []string {
	if c.includeOS {
		return namespaces
	}
	var filtered []string
	for _, ns := range namespaces {
		if strings.HasPrefix(ns, "openshift-") ||
			strings.HasPrefix(ns, "kube-") ||
			strings.HasPrefix(ns, "rrrt-") {
			continue
		}
		filtered = append(filtered, ns)
	}
	return filtered
}

func (c *Collector) logProgress(phase, namespace, resource string, index, total int, status string) {
	msg := map[string]interface{}{
		"phase":     phase,
		"namespace": namespace,
		"resource":  resource,
		"index":     index,
		"total":     total,
		"status":    status,
	}
	data, _ := json.Marshal(msg)
	fmt.Fprintln(os.Stdout, string(data))
}
```

- [ ] **Step 8: Run tests and verify they pass**

```bash
go mod tidy
go test ./internal/collector/ -v
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/collector/
git commit -m "feat: add Prometheus collector for VMs and containers"
```

---

### Task 5: PDF Report Generator — Core Structure

**Files:**
- Create: `internal/pdf/report.go`
- Create: `internal/pdf/cover.go`
- Create: `internal/pdf/charts.go`
- Create: `internal/pdf/report_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/pdf/report_test.go`:

```go
package pdf_test

import (
	"os"
	"testing"

	"github.com/kborup-redhat/rrrt/internal/pdf"
	"github.com/kborup-redhat/rrrt/internal/types"
	"github.com/stretchr/testify/require"
)

func TestGenerateReport_Empty(t *testing.T) {
	data := &types.ReportData{
		ClusterName:  "test-cluster",
		GeneratedAt:  "2026-05-07T14:30:00Z",
		Scope:        "All namespaces",
		LookbackDays: 14,
		Percentile:   95,
		HeadroomPct:  20,
		CLIVersion:   "v0.1.0",
		ImageVersion: "v0.1.0",
	}

	tmpFile := t.TempDir() + "/test-report.pdf"
	err := pdf.Generate(data, tmpFile)
	require.NoError(t, err)

	info, err := os.Stat(tmpFile)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}

func TestGenerateReport_WithData(t *testing.T) {
	data := &types.ReportData{
		ClusterName:  "prod-cluster",
		GeneratedAt:  "2026-05-07T14:30:00Z",
		Scope:        "Namespaces: default, production",
		LookbackDays: 14,
		Percentile:   95,
		HeadroomPct:  20,
		CLIVersion:   "v0.1.0",
		ImageVersion: "v0.1.0",
		VMAnalyses: []types.ResourceAnalysis{
			{
				Namespace: "default", Name: "test-vm", Kind: types.KindVM,
				Owner: "alice@example.com", ConsoleURL: "https://console.example.com/vm/test-vm",
				Direction: types.Downsize,
				CurrentCPU: 8000, CurrentMem: 16 << 30,
				RecommendedCPU: 3000, RecommendedMem: 9 << 30,
				CPUSavings: 5000, MemSavings: 7 << 30,
				CPUP95: 28.3, MemP95: 41.7, CPUMax: 62.1, MemMax: 58.4,
				CPUSamples: []float64{0.2, 0.25, 0.3, 0.28, 0.22},
				MemSamples: []float64{0.4, 0.42, 0.38, 0.45, 0.41},
				Justification: "CPU P95 utilization is 28% with 8 cores allocated.",
			},
		},
		ContainerAnalyses: []types.ResourceAnalysis{
			{
				Namespace: "production", Name: "api-server", Kind: types.KindDeployment,
				Direction: types.Downsize,
				CurrentCPU: 2000, CurrentMem: 4 << 30,
				RecommendedCPU: 500, RecommendedMem: 1 << 30,
				CPUSavings: 1500, MemSavings: 3 << 30,
				CPUP95: 15.0, MemP95: 20.0,
				CPUSamples: []float64{0.1, 0.15, 0.12},
				MemSamples: []float64{0.2, 0.18, 0.22},
				Justification: "CPU P95 utilization is 15% with 2 cores allocated.",
			},
		},
	}

	tmpFile := t.TempDir() + "/test-report-with-data.pdf"
	err := pdf.Generate(data, tmpFile)
	require.NoError(t, err)

	info, err := os.Stat(tmpFile)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(1000))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go get github.com/go-pdf/fpdf@v0.9.0
go get github.com/wcharczuk/go-chart/v2@v2.1.0
go test ./internal/pdf/ -v
```

Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement chart rendering helpers**

Create `internal/pdf/charts.go`:

```go
package pdf

import (
	"bytes"
	"image/color"
	"image/png"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

var (
	colorBlue   = drawing.Color{R: 66, G: 133, B: 244, A: 255}
	colorRed    = drawing.Color{R: 234, G: 67, B: 53, A: 255}
	colorGrey   = drawing.Color{R: 158, G: 158, B: 158, A: 255}
	colorGreen  = drawing.Color{R: 52, G: 168, B: 83, A: 255}
	colorAmber  = drawing.Color{R: 251, G: 188, B: 4, A: 255}
)

func renderLineChart(samples []float64, p95 float64, currentAllocation float64, title string, width, height int) ([]byte, error) {
	xValues := make([]float64, len(samples))
	for i := range samples {
		xValues[i] = float64(i)
	}

	graph := chart.Chart{
		Title:  title,
		Width:  width,
		Height: height,
		Background: chart.Style{
			FillColor: drawing.Color{R: 255, G: 255, B: 255, A: 255},
		},
		XAxis: chart.XAxis{
			Name: "Time",
			Style: chart.Style{
				FontSize: 8,
			},
		},
		YAxis: chart.YAxis{
			Name: "Utilization %",
			Style: chart.Style{
				FontSize: 8,
			},
		},
		Series: []chart.Series{
			chart.ContinuousSeries{
				Name:    "Utilization",
				XValues: xValues,
				YValues: samples,
				Style: chart.Style{
					StrokeColor: colorBlue,
					StrokeWidth: 1.5,
					FillColor:   colorBlue.WithAlpha(40),
				},
			},
			chart.ContinuousSeries{
				Name:    "P95",
				XValues: []float64{0, float64(len(samples) - 1)},
				YValues: []float64{p95, p95},
				Style: chart.Style{
					StrokeColor:     colorRed,
					StrokeWidth:     2,
					StrokeDashArray: []float64{5, 3},
				},
			},
			chart.ContinuousSeries{
				Name:    "Current",
				XValues: []float64{0, float64(len(samples) - 1)},
				YValues: []float64{currentAllocation, currentAllocation},
				Style: chart.Style{
					StrokeColor:     colorGrey,
					StrokeWidth:     1.5,
					StrokeDashArray: []float64{3, 3},
				},
			},
		},
	}

	graph.Elements = []chart.Renderable{chart.LegendThin(&graph)}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderDonutChart(rightSized, oversized, undersized int, width, height int) ([]byte, error) {
	pie := chart.DonutChart{
		Width:  width,
		Height: height,
		Values: []chart.Value{
			{Label: "Right-sized", Value: float64(rightSized), Style: chart.Style{FillColor: colorGreen}},
			{Label: "Oversized", Value: float64(oversized), Style: chart.Style{FillColor: colorAmber}},
			{Label: "Undersized", Value: float64(undersized), Style: chart.Style{FillColor: colorRed}},
		},
	}

	var buf bytes.Buffer
	if err := pie.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pngToBytes(data []byte) (*bytes.Reader, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	_ = img
	return bytes.NewReader(data), nil
}

// rgbaToDrawing converts a color.RGBA to a drawing.Color — not used directly
// but kept for potential future palette work.
func rgbaToDrawing(c color.RGBA) drawing.Color {
	return drawing.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}
```

- [ ] **Step 4: Implement cover page**

Create `internal/pdf/cover.go`:

```go
package pdf

import (
	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderCover(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()

	p.SetFont("Helvetica", "B", 28)
	p.Ln(60)
	p.CellFormat(0, 15, "Resource Rightsizing Report", "", 1, "C", false, 0, "")

	p.Ln(20)
	p.SetFont("Helvetica", "", 14)
	p.CellFormat(0, 10, "Cluster: "+data.ClusterName, "", 1, "C", false, 0, "")
	p.CellFormat(0, 10, "Generated: "+data.GeneratedAt, "", 1, "C", false, 0, "")
	p.CellFormat(0, 10, "Scope: "+data.Scope, "", 1, "C", false, 0, "")
	p.CellFormat(0, 10, "Lookback: "+itoa(data.LookbackDays)+" days", "", 1, "C", false, 0, "")
}
```

- [ ] **Step 5: Implement top-level report orchestrator**

Create `internal/pdf/report.go`:

```go
package pdf

import (
	"fmt"
	"strconv"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func Generate(data *types.ReportData, outputPath string) error {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetAutoPageBreak(true, 15)

	renderCover(p, data)
	renderSummary(p, data)
	renderVMSection(p, data)
	renderContainerSection(p, data)
	renderInsufficientData(p, data)
	renderAppendix(p, data)

	return p.OutputFileAndClose(outputPath)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func formatCPU(millis int64) string {
	if millis >= 1000 {
		cores := float64(millis) / 1000
		if cores == float64(int64(cores)) {
			return fmt.Sprintf("%d cores", int64(cores))
		}
		return fmt.Sprintf("%.1f cores", cores)
	}
	return fmt.Sprintf("%dm", millis)
}

func formatMem(bytes int64) string {
	if bytes < 0 {
		bytes = -bytes
	}
	const gi = 1 << 30
	const mi = 1 << 20
	if bytes >= gi {
		gib := float64(bytes) / float64(gi)
		if gib == float64(int64(gib)) {
			return fmt.Sprintf("%d GiB", int64(gib))
		}
		return fmt.Sprintf("%.1f GiB", gib)
	}
	return fmt.Sprintf("%d MiB", bytes/mi)
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go mod tidy
go test ./internal/pdf/ -v
```

Expected: FAIL — `renderSummary`, `renderVMSection`, etc. don't exist yet. That's expected — we'll add them in the next task.

- [ ] **Step 7: Add stub functions so tests pass**

Create `internal/pdf/summary.go`:

```go
package pdf

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderSummary(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()
	p.SetFont("Helvetica", "B", 18)
	p.CellFormat(0, 10, "Executive Summary", "", 1, "L", false, 0, "")
	p.Ln(5)

	vmCandidates := countCandidates(data.VMAnalyses)
	contCandidates := countCandidates(data.ContainerAnalyses)
	totalAnalyzed := len(data.VMAnalyses) + len(data.ContainerAnalyses)

	totalCPUSavings := sumCPUSavings(data.VMAnalyses) + sumCPUSavings(data.ContainerAnalyses)
	totalMemSavings := sumMemSavings(data.VMAnalyses) + sumMemSavings(data.ContainerAnalyses)

	p.SetFont("Helvetica", "", 11)
	p.CellFormat(0, 7, fmt.Sprintf("Total resources analyzed: %d (%d VMs, %d containers)",
		totalAnalyzed, len(data.VMAnalyses), len(data.ContainerAnalyses)), "", 1, "L", false, 0, "")

	downsizeCount, upsizeCount := countDirections(data.VMAnalyses, data.ContainerAnalyses)
	p.CellFormat(0, 7, fmt.Sprintf("Rightsizing candidates: %d (%d downsize, %d upsize)",
		vmCandidates+contCandidates, downsizeCount, upsizeCount), "", 1, "L", false, 0, "")
	p.CellFormat(0, 7, fmt.Sprintf("Estimated CPU savings: %s", formatCPU(totalCPUSavings)), "", 1, "L", false, 0, "")
	p.CellFormat(0, 7, fmt.Sprintf("Estimated memory savings: %s", formatMem(totalMemSavings)), "", 1, "L", false, 0, "")

	rightSized := totalAnalyzed - (vmCandidates + contCandidates)
	chartData, err := renderDonutChart(rightSized, downsizeCount, upsizeCount, 400, 300)
	if err == nil && len(chartData) > 0 {
		p.Ln(10)
		opt := fpdf.ImageOptions{ImageType: "PNG"}
		p.RegisterImageOptionsReader("summary_chart", opt, bytes.NewReader(chartData))
		p.ImageOptions("summary_chart", 40, p.GetY(), 130, 0, false, opt, 0, "")
	}
}

func countCandidates(analyses []types.ResourceAnalysis) int {
	count := 0
	for _, a := range analyses {
		if a.Direction != "" {
			count++
		}
	}
	return count
}

func countDirections(vms, containers []types.ResourceAnalysis) (downsize, upsize int) {
	all := append(vms, containers...)
	for _, a := range all {
		switch a.Direction {
		case types.Downsize:
			downsize++
		case types.Upsize:
			upsize++
		}
	}
	return
}

func sumCPUSavings(analyses []types.ResourceAnalysis) int64 {
	var total int64
	for _, a := range analyses {
		if a.CPUSavings > 0 {
			total += a.CPUSavings
		}
	}
	return total
}

func sumMemSavings(analyses []types.ResourceAnalysis) int64 {
	var total int64
	for _, a := range analyses {
		if a.MemSavings > 0 {
			total += a.MemSavings
		}
	}
	return total
}

```

Create `internal/pdf/tables.go`:

```go
package pdf

import (
	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderResourceTable(p *fpdf.Fpdf, analyses []types.ResourceAnalysis, sectionTitle string) {
	if len(analyses) == 0 {
		return
	}

	candidates := filterCandidates(analyses)
	if len(candidates) == 0 {
		p.SetFont("Helvetica", "I", 10)
		p.CellFormat(0, 7, "No rightsizing candidates found.", "", 1, "L", false, 0, "")
		return
	}

	headers := []string{"Namespace", "Name", "Owner", "Direction", "Current CPU", "Rec. CPU", "Current Mem", "Rec. Mem"}
	widths := []float64{25, 30, 30, 18, 20, 20, 22, 22}

	p.SetFont("Helvetica", "B", 8)
	p.SetFillColor(240, 240, 240)
	for i, h := range headers {
		p.CellFormat(widths[i], 6, h, "1", 0, "C", true, 0, "")
	}
	p.Ln(-1)

	p.SetFont("Helvetica", "", 7)
	for _, a := range candidates {
		dir := string(a.Direction)
		p.CellFormat(widths[0], 5, a.Namespace, "1", 0, "L", false, 0, "")
		p.CellFormat(widths[1], 5, a.Name, "1", 0, "L", false, 0, a.ConsoleURL)
		p.CellFormat(widths[2], 5, truncate(a.Owner, 20), "1", 0, "L", false, 0, "")
		p.CellFormat(widths[3], 5, dir, "1", 0, "C", false, 0, "")
		p.CellFormat(widths[4], 5, formatCPU(a.CurrentCPU), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[5], 5, formatCPU(a.RecommendedCPU), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[6], 5, formatMem(a.CurrentMem), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[7], 5, formatMem(a.RecommendedMem), "1", 0, "R", false, 0, "")
		p.Ln(-1)
	}
}

func filterCandidates(analyses []types.ResourceAnalysis) []types.ResourceAnalysis {
	var candidates []types.ResourceAnalysis
	for _, a := range analyses {
		if a.Direction != "" {
			candidates = append(candidates, a)
		}
	}
	return candidates
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
```

Create `internal/pdf/details.go`:

```go
package pdf

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderDetailCards(p *fpdf.Fpdf, analyses []types.ResourceAnalysis) {
	candidates := filterCandidates(analyses)
	for _, a := range candidates {
		p.AddPage()
		p.SetFont("Helvetica", "B", 14)
		p.CellFormat(0, 8, fmt.Sprintf("%s: %s/%s", a.Kind, a.Namespace, a.Name), "", 1, "L", false, 0, a.ConsoleURL)

		if a.Owner != "" {
			p.SetFont("Helvetica", "", 10)
			p.CellFormat(0, 6, "Owner: "+a.Owner, "", 1, "L", false, 0, "")
		}

		p.Ln(3)
		p.SetFont("Helvetica", "B", 11)
		p.CellFormat(0, 7, fmt.Sprintf("Direction: %s", a.Direction), "", 1, "L", false, 0, "")
		p.SetFont("Helvetica", "", 10)
		p.CellFormat(0, 6, fmt.Sprintf("Current: %s CPU, %s memory", formatCPU(a.CurrentCPU), formatMem(a.CurrentMem)), "", 1, "L", false, 0, "")
		p.CellFormat(0, 6, fmt.Sprintf("Recommended: %s CPU, %s memory", formatCPU(a.RecommendedCPU), formatMem(a.RecommendedMem)), "", 1, "L", false, 0, "")
		p.CellFormat(0, 6, fmt.Sprintf("Savings: %s CPU, %s memory", formatCPU(abs64(a.CPUSavings)), formatMem(abs64(a.MemSavings))), "", 1, "L", false, 0, "")

		p.Ln(5)

		if len(a.CPUSamples) > 0 {
			chartData, err := renderLineChart(a.CPUSamples, a.CPUP95, 100, "CPU Utilization (%)", 500, 200)
			if err == nil {
				opt := fpdf.ImageOptions{ImageType: "PNG"}
				imgName := fmt.Sprintf("cpu_%s_%s", a.Namespace, a.Name)
				p.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(chartData))
				p.ImageOptions(imgName, 15, p.GetY(), 180, 0, false, opt, 0, "")
				p.Ln(5)
			}
		}

		if len(a.MemSamples) > 0 {
			chartData, err := renderLineChart(a.MemSamples, a.MemP95, 100, "Memory Utilization (%)", 500, 200)
			if err == nil {
				opt := fpdf.ImageOptions{ImageType: "PNG"}
				imgName := fmt.Sprintf("mem_%s_%s", a.Namespace, a.Name)
				p.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(chartData))
				p.ImageOptions(imgName, 15, p.GetY(), 180, 0, false, opt, 0, "")
				p.Ln(5)
			}
		}

		if a.Justification != "" {
			p.Ln(3)
			p.SetFont("Helvetica", "B", 10)
			p.CellFormat(0, 6, "Justification:", "", 1, "L", false, 0, "")
			p.SetFont("Helvetica", "", 9)
			p.MultiCell(0, 5, a.Justification, "", "L", false)
		}
	}
}
```

Create `internal/pdf/insufficient.go`:

```go
package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderInsufficientData(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.InsufficientData) == 0 {
		return
	}

	p.AddPage()
	p.SetFont("Helvetica", "B", 16)
	p.CellFormat(0, 10, "Insufficient Data", "", 1, "L", false, 0, "")
	p.Ln(3)

	p.SetFont("Helvetica", "", 9)
	p.MultiCell(0, 5, "The following resources had less than 50% metric coverage of the lookback window and could not be analyzed.", "", "L", false)
	p.Ln(5)

	headers := []string{"Namespace", "Name", "Type", "Data Points", "Expected"}
	widths := []float64{35, 40, 30, 35, 35}

	p.SetFont("Helvetica", "B", 8)
	p.SetFillColor(240, 240, 240)
	for i, h := range headers {
		p.CellFormat(widths[i], 6, h, "1", 0, "C", true, 0, "")
	}
	p.Ln(-1)

	p.SetFont("Helvetica", "", 8)
	for _, entry := range data.InsufficientData {
		p.CellFormat(widths[0], 5, entry.Namespace, "1", 0, "L", false, 0, "")
		p.CellFormat(widths[1], 5, entry.Name, "1", 0, "L", false, 0, "")
		p.CellFormat(widths[2], 5, string(entry.Kind), "1", 0, "L", false, 0, "")
		p.CellFormat(widths[3], 5, fmt.Sprintf("%d", entry.DataPoints), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[4], 5, fmt.Sprintf("%d", entry.ExpectedPoints), "1", 0, "R", false, 0, "")
		p.Ln(-1)
	}
}
```

Create `internal/pdf/appendix.go`:

```go
package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderAppendix(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()
	p.SetFont("Helvetica", "B", 16)
	p.CellFormat(0, 10, "Appendix", "", 1, "L", false, 0, "")
	p.Ln(5)

	p.SetFont("Helvetica", "B", 12)
	p.CellFormat(0, 8, "Analysis Settings", "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 10)
	p.CellFormat(0, 6, fmt.Sprintf("Percentile: P%d", data.Percentile), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Headroom: %d%%", data.HeadroomPct), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Lookback: %d days", data.LookbackDays), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("VM Min CPU Savings: %s", formatCPU(int64(types.DefaultVMMinCPUSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("VM Min Memory Savings: %s", formatMem(int64(types.DefaultVMMinMemSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Container Min CPU Savings: %s", formatCPU(int64(types.DefaultContainerMinCPUSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Container Min Memory Savings: %s", formatMem(int64(types.DefaultContainerMinMemSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Upsize Threshold: %d%%", types.DefaultUpsizeThreshold), "", 1, "L", false, 0, "")

	p.Ln(10)
	p.SetFont("Helvetica", "B", 12)
	p.CellFormat(0, 8, "Version Information", "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 10)
	p.CellFormat(0, 6, "CLI Version: "+data.CLIVersion, "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, "Analyzer Image: "+data.ImageVersion, "", 1, "L", false, 0, "")

	p.Ln(20)
	p.SetFont("Helvetica", "", 9)
	p.SetTextColor(100, 100, 100)
	p.CellFormat(0, 5, "Built with RRRT open source project — github.com/kborup-redhat/rrrt", "", 1, "C", false, 0, "https://github.com/kborup-redhat/rrrt")
	p.CellFormat(0, 5, "Feedback & issues: github.com/kborup-redhat/rrrt/issues", "", 1, "C", false, 0, "https://github.com/kborup-redhat/rrrt/issues")
	p.SetTextColor(0, 0, 0)
}
```

- [ ] **Step 8: Add VM and container section renderers**

Add to `internal/pdf/report.go` — or better, add rendering calls. The VM and container sections use the shared `renderResourceTable` and `renderDetailCards`:

Add the following functions at the bottom of `internal/pdf/report.go`:

```go
func renderVMSection(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.VMAnalyses) == 0 {
		return
	}
	p.AddPage()
	p.SetFont("Helvetica", "B", 18)
	p.CellFormat(0, 10, "Virtual Machines", "", 1, "L", false, 0, "")
	p.Ln(5)
	renderResourceTable(p, data.VMAnalyses, "Virtual Machines")
	renderDetailCards(p, data.VMAnalyses)
}

func renderContainerSection(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.ContainerAnalyses) == 0 {
		return
	}
	p.AddPage()
	p.SetFont("Helvetica", "B", 18)
	p.CellFormat(0, 10, "Containers", "", 1, "L", false, 0, "")
	p.Ln(5)
	renderResourceTable(p, data.ContainerAnalyses, "Containers")
	renderDetailCards(p, data.ContainerAnalyses)
}
```

- [ ] **Step 9: Run tests to verify they pass**

```bash
go mod tidy
go test ./internal/pdf/ -v
```

Expected: all PASS. The test verifies a PDF file is generated with non-zero size.

- [ ] **Step 10: Commit**

```bash
git add internal/pdf/
git commit -m "feat: add PDF report generator with charts and detail cards"
```

---

### Task 6: CLI Orchestrator

**Files:**
- Create: `internal/cli/run.go`
- Create: `internal/cli/rbac.go`
- Create: `internal/cli/job.go`
- Create: `internal/cli/copy.go`
- Create: `internal/cli/console.go`
- Create: `internal/cli/cleanup.go`

- [ ] **Step 1: Implement cleanup logic with signal handling**

Create `internal/cli/cleanup.go`:

```go
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Cleanup struct {
	clientset     *kubernetes.Clientset
	namespace     string
	crbNames      []string
	keepNamespace bool
}

func NewCleanup(clientset *kubernetes.Clientset, namespace string, crbNames []string, keepNamespace bool) *Cleanup {
	return &Cleanup{
		clientset:     clientset,
		namespace:     namespace,
		crbNames:      crbNames,
		keepNamespace: keepNamespace,
	}
}

func (c *Cleanup) RegisterSignalHandler(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted. Cleaning up...")
		cancel()
		c.Run()
		os.Exit(1)
	}()
}

func (c *Cleanup) Run() {
	if c.keepNamespace {
		fmt.Fprintf(os.Stderr, "Keeping namespace %s (--keep-namespace set)\n", c.namespace)
		return
	}

	ctx := context.Background()

	for _, name := range c.crbNames {
		err := c.clientset.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete ClusterRoleBinding %s: %v\n", name, err)
		}
	}

	err := c.clientset.CoreV1().Namespaces().Delete(ctx, c.namespace, metav1.DeleteOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cleanup failed. Run manually:\n")
		for _, name := range c.crbNames {
			fmt.Fprintf(os.Stderr, "  oc delete clusterrolebinding %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "  oc delete namespace %s\n", c.namespace)
	}
}

// stubbed for compilation — full RBAC types imported above
var _ = rbacv1.ClusterRoleBinding{}
```

- [ ] **Step 2: Implement RBAC setup**

Create `internal/cli/rbac.go`:

```go
package cli

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	clusterRoleName    = "rrrt-analyzer"
	serviceAccountName = "rrrt-analyzer"
)

func createNamespace(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

func createServiceAccount(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}
	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	return err
}

func ensureClusterRole(ctx context.Context, clientset *kubernetes.Clientset) error {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRoleName},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"kubevirt.io"},
				Resources: []string{"virtualmachines", "virtualmachineinstances"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "statefulsets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get"},
			},
		},
	}

	_, err := clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = clientset.RbacV1().ClusterRoles().Update(ctx, cr, metav1.UpdateOptions{})
	}
	return err
}

func createClusterRoleBindings(ctx context.Context, clientset *kubernetes.Clientset, namespace string) ([]string, error) {
	analyzerCRB := fmt.Sprintf("rrrt-analyzer-%s", namespace)
	monitoringCRB := fmt.Sprintf("rrrt-monitoring-%s", namespace)

	crb1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: analyzerCRB},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	}

	crb2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: monitoringCRB},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view",
		},
	}

	if _, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb1, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("creating analyzer CRB: %w", err)
	}
	if _, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb2, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("creating monitoring CRB: %w", err)
	}

	return []string{analyzerCRB, monitoringCRB}, nil
}
```

- [ ] **Step 3: Implement Job builder**

Create `internal/cli/job.go`:

```go
package cli

import (
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JobConfig struct {
	Namespace    string
	Image        string
	Namespaces   []string
	LookbackDays int
	ConsoleURL   string
	IncludeOS    bool
	PrometheusURL string
	Timeout      time.Duration
}

func buildJob(cfg JobConfig) *batchv1.Job {
	deadlineSeconds := int64(cfg.Timeout.Seconds())
	backoffLimit := int32(0)

	env := []corev1.EnvVar{
		{Name: "RRRT_LOOKBACK_DAYS", Value: fmt.Sprintf("%d", cfg.LookbackDays)},
		{Name: "RRRT_CONSOLE_URL", Value: cfg.ConsoleURL},
		{Name: "RRRT_INCLUDE_OPENSHIFT", Value: fmt.Sprintf("%t", cfg.IncludeOS)},
		{Name: "RRRT_PROMETHEUS_URL", Value: cfg.PrometheusURL},
	}

	if len(cfg.Namespaces) > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "RRRT_NAMESPACES",
			Value: strings.Join(cfg.Namespaces, ","),
		})
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rrrt-analyzer",
			Namespace: cfg.Namespace,
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: &deadlineSeconds,
			BackoffLimit:          &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "analyzer",
							Image: cfg.Image,
							Env:   env,
						},
					},
				},
			},
		},
	}
}
```

- [ ] **Step 4: Implement pod file copy**

Create `internal/cli/copy.go`:

```go
package cli

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func copyFromPod(ctx context.Context, config *rest.Config, clientset *kubernetes.Clientset, namespace, podName, srcPath, destPath string) error {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: []string{"tar", "cf", "-", "-C", "/", srcPath[1:]},
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating executor: %w", err)
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: pw,
			Stderr: os.Stderr,
		})
		if err != nil {
			pw.CloseWithError(err)
		}
	}()

	tr := tar.NewReader(pr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		if header.Typeflag == tar.TypeReg {
			out, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("writing output file: %w", err)
			}
			out.Close()
			return nil
		}
	}

	return fmt.Errorf("file not found in tar stream: %s", srcPath)
}
```

- [ ] **Step 5: Implement console URL detection**

Create `internal/cli/console.go`:

```go
package cli

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"gopkg.in/yaml.v3"
)

func detectConsoleURL(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
	cm, err := clientset.CoreV1().ConfigMaps("openshift-console").Get(ctx, "console-config", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("reading console-config: %w", err)
	}

	consoleConfig, ok := cm.Data["console-config.yaml"]
	if !ok {
		return "", fmt.Errorf("console-config.yaml not found in ConfigMap")
	}

	var config struct {
		ClusterInfo struct {
			ConsoleBaseAddress string `yaml:"consoleBaseAddress"`
		} `yaml:"clusterInfo"`
	}

	if err := yaml.Unmarshal([]byte(consoleConfig), &config); err != nil {
		return "", fmt.Errorf("parsing console config: %w", err)
	}

	if config.ClusterInfo.ConsoleBaseAddress == "" {
		return "", fmt.Errorf("consoleBaseAddress not found")
	}

	return config.ClusterInfo.ConsoleBaseAddress, nil
}
```

- [ ] **Step 6: Implement main orchestrator**

Create `internal/cli/run.go`:

```go
package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type RunConfig struct {
	Namespaces     []string
	Output         string
	LookbackDays   int
	Image          string
	ConsoleURL     string
	IncludeOS      bool
	KeepNamespace  bool
	Timeout        time.Duration
}

func Run(ctx context.Context, config *rest.Config, clientset *kubernetes.Clientset, cfg RunConfig) error {
	// Generate ephemeral namespace name
	randBytes := make([]byte, 4)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("generating random name: %w", err)
	}
	nsName := "rrrt-" + hex.EncodeToString(randBytes)

	// Auto-detect console URL if not provided
	if cfg.ConsoleURL == "" {
		url, err := detectConsoleURL(ctx, clientset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not auto-detect console URL: %v\n", err)
		} else {
			cfg.ConsoleURL = url
		}
	}

	// Default image
	if cfg.Image == "" {
		cfg.Image = "quay.io/kborup/rrrt:latest"
	}

	prometheusURL := "https://thanos-querier.openshift-monitoring.svc:9091"

	// Set up cleanup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	fmt.Printf("Creating namespace %s...\n", nsName)
	if err := createNamespace(ctx, clientset, nsName); err != nil {
		return fmt.Errorf("creating namespace: %w", err)
	}

	cleanup := NewCleanup(clientset, nsName, nil, cfg.KeepNamespace)
	cleanup.RegisterSignalHandler(cancel)

	// Set up RBAC
	if err := createServiceAccount(ctx, clientset, nsName); err != nil {
		cleanup.Run()
		return fmt.Errorf("creating service account: %w", err)
	}

	if err := ensureClusterRole(ctx, clientset); err != nil {
		cleanup.Run()
		return fmt.Errorf("ensuring cluster role: %w", err)
	}

	crbNames, err := createClusterRoleBindings(ctx, clientset, nsName)
	if err != nil {
		cleanup.Run()
		return fmt.Errorf("creating cluster role bindings: %w", err)
	}
	cleanup.crbNames = crbNames

	// Create and run the Job
	job := buildJob(JobConfig{
		Namespace:    nsName,
		Image:        cfg.Image,
		Namespaces:   cfg.Namespaces,
		LookbackDays: cfg.LookbackDays,
		ConsoleURL:   cfg.ConsoleURL,
		IncludeOS:    cfg.IncludeOS,
		PrometheusURL: prometheusURL,
		Timeout:      cfg.Timeout,
	})

	fmt.Println("Creating analyzer Job...")
	_, err = clientset.BatchV1().Jobs(nsName).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		cleanup.Run()
		return fmt.Errorf("creating job: %w", err)
	}

	// Wait for Pod to be running and stream logs
	podName, err := waitForPod(ctx, clientset, nsName, cfg.Timeout)
	if err != nil {
		cleanup.Run()
		return fmt.Errorf("waiting for pod: %w", err)
	}

	if err := streamLogs(ctx, clientset, nsName, podName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: log streaming interrupted: %v\n", err)
	}

	// Wait for Job completion
	if err := waitForJobCompletion(ctx, clientset, nsName, cfg.Timeout); err != nil {
		cleanup.Run()
		return fmt.Errorf("job failed: %w", err)
	}

	// Copy PDF from pod
	outputPath := cfg.Output
	if outputPath == "" {
		outputPath = fmt.Sprintf("rrrt-report-%s.pdf", time.Now().Format("2006-01-07T1504"))
	}

	fmt.Println("Downloading report...")
	if err := copyFromPod(ctx, config, clientset, nsName, podName, "/output/report.pdf", outputPath); err != nil {
		cleanup.Run()
		return fmt.Errorf("copying report: %w", err)
	}

	// Cleanup
	cleanup.Run()

	fmt.Printf("Report saved to %s\n", outputPath)
	return nil
}

func waitForPod(ctx context.Context, clientset *kubernetes.Clientset, namespace string, timeout time.Duration) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		pods, err := clientset.CoreV1().Pods(namespace).List(timeoutCtx, metav1.ListOptions{
			LabelSelector: "job-name=rrrt-analyzer",
		})
		if err != nil {
			return "", err
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning ||
				pod.Status.Phase == corev1.PodSucceeded ||
				pod.Status.Phase == corev1.PodFailed {
				return pod.Name, nil
			}
		}

		time.Sleep(2 * time.Second)

		select {
		case <-timeoutCtx.Done():
			return "", fmt.Errorf("timed out waiting for pod")
		default:
		}
	}
}

func streamLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) error {
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Follow: true,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		line := scanner.Text()

		var progress struct {
			Phase     string `json:"phase"`
			Namespace string `json:"namespace"`
			Resource  string `json:"resource"`
			Index     int    `json:"index"`
			Total     int    `json:"total"`
			Status    string `json:"status"`
		}

		if json.Unmarshal([]byte(line), &progress) == nil && progress.Phase != "" {
			if progress.Resource != "" {
				fmt.Printf("[%d/%d] Analyzing %s %s/%s...\n", progress.Index, progress.Total, progress.Phase, progress.Namespace, progress.Resource)
			} else {
				fmt.Printf("[%d/%d] %s %s...\n", progress.Index, progress.Total, progress.Status, progress.Namespace)
			}
		} else {
			fmt.Println(line)
		}
	}

	return scanner.Err()
}

func waitForJobCompletion(ctx context.Context, clientset *kubernetes.Clientset, namespace string, timeout time.Duration) error {
	watcher, err := clientset.BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=rrrt-analyzer",
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}
			if event.Type == watch.Modified {
				job, ok := event.Object.(*batchv1.Job)
				if !ok {
					continue
				}
				for _, cond := range job.Status.Conditions {
					if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
						return nil
					}
					if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
						return fmt.Errorf("job failed: %s", cond.Message)
					}
				}
			}
		case <-timeoutCh:
			return fmt.Errorf("timed out waiting for job completion")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
```

- [ ] **Step 7: Verify build**

```bash
go get gopkg.in/yaml.v3@latest
go get k8s.io/client-go@latest
go mod tidy
go build ./...
```

Expected: builds with no errors.

- [ ] **Step 8: Commit**

```bash
git add internal/cli/
git commit -m "feat: add CLI orchestrator with namespace lifecycle and RBAC"
```

---

### Task 7: Wire Up CLI and Analyzer Entry Points

**Files:**
- Modify: `cmd/oc-rrrt/main.go`
- Modify: `cmd/analyzer/main.go`

- [ ] **Step 1: Wire up the CLI report command**

Replace `cmd/oc-rrrt/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kborup-redhat/rrrt/internal/cli"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "oc-rrrt",
		Short: "Resource Rightsizing Reporting Tool",
	}

	var namespaces []string
	var output string
	var lookbackDays int
	var image string
	var consoleURL string
	var includeOpenShift bool
	var keepNamespace bool
	var timeout time.Duration

	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a rightsizing report",
		RunE: func(cmd *cobra.Command, args []string) error {
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

			config, err := kubeConfig.ClientConfig()
			if err != nil {
				return fmt.Errorf("loading kubeconfig: %w", err)
			}

			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				return fmt.Errorf("creating clientset: %w", err)
			}

			return cli.Run(context.Background(), config, clientset, cli.RunConfig{
				Namespaces:    namespaces,
				Output:        output,
				LookbackDays:  lookbackDays,
				Image:         image,
				ConsoleURL:    consoleURL,
				IncludeOS:     includeOpenShift,
				KeepNamespace: keepNamespace,
				Timeout:       timeout,
			})
		},
	}

	reportCmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", nil, "Namespaces to analyze (can be repeated)")
	reportCmd.Flags().StringVarP(&output, "output", "o", "", "Output PDF path")
	reportCmd.Flags().IntVar(&lookbackDays, "lookback-days", 14, "Metrics lookback window in days")
	reportCmd.Flags().StringVar(&image, "image", "", "Override analyzer container image")
	reportCmd.Flags().StringVar(&consoleURL, "console-url", "", "OpenShift Console base URL")
	reportCmd.Flags().BoolVar(&includeOpenShift, "include-openshift", false, "Include openshift-* and kube-* namespaces")
	reportCmd.Flags().BoolVar(&keepNamespace, "keep-namespace", false, "Don't delete temporary namespace after completion")
	reportCmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Maximum time to wait for the Job")

	rootCmd.AddCommand(reportCmd)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Wire up the analyzer entry point**

Replace `cmd/analyzer/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kborup-redhat/rrrt/internal/collector"
	"github.com/kborup-redhat/rrrt/internal/owner"
	"github.com/kborup-redhat/rrrt/internal/pdf"
	"github.com/kborup-redhat/rrrt/internal/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var version = "dev"

func main() {
	fmt.Println("rrrt-analyzer", version)

	cfg := readConfig()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	restConfig := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating kubernetes client: %v\n", err)
		os.Exit(1)
	}

	promClient := collector.NewPrometheusClient(cfg.PrometheusURL, "")
	ownerResolver := owner.NewResolver(k8sClient)

	coll := collector.New(k8sClient, promClient, ownerResolver, cfg.ConsoleURL,
		cfg.LookbackDays, types.DefaultHeadroom, cfg.IncludeOpenShift)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	data, err := coll.Collect(ctx, cfg.Namespaces)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error collecting data: %v\n", err)
		os.Exit(1)
	}

	data.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	data.CLIVersion = version
	data.ImageVersion = version

	clusterName := detectClusterName(ctx, k8sClient)
	data.ClusterName = clusterName

	if len(cfg.Namespaces) > 0 {
		data.Scope = "Namespaces: " + strings.Join(cfg.Namespaces, ", ")
	} else {
		data.Scope = "All namespaces"
	}

	vmCandidates := countCandidates(data.VMAnalyses)
	contCandidates := countCandidates(data.ContainerAnalyses)
	total := len(data.VMAnalyses) + len(data.ContainerAnalyses)
	fmt.Printf("Found %d rightsizing candidates out of %d resources. Generating PDF...\n",
		vmCandidates+contCandidates, total)

	os.MkdirAll("/output", 0755)
	if err := pdf.Generate(data, "/output/report.pdf"); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating PDF: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Report generated successfully at /output/report.pdf")
}

func readConfig() types.AnalyzerConfig {
	cfg := types.AnalyzerConfig{
		PrometheusURL: types.DefaultPrometheusURL,
		LookbackDays:  types.DefaultLookbackDays,
	}

	if ns := os.Getenv("RRRT_NAMESPACES"); ns != "" {
		cfg.Namespaces = strings.Split(ns, ",")
	}
	if days := os.Getenv("RRRT_LOOKBACK_DAYS"); days != "" {
		if d, err := strconv.Atoi(days); err == nil {
			cfg.LookbackDays = d
		}
	}
	if url := os.Getenv("RRRT_CONSOLE_URL"); url != "" {
		cfg.ConsoleURL = url
	}
	if inc := os.Getenv("RRRT_INCLUDE_OPENSHIFT"); inc == "true" {
		cfg.IncludeOpenShift = true
	}
	if url := os.Getenv("RRRT_PROMETHEUS_URL"); url != "" {
		cfg.PrometheusURL = url
	}

	return cfg
}

func detectClusterName(ctx context.Context, c client.Client) string {
	cv := &unstructured.Unstructured{}
	cv.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "ClusterVersion",
	})
	if err := c.Get(ctx, client.ObjectKey{Name: "version"}, cv); err != nil {
		return "Unknown Cluster"
	}
	id, found, _ := unstructured.NestedString(cv.Object, "spec", "clusterID")
	if found && id != "" {
		return id
	}
	return "OpenShift Cluster"
}

func countCandidates(analyses []types.ResourceAnalysis) int {
	count := 0
	for _, a := range analyses {
		if a.Direction != "" {
			count++
		}
	}
	return count
}
```

- [ ] **Step 3: Verify build**

```bash
go mod tidy
go build ./...
```

Expected: builds with no errors.

- [ ] **Step 4: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat: wire up CLI and analyzer entry points"
```

---

### Task 8: Containerfile

**Files:**
- Create: `Containerfile`

- [ ] **Step 1: Create the multi-stage Containerfile**

Create `Containerfile`:

```dockerfile
FROM registry.access.redhat.com/ubi9/go-toolset:1.22 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION:-dev}" -o /tmp/oc-rrrt ./cmd/oc-rrrt/
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION:-dev}" -o /tmp/rrrt-analyzer ./cmd/analyzer/

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=builder /tmp/rrrt-analyzer /usr/local/bin/rrrt-analyzer

RUN mkdir -p /output

USER 1001

ENTRYPOINT ["rrrt-analyzer"]
```

- [ ] **Step 2: Verify Containerfile syntax with a dry build (optional — skip if no local podman)**

```bash
podman build --no-cache -f Containerfile -t rrrt-test . 2>&1 | head -20 || echo "Skipping container build (no podman)"
```

- [ ] **Step 3: Commit**

```bash
git add Containerfile
git commit -m "feat: add multi-stage Containerfile for analyzer image"
```

---

### Task 9: GitHub Actions Workflows

**Files:**
- Create: `.github/workflows/ci.yaml`
- Create: `.github/workflows/release.yaml`
- Create: `.github/workflows/docs.yaml`

- [ ] **Step 1: Create CI workflow**

Create `.github/workflows/ci.yaml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest

      - name: Test
        run: go test ./... -v -race

      - name: Build
        run: |
          go build -o oc-rrrt ./cmd/oc-rrrt/
          go build -o rrrt-analyzer ./cmd/analyzer/
```

- [ ] **Step 2: Create release workflow**

Create `.github/workflows/release.yaml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Get version
        id: version
        run: echo "VERSION=${GITHUB_REF#refs/tags/}" >> $GITHUB_OUTPUT

      - name: Build CLI binaries
        run: |
          VERSION=${{ steps.version.outputs.VERSION }}
          LDFLAGS="-s -w -X main.version=${VERSION}"

          GOOS=linux GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o dist/oc-rrrt-linux-amd64 ./cmd/oc-rrrt/
          GOOS=linux GOARCH=arm64 go build -ldflags="${LDFLAGS}" -o dist/oc-rrrt-linux-arm64 ./cmd/oc-rrrt/
          GOOS=darwin GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o dist/oc-rrrt-darwin-amd64 ./cmd/oc-rrrt/
          GOOS=darwin GOARCH=arm64 go build -ldflags="${LDFLAGS}" -o dist/oc-rrrt-darwin-arm64 ./cmd/oc-rrrt/

      - name: Build and push container image
        run: |
          VERSION=${{ steps.version.outputs.VERSION }}
          echo "${{ secrets.QUAY_PASSWORD }}" | podman login -u "${{ secrets.QUAY_USERNAME }}" --password-stdin quay.io
          podman build --build-arg VERSION=${VERSION} -f Containerfile -t quay.io/kborup/rrrt:${VERSION} .
          podman push quay.io/kborup/rrrt:${VERSION}
          podman tag quay.io/kborup/rrrt:${VERSION} quay.io/kborup/rrrt:latest
          podman push quay.io/kborup/rrrt:latest

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: dist/*
          generate_release_notes: true
```

- [ ] **Step 3: Create docs workflow**

Create `.github/workflows/docs.yaml`:

```yaml
name: Docs

on:
  push:
    branches: [main]
    paths:
      - 'docs/tutorial/**'
    tags:
      - 'v*'

permissions:
  pages: write
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install HonKit
        run: npm install -g honkit

      - name: Build docs
        run: |
          cd docs/tutorial
          honkit build . _book

      - name: Upload Pages artifact
        uses: actions/upload-pages-artifact@v3
        with:
          path: docs/tutorial/_book

      - name: Deploy to GitHub Pages
        id: deployment
        uses: actions/deploy-pages@v4
```

- [ ] **Step 4: Commit**

```bash
git add .github/
git commit -m "feat: add GitHub Actions CI, release, and docs workflows"
```

---

### Task 10: README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Create README**

Create `README.md`:

```markdown
# RRRT — Resource Rightsizing Reporting Tool

> **POC / Community Project** — This tool is not supported by Red Hat in any way.

An `oc`/`kubectl` CLI plugin that generates professional PDF rightsizing reports for KubeVirt VirtualMachines and container workloads (Deployments, StatefulSets) on OpenShift clusters.

## How It Works

1. You run `oc rrrt report` from your terminal
2. The CLI creates a temporary namespace on your cluster
3. An analysis Job runs inside the cluster with direct Prometheus access
4. The Job collects CPU/memory metrics, runs a rightsizing calculator, and generates a PDF
5. The CLI downloads the PDF and cleans up the temporary namespace

## Installation

Download the latest binary from [GitHub Releases](https://github.com/kborup-redhat/rrrt/releases) and place it in your PATH:

```bash
# Linux amd64
curl -L https://github.com/kborup-redhat/rrrt/releases/latest/download/oc-rrrt-linux-amd64 -o /usr/local/bin/oc-rrrt
chmod +x /usr/local/bin/oc-rrrt
```

## Usage

```bash
# Analyze all namespaces (excludes openshift-* and kube-* by default)
oc rrrt report

# Analyze specific namespaces
oc rrrt report -n production -n staging

# Custom output path and lookback window
oc rrrt report -o my-report.pdf --lookback-days 30

# Include OpenShift system namespaces
oc rrrt report --include-openshift
```

## Requirements

- OpenShift cluster with Prometheus/Thanos monitoring
- `cluster-admin` role (needed to create namespaces, ClusterRoleBindings, and Jobs)
- The `quay.io/kborup/rrrt` analyzer image must be pullable from the cluster

## Report Contents

- **Cover page** with cluster name, scope, and generation timestamp
- **Executive summary** with total savings and distribution chart
- **VM section** with per-VM metrics tables, utilization charts, and rightsizing justification
- **Container section** with per-workload metrics tables, charts, and justification
- **Insufficient data** section listing resources without enough metrics
- **Appendix** with analysis settings and version info

## License

Apache License 2.0
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add README with installation and usage instructions"
```

---

### Task 11: Final Integration Test

- [ ] **Step 1: Run full build**

```bash
cd /home/kborup/ai-code/rrrt
go build ./...
```

Expected: builds with no errors.

- [ ] **Step 2: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 3: Verify CLI runs**

```bash
./oc-rrrt version
./oc-rrrt report --help
```

Expected: prints version and help text correctly.

- [ ] **Step 4: Build container image locally (if podman available)**

```bash
podman build -f Containerfile -t rrrt-test . || echo "Skipping (no podman)"
```

- [ ] **Step 5: Commit any final fixes**

```bash
git add -A
git status
# Only commit if there are changes
git commit -m "fix: final integration fixes" || echo "Nothing to commit"
```
