package pdf

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

const (
	chartPxW    = 500
	chartPxH    = 200
	chartPdfW   = 180.0
	chartPdfH   = chartPdfW * float64(chartPxH) / float64(chartPxW) // ~72mm
	chartMargin = 8.0
)

func renderDetailCards(p *fpdf.Fpdf, analyses []types.ResourceAnalysis) {
	candidates := filterCandidates(analyses)
	for _, a := range candidates {
		p.AddPage()

		cardY := p.GetY()
		cardH := 42.0
		if a.Owner != "" {
			cardH += 7
		}

		setFill(p, clrLightGrey)
		p.RoundedRect(15, cardY, 180, cardH, 2, "1234", "F")
		setFill(p, clrBlue)
		p.Rect(15, cardY, 3, cardH, "F")

		p.SetFont("Helvetica", "B", 13)
		setText(p, clrNavy)
		p.SetXY(22, cardY+3)
		p.CellFormat(170, 7, fmt.Sprintf("%s: %s/%s", a.Kind, a.Namespace, a.Name), "", 1, "L", false, 0, a.ConsoleURL)

		if a.Owner != "" {
			p.SetFont("Helvetica", "", 9)
			setText(p, clrSubtext)
			p.SetX(22)
			p.CellFormat(170, 6, "Owner: "+a.Owner, "", 1, "L", false, 0, "")
		}

		p.Ln(2)
		p.SetX(22)
		dirColor := clrAmber
		if a.Direction == types.Upsize {
			dirColor = clrRed
		}
		setText(p, dirColor)
		p.SetFont("Helvetica", "B", 10)
		p.CellFormat(30, 6, string(a.Direction), "", 0, "L", false, 0, "")

		setText(p, clrDarkText)
		p.SetFont("Helvetica", "", 9)
		p.CellFormat(70, 6, fmt.Sprintf("Current: %s CPU, %s", formatCPU(a.CurrentCPU), formatMem(a.CurrentMem)), "", 0, "L", false, 0, "")
		p.CellFormat(70, 6, fmt.Sprintf("Rec: %s CPU, %s", formatCPU(a.RecommendedCPU), formatMem(a.RecommendedMem)), "", 1, "L", false, 0, "")

		p.SetX(22)
		setText(p, clrGreen)
		p.SetFont("Helvetica", "B", 9)
		p.CellFormat(170, 6, fmt.Sprintf("Savings: %s CPU, %s memory", formatCPU(abs64(a.CPUSavings)), formatMem(abs64(a.MemSavings))), "", 1, "L", false, 0, "")
		setText(p, clrDarkText)

		p.SetY(cardY + cardH + 6)

		if len(a.CPUSamples) > 0 {
			placeChart(p, a.CPUSamples, a.CPUP95, "CPU Utilization (%)", fmt.Sprintf("cpu_%s_%s", a.Namespace, a.Name))
		}

		if len(a.MemSamples) > 0 {
			placeChart(p, a.MemSamples, a.MemP95, "Memory Utilization (%)", fmt.Sprintf("mem_%s_%s", a.Namespace, a.Name))
		}

		if a.Justification != "" {
			p.Ln(3)
			p.SetFont("Helvetica", "B", 10)
			setText(p, clrNavy)
			p.CellFormat(0, 6, "Justification", "", 1, "L", false, 0, "")
			setText(p, clrDarkText)
			p.SetFont("Helvetica", "", 9)
			p.MultiCell(0, 5, a.Justification, "", "L", false)
		}
	}
}

func placeChart(p *fpdf.Fpdf, samples []float64, p95 float64, title, imgName string) {
	if p.GetY()+chartPdfH+chartMargin > 277 {
		p.AddPage()
	}

	chartData, err := renderLineChart(samples, p95, 100, title, chartPxW, chartPxH)
	if err != nil {
		return
	}

	y := p.GetY()
	opt := fpdf.ImageOptions{ImageType: "PNG"}
	p.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(chartData))
	p.ImageOptions(imgName, 15, y, chartPdfW, chartPdfH, false, opt, 0, "")
	p.SetY(y + chartPdfH + chartMargin)
}
