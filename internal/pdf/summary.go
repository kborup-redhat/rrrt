package pdf

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderSummary(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()
	p.SetFont("Helvetica", "B", 18)
	p.CellFormat(0, 10, "Executive Summary", "", 1, "L", false, 0, "")
	p.Ln(5)

	vmCandidates := countCandidates(data.VMAnalyses)
	contCandidates := countCandidates(data.ContainerAnalyses)
	totalAnalyzed := len(data.VMAnalyses) + len(data.ContainerAnalyses)

	totalCPUSavings := sumCPUSavings(data.VMAnalyses) + sumCPUSavings(data.ContainerAnalyses)
	totalMemSavings := sumMemSavings(data.VMAnalyses) + sumMemSavings(data.ContainerAnalyses)

	p.SetFont("Helvetica", "", 11)
	p.CellFormat(0, 7, fmt.Sprintf("Total resources analyzed: %d (%d VMs, %d containers)",
		totalAnalyzed, len(data.VMAnalyses), len(data.ContainerAnalyses)), "", 1, "L", false, 0, "")

	downsizeCount, upsizeCount := countDirections(data.VMAnalyses, data.ContainerAnalyses)
	p.CellFormat(0, 7, fmt.Sprintf("Rightsizing candidates: %d (%d downsize, %d upsize)",
		vmCandidates+contCandidates, downsizeCount, upsizeCount), "", 1, "L", false, 0, "")
	p.CellFormat(0, 7, fmt.Sprintf("Estimated CPU savings: %s", formatCPU(totalCPUSavings)), "", 1, "L", false, 0, "")
	p.CellFormat(0, 7, fmt.Sprintf("Estimated memory savings: %s", formatMem(totalMemSavings)), "", 1, "L", false, 0, "")

	rightSized := totalAnalyzed - (vmCandidates + contCandidates)
	chartData, err := renderDonutChart(rightSized, downsizeCount, upsizeCount, 400, 300)
	if err == nil && len(chartData) > 0 {
		p.Ln(10)
		opt := fpdf.ImageOptions{ImageType: "PNG"}
		p.RegisterImageOptionsReader("summary_chart", opt, bytes.NewReader(chartData))
		p.ImageOptions("summary_chart", 40, p.GetY(), 130, 0, false, opt, 0, "")
	}
}

func countCandidates(analyses []types.ResourceAnalysis) int {
	count := 0
	for _, a := range analyses {
		if a.Direction != "" {
			count++
		}
	}
	return count
}

func countDirections(vms, containers []types.ResourceAnalysis) (downsize, upsize int) {
	all := append(vms, containers...)
	for _, a := range all {
		switch a.Direction {
		case types.Downsize:
			downsize++
		case types.Upsize:
			upsize++
		}
	}
	return
}

func sumCPUSavings(analyses []types.ResourceAnalysis) int64 {
	var total int64
	for _, a := range analyses {
		if a.CPUSavings > 0 {
			total += a.CPUSavings
		}
	}
	return total
}

func sumMemSavings(analyses []types.ResourceAnalysis) int64 {
	var total int64
	for _, a := range analyses {
		if a.MemSavings > 0 {
			total += a.MemSavings
		}
	}
	return total
}
