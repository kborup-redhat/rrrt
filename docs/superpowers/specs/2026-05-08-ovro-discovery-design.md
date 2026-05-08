# OVRO Discovery and Calculator Improvements

## Overview

Add automatic detection of the OVRO (OpenShift Virtualization Rightsizing Operator) project running in the cluster. When OVRO is detected, rrrt queries VictoriaMetrics for metrics instead of the default Thanos Querier, benefiting from longer retention (90 days) and seeded historical data. Additionally, update rrrt's calculator to use a 30-day baseline and spike-aware analysis, matching OVRO's improved logic.

## Architecture

### Data Source Selection

The analyzer currently always connects to Thanos Querier at `https://thanos-querier.openshift-monitoring.svc:9091`. With this change, the analyzer performs OVRO discovery at startup and may switch to VictoriaMetrics at `http://victoriametrics.ovro-system.svc:8428`.

VictoriaMetrics exposes a Prometheus-compatible query API, so the same PromQL queries work unchanged. VictoriaMetrics receives all cluster metrics via Prometheus remote-write (configured in `cluster-monitoring-config` ConfigMap in `openshift-monitoring`), which includes both KubeVirt VM metrics and container metrics. OVRO only uses VM metrics, but all metrics are stored — rrrt can query both workload types from VictoriaMetrics.

Key differences when using VictoriaMetrics:
- **Protocol**: HTTP (not HTTPS) — no TLS required
- **Auth**: No bearer token required (VictoriaMetrics does not enforce auth by default)
- **Retention**: 90 days (vs ~14 days default in OpenShift Prometheus)

### Discovery Logic

The analyzer performs a 4-step discovery check when starting:

1. **CRD check** — Use the Kubernetes discovery client to verify `rightsizingrecommendations.rightsizing.redhatconsulting.io` exists as a registered API resource
2. **Namespace check** — Verify the `ovro-system` namespace exists
3. **Pod health check** — List pods in `ovro-system` matching VictoriaMetrics, confirm at least one is in `Running` phase
4. **Endpoint probe** — HTTP GET to `http://victoriametrics.ovro-system.svc:8428/health` to confirm it is responding

All four checks must pass. If any fails, the analyzer falls back to Thanos Querier and logs which check failed.

On success: `"OVRO detected: using VictoriaMetrics at http://victoriametrics.ovro-system.svc:8428"`
On failure: `"OVRO not detected: using Thanos at https://thanos-querier.openshift-monitoring.svc:9091"`

### NetworkPolicy

OVRO deploys a NetworkPolicy on VictoriaMetrics that restricts ingress. The rrrt analyzer runs in a dynamically-named `rrrt-*` namespace and would be blocked.

When OVRO is detected, the analyzer creates a NetworkPolicy in `ovro-system` that allows ingress from the rrrt analyzer namespace to VictoriaMetrics on port 8428. This NetworkPolicy is cleaned up when the rrrt namespace is deleted (as part of existing cleanup logic — the cleanup function will be extended to delete the NetworkPolicy from `ovro-system`).

### RBAC Updates

The `rrrt-analyzer` ClusterRole needs additional permissions:

- `get` on `namespaces` (currently only has `list`)
- `get`, `list` on `pods` in `ovro-system` (for pod health check)
- `create`, `delete` on `networkpolicies` in `ovro-system` (for the VictoriaMetrics ingress opening)

API discovery access for CRD checks is available by default to authenticated service accounts.

## CLI Changes

### New Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-ovro` | bool | false | Force using Thanos even when OVRO is detected |
| `--ovro-endpoint` | string | "" | Override VictoriaMetrics endpoint URL (implies OVRO mode, skips discovery) |

### Updated Defaults

| Flag | Old Default | New Default |
|------|-------------|-------------|
| `--lookback-days` | 14 | 30 |

### Environment Variable Passthrough

The CLI passes these to the analyzer Job as environment variables:

| Env Var | Source Flag |
|---------|-----------|
| `RRRT_NO_OVRO` | `--no-ovro` |
| `RRRT_OVRO_ENDPOINT` | `--ovro-endpoint` |

These follow the same pattern as existing env vars (`RRRT_NAMESPACES`, `RRRT_LOOKBACK_DAYS`, etc.).

## Calculator Improvements

These changes apply regardless of which data source is used.

### 1. Default Lookback: 14 -> 30 Days

Update `DefaultLookbackDays` in `internal/types/types.go` from 14 to 30. This ensures the analysis captures recurring spikes that may only appear weekly or bi-weekly.

### 2. Spike Detection

Add `CPUMaxPercent` and `MemMaxPercent` fields to `calculator.AnalysisInput`. These values are already computed in `vm.go` and `container.go` but not currently passed to the calculator.

Update `Analyze()` to trigger upsize when **any** of P95 or max exceeds the threshold (currently only P95 is checked):

```go
if input.CPUP95Percent >= threshold || input.MemP95Percent >= threshold ||
    input.CPUMaxPercent >= threshold || input.MemMaxPercent >= threshold {
    return analyzeUpsize(input)
}
```

### 3. Spike-Aware Upsize Calculation

In `analyzeUpsize()`, use `math.Max(P95, Max)` for the recommendation calculation, matching OVRO's approach:

```go
cpuPct := math.Max(input.CPUP95Percent, input.CPUMaxPercent)
memPct := math.Max(input.MemP95Percent, input.MemMaxPercent)
```

### 4. Spike-Aware Justification

Detect spike patterns (max >= threshold but P95 < threshold) and generate appropriate justification text:

- **Spike**: "CPU spikes to 92.3% (sustained P95: 15.2% over 30d)"
- **Sustained**: "CPU at 91.5% sustained utilization (P95 over 30d)"
- **Combined**: "CPU spikes to 95.1% and memory spikes to 92.0% (sustained P95: 45.2%/38.1% over 30d)"

Add a `Reason` field to `calculator.AnalysisResult` to carry this structured reason string.

### 5. Lookback in Justification

Include the lookback window in justification strings (e.g., "over 30d") so the report reader knows the analysis window. This requires adding `LookbackDays` to `AnalysisInput`.

## Report Changes

The PDF report should indicate the data source used:
- Add a "Data Source" field to the report metadata section
- Value: "OVRO VictoriaMetrics (90d retention)" or "OpenShift Thanos Querier"

## Files to Modify

| File | Changes |
|------|---------|
| `internal/types/types.go` | Update `DefaultLookbackDays` to 30, add `DataSource` to `ReportData`, add `NoOVRO`/`OVROEndpoint` to `AnalyzerConfig` |
| `internal/calculator/calculator.go` | Add `CPUMaxPercent`, `MemMaxPercent`, `LookbackDays` to `AnalysisInput`, add `Reason` to `AnalysisResult`, update `Analyze()` with spike detection |
| `internal/collector/vm.go` | Pass max values and lookback days to calculator |
| `internal/collector/container.go` | Pass max values and lookback days to calculator |
| `cmd/analyzer/main.go` | Add OVRO discovery logic, NetworkPolicy creation, read new env vars |
| `cmd/oc-rrrt/main.go` | Add `--no-ovro`, `--ovro-endpoint` flags, update `--lookback-days` default |
| `internal/cli/run.go` | Pass new config fields to Job env vars, extend cleanup for NetworkPolicy |
| `internal/cli/job.go` | Add new env vars to Job spec |
| `internal/cli/rbac.go` | Add NetworkPolicy and pod permissions to ClusterRole |
| `internal/cli/cleanup.go` | Add NetworkPolicy cleanup in `ovro-system` |
| `internal/pdf/` (relevant files) | Display data source in report metadata |

## New Files

| File | Purpose |
|------|---------|
| `internal/collector/discovery.go` | OVRO discovery logic (CRD check, namespace check, pod check, health probe) |

## Testing

- Unit tests for calculator spike detection (various P95/max combinations)
- Unit tests for discovery logic (mock Kubernetes client with/without OVRO resources)
- Integration: run rrrt on a cluster with OVRO installed, verify it switches to VictoriaMetrics
- Integration: run rrrt with `--no-ovro` on same cluster, verify it uses Thanos
- Integration: run rrrt on a cluster without OVRO, verify graceful fallback
