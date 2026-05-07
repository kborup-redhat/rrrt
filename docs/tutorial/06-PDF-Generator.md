---
title: "Chapter 6: PDF Generator"
order: 6
---

# Chapter 6: PDF Generator

## Introduction

The PDF Generator transforms analysis data into a polished, multi-section report that can be shared with stakeholders, attached to tickets, or archived. Think of it as a document compositor — it takes the raw data from the Collector and arranges it into pages with charts, tables, and formatted text. The output is a self-contained PDF that doesn't require access to the cluster to interpret.

## How It Works

The generator is split across several files in `internal/pdf/`, each responsible for one section of the report.

### Entry Point

```go
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
```

The report follows a logical structure: cover page, executive summary, per-type sections with tables and detail cards, an insufficient data section, and an appendix with analysis settings.

### Cover Page (`cover.go`)

The cover page is straightforward — it displays the report title, cluster name, generation timestamp, scope, and lookback period centered on the page.

### Executive Summary (`summary.go`)

The summary aggregates high-level metrics:

```go
vmCandidates := countCandidates(data.VMAnalyses)
contCandidates := countCandidates(data.ContainerAnalyses)
totalCPUSavings := sumCPUSavings(data.VMAnalyses) + sumCPUSavings(data.ContainerAnalyses)
totalMemSavings := sumMemSavings(data.VMAnalyses) + sumMemSavings(data.ContainerAnalyses)
```

It includes a **donut chart** showing the distribution of right-sized, oversized, and undersized resources:

```go
chartData, err := renderDonutChart(rightSized, downsizeCount, upsizeCount, 400, 300)
if err == nil && len(chartData) > 0 {
    opt := fpdf.ImageOptions{ImageType: "PNG"}
    p.RegisterImageOptionsReader("summary_chart", opt, bytes.NewReader(chartData))
    p.ImageOptions("summary_chart", 40, p.GetY(), 130, 0, false, opt, 0, "")
}
```

Charts are rendered to in-memory PNG buffers using `go-chart` and then embedded into the PDF as images. The chart library produces the PNG bytes, and `fpdf` embeds them without touching the filesystem.

### Resource Tables (`tables.go`)

Each resource type gets a summary table with columns for namespace, name, owner, direction, current/recommended CPU, and current/recommended memory:

```go
headers := []string{"Namespace", "Name", "Owner", "Direction",
    "Current CPU", "Rec. CPU", "Current Mem", "Rec. Mem"}
widths := []float64{25, 30, 30, 18, 20, 20, 22, 22}
```

Resource names are rendered as clickable links to the OpenShift Console:

```go
p.CellFormat(widths[1], 5, a.Name, "1", 0, "L", false, 0, a.ConsoleURL)
```

### Detail Cards (`details.go`)

Each rightsizing candidate gets its own page with:
- Resource identification (kind, namespace, name) linked to the Console
- Owner attribution
- Direction, current vs. recommended allocation, and savings
- **CPU utilization chart** — a time-series line chart with the P95 line overlaid
- **Memory utilization chart** — same format
- Human-readable justification text

The charts use `renderLineChart` from `charts.go`:

```go
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
        StrokeDashArray: []float64{5, 3},
    },
},
```

The chart shows three lines: the utilization time-series (blue fill), the P95 threshold (red dashed), and the current allocation at 100% (grey dashed). This makes it visually obvious whether a resource is over- or under-utilized.

### Insufficient Data Section (`insufficient.go`)

Resources that had less than 50% metric coverage get their own table:

```go
headers := []string{"Namespace", "Name", "Type", "Data Points", "Expected"}
```

This transparency is important — it tells the reader which resources couldn't be analyzed and why, rather than silently omitting them.

### Appendix (`appendix.go`)

The appendix documents the exact parameters used for the analysis (percentile, headroom, lookback, thresholds) and version information. This makes the report reproducible — someone can look at the appendix and know exactly how the recommendations were generated.

### Formatting Helpers

The `report.go` file includes formatting functions used throughout:

```go
func formatCPU(millis int64) string {
    if millis >= 1000 {
        return fmt.Sprintf("%.1f cores", float64(millis)/1000)
    }
    return fmt.Sprintf("%dm", millis)
}

func formatMem(bytes int64) string {
    if bytes >= gi {
        return fmt.Sprintf("%.1f GiB", float64(bytes)/float64(gi))
    }
    return fmt.Sprintf("%d MiB", bytes/mi)
}
```

These produce human-readable strings: `8 cores` instead of `8000m`, `16 GiB` instead of `17179869184 bytes`.

## Relationships

- Reads `ReportData` produced by the **Collector**
- Uses `go-chart/v2` for line and donut charts (rendered to PNG)
- Uses `go-pdf/fpdf` for PDF page layout
- Links to the OpenShift Console URL constructed by the **Collector**

## Key Takeaways

- The report is built section-by-section: cover, summary, VM table + details, container table + details, insufficient data, appendix
- Charts are rendered to in-memory PNG buffers and embedded — no temporary files
- Resource names are clickable links to the OpenShift Console
- The insufficient data section provides transparency about what couldn't be analyzed
- The appendix makes the report reproducible by documenting all analysis parameters

Next, we'll look at the CLI orchestrator — the component that ties together namespace creation, RBAC, Job dispatch, and cleanup.
