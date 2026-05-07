package pdf

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderDetailCards(p *fpdf.Fpdf, analyses []types.ResourceAnalysis) {
	candidates := filterCandidates(analyses)
	for _, a := range candidates {
		p.AddPage()
		p.SetFont("Helvetica", "B", 14)
		p.CellFormat(0, 8, fmt.Sprintf("%s: %s/%s", a.Kind, a.Namespace, a.Name), "", 1, "L", false, 0, a.ConsoleURL)

		if a.Owner != "" {
			p.SetFont("Helvetica", "", 10)
			p.CellFormat(0, 6, "Owner: "+a.Owner, "", 1, "L", false, 0, "")
		}

		p.Ln(3)
		p.SetFont("Helvetica", "B", 11)
		p.CellFormat(0, 7, fmt.Sprintf("Direction: %s", a.Direction), "", 1, "L", false, 0, "")
		p.SetFont("Helvetica", "", 10)
		p.CellFormat(0, 6, fmt.Sprintf("Current: %s CPU, %s memory", formatCPU(a.CurrentCPU), formatMem(a.CurrentMem)), "", 1, "L", false, 0, "")
		p.CellFormat(0, 6, fmt.Sprintf("Recommended: %s CPU, %s memory", formatCPU(a.RecommendedCPU), formatMem(a.RecommendedMem)), "", 1, "L", false, 0, "")
		p.CellFormat(0, 6, fmt.Sprintf("Savings: %s CPU, %s memory", formatCPU(abs64(a.CPUSavings)), formatMem(abs64(a.MemSavings))), "", 1, "L", false, 0, "")

		p.Ln(5)

		if len(a.CPUSamples) > 0 {
			chartData, err := renderLineChart(a.CPUSamples, a.CPUP95, 100, "CPU Utilization (%)", 500, 200)
			if err == nil {
				opt := fpdf.ImageOptions{ImageType: "PNG"}
				imgName := fmt.Sprintf("cpu_%s_%s", a.Namespace, a.Name)
				p.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(chartData))
				p.ImageOptions(imgName, 15, p.GetY(), 180, 0, false, opt, 0, "")
				p.Ln(5)
			}
		}

		if len(a.MemSamples) > 0 {
			chartData, err := renderLineChart(a.MemSamples, a.MemP95, 100, "Memory Utilization (%)", 500, 200)
			if err == nil {
				opt := fpdf.ImageOptions{ImageType: "PNG"}
				imgName := fmt.Sprintf("mem_%s_%s", a.Namespace, a.Name)
				p.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(chartData))
				p.ImageOptions(imgName, 15, p.GetY(), 180, 0, false, opt, 0, "")
				p.Ln(5)
			}
		}

		if a.Justification != "" {
			p.Ln(3)
			p.SetFont("Helvetica", "B", 10)
			p.CellFormat(0, 6, "Justification:", "", 1, "L", false, 0, "")
			p.SetFont("Helvetica", "", 9)
			p.MultiCell(0, 5, a.Justification, "", "L", false)
		}
	}
}
