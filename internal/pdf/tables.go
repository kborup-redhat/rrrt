package pdf

import (
	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderResourceTable(p *fpdf.Fpdf, analyses []types.ResourceAnalysis, sectionTitle string) {
	if len(analyses) == 0 {
		return
	}

	candidates := filterCandidates(analyses)
	if len(candidates) == 0 {
		p.SetFont("Helvetica", "I", 10)
		p.CellFormat(0, 7, "No rightsizing candidates found.", "", 1, "L", false, 0, "")
		return
	}

	headers := []string{"Namespace", "Name", "Owner", "Direction", "Current CPU", "Rec. CPU", "Current Mem", "Rec. Mem"}
	widths := []float64{25, 30, 30, 18, 20, 20, 22, 22}

	p.SetFont("Helvetica", "B", 8)
	p.SetFillColor(240, 240, 240)
	for i, h := range headers {
		p.CellFormat(widths[i], 6, h, "1", 0, "C", true, 0, "")
	}
	p.Ln(-1)

	p.SetFont("Helvetica", "", 7)
	for _, a := range candidates {
		dir := string(a.Direction)
		p.CellFormat(widths[0], 5, a.Namespace, "1", 0, "L", false, 0, "")
		p.CellFormat(widths[1], 5, a.Name, "1", 0, "L", false, 0, a.ConsoleURL)
		p.CellFormat(widths[2], 5, truncate(a.Owner, 20), "1", 0, "L", false, 0, "")
		p.CellFormat(widths[3], 5, dir, "1", 0, "C", false, 0, "")
		p.CellFormat(widths[4], 5, formatCPU(a.CurrentCPU), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[5], 5, formatCPU(a.RecommendedCPU), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[6], 5, formatMem(a.CurrentMem), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[7], 5, formatMem(a.RecommendedMem), "1", 0, "R", false, 0, "")
		p.Ln(-1)
	}
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
