package pdf_test

import (
	"os"
	"testing"

	"github.com/kborup-redhat/rrrt/internal/pdf"
	"github.com/kborup-redhat/rrrt/internal/types"
	"github.com/stretchr/testify/require"
)

func TestGenerateReport_Empty(t *testing.T) {
	data := &types.ReportData{
		ClusterName:  "test-cluster",
		GeneratedAt:  "2026-05-07T14:30:00Z",
		Scope:        "All namespaces",
		LookbackDays: 14,
		Percentile:   95,
		HeadroomPct:  20,
		CLIVersion:   "v0.1.0",
		ImageVersion: "v0.1.0",
	}

	tmpFile := t.TempDir() + "/test-report.pdf"
	err := pdf.Generate(data, tmpFile)
	require.NoError(t, err)

	info, err := os.Stat(tmpFile)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}

func TestGenerateReport_WithData(t *testing.T) {
	data := &types.ReportData{
		ClusterName:  "prod-cluster",
		GeneratedAt:  "2026-05-07T14:30:00Z",
		Scope:        "Namespaces: default, production",
		LookbackDays: 14,
		Percentile:   95,
		HeadroomPct:  20,
		CLIVersion:   "v0.1.0",
		ImageVersion: "v0.1.0",
		VMAnalyses: []types.ResourceAnalysis{
			{
				Namespace: "default", Name: "test-vm", Kind: types.KindVM,
				Owner: "alice@example.com", ConsoleURL: "https://console.example.com/vm/test-vm",
				Direction: types.Downsize,
				CurrentCPU: 8000, CurrentMem: 16 << 30,
				RecommendedCPU: 3000, RecommendedMem: 9 << 30,
				CPUSavings: 5000, MemSavings: 7 << 30,
				CPUP95: 28.3, MemP95: 41.7, CPUMax: 62.1, MemMax: 58.4,
				CPUSamples: []float64{0.2, 0.25, 0.3, 0.28, 0.22},
				MemSamples: []float64{0.4, 0.42, 0.38, 0.45, 0.41},
				Justification: "CPU P95 utilization is 28% with 8 cores allocated.",
			},
		},
		ContainerAnalyses: []types.ResourceAnalysis{
			{
				Namespace: "production", Name: "api-server", Kind: types.KindDeployment,
				Direction: types.Downsize,
				CurrentCPU: 2000, CurrentMem: 4 << 30,
				RecommendedCPU: 500, RecommendedMem: 1 << 30,
				CPUSavings: 1500, MemSavings: 3 << 30,
				CPUP95: 15.0, MemP95: 20.0,
				CPUSamples: []float64{0.1, 0.15, 0.12},
				MemSamples: []float64{0.2, 0.18, 0.22},
				Justification: "CPU P95 utilization is 15% with 2 cores allocated.",
			},
		},
	}

	tmpFile := t.TempDir() + "/test-report-with-data.pdf"
	err := pdf.Generate(data, tmpFile)
	require.NoError(t, err)

	info, err := os.Stat(tmpFile)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(1000))
}
