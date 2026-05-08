package calculator

import (
	"fmt"
	"math"
	"sort"

	"github.com/kborup-redhat/rrrt/internal/types"
)

type AnalysisInput struct {
	CurrentCPU         int64   // millicores
	CurrentMem         int64   // bytes
	CPUP95Percent      float64
	MemP95Percent      float64
	CPUMaxPercent      float64
	MemMaxPercent      float64
	LookbackDays       int
	HeadroomPercent    int
	MinCPUSavings      int64 // millicores
	MinMemSavings      int64 // bytes
	UpsizeThresholdPct int
}

type AnalysisResult struct {
	Direction      types.Direction
	RecommendedCPU int64
	RecommendedMem int64
	CPUSavings     int64
	MemSavings     int64
	Reason         string
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
	threshold := float64(input.UpsizeThresholdPct)
	if input.CPUP95Percent >= threshold || input.MemP95Percent >= threshold ||
		input.CPUMaxPercent >= threshold || input.MemMaxPercent >= threshold {
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

	reason := fmt.Sprintf("CPU only %.1f%% utilized (P95 over %dd), can save %d cores",
		input.CPUP95Percent, input.LookbackDays, cpuSavings/1000)

	return &AnalysisResult{
		Direction:      types.Downsize,
		RecommendedCPU: recCPU,
		RecommendedMem: recMem,
		CPUSavings:     cpuSavings,
		MemSavings:     memSavings,
		Reason:         reason,
	}
}

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
