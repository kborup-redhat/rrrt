---
title: "Chapter 1: Types and Constants"
order: 1
---

# Chapter 1: Types and Constants

## Introduction

Every application needs a common vocabulary — a set of data structures that all components agree on. In RRRT, the `types` package serves as this shared language. Think of it as the dictionary that the collector, calculator, PDF generator, and CLI all reference when passing data around.

## How It Works

The `types` package defines three categories of things: **constants** (tuning defaults and label keys), **enumerations** (directions and resource kinds), and **data structures** (the analysis results and report data that flow through the pipeline).

### Constants and Defaults

```go
const (
    LabelOwner        = "rightsizing.redhatconsulting.io/owner"
    AnnotationExclude = "rightsizing.redhatconsulting.io/exclude"

    DefaultPrometheusURL   = "https://thanos-querier.openshift-monitoring.svc:9091"
    DefaultLookbackDays    = 14
    DefaultPercentile      = 95
    DefaultHeadroom        = 20
    DefaultUpsizeThreshold = 90
)
```

`LabelOwner` is a well-known Kubernetes label that RRRT checks on resources and namespaces to attribute ownership. `AnnotationExclude` allows operators to opt specific resources out of analysis by setting the annotation to `"true"`.

The numeric defaults define the analysis parameters:
- **14-day lookback** captures two weeks of utilization data
- **P95 percentile** filters out brief spikes that don't represent sustained usage
- **20% headroom** adds a safety buffer above the P95 value when recommending
- **90% upsize threshold** triggers an upsize recommendation when utilization consistently exceeds this

Minimum savings thresholds prevent noisy recommendations for trivial differences:

```go
DefaultVMMinCPUSavings = 1000       // 1 core in millicores
DefaultVMMinMemSavings = 1 << 30    // 1 GiB
DefaultContainerMinCPUSavings = 250 // 250m
DefaultContainerMinMemSavings = 256 << 20 // 256 MiB
```

VMs have higher thresholds because they typically run with larger allocations — recommending a 100m CPU reduction on a 16-core VM would be noise.

### Enumerations

```go
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
```

`Direction` tells you whether a resource should shrink or grow. `ResourceKind` classifies the three resource types RRRT analyzes. Both use typed string constants rather than raw strings, which catches typos at compile time.

### Core Data Structures

The main output of the analysis pipeline is `ResourceAnalysis`:

```go
type ResourceAnalysis struct {
    Namespace      string
    Name           string
    Kind           ResourceKind
    Owner          string
    ConsoleURL     string
    Direction      Direction
    CurrentCPU     int64 // millicores
    CurrentMem     int64 // bytes
    RecommendedCPU int64
    RecommendedMem int64
    CPUSavings     int64
    MemSavings     int64
    CPUP95         float64
    MemP95         float64
    CPUMax         float64
    MemMax         float64
    CPUSamples     []float64
    MemSamples     []float64
    Justification  string
}
```

CPU values are stored in **millicores** (1 core = 1000 millicores) and memory in **bytes**. Using integers avoids floating-point precision issues when comparing resource quantities — this matches how Kubernetes itself represents resource values.

`CPUSamples` and `MemSamples` carry the raw time-series data for rendering charts in the PDF. `Justification` is a human-readable explanation of why the recommendation was made.

`ReportData` aggregates everything the PDF generator needs:

```go
type ReportData struct {
    ClusterName       string
    GeneratedAt       string
    Scope             string
    LookbackDays      int
    Percentile        int
    HeadroomPct       int
    VMAnalyses        []ResourceAnalysis
    ContainerAnalyses []ResourceAnalysis
    InsufficientData  []InsufficientDataEntry
    CLIVersion        string
    ImageVersion      string
}
```

And `InsufficientDataEntry` tracks resources that didn't have enough metric data for reliable analysis:

```go
type InsufficientDataEntry struct {
    Namespace      string
    Name           string
    Kind           ResourceKind
    DataPoints     int
    ExpectedPoints int
}
```

## Relationships

- The **Collector** populates `ResourceAnalysis` and `InsufficientDataEntry` structs
- The **Calculator** uses the threshold constants and returns results that map to `Direction` and savings fields
- The **PDF Generator** reads `ReportData` to render every page of the report
- The **Owner Resolver** writes to the `Owner` field on each `ResourceAnalysis`

## Key Takeaways

- All CPU values use millicores (int64) and memory uses bytes (int64) — no floating-point for resource quantities
- Typed string constants (`Direction`, `ResourceKind`) catch errors at compile time
- Threshold defaults are calibrated differently for VMs (coarser) vs. containers (finer)
- The `types` package has zero dependencies on other internal packages, keeping it at the bottom of the import graph

Next, we'll look at how the Prometheus client connects to the cluster's monitoring stack to collect the raw metric data.
