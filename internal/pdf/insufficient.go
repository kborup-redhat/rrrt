package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderInsufficientData(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.InsufficientData) == 0 {
		return
	}

	p.AddPage()
	p.SetFont("Helvetica", "B", 16)
	p.CellFormat(0, 10, "Insufficient Data", "", 1, "L", false, 0, "")
	p.Ln(3)

	p.SetFont("Helvetica", "", 9)
	p.MultiCell(0, 5, "The following resources had less than 50% metric coverage of the lookback window and could not be analyzed.", "", "L", false)
	p.Ln(5)

	headers := []string{"Namespace", "Name", "Type", "Data Points", "Expected"}
	widths := []float64{35, 40, 30, 35, 35}

	p.SetFont("Helvetica", "B", 8)
	p.SetFillColor(240, 240, 240)
	for i, h := range headers {
		p.CellFormat(widths[i], 6, h, "1", 0, "C", true, 0, "")
	}
	p.Ln(-1)

	p.SetFont("Helvetica", "", 8)
	for _, entry := range data.InsufficientData {
		p.CellFormat(widths[0], 5, entry.Namespace, "1", 0, "L", false, 0, "")
		p.CellFormat(widths[1], 5, entry.Name, "1", 0, "L", false, 0, "")
		p.CellFormat(widths[2], 5, string(entry.Kind), "1", 0, "L", false, 0, "")
		p.CellFormat(widths[3], 5, fmt.Sprintf("%d", entry.DataPoints), "1", 0, "R", false, 0, "")
		p.CellFormat(widths[4], 5, fmt.Sprintf("%d", entry.ExpectedPoints), "1", 0, "R", false, 0, "")
		p.Ln(-1)
	}
}
