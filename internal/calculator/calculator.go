package calculator

import (
	"math"
	"sort"

	"github.com/kborup-redhat/rrrt/internal/types"
)

type AnalysisInput struct {
	CurrentCPU         int64   // millicores
	CurrentMem         int64   // bytes
	CPUP95Percent      float64
	MemP95Percent      float64
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
		Direction:      types.Downsize,
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
		Direction:      types.Upsize,
		RecommendedCPU: recCPU,
		RecommendedMem: recMem,
		CPUSavings:     -cpuIncrease,
		MemSavings:     -memIncrease,
	}
}
