# OVRO Discovery & Calculator Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the OVRO operator is running in the cluster, rrrt automatically queries VictoriaMetrics instead of Thanos Querier for better retention and historical data. Additionally, upgrade the calculator to use a 30-day baseline with spike-aware analysis.

**Architecture:** The in-cluster analyzer performs a 4-step OVRO discovery check (CRD, namespace, pod health, endpoint probe) at startup. If all pass, it swaps its Prometheus endpoint to VictoriaMetrics (`http://victoriametrics.ovro-system.svc:8428`). A NetworkPolicy is created to allow traffic from the rrrt namespace. The calculator is updated with spike detection logic ported from OVRO, using max utilization alongside P95.

**Tech Stack:** Go 1.26, Kubernetes client-go, controller-runtime, Cobra CLI, fpdf

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/types/types.go` | Modify | Update `DefaultLookbackDays` to 30, add `DataSource` to `ReportData`, add `NoOVRO`/`OVROEndpoint` to `AnalyzerConfig` |
| `internal/calculator/calculator.go` | Modify | Add `CPUMaxPercent`, `MemMaxPercent`, `LookbackDays` to `AnalysisInput`, add `Reason` to `AnalysisResult`, spike detection in `Analyze()` |
| `internal/calculator/calculator_test.go` | Modify | Add spike detection tests, update existing tests for new fields |
| `internal/collector/vm.go` | Modify | Pass max values and lookback days to calculator |
| `internal/collector/container.go` | Modify | Pass max values and lookback days to calculator |
| `internal/collector/discovery.go` | Create | OVRO discovery logic (CRD, namespace, pod, health probe) |
| `internal/collector/discovery_test.go` | Create | Tests for discovery logic |
| `cmd/analyzer/main.go` | Modify | Call discovery, create NetworkPolicy, read new env vars |
| `cmd/oc-rrrt/main.go` | Modify | Add `--no-ovro`, `--ovro-endpoint` flags, update `--lookback-days` default to 30 |
| `internal/cli/run.go` | Modify | Pass new config to Job, extend cleanup for NetworkPolicy |
| `internal/cli/job.go` | Modify | Add `RRRT_NO_OVRO` and `RRRT_OVRO_ENDPOINT` env vars to Job spec |
| `internal/cli/rbac.go` | Modify | Add pods and networkpolicies permissions to ClusterRole |
| `internal/cli/cleanup.go` | Modify | Add NetworkPolicy cleanup in `ovro-system` |
| `internal/pdf/cover.go` | Modify | Display data source in report info card |
| `internal/pdf/report_test.go` | Modify | Update test data for new `DataSource` field |

---

### Task 1: Update Types — Default Lookback and New Config Fields

**Files:**
- Modify: `internal/types/types.go`

- [ ] **Step 1: Update DefaultLookbackDays and add new constants**

```go
// In internal/types/types.go, replace the constants block:

const (
	LabelOwner        = "rightsizing.redhatconsulting.io/owner"
	AnnotationExclude = "rightsizing.redhatconsulting.io/exclude"

	DefaultPrometheusURL   = "https://thanos-querier.openshift-monitoring.svc:9091"
	DefaultLookbackDays    = 30
	DefaultPercentile      = 95
	DefaultHeadroom        = 20
	DefaultUpsizeThreshold = 90

	DefaultVMMinCPUSavings = 1000    // millicores (1 core)
	DefaultVMMinMemSavings = 1 << 30 // 1 GiB in bytes

	DefaultContainerMinCPUSavings = 250       // millicores (250m)
	DefaultContainerMinMemSavings = 256 << 20 // 256 Mi in bytes

	OVRONamespace          = "ovro-system"
	OVROVictoriaMetricsURL = "http://victoriametrics.ovro-system.svc:8428"
	OVROCRD                = "rightsizingrecommendations.rightsizing.redhatconsulting.io"
)
```

- [ ] **Step 2: Add DataSource to ReportData**

```go
// Add DataSource field to ReportData struct after CLIVersion:

type ReportData struct {
	ClusterName       string
	ClusterID         string
	GeneratedAt       string
	Scope             string
	LookbackDays      int
	Percentile        int
	HeadroomPct       int
	ClusterOverview   *ClusterOverview
	VMAnalyses        []ResourceAnalysis
	ContainerAnalyses []ResourceAnalysis
	InsufficientData  []InsufficientDataEntry
	CLIVersion        string
	ImageVersion      string
	DataSource        string
}
```

- [ ] **Step 3: Add NoOVRO and OVROEndpoint to AnalyzerConfig**

```go
type AnalyzerConfig struct {
	Namespaces       []string
	LookbackDays     int
	ConsoleURL       string
	IncludeOpenShift bool
	PrometheusURL    string
	NoOVRO           bool
	OVROEndpoint     string
}
```

- [ ] **Step 4: Verify the project compiles**

Run: `cd /home/kborup/ai-code/rrrt && go build ./...`
Expected: Compile success (no functional changes yet)

- [ ] **Step 5: Commit**

```bash
git add internal/types/types.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: update default lookback to 30 days and add OVRO config types"
```

---

### Task 2: Calculator — Spike Detection

**Files:**
- Modify: `internal/calculator/calculator.go`
- Modify: `internal/calculator/calculator_test.go`

- [ ] **Step 1: Write failing tests for spike detection**

Add to `internal/calculator/calculator_test.go`:

```go
func TestAnalyze_SpikeTriggersUpsize(t *testing.T) {
	// P95 is low (15%) but max spikes to 92% — should trigger upsize
	input := calculator.AnalysisInput{
		CurrentCPU:         4000,
		CurrentMem:         8 * 1024 * 1024 * 1024,
		CPUP95Percent:      15.0,
		MemP95Percent:      20.0,
		CPUMaxPercent:       92.0,
		MemMaxPercent:       45.0,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
		LookbackDays:       30,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Upsize, result.Direction)
	assert.Greater(t, result.RecommendedCPU, input.CurrentCPU)
	assert.Contains(t, result.Reason, "spikes")
	assert.Contains(t, result.Reason, "30d")
}

func TestAnalyze_SustainedHighUsage(t *testing.T) {
	// Both P95 and max are high — sustained, not just spike
	input := calculator.AnalysisInput{
		CurrentCPU:         4000,
		CurrentMem:         8 * 1024 * 1024 * 1024,
		CPUP95Percent:      94.0,
		MemP95Percent:      92.0,
		CPUMaxPercent:       98.0,
		MemMaxPercent:       96.0,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
		LookbackDays:       30,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Upsize, result.Direction)
	assert.Contains(t, result.Reason, "sustained")
}

func TestAnalyze_DownsizeIncludesReason(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:         8000,
		CurrentMem:         16 * 1024 * 1024 * 1024,
		CPUP95Percent:      28.3,
		MemP95Percent:      41.7,
		CPUMaxPercent:       45.0,
		MemMaxPercent:       55.0,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
		LookbackDays:       30,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Downsize, result.Direction)
	assert.Contains(t, result.Reason, "30d")
}

func TestAnalyze_CombinedCPUAndMemSpike(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:         4000,
		CurrentMem:         8 * 1024 * 1024 * 1024,
		CPUP95Percent:      15.0,
		MemP95Percent:      20.0,
		CPUMaxPercent:       95.0,
		MemMaxPercent:       92.0,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
		LookbackDays:       30,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Upsize, result.Direction)
	assert.Contains(t, result.Reason, "CPU spikes")
	assert.Contains(t, result.Reason, "memory spikes")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kborup/ai-code/rrrt && go test ./internal/calculator/ -run "Spike|Sustained|DownsizeIncludesReason|CombinedCPUAndMem" -v`
Expected: FAIL — `CPUMaxPercent` field doesn't exist yet, compilation error

- [ ] **Step 3: Update AnalysisInput and AnalysisResult structs**

In `internal/calculator/calculator.go`, replace the structs:

```go
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
	LookbackDays       int
}

type AnalysisResult struct {
	Direction      types.Direction
	RecommendedCPU int64
	RecommendedMem int64
	CPUSavings     int64
	MemSavings     int64
	Reason         string
}
```

- [ ] **Step 4: Update Analyze() to check max values**

Replace the `Analyze` function:

```go
func Analyze(input AnalysisInput) *AnalysisResult {
	threshold := float64(input.UpsizeThresholdPct)

	if input.CPUP95Percent >= threshold || input.MemP95Percent >= threshold ||
		input.CPUMaxPercent >= threshold || input.MemMaxPercent >= threshold {
		return analyzeUpsize(input)
	}
	return analyzeDownsize(input)
}
```

- [ ] **Step 5: Update analyzeDownsize with Reason**

Replace the `analyzeDownsize` function:

```go
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
		Direction:      types.Downsize,
		RecommendedCPU: recCPU,
		RecommendedMem: recMem,
		CPUSavings:     cpuSavings,
		MemSavings:     memSavings,
		Reason: fmt.Sprintf("CPU only %.1f%% utilized (P95 over %dd), can save %d cores",
			input.CPUP95Percent, input.LookbackDays, cpuSavings/1000),
	}
}
```

- [ ] **Step 6: Update analyzeUpsize with spike detection and Reason**

Replace the `analyzeUpsize` function:

```go
func analyzeUpsize(input AnalysisInput) *AnalysisResult {
	cpuPct := math.Max(input.CPUP95Percent, input.CPUMaxPercent)
	memPct := math.Max(input.MemP95Percent, input.MemMaxPercent)

	cpuUsage := float64(input.CurrentCPU) * cpuPct / 100.0
	memUsage := float64(input.CurrentMem) * memPct / 100.0

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

	threshold := float64(input.UpsizeThresholdPct)
	cpuSpike := input.CPUMaxPercent >= threshold && input.CPUP95Percent < threshold
	memSpike := input.MemMaxPercent >= threshold && input.MemP95Percent < threshold

	var reason string
	switch {
	case cpuSpike && memSpike:
		reason = fmt.Sprintf("CPU spikes to %.1f%% and memory spikes to %.1f%% (sustained P95: %.1f%%/%.1f%% over %dd)",
			input.CPUMaxPercent, input.MemMaxPercent, input.CPUP95Percent, input.MemP95Percent, input.LookbackDays)
	case cpuSpike:
		reason = fmt.Sprintf("CPU spikes to %.1f%% (sustained P95: %.1f%% over %dd)",
			input.CPUMaxPercent, input.CPUP95Percent, input.LookbackDays)
	case memSpike:
		reason = fmt.Sprintf("Memory spikes to %.1f%% (sustained P95: %.1f%% over %dd)",
			input.MemMaxPercent, input.MemP95Percent, input.LookbackDays)
	default:
		reason = fmt.Sprintf("CPU at %.1f%% sustained utilization (P95 over %dd)",
			input.CPUP95Percent, input.LookbackDays)
	}

	return &AnalysisResult{
		Direction:      types.Upsize,
		RecommendedCPU: recCPU,
		RecommendedMem: recMem,
		CPUSavings:     -cpuIncrease,
		MemSavings:     -memIncrease,
		Reason:         reason,
	}
}
```

- [ ] **Step 7: Add fmt import**

Add `"fmt"` to the import block in `internal/calculator/calculator.go` (alongside existing `"math"`, `"sort"`).

- [ ] **Step 8: Update existing tests to include new fields**

The existing tests in `calculator_test.go` need `CPUMaxPercent`, `MemMaxPercent`, and `LookbackDays` fields added. Update each existing test's `AnalysisInput`:

For `TestAnalyze_Downsize`:
```go
input := calculator.AnalysisInput{
	CurrentCPU:         8000,
	CurrentMem:         16 * 1024 * 1024 * 1024,
	CPUP95Percent:      28.3,
	MemP95Percent:      41.7,
	CPUMaxPercent:       45.0,
	MemMaxPercent:       55.0,
	HeadroomPercent:    20,
	MinCPUSavings:      1000,
	MinMemSavings:      1 << 30,
	UpsizeThresholdPct: 90,
	LookbackDays:       30,
}
```

For `TestAnalyze_Upsize`:
```go
input := calculator.AnalysisInput{
	CurrentCPU:         4000,
	CurrentMem:         8 * 1024 * 1024 * 1024,
	CPUP95Percent:      94.0,
	MemP95Percent:      92.0,
	CPUMaxPercent:       98.0,
	MemMaxPercent:       96.0,
	HeadroomPercent:    20,
	MinCPUSavings:      1000,
	MinMemSavings:      1 << 30,
	UpsizeThresholdPct: 90,
	LookbackDays:       30,
}
```

For `TestAnalyze_NoRecommendation`:
```go
input := calculator.AnalysisInput{
	CurrentCPU:         4000,
	CurrentMem:         8 * 1024 * 1024 * 1024,
	CPUP95Percent:      75.0,
	MemP95Percent:      75.0,
	CPUMaxPercent:       80.0,
	MemMaxPercent:       80.0,
	HeadroomPercent:    20,
	MinCPUSavings:      1000,
	MinMemSavings:      1 << 30,
	UpsizeThresholdPct: 90,
	LookbackDays:       30,
}
```

For `TestAnalyze_ContainerLowThreshold`:
```go
input := calculator.AnalysisInput{
	CurrentCPU:         1000,
	CurrentMem:         1 * 1024 * 1024 * 1024,
	CPUP95Percent:      20.0,
	MemP95Percent:      20.0,
	CPUMaxPercent:       35.0,
	MemMaxPercent:       30.0,
	HeadroomPercent:    20,
	MinCPUSavings:      250,
	MinMemSavings:      256 << 20,
	UpsizeThresholdPct: 90,
	LookbackDays:       30,
}
```

- [ ] **Step 9: Run all calculator tests**

Run: `cd /home/kborup/ai-code/rrrt && go test ./internal/calculator/ -v`
Expected: All 8 tests pass

- [ ] **Step 10: Commit**

```bash
git add internal/calculator/calculator.go internal/calculator/calculator_test.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: add spike detection and 30-day baseline to calculator"
```

---

### Task 3: Wire Max Values Into Collector

**Files:**
- Modify: `internal/collector/vm.go`
- Modify: `internal/collector/container.go`

- [ ] **Step 1: Update VM collector to pass max values and lookback to calculator**

In `internal/collector/vm.go`, update the `calculator.Analyze` call at line 92 to include the new fields:

```go
result := calculator.Analyze(calculator.AnalysisInput{
	CurrentCPU:         cpuMillis,
	CurrentMem:         memBytes,
	CPUP95Percent:      cpuP95,
	MemP95Percent:      memP95,
	CPUMaxPercent:       cpuMax,
	MemMaxPercent:       memMax,
	HeadroomPercent:    c.headroomPct,
	MinCPUSavings:      types.DefaultVMMinCPUSavings,
	MinMemSavings:      types.DefaultVMMinMemSavings,
	UpsizeThresholdPct: types.DefaultUpsizeThreshold,
	LookbackDays:       c.lookbackDays,
})
```

Also update `buildJustification` usage — if the calculator returns a `Reason`, use it as the `Justification`. Replace lines 116-123:

```go
if result != nil {
	analysis.Direction = result.Direction
	analysis.RecommendedCPU = result.RecommendedCPU
	analysis.RecommendedMem = result.RecommendedMem
	analysis.CPUSavings = result.CPUSavings
	analysis.MemSavings = result.MemSavings
	analysis.Justification = result.Reason
}
```

- [ ] **Step 2: Update container collector the same way**

In `internal/collector/container.go`, update the `calculator.Analyze` call at line 113:

```go
result := calculator.Analyze(calculator.AnalysisInput{
	CurrentCPU:         w.cpuMillis,
	CurrentMem:         w.memBytes,
	CPUP95Percent:      cpuP95,
	MemP95Percent:      memP95,
	CPUMaxPercent:       cpuMax,
	MemMaxPercent:       memMax,
	HeadroomPercent:    c.headroomPct,
	MinCPUSavings:      types.DefaultContainerMinCPUSavings,
	MinMemSavings:      types.DefaultContainerMinMemSavings,
	UpsizeThresholdPct: types.DefaultUpsizeThreshold,
	LookbackDays:       c.lookbackDays,
})
```

Replace lines 142-149 (same pattern as VM):

```go
if result != nil {
	analysis.Direction = result.Direction
	analysis.RecommendedCPU = result.RecommendedCPU
	analysis.RecommendedMem = result.RecommendedMem
	analysis.CPUSavings = result.CPUSavings
	analysis.MemSavings = result.MemSavings
	analysis.Justification = result.Reason
}
```

- [ ] **Step 3: Remove the buildJustification function from vm.go**

Delete the `buildJustification` and `formatBytes` functions from `internal/collector/vm.go` (lines 144-175). These are no longer needed since the calculator now provides the Reason string.

- [ ] **Step 4: Verify the project compiles and tests pass**

Run: `cd /home/kborup/ai-code/rrrt && go build ./... && go test ./...`
Expected: Compile success, all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/collector/vm.go internal/collector/container.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: wire max utilization and lookback into calculator calls"
```

---

### Task 4: OVRO Discovery Logic

**Files:**
- Create: `internal/collector/discovery.go`
- Create: `internal/collector/discovery_test.go`

- [ ] **Step 1: Write failing tests for discovery**

Create `internal/collector/discovery_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kborup/ai-code/rrrt && go test ./internal/collector/ -run "DiscoverOVRO" -v`
Expected: FAIL — `ProbeHealth` not defined

- [ ] **Step 3: Implement discovery.go**

Create `internal/collector/discovery.go`:

```go
package collector

import (
	"context"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

// OVRODiscoveryResult holds the outcome of OVRO detection.
type OVRODiscoveryResult struct {
	Detected bool
	Endpoint string
	Message  string
}

// DiscoverOVRO runs a 4-step check to determine if OVRO is running in the cluster.
// Steps: CRD registered, ovro-system namespace exists, VictoriaMetrics pod running, health endpoint responds.
func DiscoverOVRO(ctx context.Context, clientset *kubernetes.Clientset, ovroNamespace, crdName, vmURL string) OVRODiscoveryResult {
	if !checkCRD(clientset.Discovery(), crdName) {
		return OVRODiscoveryResult{Message: fmt.Sprintf("CRD %s not found", crdName)}
	}

	if !checkNamespace(ctx, clientset, ovroNamespace) {
		return OVRODiscoveryResult{Message: fmt.Sprintf("namespace %s not found", ovroNamespace)}
	}

	if !checkVictoriaMetricsPod(ctx, clientset, ovroNamespace) {
		return OVRODiscoveryResult{Message: "VictoriaMetrics pod not running in " + ovroNamespace}
	}

	if !ProbeHealth(ctx, vmURL) {
		return OVRODiscoveryResult{Message: "VictoriaMetrics health probe failed at " + vmURL}
	}

	return OVRODiscoveryResult{
		Detected: true,
		Endpoint: vmURL,
		Message:  "OVRO detected: using VictoriaMetrics at " + vmURL,
	}
}

func checkCRD(disc discovery.DiscoveryInterface, crdName string) bool {
	_, resourceLists, err := disc.ServerGroupsAndResources()
	if err != nil {
		return false
	}
	parts := splitCRDName(crdName)
	if parts == nil {
		return false
	}
	for _, rl := range resourceLists {
		for _, r := range rl.APIResources {
			if r.Name == parts[0] {
				return true
			}
		}
	}
	return false
}

func splitCRDName(name string) []string {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return []string{name[:i], name[i+1:]}
		}
	}
	return nil
}

func checkNamespace(ctx context.Context, clientset *kubernetes.Clientset, name string) bool {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

func checkVictoriaMetricsPod(ctx context.Context, clientset *kubernetes.Clientset, namespace string) bool {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=victoriametrics",
	})
	if err != nil || len(pods.Items) == 0 {
		return false
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return true
		}
	}
	return false
}

// ProbeHealth performs an HTTP GET to the /health endpoint and returns true if it responds 200 OK.
func ProbeHealth(ctx context.Context, baseURL string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
```

- [ ] **Step 4: Run discovery tests**

Run: `cd /home/kborup/ai-code/rrrt && go test ./internal/collector/ -run "DiscoverOVRO" -v`
Expected: All 3 tests pass

- [ ] **Step 5: Verify full test suite**

Run: `cd /home/kborup/ai-code/rrrt && go test ./...`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/collector/discovery.go internal/collector/discovery_test.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: add OVRO discovery logic with CRD, namespace, pod, and health checks"
```

---

### Task 5: RBAC — Add Pods and NetworkPolicy Permissions

**Files:**
- Modify: `internal/cli/rbac.go`

- [ ] **Step 1: Add new RBAC rules to ensureClusterRole**

In `internal/cli/rbac.go`, add two new `PolicyRule` entries to the `Rules` slice in the `ensureClusterRole` function, after the existing rules:

```go
{
	APIGroups: []string{""},
	Resources: []string{"pods"},
	Verbs:     []string{"get", "list"},
},
{
	APIGroups: []string{"networking.k8s.io"},
	Resources: []string{"networkpolicies"},
	Verbs:     []string{"get", "create", "delete"},
},
```

- [ ] **Step 2: Verify the project compiles**

Run: `cd /home/kborup/ai-code/rrrt && go build ./...`
Expected: Compile success

- [ ] **Step 3: Commit**

```bash
git add internal/cli/rbac.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: add pods and networkpolicies RBAC for OVRO discovery"
```

---

### Task 6: CLI Flags and Job Env Vars

**Files:**
- Modify: `cmd/oc-rrrt/main.go`
- Modify: `internal/cli/run.go`
- Modify: `internal/cli/job.go`

- [ ] **Step 1: Add --no-ovro and --ovro-endpoint flags to CLI**

In `cmd/oc-rrrt/main.go`, add two new variable declarations after `keepNamespace`:

```go
var noOVRO bool
var ovroEndpoint string
```

Add two new flag bindings after the `keepNamespace` flag registration (before `rootCmd.AddCommand(reportCmd)`):

```go
reportCmd.Flags().BoolVar(&noOVRO, "no-ovro", false, "Force using Thanos even when OVRO is detected")
reportCmd.Flags().StringVar(&ovroEndpoint, "ovro-endpoint", "", "Override VictoriaMetrics endpoint URL (implies OVRO mode)")
```

Update the `--lookback-days` flag default from 14 to 30:

```go
reportCmd.Flags().IntVar(&lookbackDays, "lookback-days", 30, "Metrics lookback window in days")
```

- [ ] **Step 2: Add new fields to RunConfig and pass them**

In `internal/cli/run.go`, add `NoOVRO` and `OVROEndpoint` fields to the `RunConfig` struct:

```go
type RunConfig struct {
	Namespaces    []string
	Output        string
	LookbackDays  int
	Image         string
	ConsoleURL    string
	IncludeOS     bool
	KeepNamespace bool
	Timeout       time.Duration
	Version       string
	NoOVRO        bool
	OVROEndpoint  string
}
```

In `cmd/oc-rrrt/main.go`, pass the new fields in the `cli.Run` call:

```go
return cli.Run(context.Background(), config, clientset, cli.RunConfig{
	Namespaces:    namespaces,
	Output:        output,
	LookbackDays:  lookbackDays,
	Image:         image,
	ConsoleURL:    cfg.ConsoleURL,
	IncludeOS:     includeOpenShift,
	KeepNamespace: keepNamespace,
	Timeout:       timeout,
	Version:       version,
	NoOVRO:        noOVRO,
	OVROEndpoint:  ovroEndpoint,
})
```

- [ ] **Step 3: Add new fields to JobConfig and pass them through**

In `internal/cli/job.go`, add `NoOVRO` and `OVROEndpoint` to `JobConfig`:

```go
type JobConfig struct {
	Namespace     string
	Image         string
	Namespaces    []string
	LookbackDays  int
	ConsoleURL    string
	IncludeOS     bool
	PrometheusURL string
	Timeout       time.Duration
	NoOVRO        bool
	OVROEndpoint  string
}
```

In the `buildJob` function, add the new env vars to the `env` slice after the existing entries:

```go
if cfg.NoOVRO {
	env = append(env, corev1.EnvVar{Name: "RRRT_NO_OVRO", Value: "true"})
}
if cfg.OVROEndpoint != "" {
	env = append(env, corev1.EnvVar{Name: "RRRT_OVRO_ENDPOINT", Value: cfg.OVROEndpoint})
}
```

- [ ] **Step 4: Pass new fields from Run() to buildJob()**

In `internal/cli/run.go`, update the `buildJob` call (around line 95) to include the new fields:

```go
job := buildJob(JobConfig{
	Namespace:     nsName,
	Image:         cfg.Image,
	Namespaces:    cfg.Namespaces,
	LookbackDays:  cfg.LookbackDays,
	ConsoleURL:    cfg.ConsoleURL,
	IncludeOS:     cfg.IncludeOS,
	PrometheusURL: prometheusURL,
	Timeout:       cfg.Timeout,
	NoOVRO:        cfg.NoOVRO,
	OVROEndpoint:  cfg.OVROEndpoint,
})
```

- [ ] **Step 5: Verify the project compiles**

Run: `cd /home/kborup/ai-code/rrrt && go build ./...`
Expected: Compile success

- [ ] **Step 6: Commit**

```bash
git add cmd/oc-rrrt/main.go internal/cli/run.go internal/cli/job.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: add --no-ovro and --ovro-endpoint CLI flags with env var passthrough"
```

---

### Task 7: Analyzer — OVRO Discovery, NetworkPolicy, and Data Source Selection

**Files:**
- Modify: `cmd/analyzer/main.go`

- [ ] **Step 1: Read new env vars in readConfig()**

In `cmd/analyzer/main.go`, add reading of the two new env vars at the end of `readConfig()`:

```go
if os.Getenv("RRRT_NO_OVRO") == "true" {
	cfg.NoOVRO = true
}
if ep := os.Getenv("RRRT_OVRO_ENDPOINT"); ep != "" {
	cfg.OVROEndpoint = ep
}
```

- [ ] **Step 2: Add imports for NetworkPolicy and discovery**

Add these to the import block in `cmd/analyzer/main.go`:

```go
networkingv1 "k8s.io/api/networking/v1"
"k8s.io/client-go/kubernetes"
"k8s.io/apimachinery/pkg/util/intstr"
```

Also add the collector import alias if not already present (it should be, as `collector` is already imported).

- [ ] **Step 3: Add OVRO discovery and NetworkPolicy creation in main()**

In `cmd/analyzer/main.go`, after the `k8sClient` creation (around line 39) and before `promClient` creation (line 45), add the OVRO discovery and endpoint selection logic. Replace lines 45-46 with:

```go
clientset, err := kubernetes.NewForConfig(restConfig)
if err != nil {
	fmt.Fprintf(os.Stderr, "Error creating clientset: %v\n", err)
	os.Exit(1)
}

prometheusURL := cfg.PrometheusURL
dataSource := "OpenShift Thanos Querier"

if cfg.OVROEndpoint != "" {
	prometheusURL = cfg.OVROEndpoint
	dataSource = "OVRO VictoriaMetrics (custom endpoint)"
	fmt.Printf("Using custom OVRO endpoint: %s\n", cfg.OVROEndpoint)
} else if !cfg.NoOVRO {
	result := collector.DiscoverOVRO(ctx, clientset,
		types.OVRONamespace, types.OVROCRD, types.OVROVictoriaMetricsURL)
	fmt.Println(result.Message)
	if result.Detected {
		if err := createOVRONetworkPolicy(ctx, clientset, cfg.AnalyzerNamespace); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create NetworkPolicy for OVRO access: %v\n", err)
		}
		prometheusURL = result.Endpoint
		dataSource = "OVRO VictoriaMetrics (90d retention)"
	}
}

promClient := collector.NewPrometheusClient(prometheusURL, "")
```

Note: `cfg.AnalyzerNamespace` needs to be passed. Add it to `AnalyzerConfig` — see step below.

- [ ] **Step 4: Add AnalyzerNamespace to config and types**

In `internal/types/types.go`, add `AnalyzerNamespace` to `AnalyzerConfig`:

```go
type AnalyzerConfig struct {
	Namespaces        []string
	LookbackDays      int
	ConsoleURL        string
	IncludeOpenShift  bool
	PrometheusURL     string
	NoOVRO            bool
	OVROEndpoint      string
	AnalyzerNamespace string
}
```

In `cmd/analyzer/main.go`, in `readConfig()`, derive the namespace from the downward API or hostname:

```go
if ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
	cfg.AnalyzerNamespace = strings.TrimSpace(string(ns))
}
```

- [ ] **Step 5: Add createOVRONetworkPolicy function**

Add to `cmd/analyzer/main.go`:

```go
func createOVRONetworkPolicy(ctx context.Context, clientset *kubernetes.Clientset, fromNamespace string) error {
	port := intstr.FromInt32(8428)
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-rrrt-to-victoriametrics",
			Namespace: types.OVRONamespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "victoriametrics"},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": fromNamespace,
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Port: &port},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		},
	}

	_, err := clientset.NetworkingV1().NetworkPolicies(types.OVRONamespace).Create(ctx, np, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating network policy: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Set DataSource on report data**

In `cmd/analyzer/main.go`, after the `data.ImageVersion = version` line, add:

```go
data.DataSource = dataSource
```

- [ ] **Step 7: Add the networkingv1 import to analyzer**

Ensure `cmd/analyzer/main.go` imports include:

```go
networkingv1 "k8s.io/api/networking/v1"
"k8s.io/apimachinery/pkg/util/intstr"
"k8s.io/client-go/kubernetes"
```

- [ ] **Step 8: Verify the project compiles**

Run: `cd /home/kborup/ai-code/rrrt && go build ./...`
Expected: Compile success

- [ ] **Step 9: Commit**

```bash
git add cmd/analyzer/main.go internal/types/types.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: add OVRO discovery and NetworkPolicy creation to analyzer"
```

---

### Task 8: Cleanup — NetworkPolicy Deletion

**Files:**
- Modify: `internal/cli/cleanup.go`
- Modify: `internal/cli/run.go`

- [ ] **Step 1: Update Run() to add best-effort OVRO NetworkPolicy cleanup**

The analyzer creates the NetworkPolicy inside the pod, so the CLI doesn't know at cleanup time whether OVRO was detected. Use a best-effort unconditional delete — if the NetworkPolicy doesn't exist, the error is silently ignored.

In `internal/cli/cleanup.go`, replace the entire `Run()` method:

```go
func (c *Cleanup) Run() {
	if c.keepNamespace {
		fmt.Fprintf(os.Stderr, "Keeping namespace %s (--keep-namespace set)\n", c.namespace)
		return
	}

	ctx := context.Background()

	// Best-effort cleanup of OVRO NetworkPolicy (created by analyzer if OVRO was detected)
	_ = c.clientset.NetworkingV1().NetworkPolicies("ovro-system").Delete(
		ctx, "allow-rrrt-to-victoriametrics", metav1.DeleteOptions{})

	for _, name := range c.crbNames {
		err := c.clientset.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete ClusterRoleBinding %s: %v\n", name, err)
		}
	}

	err := c.clientset.CoreV1().Namespaces().Delete(ctx, c.namespace, metav1.DeleteOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cleanup failed. Run manually:\n")
		fmt.Fprintf(os.Stderr, "  oc delete networkpolicy allow-rrrt-to-victoriametrics -n ovro-system\n")
		for _, name := range c.crbNames {
			fmt.Fprintf(os.Stderr, "  oc delete clusterrolebinding %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "  oc delete namespace %s\n", c.namespace)
	}
}
```

- [ ] **Step 4: Verify the project compiles**

Run: `cd /home/kborup/ai-code/rrrt && go build ./...`
Expected: Compile success

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cleanup.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: add best-effort OVRO NetworkPolicy cleanup"
```

---

### Task 9: PDF Report — Display Data Source

**Files:**
- Modify: `internal/pdf/cover.go`
- Modify: `internal/pdf/report_test.go`

- [ ] **Step 1: Add DataSource to the cover info card**

In `internal/pdf/cover.go`, update the `infoCard` call to include the data source. Replace the `infoCard` call:

```go
infoCard(p, 20, 110, 170, [][2]string{
	{"Cluster", data.ClusterName},
	{"Scope", data.Scope},
	{"Lookback", itoa(data.LookbackDays) + " days"},
	{"Data Source", data.DataSource},
	{"Generated", data.GeneratedAt},
})
```

- [ ] **Step 2: Update PDF tests with DataSource field**

In `internal/pdf/report_test.go`, add `DataSource` to both test cases.

In `TestGenerateReport_Empty`:
```go
data := &types.ReportData{
	ClusterName:  "test-cluster",
	GeneratedAt:  "2026-05-07T14:30:00Z",
	Scope:        "All namespaces",
	LookbackDays: 30,
	Percentile:   95,
	HeadroomPct:  20,
	CLIVersion:   "v0.1.0",
	ImageVersion: "v0.1.0",
	DataSource:   "OpenShift Thanos Querier",
}
```

In `TestGenerateReport_WithData`:
```go
data := &types.ReportData{
	ClusterName:  "prod-cluster",
	ClusterID:    "d4e5f6a7-b8c9-0123-4567-89abcdef0123",
	GeneratedAt:  "2026-05-07T14:30:00Z",
	Scope:        "Namespaces: default, production",
	LookbackDays: 30,
	Percentile:   95,
	HeadroomPct:  20,
	CLIVersion:   "v0.1.0",
	ImageVersion: "v0.1.0",
	DataSource:   "OVRO VictoriaMetrics (90d retention)",
	// ... rest of fields unchanged
```

- [ ] **Step 3: Run tests**

Run: `cd /home/kborup/ai-code/rrrt && go test ./internal/pdf/ -v`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/pdf/cover.go internal/pdf/report_test.go
git commit --author="Kim Borup <kborup@redhat.com>" -m "feat: display data source in PDF report cover"
```

---

### Task 10: Final Verification

**Files:** None (verification only)

- [ ] **Step 1: Run full linter**

Run: `cd /home/kborup/ai-code/rrrt && golangci-lint run ./...`
Expected: No errors

- [ ] **Step 2: Run full test suite**

Run: `cd /home/kborup/ai-code/rrrt && go test -race ./...`
Expected: All tests pass with race detector enabled

- [ ] **Step 3: Build both binaries**

Run: `cd /home/kborup/ai-code/rrrt && go build -o /dev/null ./cmd/oc-rrrt/ && go build -o /dev/null ./cmd/analyzer/`
Expected: Both binaries compile successfully

- [ ] **Step 4: Verify CLI help shows new flags**

Run: `cd /home/kborup/ai-code/rrrt && go run ./cmd/oc-rrrt/ report --help`
Expected output should include:
```
--lookback-days int    Metrics lookback window in days (default 30)
--no-ovro              Force using Thanos even when OVRO is detected
--ovro-endpoint string Override VictoriaMetrics endpoint URL (implies OVRO mode)
```
