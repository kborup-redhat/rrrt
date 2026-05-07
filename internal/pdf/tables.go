package pdf

import (
	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderResourceTable(p *fpdf.Fpdf, analyses []types.ResourceAnalysis, _ string) {
	candidates := filterCandidates(analyses)
	if len(candidates) == 0 {
		p.SetFont("Helvetica", "I", 10)
		setText(p, clrSubtext)
		p.CellFormat(0, 7, "No rightsizing candidates found.", "", 1, "L", false, 0, "")
		setText(p, clrDarkText)
		return
	}

	headers := []string{"Namespace", "Name", "Owner", "Dir", "Cur CPU", "Rec CPU", "Cur Mem", "Rec Mem"}
	widths := []float64{25, 30, 30, 16, 20, 20, 24, 24}

	p.SetFont("Helvetica", "B", 8)
	setFill(p, clrNavy)
	setText(p, clrWhite)
	for i, h := range headers {
		p.CellFormat(widths[i], 7, h, "", 0, "C", true, 0, "")
	}
	p.Ln(-1)

	p.SetFont("Helvetica", "", 7)
	setText(p, clrDarkText)
	for row, a := range candidates {
		if row%2 == 0 {
			setFill(p, clrLightGrey)
		} else {
			setFill(p, clrWhite)
		}

		dir := ""
		switch a.Direction {
		case types.Downsize:
			dir = "downsize"
		case types.Upsize:
			dir = "upsize"
		}

		p.CellFormat(widths[0], 5.5, a.Namespace, "", 0, "L", true, 0, "")
		p.CellFormat(widths[1], 5.5, a.Name, "", 0, "L", true, 0, a.ConsoleURL)
		p.CellFormat(widths[2], 5.5, truncate(a.Owner, 20), "", 0, "L", true, 0, "")
		p.CellFormat(widths[3], 5.5, dir, "", 0, "C", true, 0, "")
		p.CellFormat(widths[4], 5.5, formatCPU(a.CurrentCPU), "", 0, "R", true, 0, "")
		p.CellFormat(widths[5], 5.5, formatCPU(a.RecommendedCPU), "", 0, "R", true, 0, "")
		p.CellFormat(widths[6], 5.5, formatMem(a.CurrentMem), "", 0, "R", true, 0, "")
		p.CellFormat(widths[7], 5.5, formatMem(a.RecommendedMem), "", 0, "R", true, 0, "")
		p.Ln(-1)
	}

	setDraw(p, clrMidGrey)
	y := p.GetY()
	p.Line(p.GetX(), y, p.GetX()+189, y)
}

func filterCandidates(analyses []types.ResourceAnalysis) []types.ResourceAnalysis {
	var candidates []types.ResourceAnalysis
	for _, a := range analyses {
		if a.Direction != "" {
			candidates = append(candidates, a)
		}
	}
	return candidates
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
