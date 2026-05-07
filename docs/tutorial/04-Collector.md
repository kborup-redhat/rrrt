---
title: "Chapter 4: Collector"
order: 4
---

# Chapter 4: Collector

## Introduction

The Collector is the coordinator that ties together the Prometheus client, calculator, and owner resolver. Think of it as a field inspector — it walks through every namespace, examines each VM and container workload, gathers their usage data, runs the analysis, and compiles the findings into a structured report. It's where the individual components come together into a pipeline.

## How It Works

The Collector is defined in `internal/collector/collector.go` with resource-specific logic split across `vm.go` and `container.go`.

### Structure and Construction

```go
type Collector struct {
    k8s          client.Client
    prom         *PrometheusClient
    owner        *owner.Resolver
    consoleURL   string
    lookbackDays int
    headroomPct  int
    includeOS    bool
}

func New(k8s client.Client, prom *PrometheusClient, ownerResolver *owner.Resolver,
    consoleURL string, lookbackDays, headroomPct int, includeOpenShift bool) *Collector
```

The Collector holds references to all its dependencies — it doesn't create them. This makes testing straightforward: you can inject mock or test implementations of each dependency.

### The Collection Pipeline

```go
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

    for i, ns := range namespaces {
        c.logProgress("scan", ns, "", i+1, total, "scanning namespace")
        vms, vmInsuf := c.collectVMs(ctx, ns)
        containers, contInsuf := c.collectContainers(ctx, ns)
        // aggregate results...
    }
    return data, nil
}
```

If no namespaces are specified, the Collector discovers all namespaces from the Kubernetes API. It then filters out system namespaces (unless `includeOpenShift` is set) and iterates namespace-by-namespace.

### Namespace Filtering

```go
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
```

By default, `openshift-*`, `kube-*`, and `rrrt-*` (the tool's own temporary namespaces) are excluded. This prevents the report from being cluttered with platform infrastructure resources that operators typically don't want to rightsize.

### VM Collection

The VM collector in `vm.go` uses the **unstructured client** because KubeVirt CRDs aren't available as Go types in the standard Kubernetes client libraries:

```go
vmList := &unstructured.UnstructuredList{}
vmList.SetGroupVersionKind(schema.GroupVersionKind{
    Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineList",
})
if err := c.k8s.List(ctx, vmList, opts...); err != nil {
    // VMs not available — skip silently
    return nil, nil
}
```

This means RRRT works on clusters both with and without KubeVirt installed. If the CRD doesn't exist, the `List` call returns an error and the collector moves on.

Resource specs are extracted using nested field access:

```go
cores, _, _ := unstructured.NestedInt64(vm.Object, "spec", "template", "spec", "domain", "cpu", "cores")
memStr, _, _ := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")
```

Each VM goes through a data sufficiency check — at least 50% of the expected data points must be present:

```go
expectedPoints := c.lookbackDays * 24 * 60 // 1 sample per minute
minPoints := expectedPoints / 2
if len(cpuVals) < minPoints {
    insufficient = append(insufficient, types.InsufficientDataEntry{...})
    continue
}
```

Resources with insufficient data are tracked separately and reported in a dedicated section of the PDF rather than silently dropped.

### Container Collection

The container collector in `container.go` uses **typed clients** for Deployments and StatefulSets since these are standard Kubernetes types:

```go
var deployments appsv1.DeploymentList
if err := c.k8s.List(ctx, &deployments, client.InNamespace(namespace)); err != nil {
    return nil, nil
}
```

Container resource requests are summed across all containers in the pod template:

```go
for _, ctr := range d.Spec.Template.Spec.Containers {
    w.cpuMillis += ctr.Resources.Requests.Cpu().MilliValue()
    w.memBytes += ctr.Resources.Requests.Memory().Value()
}
```

Workloads with zero CPU **or** zero memory requests are skipped — there's no basis for rightsizing if the resource has no requests set.

### Justification Generation

After the calculator returns a result, the VM collector generates a human-readable justification:

```go
func buildJustification(a types.ResourceAnalysis) string {
    if a.Direction == types.Downsize {
        return fmt.Sprintf(
            "CPU P95 utilization is %.0f%% with %.1f cores allocated. "+
            "Reducing to %.1f cores provides adequate headroom...",
            a.CPUP95, cpuCores, recCPUCores, ...)
    }
    return fmt.Sprintf("CPU P95 utilization is %.0f%% ... indicating resource pressure...")
}
```

Each justification cites the specific P95 numbers and concrete savings amounts, giving the reader enough data to evaluate the recommendation without digging into the raw charts.

### Progress Logging

The Collector emits structured JSON progress messages to stdout:

```go
func (c *Collector) logProgress(phase, namespace, resource string, index, total int, status string) {
    msg := map[string]interface{}{
        "phase": phase, "namespace": namespace,
        "resource": resource, "index": index, "total": total, "status": status,
    }
    data, _ := json.Marshal(msg)
    _, _ = fmt.Fprintln(os.Stdout, string(data))
}
```

These JSON lines are parsed by the CLI's log streamer (Chapter 7) and rendered as human-friendly progress messages on the user's terminal.

## Relationships

- Uses the **Prometheus Client** to fetch metric time-series for each resource
- Uses the **Calculator** to compute percentiles and determine rightsizing direction
- Uses the **Owner Resolver** to attribute ownership from resource/namespace labels
- Produces `ReportData` consumed by the **PDF Generator**
- Progress logs are streamed and parsed by the **CLI Orchestrator**

## Key Takeaways

- The Collector uses unstructured clients for KubeVirt VMs (no dependency on KubeVirt Go types) and typed clients for Deployments/StatefulSets
- Resources with `rightsizing.redhatconsulting.io/exclude: "true"` are skipped
- Data sufficiency is enforced — resources with less than 50% metric coverage go to the "insufficient data" section
- Structured JSON progress logging enables the CLI to show user-friendly status updates
- System namespaces (`openshift-*`, `kube-*`, `rrrt-*`) are filtered by default

Next, we'll look at the Owner Resolver — how RRRT figures out who is responsible for each resource.
