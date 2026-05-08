package pdf

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderSummary(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()
	sectionHeader(p, "Executive Summary")
	p.Ln(3)

	p.SetFont("Helvetica", "", 9)
	setText(p, clrSubtext)
	p.MultiCell(180, 4.5,
		"This report identifies virtual machines and containers in your OpenShift cluster that are using "+
			"significantly more or less resources (CPU and memory) than they actually need. "+
			"Oversized workloads waste capacity and increase cost. Undersized workloads risk performance "+
			"problems and outages. Each recommendation is based on actual utilization data collected over "+
			fmt.Sprintf("%d days", data.LookbackDays)+", using the 95th percentile to account for normal usage spikes "+
			"while filtering out rare outliers.",
		"", "L", false)
	setText(p, clrDarkText)
	p.Ln(4)

	vmCandidates := countCandidates(data.VMAnalyses)
	contCandidates := countCandidates(data.ContainerAnalyses)
	totalAnalyzed := len(data.VMAnalyses) + len(data.ContainerAnalyses)
	totalCandidates := vmCandidates + contCandidates

	totalCPUSavings := sumCPUSavings(data.VMAnalyses) + sumCPUSavings(data.ContainerAnalyses)
	totalMemSavings := sumMemSavings(data.VMAnalyses) + sumMemSavings(data.ContainerAnalyses)

	cardY := p.GetY()
	cardW := 55.0
	cardH := 32.0
	gap := 7.5

	statCard(p, 15, cardY, cardW, cardH, clrBlue,
		fmt.Sprintf("%d", totalAnalyzed), "Resources Analyzed")

	statCard(p, 15+cardW+gap, cardY, cardW, cardH, clrGreen,
		fmt.Sprintf("%d", totalCandidates), "Candidates Found")

	statCard(p, 15+2*(cardW+gap), cardY, cardW, cardH, clrAmber,
		formatCPU(totalCPUSavings), "CPU Savings")

	p.SetY(cardY + cardH + 8)

	downsizeCount, upsizeCount := countDirections(data.VMAnalyses, data.ContainerAnalyses)

	p.SetFont("Helvetica", "", 10)
	setText(p, clrSubtext)
	p.CellFormat(0, 6, fmt.Sprintf("%d VMs, %d containers analyzed  -  %d downsize, %d upsize  -  %s memory savings",
		len(data.VMAnalyses), len(data.ContainerAnalyses),
		downsizeCount, upsizeCount, formatMem(totalMemSavings)), "", 1, "C", false, 0, "")
	setText(p, clrDarkText)

	rightSized := totalAnalyzed - totalCandidates
	chartData, err := renderDonutChart(rightSized, downsizeCount, upsizeCount, 450, 350)
	if err == nil && len(chartData) > 0 {
		p.Ln(8)
		opt := fpdf.ImageOptions{ImageType: "PNG"}
		p.RegisterImageOptionsReader("summary_chart", opt, bytes.NewReader(chartData))
		chartW := 120.0
		chartX := (210 - chartW) / 2
		p.ImageOptions("summary_chart", chartX, p.GetY(), chartW, 0, false, opt, 0, "")
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
	all := make([]types.ResourceAnalysis, 0, len(vms)+len(containers))
	all = append(all, vms...)
	all = append(all, containers...)
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
