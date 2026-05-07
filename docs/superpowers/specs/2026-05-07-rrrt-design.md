# RRRT — Resource Rightsizing Reporting Tool

## Overview

RRRT is a standalone `oc`/`kubectl` CLI plugin that generates professional PDF rightsizing reports for both KubeVirt VirtualMachines and container workloads (Deployments, StatefulSets) on OpenShift clusters. It dispatches an analysis Job on-cluster with direct access to Prometheus, collects utilization metrics, runs a percentile-based rightsizing calculator, resolves resource owners, and produces a PDF with per-resource charts, justification text, and clickable OpenShift Console links.

**Repository**: `github.com/kborup-redhat/rrrt`
**Project directory**: `/home/kborup/ai-code/rrrt`
**Language**: Go
**Platform**: OpenShift (works on vanilla Kubernetes for container analysis if KubeVirt is absent)

## Architecture

The system has two components:

### 1. `oc-rrrt` CLI Binary

A thin orchestrator that runs on the user's machine. It:

- Parses CLI flags and validates user permissions
- Creates an ephemeral namespace (`rrrt-<8-char-random>`)
- Sets up a ServiceAccount with ClusterRole/ClusterRoleBinding for resource listing and Prometheus access
- Creates a Kubernetes Job running the analysis container image
- Streams Job Pod logs to the terminal in real-time, showing progress
- When the Job completes, copies the PDF from the Pod via the Kubernetes tar stream API
- Deletes ClusterRoleBindings and the ephemeral namespace (full cleanup)
- On SIGINT/SIGTERM or error, cleanup still runs (unless `--keep-namespace` is set)

The CLI contains no Prometheus, PDF, or chart dependencies — only `client-go` and `cobra`.

### 2. `rrrt-analyzer` Container Image

Runs on-cluster as a Job. It:

- Queries Prometheus/Thanos directly via internal cluster URL
- Lists all VirtualMachines, Deployments, and StatefulSets in scope
- For each resource: collects CPU/memory metrics over the lookback window, runs the rightsizing calculator, resolves owner labels
- Generates a professional PDF with utilization charts and writes it to `/output/report.pdf`
- Exits 0 on success

**Base image**: `registry.access.redhat.com/ubi9/ubi-minimal`

### Shared Code

The CLI and analysis container are separate binaries in separate `cmd/` directories. They share some Go packages within the repo:

- `internal/calculator` — percentile + headroom rightsizing logic (ported from OVRO)
- `internal/owner` — owner label resolution
- `internal/types` — shared data structures

## CLI Interface

```
oc rrrt report [flags]

Flags:
  --namespace, -n       Analyze a specific namespace (can be repeated)
  --all-namespaces      Analyze all accessible namespaces (default behavior)
  --output, -o          Output PDF path (default: ./rrrt-report-<timestamp>.pdf)
  --lookback-days       Metrics lookback window in days (default: 14)
  --image               Override the analysis container image
  --console-url         OpenShift Console base URL for resource links (auto-detected)
  --include-openshift   Include openshift-* and kube-* namespaces (excluded by default)
  --keep-namespace      Don't delete the temporary namespace after completion
  --timeout             Maximum time to wait for the Job (default: 30m)
```

### Namespace Exclusions

By default, the following namespaces are excluded:

- `openshift-*` (all OpenShift platform namespaces)
- `kube-*` (`kube-system`, `kube-public`, `kube-node-lease`)
- The ephemeral `rrrt-*` namespace itself

When `--namespace` is explicitly provided, no exclusions apply — the user asked for that namespace specifically.

The `--include-openshift` flag disables the default exclusions for cluster-wide scans.

### Console URL Auto-Detection

The CLI reads the `console-config` ConfigMap in the `openshift-config` namespace to extract the Console base URL. This is used by the analysis container to generate clickable resource links in the PDF. If auto-detection fails, the user can pass `--console-url` explicitly.

## Data Collection

### Virtual Machines (KubeVirt)

- Lists all `VirtualMachine` resources in scope via the dynamic Kubernetes client
- For each VM, queries Prometheus for:
  - CPU: `rate(kubevirt_vmi_cpu_usage_seconds_total{name="<vm>",namespace="<ns>"}[5m])[<lookback>d:1m]`
  - Memory: `kubevirt_vmi_memory_resident_bytes{name="<vm>",namespace="<ns>"}[<lookback>d]`
- Extracts current CPU cores and memory from the VM spec (`spec.template.spec.domain.cpu.cores` and `spec.template.spec.domain.resources.requests.memory`)
- Runs the percentile + headroom calculator
- Resolves owner from VM label `rightsizing.redhatconsulting.io/owner`, falling back to namespace label with the same key

### Containers (Deployments / StatefulSets)

- Lists Deployments and StatefulSets in scope via typed Kubernetes client
- For each, queries Prometheus for container metrics:
  - CPU: `rate(container_cpu_usage_seconds_total{namespace="<ns>",pod=~"<name>-.*",container!=""}[5m])[<lookback>d:1m]`
  - Memory: `container_memory_working_set_bytes{namespace="<ns>",pod=~"<name>-.*",container!=""}[<lookback>d]`
  - Aggregated per container name within the workload
- Extracts current CPU and memory requests from the pod spec (requests are what the scheduler uses for placement)
- Runs the calculator against requests
- Resolves owner from the same `rightsizing.redhatconsulting.io/owner` label on the Deployment/StatefulSet, falling back to namespace label

### Exclusions

Resources with the `rightsizing.redhatconsulting.io/exclude: "true"` annotation are skipped, consistent with OVRO operator behavior.

### Insufficient Data

Resources with less than 50% metric coverage of the lookback window are not analyzed but are listed in a separate "Insufficient Data" section in the report, showing how many data points were found vs. expected.

## Calculator Logic

The calculator is ported from OVRO's `internal/calculator` package. It uses the same algorithm:

- **P95 percentile** computed via linear interpolation over the collected samples
- **Headroom**: recommended allocation = P95 usage * (1 + headroom%)
- **Downsize**: triggered when current allocation minus recommended exceeds minimum savings threshold
- **Upsize**: triggered when P95 utilization exceeds the upsize threshold (default 90%). Recommended allocation = P95 / 0.70 (targeting 70% utilization)
- Returns no recommendation if neither condition is met

Default policy settings:

**Virtual Machines** (matching OVRO defaults):
- Lookback: 14 days
- Percentile: P95
- Headroom: 20%
- Min CPU savings: 1 core
- Min memory savings: 1 GiB
- Upsize utilization threshold: 90%

**Containers** (lower thresholds — requests/limits are frequently misconfigured):
- Lookback: 14 days
- Percentile: P95
- Headroom: 20%
- Min CPU savings: 250m (0.25 cores)
- Min memory savings: 256Mi
- Upsize utilization threshold: 90%

## PDF Report Structure

### Cover Page

- Title: "Resource Rightsizing Report"
- Cluster name (from `clusterversion` API)
- Generation timestamp
- Scope summary (e.g., "All namespaces" or "Namespaces: default, production")
- Lookback period (e.g., "14 days")

### Executive Summary

- Total resources analyzed (VMs and containers counted separately)
- Total rightsizing candidates (downsize / upsize breakdown)
- Estimated total CPU and memory savings
- Bar or donut chart showing distribution: right-sized vs. oversized vs. undersized

### Virtual Machines Section

**Summary table** with one row per VM:
- Namespace, VM name (clickable Console link), owner, direction (downsize/upsize), current CPU/memory, recommended CPU/memory, savings, P95 utilization

**Detail cards** for each candidate VM:
- CPU utilization line chart over the lookback period, with P95 line and current allocation line drawn as reference
- Memory utilization line chart (same format)
- Justification text, e.g.: "CPU P95 utilization is 25% with 4 cores allocated. Reducing to 2 cores provides 50% headroom above P95 and saves 2 cores."

### Containers Section

Same structure as VMs:
- Summary table with clickable Console links to the Deployment/StatefulSet page
- Detail cards with charts and justification per candidate

### Insufficient Data Section

- Table of resources that had less than 50% metric coverage
- Columns: namespace, name, type (VM/Deployment/StatefulSet), data points found, data points expected

### Appendix

- Policy settings used for analysis (percentile, headroom %, thresholds)
- CLI version and analysis container image version

### Footer (last page only)

The final page of the report includes:

> Built with RRRT open source project — github.com/kborup-redhat/rrrt
> Feedback & issues: github.com/kborup-redhat/rrrt/issues

## PDF Technical Implementation

- **PDF layout and text**: `github.com/go-pdf/fpdf` (maintained fork of gofpdf)
- **Charts**: `github.com/wcharczuk/go-chart/v2` renders CPU/memory utilization line charts and executive summary charts as PNG images, embedded into the PDF
- Charts use a consistent color scheme: current allocation in grey, P95 line in red/amber, utilization area in blue

## RBAC & Security

### Ephemeral Resources (created per run, cleaned up after)

**Namespace**: `rrrt-<8-char-random>` — contains the ServiceAccount and Job

**ServiceAccount**: `rrrt-analyzer` — mounted into the Job Pod

**ClusterRoleBinding**: `rrrt-analyzer-<namespace>` — binds the ServiceAccount to the `rrrt-analyzer` ClusterRole

**ClusterRoleBinding**: `rrrt-monitoring-<namespace>` — binds the ServiceAccount to the existing `cluster-monitoring-view` ClusterRole for Prometheus access

### Persistent Resources (created idempotently, not cleaned up)

**ClusterRole**: `rrrt-analyzer` with permissions:
- `virtualmachines.kubevirt.io`: get, list
- `virtualmachineinstances.kubevirt.io`: get, list
- `deployments.apps`: get, list
- `statefulsets.apps`: get, list
- `namespaces`: get, list
- `configmaps` in `openshift-config`: get (for console URL)

### User Permissions

The user running `oc rrrt` must have permissions to:
- Create and delete namespaces
- Create and delete ClusterRoleBindings
- Create ClusterRoles
- Create Jobs and ServiceAccounts

Typically this means `cluster-admin`. The CLI checks upfront and fails fast with a clear error message if permissions are missing.

### Prometheus Authentication

The Job Pod authenticates to Prometheus using the ServiceAccount token mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`. On OpenShift, Thanos requires a bearer token with `cluster-monitoring-view` permissions, which the second ClusterRoleBinding provides.

## Lifecycle & Error Handling

### Happy Path

1. CLI validates user permissions (fail fast if insufficient)
2. Creates namespace `rrrt-<random>`
3. Creates ServiceAccount, ClusterRole (idempotent), ClusterRoleBindings
4. Creates Job with the analysis container, passing config via environment variables: `RRRT_NAMESPACES` (comma-separated list or empty for all), `RRRT_LOOKBACK_DAYS`, `RRRT_CONSOLE_URL`, `RRRT_INCLUDE_OPENSHIFT` (true/false), `RRRT_PROMETHEUS_URL` (defaults to `https://thanos-querier.openshift-monitoring.svc:9091`)
5. Streams Pod logs to terminal — user sees progress: `[3/12] Analyzing VM prod/database...`
6. Job exits 0
7. CLI copies `/output/report.pdf` from the Pod
8. Saves to `--output` path (default: `./rrrt-report-<timestamp>.pdf`)
9. Deletes ClusterRoleBindings, then deletes namespace
10. Prints: `Report saved to ./rrrt-report-2026-05-07T1430.pdf`

### Progress Reporting

The analysis container writes structured log lines to stdout:

```json
{"phase":"vms","namespace":"prod","resource":"database","index":3,"total":12,"status":"analyzing"}
```

The CLI parses these and renders a clean progress line:

```
[3/12] Analyzing VM prod/database...
```

Final summary before PDF generation:

```
Found 8 rightsizing candidates out of 12 resources. Generating PDF...
```

### Cleanup on Failure / Interrupt

- CLI registers a signal handler for SIGINT and SIGTERM
- On interrupt or any error after namespace creation, cleanup runs: delete ClusterRoleBindings, delete namespace
- `--keep-namespace` skips cleanup for debugging
- If cleanup itself fails, CLI prints the namespace name and manual cleanup commands:
  ```
  Warning: cleanup failed. Run manually:
    oc delete clusterrolebinding rrrt-analyzer-rrrt-a1b2c3d4 rrrt-monitoring-rrrt-a1b2c3d4
    oc delete namespace rrrt-a1b2c3d4
  ```

### Job Timeout

- Default 30 minutes via `--timeout`
- The Job spec sets `activeDeadlineSeconds` to match, so the Pod is killed server-side
- If the timeout is reached, CLI prints an error, runs cleanup, and exits non-zero

## Project Structure

```
rrrt/
├── .github/
│   └── workflows/
│       ├── ci.yaml           # Lint, test, build on push to main
│       ├── release.yaml      # Build binaries + container image on tag, create GitHub Release
│       └── docs.yaml         # Build tutorial docs and deploy to GitHub Pages
├── cmd/
│   ├── oc-rrrt/              # CLI binary entry point
│   │   └── main.go
│   └── analyzer/             # Analysis container entry point
│       └── main.go
├── internal/
│   ├── cli/                  # CLI orchestration (namespace, Job, cleanup)
│   ├── collector/            # Prometheus metric collection (VM + container queries)
│   ├── calculator/           # Rightsizing analysis (ported from OVRO)
│   ├── owner/                # Owner label resolution
│   ├── pdf/                  # PDF generation (layout, charts, tables)
│   └── types/                # Shared data structures
├── docs/
│   └── tutorial/             # HonKit tutorial site (generated via /tutorial build)
├── Containerfile             # Multi-stage: builds both binaries, final image for analyzer
├── go.mod
├── go.sum
└── README.md
```

## Build & Distribution

- **CLI binary**: `go build -o oc-rrrt ./cmd/oc-rrrt/` — user places in PATH
- **Container image**: Built via Containerfile, pushed to `quay.io/kborup/rrrt`. The CLI's `--image` flag defaults to `quay.io/kborup/rrrt:<version>` where `<version>` matches the CLI binary version. Users override this if they host the image elsewhere.
- **GitHub Actions CI/CD**:
  - On push to main: lint, test, build binaries and container image
  - On tag (e.g., `v1.0.0`): full release pipeline — build multi-arch CLI binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64), build and push container image to `quay.io/kborup/rrrt:<tag>`, create GitHub Release with binaries attached
  - On push to main or tag: build tutorial docs via HonKit from `docs/tutorial/` and deploy to GitHub Pages
- **Tutorial docs**: Generated via `/tutorial build` with output in `docs/tutorial/`. Published as a GitHub Pages site via GitHub Actions. The `docs/tutorial/` directory contains HonKit-compatible markdown (SUMMARY.md, book.json, chapter files).
- **Container registry**: `quay.io/kborup/rrrt` (created by the user)
- **GitHub repository**: `github.com/kborup-redhat/rrrt` (created by the user)
