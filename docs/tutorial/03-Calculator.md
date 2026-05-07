---
title: "Chapter 3: Calculator"
order: 3
---

# Chapter 3: Calculator

## Introduction

The Calculator is the brain of RRRT. While the Prometheus client fetches raw utilization data, the calculator answers the question: *"Given this usage pattern, should we change the resource allocation, and by how much?"* Think of it as an accountant reviewing spending reports — it looks at what was actually used, adds a safety margin, and recommends a budget that fits.

## How It Works

The calculator has two public functions: `ComputePercentile` for statistical analysis and `Analyze` for the rightsizing decision.

### Percentile Computation

```go
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
```

This uses **linear interpolation** between the two nearest data points rather than simply rounding to the nearest rank. For example, if P95 falls between the 190th and 191st values in a 200-sample dataset, it interpolates between them for a more accurate result.

The function makes a copy of the input slice before sorting to avoid mutating the caller's data — those same samples are used later for rendering charts in the PDF.

### The Analysis Decision

```go
func Analyze(input AnalysisInput) *AnalysisResult {
    if input.CPUP95Percent >= float64(input.UpsizeThresholdPct) ||
        input.MemP95Percent >= float64(input.UpsizeThresholdPct) {
        return analyzeUpsize(input)
    }
    return analyzeDownsize(input)
}
```

The decision tree is simple: if **either** CPU or memory P95 utilization exceeds the upsize threshold (default 90%), the resource needs more capacity. Otherwise, evaluate for downsizing.

Returning `*AnalysisResult` (a pointer) allows the function to return `nil` when neither direction produces a meaningful recommendation — when the savings would be below the minimum thresholds.

### Downsizing

```go
func analyzeDownsize(input AnalysisInput) *AnalysisResult {
    headroom := 1.0 + float64(input.HeadroomPercent)/100.0

    cpuUsage := float64(input.CurrentCPU) * input.CPUP95Percent / 100.0
    recCPU := int64(math.Ceil(cpuUsage * headroom))

    memUsage := float64(input.CurrentMem) * input.MemP95Percent / 100.0
    recMem := int64(math.Ceil(memUsage * headroom))
    // ...
}
```

The formula: take the P95 utilization (as an absolute value), multiply by the headroom factor (1.20 with 20% headroom), and round up. This gives a recommended allocation that covers 95% of observed usage with a 20% buffer.

Safety floors prevent the recommendation from going absurdly low:

```go
if recCPU < 100 {
    recCPU = 100 // minimum 100m
}
if recMem < 64<<20 {
    recMem = 64 << 20 // minimum 64Mi
}
```

And finally, the minimum savings check filters out trivial recommendations:

```go
if cpuSavings < input.MinCPUSavings && memSavings < input.MinMemSavings {
    return nil
}
```

Both CPU **and** memory savings must be below the thresholds for the recommendation to be suppressed. If either dimension has meaningful savings, the recommendation stands.

### Upsizing

```go
func analyzeUpsize(input AnalysisInput) *AnalysisResult {
    cpuUsage := float64(input.CurrentCPU) * input.CPUP95Percent / 100.0
    memUsage := float64(input.CurrentMem) * input.MemP95Percent / 100.0

    recCPU := int64(math.Ceil(cpuUsage / 0.70))
    recMem := int64(math.Ceil(memUsage / 0.70))
    // ...
}
```

The upsize calculation targets **70% utilization** — it divides the actual usage by 0.70 to determine the allocation that would place usage at 70%. This gives the resource meaningful headroom while acknowledging it's under pressure.

Upsize results use **negative savings** to indicate cost increases:

```go
return &AnalysisResult{
    Direction:      types.Upsize,
    CPUSavings:     -cpuIncrease,
    MemSavings:     -memIncrease,
    // ...
}
```

This convention allows the PDF summary to simply sum all savings across resources — negative values from upsizes naturally offset positive values from downsizes, giving the net savings.

### Testing the Calculator

The test suite covers the key scenarios:

```go
func TestAnalyze_Downsize(t *testing.T) {
    input := calculator.AnalysisInput{
        CurrentCPU:    8000,  // 8 cores
        CurrentMem:    16 * 1024 * 1024 * 1024,
        CPUP95Percent: 28.3,  // only using 28% of 8 cores
        MemP95Percent: 41.7,
        // ...
    }
    result := calculator.Analyze(input)
    require.NotNil(t, result)
    assert.Equal(t, types.Downsize, result.Direction)
    assert.Less(t, result.RecommendedCPU, input.CurrentCPU)
}
```

The tests verify directional correctness (downsize means recommended < current, upsize means recommended > current) and the nil case when savings are too small to matter.

## Relationships

- The **Collector** calls `ComputePercentile` to compute P95 values from raw samples, then calls `Analyze` with the results
- The **Types** package provides `Direction` constants and threshold defaults used by the analysis logic
- `AnalysisResult` fields map directly to `ResourceAnalysis` fields that the **PDF Generator** renders

## Key Takeaways

- P95 percentile uses linear interpolation for accuracy, and copies the input to avoid mutating chart data
- Downsizing: P95 usage x headroom factor, with safety floors (100m CPU, 64Mi memory)
- Upsizing: targets 70% utilization — triggered when P95 exceeds 90% on either CPU or memory
- Returns `nil` when savings are below minimum thresholds (both dimensions must be below for suppression)
- Negative savings values for upsizes allow simple summation for net savings in the report

Next, we'll look at how the Collector orchestrates the Prometheus client and calculator to process entire namespaces of resources.
