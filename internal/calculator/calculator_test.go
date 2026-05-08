package calculator_test

import (
	"testing"

	"github.com/kborup-redhat/rrrt/internal/calculator"
	"github.com/kborup-redhat/rrrt/internal/types"
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
		CurrentCPU:         8000, // 8 cores in millicores
		CurrentMem:         16 * 1024 * 1024 * 1024,
		CPUP95Percent:      28.3,
		MemP95Percent:      41.7,
		CPUMaxPercent:      45.0,
		MemMaxPercent:      55.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      1000, // 1 core
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Downsize, result.Direction)
	assert.Less(t, result.RecommendedCPU, input.CurrentCPU)
	assert.Less(t, result.RecommendedMem, input.CurrentMem)
	assert.Greater(t, result.CPUSavings, int64(0))
	assert.Greater(t, result.MemSavings, int64(0))
}

func TestAnalyze_Upsize(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:         4000,
		CurrentMem:         8 * 1024 * 1024 * 1024,
		CPUP95Percent:      94.0,
		MemP95Percent:      92.0,
		CPUMaxPercent:      98.0,
		MemMaxPercent:      96.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Upsize, result.Direction)
	assert.Greater(t, result.RecommendedCPU, input.CurrentCPU)
	assert.Greater(t, result.RecommendedMem, input.CurrentMem)
}

func TestAnalyze_NoRecommendation(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:         4000,
		CurrentMem:         8 * 1024 * 1024 * 1024,
		CPUP95Percent:      75.0,
		MemP95Percent:      75.0,
		CPUMaxPercent:      80.0,
		MemMaxPercent:      80.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	assert.Nil(t, result)
}

func TestAnalyze_ContainerLowThreshold(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:         1000, // 1 core
		CurrentMem:         1 * 1024 * 1024 * 1024,
		CPUP95Percent:      20.0,
		MemP95Percent:      20.0,
		CPUMaxPercent:      35.0,
		MemMaxPercent:      30.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      250,       // 250m
		MinMemSavings:      256 << 20, // 256Mi
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Downsize, result.Direction)
}

func TestAnalyze_SpikeTriggersUpsize(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:         4000,
		CurrentMem:         8 * 1024 * 1024 * 1024,
		CPUP95Percent:      15.0, // low P95
		MemP95Percent:      20.0,
		CPUMaxPercent:      92.0, // high max
		MemMaxPercent:      25.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Upsize, result.Direction)
	assert.Contains(t, result.Reason, "spike")
}

func TestAnalyze_SustainedHighUsage(t *testing.T) {
	input := calculator.AnalysisInput{
		CurrentCPU:         4000,
		CurrentMem:         8 * 1024 * 1024 * 1024,
		CPUP95Percent:      93.0, // high P95
		MemP95Percent:      91.0,
		CPUMaxPercent:      98.0, // high max
		MemMaxPercent:      96.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
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
		CPUP95Percent:      25.0,
		MemP95Percent:      30.0,
		CPUMaxPercent:      40.0,
		MemMaxPercent:      45.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
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
		CPUP95Percent:      50.0, // low P95
		MemP95Percent:      55.0,
		CPUMaxPercent:      95.0, // high max
		MemMaxPercent:      93.0,
		LookbackDays:       30,
		HeadroomPercent:    20,
		MinCPUSavings:      1000,
		MinMemSavings:      1 << 30,
		UpsizeThresholdPct: 90,
	}

	result := calculator.Analyze(input)
	require.NotNil(t, result)
	assert.Equal(t, types.Upsize, result.Direction)
	assert.Contains(t, result.Reason, "CPU spikes")
	assert.Contains(t, result.Reason, "memory spikes")
}
